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
	} else if userID.Validate() != nil || sessionID == uuid.Nil || !sessionExpiry.After(now) {
		err = security.ErrInvalidSession
	} else {
		accessID, accessErr := uuid.NewV7()
		refreshID, refreshErr := uuid.NewV7()

		if accessErr != nil {
			err = fmt.Errorf("generate access JWT ID: %w", accessErr)
		} else if refreshErr != nil {
			err = fmt.Errorf("generate refresh JWT ID: %w", refreshErr)
		} else {
			pair, err = m.Reissue(ctx, userID, sessionID, security.TokenIssuance{
				KeyID: m.config.KeyID, AccessID: accessID, RefreshID: refreshID, IssuedAt: now,
				AccessExpiresAt: minTime(now.Add(m.config.AccessTTL), sessionExpiry), RefreshExpiresAt: sessionExpiry,
			})
		}
	}

	if err != nil {
		pair = security.TokenPair{}
	}

	return pair, err
}

// Reissue recreates the same Ed25519-signed tokens from shared public claims.
func (m *Manager) Reissue(ctx context.Context, userID security.UserID, sessionID uuid.UUID, issuance security.TokenIssuance) (security.TokenPair, error) {
	var pair security.TokenPair
	var err error

	if contextErr := ctx.Err(); contextErr != nil {
		err = fmt.Errorf("reissue JWT: %w", contextErr)
	} else if userID.Validate() != nil || sessionID == uuid.Nil || issuance.Validate() != nil ||
		issuance.KeyID != m.config.KeyID || !issuance.RefreshExpiresAt.After(m.now()) {
		err = security.ErrInvalidSession
	} else {
		pair.AccessToken, err = m.sign(userID, sessionID, issuance.AccessID, security.AccessToken, issuance.IssuedAt, issuance.AccessExpiresAt)

		if err == nil {
			pair.RefreshToken, err = m.sign(userID, sessionID, issuance.RefreshID, security.RefreshToken, issuance.IssuedAt, issuance.RefreshExpiresAt)
		}

		if err == nil {
			pair.Issuance = issuance
			pair.AccessExpiresAt, pair.RefreshExpiresAt = issuance.AccessExpiresAt, issuance.RefreshExpiresAt
		} else {
			pair = security.TokenPair{}
		}
	}

	return pair, err
}

func (m *Manager) sign(userID security.UserID, sessionID, tokenID uuid.UUID, use security.TokenUse, now, expiry time.Time) (string, error) {
	audience, typ := m.tokenType(use)
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodEdDSA, claims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer: m.config.Issuer, Subject: uuid.UUID(userID).String(), Audience: jwtlib.ClaimStrings{audience},
			ExpiresAt: jwtlib.NewNumericDate(expiry), IssuedAt: jwtlib.NewNumericDate(now), NotBefore: jwtlib.NewNumericDate(now), ID: tokenID.String(),
		},
		SessionID: sessionID.String(), Use: use,
	})
	token.Header["typ"], token.Header["kid"] = typ, m.config.KeyID
	encoded, err := token.SignedString(m.config.PrivateKey)

	if err != nil {
		err = fmt.Errorf("sign JWT: %w", err)
	}

	return encoded, err
}

func (m *Manager) Verify(ctx context.Context, raw string, use security.TokenUse) (security.TokenClaims, error) {
	var result security.TokenClaims
	var err error

	if contextErr := ctx.Err(); contextErr != nil {
		err = fmt.Errorf("verify JWT: %w", contextErr)
	} else if len(raw) == 0 || len(raw) > 4096 || use.Validate() != nil {
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
		}, jwtlib.WithValidMethods([]string{jwtlib.SigningMethodEdDSA.Alg()}),
			jwtlib.WithStrictDecoding(), jwtlib.WithoutClaimsValidation())

		if parseErr != nil {
			err = fmt.Errorf("verify JWT: %w: %w", security.ErrInvalidToken, parseErr)
		} else {
			rawUserID, userErr := uuid.Parse(parsed.Subject)
			userID := security.UserID(rawUserID)
			sessionID, sessionErr := uuid.Parse(parsed.SessionID)
			tokenID, idErr := uuid.Parse(parsed.ID)

			if !token.Valid || parsed.Use != use || userErr != nil || userID.Validate() != nil || sessionErr != nil || sessionID == uuid.Nil ||
				idErr != nil || tokenID == uuid.Nil || parsed.IssuedAt == nil || parsed.ExpiresAt == nil || parsed.NotBefore == nil ||
				!parsed.ExpiresAt.After(parsed.IssuedAt.Time) ||
				!parsed.NotBefore.Equal(parsed.IssuedAt.Time) || parsed.Issuer != m.config.Issuer ||
				len(parsed.Audience) != 1 || parsed.Audience[0] != audience || (use == security.AccessToken && parsed.ExpiresAt.Sub(parsed.IssuedAt.Time) > 15*time.Minute) {
				err = security.ErrInvalidToken
			} else {
				validator := jwtlib.NewValidator(jwtlib.WithExpirationRequired(), jwtlib.WithNotBeforeRequired(),
					jwtlib.WithIssuedAt(), jwtlib.WithTimeFunc(m.now), jwtlib.WithLeeway(30*time.Second))
				validationErr := validator.Validate(parsed)

				if errors.Is(validationErr, jwtlib.ErrTokenExpired) {
					// Signature, purpose and structural claims were checked first:
					// only a genuinely expired token can request automatic renewal.
					err = fmt.Errorf("verify JWT: %w: %w: %w", security.ErrInvalidToken, security.ErrTokenExpired, validationErr)
				} else if validationErr != nil {
					err = fmt.Errorf("verify JWT: %w: %w", security.ErrInvalidToken, validationErr)
				} else {
					result = security.TokenClaims{UserID: userID, SessionID: sessionID, ExpiresAt: parsed.ExpiresAt.Time}
				}
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
