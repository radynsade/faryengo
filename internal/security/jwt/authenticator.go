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
	"github.com/radynsade/faryengo/internal/users"
)

//
// Errors
//

var (
	ErrAuthenticatorNil   = errors.New("JWT authenticator is nil")
	ErrConfigInvalid      = errors.New("invalid JWT authenticator configuration")
	ErrTokenInvalid       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
	ErrTokenMissing       = errors.New("no token given")
	ErrTokenSuperseded    = errors.New("refresh token superseded")
	ErrRefreshTokenReused = errors.New("refresh token reused")
	ErrSessionExpired     = errors.New("session expired")
)

//
// Config
//

const (
	MinAccessTTL   = time.Second
	MaxAccessTTL   = 15 * time.Minute
	MaxTokenBytes  = 4096
	ClockLeeway    = 30 * time.Second
	accessType     = "at+jwt"
	refreshType    = "refresh+jwt"
	accessPurpose  = "access"
	refreshPurpose = "refresh"
)

type Config struct {
	PrivateKey      ed25519.PrivateKey
	KeyID           string
	Issuer          string
	AccessAudience  string
	RefreshAudience string
	AccessTTL       time.Duration
}

func (c Config) Validate() error {
	var err error

	if len(c.PrivateKey) != ed25519.PrivateKeySize ||
		!bytes.Equal(c.PrivateKey, ed25519.NewKeyFromSeed(c.PrivateKey.Seed())) {
		err = ErrConfigInvalid
	} else if strings.TrimSpace(c.KeyID) == "" || strings.TrimSpace(c.Issuer) == "" {
		err = ErrConfigInvalid
	} else if strings.TrimSpace(c.AccessAudience) == "" || strings.TrimSpace(c.RefreshAudience) == "" {
		err = ErrConfigInvalid
	} else if c.AccessAudience == c.RefreshAudience {
		err = ErrConfigInvalid
	} else if c.AccessTTL < MinAccessTTL || c.AccessTTL > MaxAccessTTL {
		err = ErrConfigInvalid
	}

	return err
}

//
// Credentials
//

type Credentials struct {
	AccessToken  string
	RefreshToken string
}

//
// Tokens
//

type Tokens struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

//
// Token storage
//

var (
	ErrSessionNotFound      = errors.New("session not found")
	ErrRefreshTokenMismatch = errors.New("refresh token is not the current one")
)

// Storage keeps each Session with the ID of its only valid refresh token, never
// a token itself. Create never replaces an existing Session. FindByID reports a
// missing or expired Session as ErrSessionNotFound. Rotate replaces the
// refresh token ID only while it still equals current and otherwise reports
// ErrRefreshTokenMismatch without changes.

type TokenStorage interface {
	Create(ctx context.Context, session *security.Session, refreshID uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*security.Session, uuid.UUID, error)
	Rotate(ctx context.Context, id uuid.UUID, current uuid.UUID, next uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
}

//
// Authenticator
//

type Authenticator struct {
	config      Config
	publicKey   ed25519.PublicKey
	tokens      TokenStorage
	credentials users.CredentialsSnapshotRepository
}

var _ security.Authenticator[Credentials] = (*Authenticator)(nil)

func NewAuthenticator(
	config Config,
	tokens TokenStorage,
	credentials users.CredentialsSnapshotRepository,
) (*Authenticator, error) {
	var (
		authenticator *Authenticator
		err           error
	)

	if tokens == nil || credentials == nil {
		err = ErrConfigInvalid
	} else {
		err = config.Validate()
	}

	if err == nil {
		config.PrivateKey = bytes.Clone(config.PrivateKey)

		authenticator = &Authenticator{
			config:      config,
			publicKey:   config.PrivateKey.Public().(ed25519.PublicKey),
			tokens:      tokens,
			credentials: credentials,
		}
	} else {
		err = fmt.Errorf("create a JWT authenticator: %w", err)
	}

	return authenticator, err
}

// Issue stores a Session established by another mechanism, such as emailpass,
// and signs its first token pair. The refresh token lives as long as the
// Session; refreshing never extends it.

func (a *Authenticator) Issue(
	ctx context.Context,
	session *security.Session,
) (*Tokens, error) {
	var (
		tokens *Tokens
		err    error
	)

	if a == nil || a.tokens == nil {
		err = ErrAuthenticatorNil
	} else if validationErr := session.Validate(); validationErr != nil {
		err = validationErr
	} else if !time.Now().Before(session.ExpiresAt) {
		err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, ErrSessionExpired)
	} else {
		var refreshID uuid.UUID

		tokens, refreshID, err = a.sign(session)

		if err == nil {
			err = a.tokens.Create(ctx, session, refreshID)
		}
	}

	if err != nil {
		tokens = nil
		err = fmt.Errorf("issue JWTs: %w", err)
	}

	return tokens, err
}

// The access token is used when given, and the refresh token only when the
// access token is missing or merely expired; any other access token failure
// rejects the request. A refresh token signs in only while it is the Session's
// current one, and signing in never rotates it: use Refresh for that.

func (a *Authenticator) SignIn(
	ctx context.Context,
	credentials Credentials,
) (*security.Session, error) {
	var (
		session *security.Session
		err     error
	)

	if a == nil || a.tokens == nil {
		err = ErrAuthenticatorNil
	} else if credentials.AccessToken == "" && credentials.RefreshToken == "" {
		err = fmt.Errorf("%w: %w", security.ErrCredentialsInvalid, ErrTokenMissing)
	} else {
		useRefresh := credentials.AccessToken == ""

		if !useRefresh {
			session, err = a.signInWithAccess(ctx, credentials.AccessToken)
			useRefresh = errors.Is(err, ErrTokenExpired) && credentials.RefreshToken != ""
		}

		if useRefresh {
			session, _, err = a.signInWithRefresh(ctx, credentials.RefreshToken)
		}
	}

	if err != nil {
		session = nil
		err = fmt.Errorf("sign in with JWTs: %w", err)
	}

	return session, err
}

// Each refresh token works once. Presenting one that was already replaced
// means a copy exists outside the client, so the whole Session is revoked,
// including access tokens issued to it.

func (a *Authenticator) Refresh(
	ctx context.Context,
	refreshToken string,
) (*Tokens, error) {
	var (
		tokens *Tokens
		err    error
	)

	if a == nil || a.tokens == nil {
		err = ErrAuthenticatorNil
	} else {
		var (
			session   *security.Session
			currentID uuid.UUID
			nextID    uuid.UUID
		)

		session, currentID, err = a.signInWithRefresh(ctx, refreshToken)

		if errors.Is(err, ErrTokenSuperseded) {
			err = a.revokeReused(ctx, session)
		}

		if err == nil {
			tokens, nextID, err = a.sign(session)
		}

		if err == nil {
			err = a.tokens.Rotate(ctx, session.ID, currentID, nextID)

			if errors.Is(err, ErrRefreshTokenMismatch) {
				err = a.revokeReused(ctx, session)
			} else if errors.Is(err, ErrSessionNotFound) {
				err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, err)
			}
		}
	}

	if err != nil {
		tokens = nil
		err = fmt.Errorf("refresh JWTs: %w", err)
	}

	return tokens, err
}

// Signing out everywhere rotates the durable credentials version before
// deleting the current Session, so a storage failure cannot leave the other
// Sessions valid.

func (a *Authenticator) SignOut(
	ctx context.Context,
	session *security.Session,
	all bool,
) error {
	var err error

	if a == nil || a.tokens == nil {
		err = ErrAuthenticatorNil
	} else if validationErr := session.Validate(); validationErr != nil {
		err = validationErr
	} else {
		if all {
			err = a.credentials.RotateVersion(ctx, session.UserID)
		}

		if err == nil {
			err = a.tokens.Delete(ctx, session.ID)
		}
	}

	if err != nil {
		err = fmt.Errorf("sign out with JWTs: %w", err)
	}

	return err
}

//
// Helpers
//

type claims struct {
	jwtlib.RegisteredClaims
	SessionID string `json:"sid"`
	Purpose   string `json:"purpose"`
}

func (a *Authenticator) signInWithAccess(
	ctx context.Context,
	accessToken string,
) (*security.Session, error) {
	var session *security.Session

	parsed, err := a.verify(accessToken, accessPurpose)

	if err == nil {
		session, _, err = a.findSession(ctx, parsed)
	}

	return session, err
}

// A superseded refresh token still returns its Session, so Refresh can revoke
// it.

func (a *Authenticator) signInWithRefresh(
	ctx context.Context,
	refreshToken string,
) (*security.Session, uuid.UUID, error) {
	var (
		session   *security.Session
		refreshID uuid.UUID
	)

	parsed, err := a.verify(refreshToken, refreshPurpose)

	if err == nil {
		var storedID uuid.UUID

		session, storedID, err = a.findSession(ctx, parsed)
		refreshID, _ = uuid.Parse(parsed.ID)

		if err == nil && storedID != refreshID {
			err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, ErrTokenSuperseded)
		}
	}

	if err != nil && !errors.Is(err, ErrTokenSuperseded) {
		session, refreshID = nil, uuid.Nil
	}

	return session, refreshID, err
}

func (a *Authenticator) findSession(
	ctx context.Context,
	parsed *claims,
) (*security.Session, uuid.UUID, error) {
	var (
		session   *security.Session
		refreshID uuid.UUID
		err       error
	)

	sessionID, sessionErr := uuid.Parse(parsed.SessionID)
	userID, userErr := uuid.Parse(parsed.Subject)

	if sessionErr != nil || userErr != nil || sessionID == uuid.Nil {
		err = fmt.Errorf("%w: %w", security.ErrCredentialsInvalid, ErrTokenInvalid)
	} else {
		session, refreshID, err = a.tokens.FindByID(ctx, sessionID)

		if errors.Is(err, ErrSessionNotFound) {
			err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, err)
		} else if err != nil {
			err = fmt.Errorf("find a session %s: %w", sessionID, err)
		} else if validationErr := session.Validate(); validationErr != nil {
			err = validationErr
		} else if session.ID != sessionID {
			err = security.ErrSessionInvalid
		} else if session.UserID != users.UserID(userID) {
			err = fmt.Errorf("%w: %w", security.ErrCredentialsInvalid, ErrTokenInvalid)
		} else if !time.Now().Before(session.ExpiresAt) {
			err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, ErrSessionExpired)
		} else {
			err = a.checkVersion(ctx, session)
		}
	}

	if err != nil {
		session, refreshID = nil, uuid.Nil
	}

	return session, refreshID, err
}

func (a *Authenticator) checkVersion(
	ctx context.Context,
	session *security.Session,
) error {
	version, err := a.credentials.FindVersionByUserID(ctx, session.UserID)

	if errors.Is(err, users.ErrUserNotFound) {
		err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, err)
	} else if err != nil {
		err = fmt.Errorf("check the credentials version: %w", err)
	} else if version != session.CredentialsVersion {
		err = security.ErrSessionRevoked
	}

	return err
}

func (a *Authenticator) revokeReused(
	ctx context.Context,
	session *security.Session,
) error {
	err := a.tokens.Delete(ctx, session.ID)

	if err == nil {
		err = fmt.Errorf("%w: %w", security.ErrSessionRevoked, ErrRefreshTokenReused)
	} else {
		err = fmt.Errorf("revoke a session %s after refresh token reuse: %w", session.ID, err)
	}

	return err
}

// Signing has no I/O, so a pair is signed before storage records its refresh
// token ID and is returned only once that write succeeds.

func (a *Authenticator) sign(session *security.Session) (*Tokens, uuid.UUID, error) {
	var tokens *Tokens

	accessID, accessErr := uuid.NewV7()
	refreshID, refreshErr := uuid.NewV7()
	err := errors.Join(accessErr, refreshErr)

	if err != nil {
		err = fmt.Errorf("generate token IDs: %w", err)
	} else {
		now := time.Now().Truncate(time.Second)
		refreshExpiresAt := session.ExpiresAt.Truncate(time.Second)
		accessExpiresAt := now.Add(a.config.AccessTTL)

		if refreshExpiresAt.Before(accessExpiresAt) {
			accessExpiresAt = refreshExpiresAt
		}

		tokens = &Tokens{
			AccessExpiresAt:  accessExpiresAt,
			RefreshExpiresAt: refreshExpiresAt,
		}

		tokens.AccessToken, err = a.signToken(session, accessID, accessPurpose, now, accessExpiresAt)

		if err == nil {
			tokens.RefreshToken, err = a.signToken(session, refreshID, refreshPurpose, now, refreshExpiresAt)
		}
	}

	if err != nil {
		tokens, refreshID = nil, uuid.Nil
	}

	return tokens, refreshID, err
}

func (a *Authenticator) signToken(
	session *security.Session,
	tokenID uuid.UUID,
	purpose string,
	issuedAt time.Time,
	expiresAt time.Time,
) (string, error) {
	audience, tokenType := a.tokenType(purpose)

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodEdDSA, claims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    a.config.Issuer,
			Subject:   uuid.UUID(session.UserID).String(),
			Audience:  jwtlib.ClaimStrings{audience},
			ExpiresAt: jwtlib.NewNumericDate(expiresAt),
			NotBefore: jwtlib.NewNumericDate(issuedAt),
			IssuedAt:  jwtlib.NewNumericDate(issuedAt),
			ID:        tokenID.String(),
		},
		SessionID: session.ID.String(),
		Purpose:   purpose,
	})

	token.Header["typ"] = tokenType
	token.Header["kid"] = a.config.KeyID

	signed, err := token.SignedString(a.config.PrivateKey)

	if err != nil {
		err = fmt.Errorf("sign a %s token: %w", purpose, err)
	}

	return signed, err
}

// The algorithm, key ID, header type, issuer, audience and purpose are all
// pinned, so an access token and a refresh token can never stand in for each
// other. A token is reported as expired only when it would pass every check at
// the moment it was issued, so expiry never hides another failure.

func (a *Authenticator) verify(raw string, purpose string) (*claims, error) {
	var (
		parsed = &claims{}
		err    error
	)

	audience, tokenType := a.tokenType(purpose)

	if raw == "" || len(raw) > MaxTokenBytes {
		err = ErrTokenInvalid
	} else {
		_, err = jwtlib.ParseWithClaims(
			raw,
			parsed,
			func(token *jwtlib.Token) (any, error) {
				var (
					key    any
					keyErr error
				)

				if token.Header["typ"] != tokenType || token.Header["kid"] != a.config.KeyID {
					keyErr = ErrTokenInvalid
				} else {
					key = a.publicKey
				}

				return key, keyErr
			},
			jwtlib.WithValidMethods([]string{jwtlib.SigningMethodEdDSA.Alg()}),
			jwtlib.WithStrictDecoding(),
			jwtlib.WithoutClaimsValidation(),
		)

		if err != nil {
			err = fmt.Errorf("%w: %w", ErrTokenInvalid, err)
		} else if parsed.Purpose != purpose || parsed.NotBefore == nil || parsed.IssuedAt == nil {
			err = ErrTokenInvalid
		} else if tokenID, idErr := uuid.Parse(parsed.ID); idErr != nil || tokenID == uuid.Nil {
			err = ErrTokenInvalid
		} else if validationErr := a.validator(audience, time.Now).Validate(parsed); validationErr != nil {
			issuedAt := parsed.IssuedAt.Time
			issuedErr := a.validator(audience, func() time.Time { return issuedAt }).Validate(parsed)

			if errors.Is(validationErr, jwtlib.ErrTokenExpired) && issuedErr == nil {
				err = fmt.Errorf("%w: %w", ErrTokenExpired, validationErr)
			} else {
				err = fmt.Errorf("%w: %w", ErrTokenInvalid, validationErr)
			}
		}
	}

	if err != nil {
		parsed = nil
		err = fmt.Errorf("%w: %w", security.ErrCredentialsInvalid, err)
	}

	return parsed, err
}

func (a *Authenticator) validator(audience string, now func() time.Time) *jwtlib.Validator {
	return jwtlib.NewValidator(
		jwtlib.WithIssuer(a.config.Issuer),
		jwtlib.WithAudience(audience),
		jwtlib.WithExpirationRequired(),
		jwtlib.WithIssuedAt(),
		jwtlib.WithLeeway(ClockLeeway),
		jwtlib.WithTimeFunc(now),
	)
}

func (a *Authenticator) tokenType(purpose string) (string, string) {
	audience, tokenType := a.config.AccessAudience, accessType

	if purpose == refreshPurpose {
		audience, tokenType = a.config.RefreshAudience, refreshType
	}

	return audience, tokenType
}
