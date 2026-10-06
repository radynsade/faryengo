package app

import (
	"context"
	"crypto/rand"
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

// Consumer interfaces keep persistence implementations outside the service.
type CredentialRepository interface {
	FindByEmail(context.Context, security.Email) (*security.Credentials, error)
	FindByUserID(context.Context, security.UserID) (*security.Credentials, error)
}

type AuthorizationRoleRepository interface {
	FindByID(context.Context, security.RoleID) (*security.Role, error)
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
	Revoker     SessionRevoker
}

type AuthenticationService struct {
	dependencies  AuthenticationDependencies
	dummyHash     security.PasswordHash
	verifications *semaphore.Weighted
	now           func() time.Time
}

func NewAuthenticationService(ctx context.Context, deps AuthenticationDependencies) (*AuthenticationService, error) {
	var service *AuthenticationService
	var err error

	if deps.Credentials == nil || deps.Invalidator == nil || deps.Roles == nil || deps.Hasher == nil || deps.Revoker == nil {
		err = ErrInvalidAuthenticationConfig
	} else {
		hash, hashErr := deps.Hasher.Hash(ctx, rand.Text())

		if hashErr != nil {
			err = fmt.Errorf("create dummy credential: %w", hashErr)
		} else {
			service = &AuthenticationService{dependencies: deps, dummyHash: hash, verifications: semaphore.NewWeighted(4), now: time.Now}
		}
	}

	return service, err
}

func (s *AuthenticationService) VerifyCredentials(ctx context.Context, request input.SignInInput) (*security.Credentials, error) {
	var result *security.Credentials
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
				result = credentials
			}
		}
	}

	return result, err
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

// loadCredentials checks durable identity and revocation state for every mechanism.
// Session repositories must check their own expiration and revocation generation.
func (s *AuthenticationService) loadCredentials(ctx context.Context, session security.Session) (*security.Credentials, error) {
	var credentials *security.Credentials
	var err error

	if session.ID == uuid.Nil || uuid.UUID(session.UserID) == uuid.Nil || !session.ExpiresAt.After(s.now()) {
		err = security.ErrSessionRevoked
	} else {
		credentials, err = s.dependencies.Credentials.FindByUserID(ctx, session.UserID)

		if errors.Is(err, security.ErrUserNotFound) {
			err = security.ErrSessionRevoked
		} else if err != nil {
			err = fmt.Errorf("load current credentials: %w", err)
		} else if credentials == nil || credentials.User == nil || credentials.User.ID() != session.UserID ||
			credentials.Version == uuid.Nil || credentials.Version != session.CredentialVersion {
			err = security.ErrSessionRevoked
		}
	}

	if err != nil {
		credentials = nil
	}

	return credentials, err
}

// ResolvePrincipal loads the same current identity and permissions for every
// authentication mechanism, after that mechanism has verified its credential.
func (s *AuthenticationService) ResolvePrincipal(ctx context.Context, session security.Session) (security.Principal, error) {
	var principal security.Principal
	credentials, err := s.loadCredentials(ctx, session)

	if err == nil {
		role, roleErr := s.dependencies.Roles.FindByID(ctx, credentials.User.RoleID())

		if errors.Is(roleErr, security.ErrRoleNotFound) || (roleErr == nil && role == nil) {
			err = security.ErrPermissionDenied
		} else if roleErr != nil {
			err = fmt.Errorf("load current authorization role: %w", roleErr)
		} else {
			principal = security.Principal{UserID: session.UserID, SessionID: session.ID,
				FirstName: credentials.User.FirstName(), LastName: credentials.User.LastName(), Email: credentials.User.Email(),
				RoleID: role.ID(), Permissions: role.Permissions(), IsSuper: role.IsSuper()}
		}
	}

	return principal, err
}

func (s *AuthenticationService) SignOut(ctx context.Context, principal security.Principal) error {
	var err error

	if uuid.UUID(principal.UserID) == uuid.Nil || principal.SessionID == uuid.Nil {
		err = security.ErrInvalidSession
	} else if revokeErr := s.dependencies.Revoker.Revoke(ctx, principal.UserID, principal.SessionID); revokeErr != nil {
		err = fmt.Errorf("sign out session: %w", revokeErr)
	}

	return err
}

func (s *AuthenticationService) SignOutAll(ctx context.Context, principal security.Principal) error {
	return s.RevokeUserSessions(ctx, principal.UserID)
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
