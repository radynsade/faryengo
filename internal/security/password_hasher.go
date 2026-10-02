package security

import (
	"context"
	"errors"
)

const MaxPasswordBytes = 4096

var ErrInvalidPassword = errors.New("invalid password")

// PasswordHasher creates password credentials and checks candidate passwords.
type PasswordHasher interface {
	Hash(ctx context.Context, password string) (PasswordHash, error)
	Verify(ctx context.Context, password string, hash PasswordHash) (bool, error)
}
