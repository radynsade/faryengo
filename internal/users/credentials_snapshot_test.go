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
		user     *User
		version  uuid.UUID
		wantErrs []error
	}{
		{"valid", user, uuid.Must(uuid.NewV7()), nil},
		{"missing user", nil, uuid.Must(uuid.NewV7()), []error{ErrUserNil}},
		{"invalid user", &User{}, uuid.Must(uuid.NewV7()), []error{ErrUserInvalid}},
		{"nil version", user, uuid.Nil, []error{ErrCredentialsVersionNil}},
		{"invalid user and nil version", &User{}, uuid.Nil, []error{ErrUserInvalid, ErrCredentialsVersionNil}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := NewCredentialsSnapshot(test.user, test.version)

			if snapshot == nil || snapshot.User != test.user || snapshot.Version != test.version {
				t.Fatalf("got %+v, want the given user and version", snapshot)
			}

			err := snapshot.Validate()

			if (err == nil) != (len(test.wantErrs) == 0) {
				t.Fatalf("got %v, want %v", err, test.wantErrs)
			}

			for _, want := range test.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("got %v, want %v", err, want)
				}
			}

			if err != nil && !errors.Is(err, ErrCredentialsSnapshotInvalid) {
				t.Fatalf("got %v, want it wrapped in %v", err, ErrCredentialsSnapshotInvalid)
			}
		})
	}
}

func TestCredentialsSnapshotValidateNil(t *testing.T) {
	err := (*CredentialsSnapshot)(nil).Validate()

	if !errors.Is(err, ErrCredentialsSnapshotNil) || !errors.Is(err, ErrCredentialsSnapshotInvalid) {
		t.Fatalf("got %v, want %v wrapped in %v", err, ErrCredentialsSnapshotNil, ErrCredentialsSnapshotInvalid)
	}
}
