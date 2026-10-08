package security

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Authentication snapshot

type AuthenticationSnapshot struct {
	User    *User
	Version uuid.UUID
}

var ErrInvalidAuthenticationSnapshot = errors.New("invalid authentication snapshot")

func NewAuthenticationSnapshot(user *User, version uuid.UUID) *AuthenticationSnapshot {
	return &AuthenticationSnapshot{User: user, Version: version}
}

func (s *AuthenticationSnapshot) Validate() error {
	var err error

	if s == nil || s.User == nil || s.Version == uuid.Nil {
		err = ErrInvalidAuthenticationSnapshot
	} else if validationErr := s.User.Validate(); validationErr != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidAuthenticationSnapshot, validationErr)
	}

	return err
}

type AuthenticationSnapshotRepository interface {
	FindByEmail(ctx context.Context, email Email) (*AuthenticationSnapshot, error)
	FindByUserID(ctx context.Context, id UserID) (*AuthenticationSnapshot, error)
}

type AuthenticationSnapshotInvalidator interface {
	Invalidate(ctx context.Context, id UserID) error
}

// Session

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionRevoked     = errors.New("session revoked or expired")
	ErrInvalidSession     = errors.New("invalid session")
)

type Session struct {
	ID                            uuid.UUID
	UserID                        UserID
	AuthenticationSnapshotVersion uuid.UUID
	ExpiresAt                     time.Time
}

func NewSession(id uuid.UUID, userID UserID, version uuid.UUID, expiresAt time.Time) *Session {
	return &Session{ID: id, UserID: userID, AuthenticationSnapshotVersion: version, ExpiresAt: expiresAt}
}

// Validate checks intrinsic state. Expiration and current account versions are
// checked by application services and the session repository.
func (s Session) Validate() error {
	var err error

	if s.ID == uuid.Nil || s.UserID.Validate() != nil || s.AuthenticationSnapshotVersion == uuid.Nil || s.ExpiresAt.IsZero() {
		err = ErrInvalidSession
	}

	return err
}

// SessionGrant contains the opaque bearer credential issued to a browser.
// Its ID is a secret, distinct from the internal Session.ID.
type SessionGrant struct {
	ID        string
	ExpiresAt time.Time
}

type SessionRepository interface {
	CreateSession(ctx context.Context, session Session, digest string) error
	FindSession(ctx context.Context, digest string) (Session, error)
}

type SessionRevoker interface {
	Revoke(ctx context.Context, userID UserID, sessionID uuid.UUID) error
	RevokeAll(ctx context.Context, userID UserID) error
}

// Principal

type Principal struct {
	UserID      UserID
	RoleID      RoleID
	Permissions []Permission
	IsSuper     bool
	FirstName   FirstName
	LastName    LastName
	Email       Email
	Phone       Phone
}

var ErrInvalidPrincipal = errors.New("invalid principal")

func NewPrincipal(userID UserID, roleID RoleID, permissions []Permission, isSuper bool,
	firstName FirstName, lastName LastName, email Email, phone Phone) *Principal {
	return &Principal{UserID: userID, RoleID: roleID, Permissions: slices.Clone(permissions),
		IsSuper: isSuper, FirstName: firstName, LastName: lastName, Email: email, Phone: phone}
}

func (p *Principal) Validate() error {
	var err error

	if p == nil {
		err = ErrInvalidPrincipal
	} else {
		err = p.UserID.Validate()

		if err == nil {
			err = p.RoleID.Validate()
		}

		if err == nil {
			err = p.FirstName.Validate()
		}

		if err == nil {
			err = p.LastName.Validate()
		}

		if err == nil {
			err = p.Email.Validate()
		}

		if err == nil {
			err = p.Phone.Validate()
		}

		if err == nil {
			err = Permissions(p.Permissions).Validate()
		}

		if err != nil {
			err = fmt.Errorf("%w: %w", ErrInvalidPrincipal, err)
		}
	}

	return err
}

func (p Principal) HasPermission(permission Permission) bool {
	return permission.Validate() == nil && (p.IsSuper || slices.Contains(p.Permissions, permission))
}

type PrincipalRepository interface {
	FindByEmail(ctx context.Context, email Email) (*Principal, error)
	FindByUserID(ctx context.Context, id UserID) (*Principal, error)
}

// Services

type SessionResolver[C any] interface {
	Resolve(ctx context.Context, credentials C) (*Session, error)
}

type Authenticator[C any] interface {
	Authenticate(ctx context.Context, credentials C) (*Principal, error)
}
