package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

type TokenManager interface {
	Issue(context.Context, security.UserID, uuid.UUID, time.Time) (security.TokenPair, error)
	Verify(context.Context, string, security.TokenUse) (security.TokenClaims, error)
}

type TokenSessionRepository interface {
	Create(context.Context, security.TokenSession) error
	FindByID(context.Context, security.UserID, uuid.UUID) (security.TokenSession, error)
}

type TokenReissuer interface {
	Reissue(context.Context, security.UserID, uuid.UUID, security.TokenIssuance) (security.TokenPair, error)
}

type SessionRotator interface {
	Rotate(context.Context, security.TokenSession, string, security.TokenIssuance) (security.TokenIssuance, error)
}

type JWTAuthenticationDependencies struct {
	Tokens   TokenManager
	Reissuer TokenReissuer
	Sessions TokenSessionRepository
	Rotator  SessionRotator
}

// JWTAuthenticationService owns access/refresh credentials and their rotation.
// Business code consumes Principal rather than a token or this service.
type JWTAuthenticationService struct {
	authentication *AuthenticationService
	dependencies   JWTAuthenticationDependencies
	refreshTTL     time.Duration
	now            func() time.Time
}

func NewJWTAuthenticationService(authentication *AuthenticationService, deps JWTAuthenticationDependencies, refreshTTL time.Duration) (*JWTAuthenticationService, error) {
	var service *JWTAuthenticationService
	var err error

	if authentication == nil || deps.Tokens == nil || deps.Reissuer == nil || deps.Sessions == nil || deps.Rotator == nil ||
		refreshTTL < time.Second || refreshTTL > 90*24*time.Hour {
		err = ErrInvalidAuthenticationConfig
	} else {
		service = &JWTAuthenticationService{authentication: authentication, dependencies: deps, refreshTTL: refreshTTL, now: time.Now}
	}

	return service, err
}

func (s *JWTAuthenticationService) SignIn(ctx context.Context, request input.SignInInput) (security.TokenPair, error) {
	var pair security.TokenPair
	credentials, err := s.authentication.VerifyCredentials(ctx, request)

	if err == nil {
		pair, err = s.createSession(ctx, credentials)
	}

	return pair, err
}

func (s *JWTAuthenticationService) createSession(ctx context.Context, credentials *security.Credentials) (security.TokenPair, error) {
	var pair security.TokenPair
	id, err := uuid.NewV7()

	if err != nil {
		err = fmt.Errorf("generate session ID: %w", err)
	} else {
		expires := s.now().UTC().Truncate(time.Second).Add(s.refreshTTL)
		pair, err = s.dependencies.Tokens.Issue(ctx, credentials.User.ID(), id, expires)

		if err == nil {
			session := security.TokenSession{Session: security.Session{ID: id, UserID: credentials.User.ID(), AuthenticationSnapshotVersion: credentials.Version, ExpiresAt: pair.RefreshExpiresAt}, RefreshHash: credentialDigest(pair.RefreshToken)}

			if createErr := s.dependencies.Sessions.Create(ctx, session); createErr != nil {
				err = fmt.Errorf("create authenticated session: %w", createErr)
			}
		}
	}

	if err != nil {
		pair = security.TokenPair{}
	}

	return pair, err
}

func (s *JWTAuthenticationService) Refresh(ctx context.Context, raw string) (security.TokenPair, error) {
	var pair security.TokenPair
	claims, err := s.dependencies.Tokens.Verify(ctx, raw, security.RefreshToken)

	if err == nil {
		session, loadErr := s.loadSession(ctx, claims)

		if loadErr != nil {
			err = loadErr
		} else if _, credentialErr := s.authentication.loadCredentials(ctx, session.Session); credentialErr != nil {
			err = credentialErr
		} else if !session.ExpiresAt.Equal(claims.ExpiresAt) {
			err = security.ErrInvalidToken
		} else {
			pair, err = s.dependencies.Tokens.Issue(ctx, claims.UserID, claims.SessionID, session.ExpiresAt)

			if err == nil {
				// Compare the presented token, not the current stored hash. Old
				// signed tokens must reach the atomic overlap/replay detection path.
				session.RefreshHash = credentialDigest(raw)
				var issuance security.TokenIssuance
				issuance, err = s.dependencies.Rotator.Rotate(ctx, session, credentialDigest(pair.RefreshToken), pair.Issuance)

				if err != nil {
					err = fmt.Errorf("rotate refresh token: %w", err)
				} else if !issuance.RefreshExpiresAt.Equal(session.ExpiresAt) {
					err = security.ErrInvalidSession
				} else {
					// Redis chooses one issuance for all overlapping requests.
					// Recreate that exact pair, rather than returning our proposal.
					pair, err = s.dependencies.Reissuer.Reissue(ctx, claims.UserID, claims.SessionID, issuance)
				}
			}
		}
	}

	if err != nil {
		pair = security.TokenPair{}
	}

	return pair, err
}

func (s *JWTAuthenticationService) loadSession(ctx context.Context, claims security.TokenClaims) (security.TokenSession, error) {
	session, err := s.dependencies.Sessions.FindByID(ctx, claims.UserID, claims.SessionID)

	if err != nil {
		err = fmt.Errorf("load authenticated session: %w", err)
	} else if session.UserID != claims.UserID || session.ID != claims.SessionID {
		err = security.ErrInvalidSession
	}

	return session, err
}

func (s *JWTAuthenticationService) Authenticate(ctx context.Context, raw string) (security.Principal, error) {
	var principal security.Principal
	claims, err := s.dependencies.Tokens.Verify(ctx, raw, security.AccessToken)

	if err == nil {
		session, loadErr := s.loadSession(ctx, claims)

		if loadErr != nil {
			err = loadErr
		} else {
			principal, err = s.authentication.ResolvePrincipal(ctx, session.Session)
		}
	}

	return principal, err
}

func (s *JWTAuthenticationService) SignOut(ctx context.Context, raw string, use security.TokenUse) error {
	claims, err := s.dependencies.Tokens.Verify(ctx, raw, use)

	if err == nil {
		err = s.authentication.SignOut(ctx, security.Principal{UserID: claims.UserID, SessionID: claims.SessionID})
	}

	return err
}

func (s *JWTAuthenticationService) SignOutAll(ctx context.Context, access string) error {
	principal, err := s.Authenticate(ctx, access)

	if err == nil {
		err = s.authentication.SignOutAll(ctx, principal)
	}

	return err
}
