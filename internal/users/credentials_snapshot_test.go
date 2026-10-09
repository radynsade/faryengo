package users

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestCredentialsSnapshotValidate(t *testing.T) {
	user := NewUser(
		UserID(uuid.Must(uuid.NewV7())),
		RoleID(uuid.Must(uuid.NewV7())),
		"ada@example.com",
		time.Now(),
		"+15551234567",
		time.Now(),
		"hash",
		time.Now(),
		"Ada",
		"Lovelace",
		time.Now(),
		time.Now(),
	)

	tests := []struct {
		name     string
		snapshot *CredentialsSnapshot
		want     error
	}{
		{"valid", NewCredentialsSnapshot(user, uuid.Must(uuid.NewV7())), nil},
		{"nil", nil, ErrCredentialsSnapshotNil},
		{"missing user", NewCredentialsSnapshot(nil, uuid.Must(uuid.NewV7())), ErrUserNil},
		{"invalid user", NewCredentialsSnapshot(&User{}, uuid.Must(uuid.NewV7())), ErrUserInvalid},
		{"nil version", NewCredentialsSnapshot(user, uuid.Nil), ErrCredentialsVersionNil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.snapshot.Validate()

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if test.want != nil && !errors.Is(err, ErrCredentialsSnapshotInvalid) {
				t.Fatalf("got %v, want it wrapped in %v", err, ErrCredentialsSnapshotInvalid)
			}
		})
	}
}
