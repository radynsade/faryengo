package jwt

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/security"
)

var ErrInvalidConfig = errors.New("invalid JWT configuration")

// Config pins an issuer, separate audiences, and an Ed25519 signing key.
// VerificationKeys can retain old public keys during an intentional key rotation.
type Config struct {
	PrivateKey       ed25519.PrivateKey
	KeyID            string
	VerificationKeys map[string]ed25519.PublicKey
	Issuer           string
	AccessAudience   string
	RefreshAudience  string
	AccessTTL        time.Duration
}

type Manager struct {
	config Config
	now    func() time.Time
}

type claims struct {
	jwtlib.RegisteredClaims
	SessionID string            `json:"sid"`
	Use       security.TokenUse `json:"token_use"`
}

func NewManager(config Config) (*Manager, error) {
	var manager *Manager
	var err error

	if len(config.PrivateKey) != ed25519.PrivateKeySize || strings.TrimSpace(config.KeyID) == "" ||
		strings.TrimSpace(config.Issuer) == "" || strings.TrimSpace(config.AccessAudience) == "" ||
		strings.TrimSpace(config.RefreshAudience) == "" || config.AccessAudience == config.RefreshAudience ||
		config.AccessTTL < time.Second || config.AccessTTL > 15*time.Minute {
		err = ErrInvalidConfig
	} else if !bytes.Equal(config.PrivateKey, ed25519.NewKeyFromSeed(config.PrivateKey.Seed())) {
		err = ErrInvalidConfig
	} else {
		config.PrivateKey = bytes.Clone(config.PrivateKey)
		keys := make(map[string]ed25519.PublicKey, len(config.VerificationKeys)+1)

		for id, key := range config.VerificationKeys {
			if id == "" || len(key) != ed25519.PublicKeySize {
				err = ErrInvalidConfig
			}

			keys[id] = bytes.Clone(key)
		}

		keys[config.KeyID] = bytes.Clone(config.PrivateKey.Public().(ed25519.PublicKey))
		config.VerificationKeys = keys

		if err == nil {
			manager = &Manager{config: config, now: time.Now}
		}
	}

	return manager, err
}

func (m *Manager) Issue(ctx context.Context, userID security.UserID, sessionID uuid.UUID, sessionExpiry time.Time) (security.TokenPair, error) {
	var pair security.TokenPair
	var err error
	now := m.now().UTC().Truncate(time.Second)
	sessionExpiry = sessionExpiry.UTC().Truncate(time.Second)

	if contextErr := ctx.Err(); contextErr != nil {
		err = fmt.Errorf("issue JWT: %w", contextErr)
	} else if uuid.UUID(userID) == uuid.Nil || sessionID == uuid.Nil || !sessionExpiry.After(now) {
		err = security.ErrInvalidSession
	} else {
		accessExpiry := minTime(now.Add(m.config.AccessTTL), sessionExpiry)
		pair.AccessToken, err = m.sign(userID, sessionID, security.AccessToken, now, accessExpiry)

		if err == nil {
			pair.RefreshToken, err = m.sign(userID, sessionID, security.RefreshToken, now, sessionExpiry)
		}

		pair.AccessExpiresAt, pair.RefreshExpiresAt = accessExpiry, sessionExpiry
	}

	if err != nil {
		pair = security.TokenPair{}
	}

	return pair, err
}

func (m *Manager) sign(userID security.UserID, sessionID uuid.UUID, use security.TokenUse, now, expiry time.Time) (string, error) {
	var encoded string
	nonce, err := uuid.NewRandom()

	if err != nil {
		err = fmt.Errorf("generate JWT ID: %w", err)
	} else {
		audience, typ := m.tokenType(use)
		token := jwtlib.NewWithClaims(jwtlib.SigningMethodEdDSA, claims{
			RegisteredClaims: jwtlib.RegisteredClaims{
				Issuer: m.config.Issuer, Subject: uuid.UUID(userID).String(), Audience: jwtlib.ClaimStrings{audience},
				ExpiresAt: jwtlib.NewNumericDate(expiry), IssuedAt: jwtlib.NewNumericDate(now), NotBefore: jwtlib.NewNumericDate(now), ID: nonce.String(),
			},
			SessionID: sessionID.String(), Use: use,
		})
		token.Header["typ"], token.Header["kid"] = typ, m.config.KeyID
		encoded, err = token.SignedString(m.config.PrivateKey)

		if err != nil {
			err = fmt.Errorf("sign JWT: %w", err)
		}
	}

	return encoded, err
}

func (m *Manager) Verify(ctx context.Context, raw string, use security.TokenUse) (security.TokenClaims, error) {
	var result security.TokenClaims
	var err error

	if contextErr := ctx.Err(); contextErr != nil {
		err = fmt.Errorf("verify JWT: %w", contextErr)
	} else if len(raw) == 0 || len(raw) > 4096 || (use != security.AccessToken && use != security.RefreshToken) {
		err = security.ErrInvalidToken
	} else {
		audience, typ := m.tokenType(use)
		parsed := &claims{}
		token, parseErr := jwtlib.ParseWithClaims(raw, parsed, func(token *jwtlib.Token) (any, error) {
			var key ed25519.PublicKey
			var keyErr error
			keyID, valid := token.Header["kid"].(string)

			if token.Method.Alg() != jwtlib.SigningMethodEdDSA.Alg() || token.Header["typ"] != typ || !valid {
				keyErr = security.ErrInvalidToken
			} else if candidate, exists := m.config.VerificationKeys[keyID]; !exists {
				keyErr = security.ErrInvalidToken
			} else {
				key = candidate
			}

			return key, keyErr
		}, jwtlib.WithValidMethods([]string{jwtlib.SigningMethodEdDSA.Alg()}), jwtlib.WithIssuer(m.config.Issuer),
			jwtlib.WithAudience(audience), jwtlib.WithExpirationRequired(), jwtlib.WithNotBeforeRequired(),
			jwtlib.WithIssuedAt(), jwtlib.WithStrictDecoding(), jwtlib.WithTimeFunc(m.now), jwtlib.WithLeeway(30*time.Second))

		if parseErr != nil {
			err = fmt.Errorf("verify JWT: %w: %w", security.ErrInvalidToken, parseErr)
		} else {
			userID, userErr := uuid.Parse(parsed.Subject)
			sessionID, sessionErr := uuid.Parse(parsed.SessionID)
			tokenID, idErr := uuid.Parse(parsed.ID)

			if !token.Valid || parsed.Use != use || userErr != nil || userID == uuid.Nil || sessionErr != nil || sessionID == uuid.Nil ||
				idErr != nil || tokenID == uuid.Nil || parsed.IssuedAt == nil || !parsed.ExpiresAt.After(parsed.IssuedAt.Time) ||
				!parsed.NotBefore.Equal(parsed.IssuedAt.Time) || len(parsed.Audience) != 1 || (use == security.AccessToken && parsed.ExpiresAt.Sub(parsed.IssuedAt.Time) > 15*time.Minute) {
				err = security.ErrInvalidToken
			} else {
				result = security.TokenClaims{UserID: security.UserID(userID), SessionID: sessionID, ExpiresAt: parsed.ExpiresAt.Time}
			}
		}
	}

	return result, err
}

func (m *Manager) tokenType(use security.TokenUse) (string, string) {
	audience, typ := m.config.AccessAudience, "at+jwt"

	if use == security.RefreshToken {
		audience, typ = m.config.RefreshAudience, "refresh+jwt"
	}

	return audience, typ
}

func minTime(a, b time.Time) time.Time {
	result := a

	if b.Before(a) {
		result = b
	}

	return result
}
