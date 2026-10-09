package mock

import (
	"context"

	"github.com/radynsade/faryengo/internal/users"
)

//
// Password hasher
//

type PasswordHasher struct {
	HashFunc   func(ctx context.Context, password string) (users.PasswordHash, error)
	VerifyFunc func(ctx context.Context, password string, hash users.PasswordHash) (bool, error)
}

var _ users.PasswordHasher = (*PasswordHasher)(nil)

// Hash

func (h *PasswordHasher) Hash(ctx context.Context, password string) (users.PasswordHash, error) {
	var (
		hash users.PasswordHash
		err  = ErrNotConfigured
	)

	if h.HashFunc != nil {
		hash, err = h.HashFunc(ctx, password)
	}

	return hash, err
}

// Verify

func (h *PasswordHasher) Verify(
	ctx context.Context,
	password string,
	hash users.PasswordHash,
) (bool, error) {
	var (
		matches bool
		err     = ErrNotConfigured
	)

	if h.VerifyFunc != nil {
		matches, err = h.VerifyFunc(ctx, password, hash)
	}

	return matches, err
}
