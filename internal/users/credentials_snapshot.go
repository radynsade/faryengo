package users

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

//
// Credentials snapshot
//

var (
	ErrCredentialsSnapshotInvalid = errors.New("invalid credentials snapshot")
	ErrCredentialsSnapshotNil     = errors.New("is nil")
	ErrCredentialsVersionNil      = errors.New("credentials version is nil")
)

// A credentials snapshot pairs a User with the credentials version current
// when it was read. Any change to the email or password hash replaces the
// version, so a Session holding an older version can no longer authenticate.

type CredentialsSnapshot struct {
	User    *User
	Version uuid.UUID
}

func NewCredentialsSnapshot(user *User, version uuid.UUID) *CredentialsSnapshot {
	return &CredentialsSnapshot{
		User:    user,
		Version: version,
	}
}

func (s *CredentialsSnapshot) Validate() error {
	var err error

	if s == nil {
		err = ErrCredentialsSnapshotNil
	} else {
		err = s.User.Validate()
	}

	if err == nil && s.Version == uuid.Nil {
		err = ErrCredentialsVersionNil
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrCredentialsSnapshotInvalid, err)
	}

	return err
}
