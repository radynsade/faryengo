package security

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Session
//

var (
	ErrCredentialsInvalid           = errors.New("invalid credentials")
	ErrSessionInvalid               = errors.New("invalid session")
	ErrSessionRevoked               = errors.New("session revoked or expired")
	ErrSessionNil                   = errors.New("is nil")
	ErrSessionNilID                 = errors.New("id is nil")
	ErrSessionNilCredentialsVersion = errors.New("credentials version is nil")
	ErrSessionZeroExpiresAt         = errors.New("expires at is zero")
)

type Session struct {
	ID                 uuid.UUID
	UserID             users.UserID
	CredentialsVersion uuid.UUID
	ExpiresAt          time.Time
}

func (s *Session) Validate() error {
	var err error

	if s == nil {
		err = ErrSessionNil
	} else if s.ID == uuid.Nil {
		err = ErrSessionNilID
	}

	if err == nil {
		err = s.UserID.Validate()
	}

	if err == nil && s.CredentialsVersion == uuid.Nil {
		err = ErrSessionNilCredentialsVersion
	}

	if err == nil && s.ExpiresAt.IsZero() {
		err = ErrSessionZeroExpiresAt
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrSessionInvalid, err)
	}

	return err
}

//
// Identity
//

type Identity struct {
	Session *Session
	User    *users.User
	Role    *users.Role
}

// An operator acting outside any account, such as through the CLI, holds the
// system identity: it has no Session or User, and its super Role grants every
// permission. Only entry points that act for an operator create it; transports
// that serve accounts never do.

func NewSystemIdentity() *Identity {
	return &Identity{Role: &users.Role{IsSuper: true}}
}

func (i *Identity) Can(permission users.Permission) bool {
	return i != nil && i.Role.Grants(permission)
}

//
// Identity context
//

// The identity of the caller travels to application services in the context.
// A context without one, or with a nil one, reports ErrIdentityMissing.

var ErrIdentityMissing = errors.New("no identity in the context")

type identityContextKey struct{}

func WithIdentity(ctx context.Context, identity *Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (*Identity, error) {
	var err error

	identity, _ := ctx.Value(identityContextKey{}).(*Identity)

	if identity == nil {
		err = ErrIdentityMissing
	}

	return identity, err
}

//
// Authenticator
//

type Authenticator[CredentialsType any] interface {
	SignIn(ctx context.Context, credentials CredentialsType) (*Session, error)
	SignOut(ctx context.Context, session *Session, all bool) error
}
