package emailpass

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/mock"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

const (
	testEmail    = "ada@example.com"
	testPassword = "correct horse"
	testHash     = "hash:" + testPassword
	dummyPrefix  = "hash:"
)

func newHasher(verified *[]users.PasswordHash) *mock.PasswordHasher {
	var mutex sync.Mutex

	return &mock.PasswordHasher{
		HashFunc: func(_ context.Context, password string) (users.PasswordHash, error) {
			return users.PasswordHash(dummyPrefix + password), nil
		},
		VerifyFunc: func(_ context.Context, password string, hash users.PasswordHash) (bool, error) {
			mutex.Lock()
			*verified = append(*verified, hash)
			mutex.Unlock()

			return string(hash) == dummyPrefix+password, nil
		},
	}
}

func newSnapshot(t *testing.T) *users.CredentialsSnapshot {
	t.Helper()

	user := users.NewUser(
		users.UserID(uuid.Must(uuid.NewV7())),
		users.RoleID(uuid.Must(uuid.NewV7())),
		testEmail,
		time.Now(),
		"+15551234567",
		time.Now(),
		testHash,
		time.Now(),
		"Ada",
		"Lovelace",
		time.Now(),
		time.Now(),
	)

	return users.NewCredentialsSnapshot(user, uuid.Must(uuid.NewV7()))
}

func TestNewAuthenticatorRejectsInvalidConfig(t *testing.T) {
	ctx := context.Background()
	credentials := &mock.CredentialsRepository{}
	hasher := newHasher(new([]users.PasswordHash))

	tests := []struct {
		name        string
		credentials users.CredentialsSnapshotRepository
		hasher      users.PasswordHasher
		ttl         time.Duration
	}{
		{"missing credentials", nil, hasher, time.Hour},
		{"missing hasher", credentials, nil, time.Hour},
		{"too short TTL", credentials, hasher, MinSessionTTL - time.Nanosecond},
		{"too long TTL", credentials, hasher, MaxSessionTTL + time.Nanosecond},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewAuthenticator(ctx, test.credentials, test.hasher, test.ttl)

			if !errors.Is(err, ErrConfigInvalid) {
				t.Fatalf("got %v, want %v", err, ErrConfigInvalid)
			}
		})
	}
}

func TestSignIn(t *testing.T) {
	ctx := context.Background()
	snapshot := newSnapshot(t)
	storageErr := errors.New("storage unavailable")

	tests := []struct {
		name        string
		credentials Credentials
		findErr     error
		want        error
		wantHash    string
	}{
		{"matching password", Credentials{testEmail, testPassword}, nil, nil, testHash},
		{"wrong password", Credentials{testEmail, "wrong password"}, nil, ErrCredentialsMismatch, testHash},
		{"unknown email", Credentials{"bob@example.com", testPassword}, users.ErrUserNotFound, ErrCredentialsMismatch, "dummy"},
		{"storage failure", Credentials{testEmail, testPassword}, storageErr, storageErr, ""},
		{"invalid email", Credentials{"Ada <ada@example.com>", testPassword}, nil, ErrCredentialsEmailInvalid, ""},
		{"empty password", Credentials{testEmail, ""}, nil, ErrCredentialsPasswordInvalid, ""},
		{"too long password", Credentials{
			testEmail,
			strings.Repeat("a", users.MaxPasswordBytes+1),
		}, nil, ErrCredentialsPasswordInvalid, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var verified []users.PasswordHash

			credentials := &mock.CredentialsRepository{
				FindByEmailFunc: func(_ context.Context, email users.Email) (*users.CredentialsSnapshot, error) {
					var found *users.CredentialsSnapshot

					if test.findErr == nil {
						found = snapshot
					}

					return found, test.findErr
				},
			}

			authenticator, err := NewAuthenticator(ctx, credentials, newHasher(&verified), time.Hour)

			if err != nil {
				t.Fatalf("create: %v", err)
			}

			session, err := authenticator.SignIn(ctx, test.credentials)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if test.want == nil {
				if session == nil ||
					session.UserID != snapshot.User.ID ||
					session.CredentialsVersion != snapshot.Version ||
					session.Validate() != nil {
					t.Fatalf("got session %+v, want one for the snapshot", session)
				}
			} else if session != nil {
				t.Fatalf("got session %+v, want nil", session)
			}

			switch {
			case test.wantHash == "" && len(verified) != 0:
				t.Fatalf("verified %v, want no verification", verified)
			case test.wantHash == "dummy" && (len(verified) != 1 || verified[0] == testHash):
				t.Fatalf("verified %v, want the dummy hash", verified)
			case test.wantHash == testHash && (len(verified) != 1 || verified[0] != testHash):
				t.Fatalf("verified %v, want %q", verified, testHash)
			}
		})
	}
}

func TestSignInRejectsWrongPasswordAsInvalidCredentials(t *testing.T) {
	ctx := context.Background()
	snapshot := newSnapshot(t)

	credentials := &mock.CredentialsRepository{
		FindByEmailFunc: func(context.Context, users.Email) (*users.CredentialsSnapshot, error) {
			return snapshot, nil
		},
	}

	authenticator, err := NewAuthenticator(ctx, credentials, newHasher(new([]users.PasswordHash)), time.Hour)

	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = authenticator.SignIn(ctx, Credentials{testEmail, "wrong password"})

	if !errors.Is(err, security.ErrCredentialsInvalid) {
		t.Fatalf("got %v, want %v", err, security.ErrCredentialsInvalid)
	}
}

func TestSignInBoundsConcurrentVerifications(t *testing.T) {
	ctx := context.Background()
	snapshot := newSnapshot(t)
	release := make(chan struct{})
	started := make(chan struct{}, MaxConcurrentVerifications)

	credentials := &mock.CredentialsRepository{
		FindByEmailFunc: func(context.Context, users.Email) (*users.CredentialsSnapshot, error) {
			return snapshot, nil
		},
	}

	hasher := newHasher(new([]users.PasswordHash))
	hasher.VerifyFunc = func(context.Context, string, users.PasswordHash) (bool, error) {
		started <- struct{}{}
		<-release

		return true, nil
	}

	authenticator, err := NewAuthenticator(ctx, credentials, hasher, time.Hour)

	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var group sync.WaitGroup

	for range MaxConcurrentVerifications {
		group.Go(func() {
			_, _ = authenticator.SignIn(ctx, Credentials{testEmail, testPassword})
		})
	}

	for range MaxConcurrentVerifications {
		<-started
	}

	_, err = authenticator.SignIn(ctx, Credentials{testEmail, testPassword})

	close(release)
	group.Wait()

	if !errors.Is(err, ErrVerificationBusy) {
		t.Fatalf("got %v, want %v", err, ErrVerificationBusy)
	}
}

func TestSignOut(t *testing.T) {
	ctx := context.Background()
	snapshot := newSnapshot(t)

	session := &security.Session{
		ID:                 uuid.Must(uuid.NewV7()),
		UserID:             snapshot.User.ID,
		CredentialsVersion: snapshot.Version,
		ExpiresAt:          time.Now().Add(time.Hour),
	}

	tests := []struct {
		name        string
		session     *security.Session
		all         bool
		want        error
		wantRotated bool
	}{
		{"everywhere rotates the version", session, true, nil, true},
		{"one session is not stored here", session, false, ErrSessionNotStored, false},
		{"nil session", nil, true, security.ErrSessionInvalid, false},
		{"invalid session", &security.Session{}, true, security.ErrSessionInvalid, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var rotated []users.UserID

			credentials := &mock.CredentialsRepository{
				RotateVersionFunc: func(_ context.Context, id users.UserID) error {
					rotated = append(rotated, id)

					return nil
				},
			}

			authenticator, err := NewAuthenticator(ctx, credentials, newHasher(new([]users.PasswordHash)), time.Hour)

			if err != nil {
				t.Fatalf("create: %v", err)
			}

			err = authenticator.SignOut(ctx, test.session, test.all)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if test.wantRotated != (len(rotated) == 1 && rotated[0] == snapshot.User.ID) {
				t.Fatalf("rotated %v, want rotated %v", rotated, test.wantRotated)
			}
		})
	}
}
