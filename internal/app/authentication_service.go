package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

var (
	ErrInvalidAuthenticationConfig = errors.New("invalid authentication dependencies or settings")
	ErrAuthenticationBusy          = errors.New("password verification capacity exhausted")
)

// Consumer interfaces keep persistence and token libraries outside the service.
type CredentialRepository interface {
	FindByEmail(context.Context, security.Email) (*security.Credentials, error)
	FindByUserID(context.Context, security.UserID) (*security.Credentials, error)
}

type AuthorizationRoleRepository interface {
	FindByID(context.Context, security.RoleID) (*security.Role, error)
}

type TokenManager interface {
	Issue(context.Context, security.UserID, uuid.UUID, time.Time) (security.TokenPair, error)
	Verify(context.Context, string, security.TokenUse) (security.TokenClaims, error)
}

type SessionRepository interface {
	Create(context.Context, security.Session) error
	FindByID(context.Context, security.UserID, uuid.UUID) (security.Session, error)
}

type SessionRotator interface {
	Rotate(context.Context, security.Session, string) error
}

type SessionRevoker interface {
	Revoke(context.Context, security.UserID, uuid.UUID) error
	RevokeAll(context.Context, security.UserID) error
}

type CredentialInvalidator interface {
	Invalidate(context.Context, security.UserID) error
}

type AuthenticationDependencies struct {
	Credentials CredentialRepository
	Invalidator CredentialInvalidator
	Roles       AuthorizationRoleRepository
	Hasher      security.PasswordHasher
	Tokens      TokenManager
	Sessions    SessionRepository
	Rotator     SessionRotator
	Revoker     SessionRevoker
}

type AuthenticationService struct {
	dependencies  AuthenticationDependencies
	refreshTTL    time.Duration
	dummyHash     security.PasswordHash
	verifications *semaphore.Weighted
	now           func() time.Time
}

func NewAuthenticationService(ctx context.Context, deps AuthenticationDependencies, refreshTTL time.Duration) (*AuthenticationService, error) {
	var service *AuthenticationService
	var err error

	if deps.Credentials == nil || deps.Invalidator == nil || deps.Roles == nil || deps.Hasher == nil || deps.Tokens == nil ||
		deps.Sessions == nil || deps.Rotator == nil || deps.Revoker == nil || refreshTTL < time.Second || refreshTTL > 90*24*time.Hour {
		err = ErrInvalidAuthenticationConfig
	} else {
		// Unknown accounts still run the same password verification work.
		hash, hashErr := deps.Hasher.Hash(ctx, rand.Text())

		if hashErr != nil {
			err = fmt.Errorf("create dummy credential: %w", hashErr)
		} else {
			service = &AuthenticationService{dependencies: deps, refreshTTL: refreshTTL, dummyHash: hash,
				verifications: semaphore.NewWeighted(4), now: time.Now}
		}
	}

	return service, err
}

func (s *AuthenticationService) SignIn(ctx context.Context, request input.SignInInput) (security.TokenPair, error) {
	var pair security.TokenPair
	var err error

	if validationErr := request.Validate(); validationErr != nil {
		err = fmt.Errorf("sign in: %w", validationErr)
	} else {
		credentials, lookupErr := s.dependencies.Credentials.FindByEmail(ctx, security.Email(request.Email))
		missing := errors.Is(lookupErr, security.ErrUserNotFound) || (lookupErr == nil && credentials == nil)

		if lookupErr != nil && !missing {
			err = fmt.Errorf("load sign-in credentials: %w", lookupErr)
		} else {
			hash := s.dummyHash

			if !missing && credentials.User != nil {
				hash = credentials.User.PasswordHash()
			}

			matches, verifyErr := s.verifyPassword(ctx, request.Password, hash)

			if verifyErr != nil {
				err = fmt.Errorf("verify sign-in credentials: %w", verifyErr)
			} else if missing || credentials.User == nil || credentials.Version == uuid.Nil || !matches {
				err = security.ErrInvalidCredentials
			} else {
				pair, err = s.createSession(ctx, credentials)
			}
		}
	}

	return pair, err
}

func (s *AuthenticationService) verifyPassword(ctx context.Context, password string, hash security.PasswordHash) (bool, error) {
	var matches bool
	err := ctx.Err()

	if err == nil {
		if !s.verifications.TryAcquire(1) {
			err = ErrAuthenticationBusy
		} else {
			defer s.verifications.Release(1)
			matches, err = s.dependencies.Hasher.Verify(ctx, password, hash)
		}
	}

	return matches, err
}

func (s *AuthenticationService) createSession(ctx context.Context, credentials *security.Credentials) (security.TokenPair, error) {
	var pair security.TokenPair
	id, err := uuid.NewRandom()

	if err != nil {
		err = fmt.Errorf("generate session ID: %w", err)
	} else {
		expires := s.now().UTC().Truncate(time.Second).Add(s.refreshTTL)
		pair, err = s.dependencies.Tokens.Issue(ctx, credentials.User.ID(), id, expires)

		if err == nil {
			session := security.Session{ID: id, UserID: credentials.User.ID(), CredentialVersion: credentials.Version,
				RefreshHash: tokenHash(pair.RefreshToken), ExpiresAt: pair.RefreshExpiresAt}

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

func (s *AuthenticationService) Refresh(ctx context.Context, raw string) (security.TokenPair, error) {
	var pair security.TokenPair
	claims, err := s.dependencies.Tokens.Verify(ctx, raw, security.RefreshToken)

	if err == nil {
		session, _, loadErr := s.loadSession(ctx, claims)

		if loadErr != nil {
			err = loadErr
		} else if !session.ExpiresAt.Equal(claims.ExpiresAt) {
			err = security.ErrInvalidToken
		} else {
			pair, err = s.dependencies.Tokens.Issue(ctx, claims.UserID, claims.SessionID, session.ExpiresAt)

			if err == nil {
				// Compare the presented token, not the current stored hash. Old
				// signed tokens must reach the atomic replay detection path.
				session.RefreshHash = tokenHash(raw)
				err = s.dependencies.Rotator.Rotate(ctx, session, tokenHash(pair.RefreshToken))

				if err != nil {
					err = fmt.Errorf("rotate refresh token: %w", err)
				}
			}
		}
	}

	if err != nil {
		pair = security.TokenPair{}
	}

	return pair, err
}

func (s *AuthenticationService) loadSession(ctx context.Context, claims security.TokenClaims) (security.Session, *security.Credentials, error) {
	var credentials *security.Credentials
	session, err := s.dependencies.Sessions.FindByID(ctx, claims.UserID, claims.SessionID)

	if err != nil {
		err = fmt.Errorf("load authenticated session: %w", err)
	} else {
		credentials, err = s.dependencies.Credentials.FindByUserID(ctx, claims.UserID)

		if errors.Is(err, security.ErrUserNotFound) {
			err = security.ErrSessionRevoked
		} else if err != nil {
			err = fmt.Errorf("load current credentials: %w", err)
		} else if credentials == nil || credentials.User == nil || credentials.User.ID() != claims.UserID ||
			credentials.Version == uuid.Nil || credentials.Version != session.CredentialVersion || !session.ExpiresAt.After(s.now()) {
			err = security.ErrSessionRevoked
		}
	}

	return session, credentials, err
}

func (s *AuthenticationService) Authenticate(ctx context.Context, raw string) (security.Principal, error) {
	var principal security.Principal
	claims, err := s.dependencies.Tokens.Verify(ctx, raw, security.AccessToken)

	if err == nil {
		_, credentials, loadErr := s.loadSession(ctx, claims)

		if loadErr != nil {
			err = loadErr
		} else {
			role, roleErr := s.dependencies.Roles.FindByID(ctx, credentials.User.RoleID())

			if errors.Is(roleErr, security.ErrRoleNotFound) || (roleErr == nil && role == nil) {
				err = security.ErrPermissionDenied
			} else if roleErr != nil {
				err = fmt.Errorf("load current authorization role: %w", roleErr)
			} else {
				principal = security.Principal{UserID: claims.UserID, SessionID: claims.SessionID,
					RoleID: role.ID(), Permissions: role.Permissions()}
			}
		}
	}

	return principal, err
}

func (s *AuthenticationService) Authorize(ctx context.Context, raw string, permission security.Permission) (security.Principal, error) {
	var principal security.Principal
	var err error

	if validationErr := permission.Validate(); validationErr != nil {
		err = fmt.Errorf("authorize permission: %w", validationErr)
	} else {
		principal, err = s.Authenticate(ctx, raw)

		if err == nil && !principal.HasPermission(permission) {
			principal = security.Principal{}
			err = security.ErrPermissionDenied
		}
	}

	return principal, err
}

func (s *AuthenticationService) SignOut(ctx context.Context, raw string, use security.TokenUse) error {
	claims, err := s.dependencies.Tokens.Verify(ctx, raw, use)

	if err == nil {
		err = s.dependencies.Revoker.Revoke(ctx, claims.UserID, claims.SessionID)

		if err != nil {
			err = fmt.Errorf("sign out session: %w", err)
		}
	}

	return err
}

func (s *AuthenticationService) SignOutAll(ctx context.Context, access string) error {
	principal, err := s.Authenticate(ctx, access)

	if err == nil {
		err = s.RevokeUserSessions(ctx, principal.UserID)
	}

	return err
}

// RevokeUserSessions is for trusted application use cases. HTTP callers must
// authenticate and authorize before choosing another user's ID.
func (s *AuthenticationService) RevokeUserSessions(ctx context.Context, userID security.UserID) error {
	var err error

	if uuid.UUID(userID) == uuid.Nil {
		err = security.ErrInvalidSession
	} else if invalidateErr := s.dependencies.Invalidator.Invalidate(ctx, userID); invalidateErr != nil {
		err = fmt.Errorf("invalidate user credentials: %w", invalidateErr)
	} else if revokeErr := s.dependencies.Revoker.RevokeAll(ctx, userID); revokeErr != nil {
		err = fmt.Errorf("revoke user sessions: %w", revokeErr)
	}

	return err
}

func tokenHash(raw string) string {
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
