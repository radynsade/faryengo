package mock

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
)

//
// Errors
//

var ErrNotConfigured = errors.New("mock operation is not configured")

//
// Credentials repository
//

type CredentialsRepository struct {
	FindByEmailFunc         func(ctx context.Context, email users.Email) (*users.CredentialsSnapshot, error)
	FindVersionByUserIDFunc func(ctx context.Context, id users.UserID) (uuid.UUID, error)
	RotateVersionFunc       func(ctx context.Context, id users.UserID) error
}

var _ users.CredentialsSnapshotRepository = (*CredentialsRepository)(nil)

// Find by an email

func (r *CredentialsRepository) FindByEmail(
	ctx context.Context,
	email users.Email,
) (*users.CredentialsSnapshot, error) {
	var (
		snapshot *users.CredentialsSnapshot
		err      = ErrNotConfigured
	)

	if r.FindByEmailFunc != nil {
		snapshot, err = r.FindByEmailFunc(ctx, email)
	}

	return snapshot, err
}

// Find a version by a user ID

func (r *CredentialsRepository) FindVersionByUserID(
	ctx context.Context,
	id users.UserID,
) (uuid.UUID, error) {
	var (
		version uuid.UUID
		err     = ErrNotConfigured
	)

	if r.FindVersionByUserIDFunc != nil {
		version, err = r.FindVersionByUserIDFunc(ctx, id)
	}

	return version, err
}

// Rotate a version

func (r *CredentialsRepository) RotateVersion(
	ctx context.Context,
	id users.UserID,
) error {
	err := ErrNotConfigured

	if r.RotateVersionFunc != nil {
		err = r.RotateVersionFunc(ctx, id)
	}

	return err
}
