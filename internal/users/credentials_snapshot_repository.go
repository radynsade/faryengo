package users

import (
	"context"

	"github.com/google/uuid"
)

//
// Credentials repository
//

// A missing User is reported as ErrUserNotFound. RotateVersion replaces the
// version even when the credentials stay the same, which revokes every Session
// of the User.

type CredentialsSnapshotRepository interface {
	FindByEmail(ctx context.Context, email Email) (*CredentialsSnapshot, error)
	FindVersionByUserID(ctx context.Context, id UserID) (uuid.UUID, error)
	RotateVersion(ctx context.Context, id UserID) error
}
