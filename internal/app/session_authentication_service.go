package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

type OpaqueSessionRepository interface {
	CreateSession(context.Context, security.Session, string) error
	FindSession(context.Context, string) (security.Session, error)
}

// SessionAuthenticationService accepts only random opaque session credentials.
// Cookie extraction and cookie policy belong to the HTTP transport.
type SessionAuthenticationService struct {
	authentication *AuthenticationService
	sessions       OpaqueSessionRepository
	lifetime       time.Duration
	now            func() time.Time
}

func NewSessionAuthenticationService(authentication *AuthenticationService, sessions OpaqueSessionRepository, lifetime time.Duration) (*SessionAuthenticationService, error) {
	var service *SessionAuthenticationService
	var err error

	if authentication == nil || sessions == nil || lifetime < time.Second || lifetime > 90*24*time.Hour {
		err = ErrInvalidAuthenticationConfig
	} else {
		service = &SessionAuthenticationService{authentication: authentication, sessions: sessions, lifetime: lifetime, now: time.Now}
	}

	return service, err
}

func (s *SessionAuthenticationService) SignIn(ctx context.Context, request input.SignInInput) (security.SessionGrant, error) {
	var grant security.SessionGrant
	credentials, err := s.authentication.VerifyCredentials(ctx, request)

	if err == nil {
		var id uuid.UUID
		id, err = uuid.NewRandom()

		if err != nil {
			err = fmt.Errorf("generate session ID: %w", err)
		} else {
			secret := make([]byte, 32)
			_, err = rand.Read(secret)

			if err != nil {
				err = fmt.Errorf("generate session credential: %w", err)
			} else {
				grant = security.SessionGrant{ID: base64.RawURLEncoding.EncodeToString(secret), ExpiresAt: s.now().UTC().Truncate(time.Second).Add(s.lifetime)}
				session := security.Session{ID: id, UserID: credentials.User.ID(), CredentialVersion: credentials.Version, ExpiresAt: grant.ExpiresAt}
				err = s.sessions.CreateSession(ctx, session, credentialDigest(grant.ID))

				if err != nil {
					err = fmt.Errorf("create browser session: %w", err)
				}
			}
		}
	}

	if err != nil {
		grant = security.SessionGrant{}
	}

	return grant, err
}

func sessionCredentialHash(raw string) (string, error) {
	var digest string
	var err error

	if len(raw) != base64.RawURLEncoding.EncodedLen(32) {
		err = security.ErrInvalidSession
	} else {
		decoded, decodeErr := base64.RawURLEncoding.Strict().DecodeString(raw)

		if decodeErr != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw {
			err = security.ErrInvalidSession
		} else {
			digest = credentialDigest(raw)
		}
	}

	return digest, err
}

func (s *SessionAuthenticationService) Authenticate(ctx context.Context, raw string) (security.Principal, error) {
	var principal security.Principal
	digest, err := sessionCredentialHash(raw)

	if err == nil {
		session, findErr := s.sessions.FindSession(ctx, digest)

		if findErr != nil {
			err = fmt.Errorf("load browser session: %w", findErr)
		} else {
			principal, err = s.authentication.ResolvePrincipal(ctx, session)
		}
	}

	return principal, err
}

func (s *SessionAuthenticationService) SignOut(ctx context.Context, raw string) error {
	digest, err := sessionCredentialHash(raw)

	if err == nil {
		session, findErr := s.sessions.FindSession(ctx, digest)

		if findErr != nil {
			err = fmt.Errorf("load sign-out session: %w", findErr)
		} else {
			// Logout needs possession of the credential, not a current role.
			err = s.authentication.SignOut(ctx, security.Principal{UserID: session.UserID, SessionID: session.ID})
		}
	}

	return err
}
