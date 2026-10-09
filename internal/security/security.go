package security

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/radynsade/faryengo/internal/users"
)

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

type Identity struct {
	Session *Session
	User    *users.User
	Role    *users.Role
}

func (i *Identity) Can(permission users.Permission) bool {
	return i != nil && i.Role.Grants(permission)
}

type Authenticator[CredentialsType any] interface {
	SignIn(ctx context.Context, credentials CredentialsType) (*Session, error)
	SignOut(ctx context.Context, session *Session, all bool) error
}
