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

func NewCredentialsSnapshot(user *User, version uuid.UUID) (*CredentialsSnapshot, error) {
	var (
		err      error
		snapshot *CredentialsSnapshot
	)

	snapshot = &CredentialsSnapshot{
		User:    user,
		Version: version,
	}

	err = snapshot.Validate()
	return snapshot, err
}

func (s *CredentialsSnapshot) Validate() error {
	var err error

	if s == nil {
		err = ErrCredentialsSnapshotNil
	} else {
		err = errors.Join(
			s.User.Validate(),
			s.validateVersion(),
		)
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrCredentialsSnapshotInvalid, err)
	}

	return err
}

func (s *CredentialsSnapshot) validateVersion() error {
	var err error

	if s.Version == uuid.Nil {
		err = ErrCredentialsVersionNil
	}

	return err
}
