package emailpass_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/emailpass/redis"
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

func newAttempts(t *testing.T) (*redis.AttemptStorage, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr(), MaxRetries: -1})

	t.Cleanup(func() { _ = client.Close() })

	attempts, err := redis.NewAttemptStorage(client)

	if err != nil {
		t.Fatalf("create attempt storage: %v", err)
	}

	return attempts, server
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

	snapshot := users.NewCredentialsSnapshot(user, uuid.Must(uuid.NewV7()))

	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}

	return snapshot
}

func TestNewAuthenticatorRejectsInvalidConfig(t *testing.T) {
	ctx := context.Background()
	credentials := &mock.CredentialsRepository{}
	hasher := newHasher(new([]users.PasswordHash))
	attempts, _ := newAttempts(t)

	tests := []struct {
		name        string
		credentials users.CredentialsSnapshotRepository
		hasher      users.PasswordHasher
		attempts    emailpass.AttemptStorage
		ttl         time.Duration
	}{
		{"missing credentials", nil, hasher, attempts, time.Hour},
		{"missing hasher", credentials, nil, attempts, time.Hour},
		{"missing attempt storage", credentials, hasher, nil, time.Hour},
		{"too short TTL", credentials, hasher, attempts, emailpass.MinSessionTTL - time.Nanosecond},
		{"too long TTL", credentials, hasher, attempts, emailpass.MaxSessionTTL + time.Nanosecond},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := emailpass.NewAuthenticator(ctx, test.credentials, test.hasher, test.attempts, test.ttl)

			if !errors.Is(err, emailpass.ErrConfigInvalid) {
				t.Fatalf("got %v, want %v", err, emailpass.ErrConfigInvalid)
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
		credentials emailpass.Credentials
		findErr     error
		want        error
		wantHash    string
	}{
		{"matching password", emailpass.Credentials{Email: testEmail, Password: testPassword}, nil, nil, testHash},
		{"wrong password", emailpass.Credentials{Email: testEmail, Password: "wrong password"}, nil, emailpass.ErrCredentialsMismatch, testHash},
		{"unknown email", emailpass.Credentials{Email: "bob@example.com", Password: testPassword}, users.ErrUserNotFound, emailpass.ErrCredentialsMismatch, "dummy"},
		{"storage failure", emailpass.Credentials{Email: testEmail, Password: testPassword}, storageErr, storageErr, ""},
		{"invalid email", emailpass.Credentials{Email: "Ada <ada@example.com>", Password: testPassword}, nil, emailpass.ErrCredentialsEmailInvalid, ""},
		{"empty password", emailpass.Credentials{Email: testEmail, Password: ""}, nil, emailpass.ErrCredentialsPasswordInvalid, ""},
		{"too long password", emailpass.Credentials{
			Email:    testEmail,
			Password: strings.Repeat("a", users.MaxPasswordBytes+1),
		}, nil, emailpass.ErrCredentialsPasswordInvalid, ""},
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

			attempts, _ := newAttempts(t)
			authenticator, err := emailpass.NewAuthenticator(ctx, credentials, newHasher(&verified), attempts, time.Hour)

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

	attempts, _ := newAttempts(t)
	authenticator, err := emailpass.NewAuthenticator(ctx, credentials, newHasher(new([]users.PasswordHash)), attempts, time.Hour)

	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = authenticator.SignIn(ctx, emailpass.Credentials{Email: testEmail, Password: "wrong password"})

	if !errors.Is(err, security.ErrCredentialsInvalid) {
		t.Fatalf("got %v, want %v", err, security.ErrCredentialsInvalid)
	}
}

func TestSignInBoundsConcurrentVerifications(t *testing.T) {
	ctx := context.Background()
	snapshot := newSnapshot(t)
	release := make(chan struct{})
	started := make(chan struct{}, emailpass.MaxConcurrentVerifications)

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

	attempts, _ := newAttempts(t)
	authenticator, err := emailpass.NewAuthenticator(ctx, credentials, hasher, attempts, time.Hour)

	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var group sync.WaitGroup

	for range emailpass.MaxConcurrentVerifications {
		group.Go(func() {
			_, _ = authenticator.SignIn(ctx, emailpass.Credentials{Email: testEmail, Password: testPassword})
		})
	}

	for range emailpass.MaxConcurrentVerifications {
		<-started
	}

	// The extra attempt gives up once its deadline passes before a
	// verification frees up, however long MaxVerificationWait is.
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	_, err = authenticator.SignIn(waitCtx, emailpass.Credentials{Email: testEmail, Password: testPassword})

	cancel()
	close(release)
	group.Wait()

	if !errors.Is(err, emailpass.ErrVerificationBusy) {
		t.Fatalf("got %v, want %v", err, emailpass.ErrVerificationBusy)
	}
}

func TestSignInBoundsAttempts(t *testing.T) {
	ctx := context.Background()
	snapshot := newSnapshot(t)
	wrong := emailpass.Credentials{Email: testEmail, Password: "wrong password", Client: "192.0.2.1"}

	tests := []struct {
		name      string
		failures  int
		last      emailpass.Credentials
		forward   time.Duration
		want      error
		wantCheck bool
	}{
		{
			name:      "the last allowed attempt per email is verified",
			failures:  emailpass.MaxAttemptsPerEmail - 1,
			last:      emailpass.Credentials{Email: testEmail, Password: testPassword, Client: "192.0.2.1"},
			wantCheck: true,
		},
		{
			name:     "an attempt over the email limit is refused unverified",
			failures: emailpass.MaxAttemptsPerEmail,
			last:     emailpass.Credentials{Email: testEmail, Password: testPassword, Client: "192.0.2.2"},
			want:     emailpass.ErrAttemptsExceeded,
		},
		{
			name:     "the email limit ignores case",
			failures: emailpass.MaxAttemptsPerEmail,
			last:     emailpass.Credentials{Email: "ADA@example.com", Password: testPassword, Client: "192.0.2.2"},
			want:     emailpass.ErrAttemptsExceeded,
		},
		{
			name:     "an attempt over the client limit is refused unverified",
			failures: emailpass.MaxAttemptsPerClient,
			last:     emailpass.Credentials{Email: testEmail, Password: testPassword, Client: "192.0.2.1"},
			want:     emailpass.ErrAttemptsExceeded,
		},
		{
			name:      "attempts are forgotten after the window",
			failures:  emailpass.MaxAttemptsPerClient,
			last:      emailpass.Credentials{Email: testEmail, Password: testPassword, Client: "192.0.2.1"},
			forward:   emailpass.AttemptWindow,
			wantCheck: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verified := 0
			attempts, server := newAttempts(t)

			credentials := &mock.CredentialsRepository{
				FindByEmailFunc: func(context.Context, users.Email) (*users.CredentialsSnapshot, error) {
					return snapshot, nil
				},
			}

			hasher := newHasher(new([]users.PasswordHash))
			verify := hasher.VerifyFunc
			hasher.VerifyFunc = func(ctx context.Context, password string, hash users.PasswordHash) (bool, error) {
				verified++

				return verify(ctx, password, hash)
			}

			authenticator, err := emailpass.NewAuthenticator(ctx, credentials, hasher, attempts, time.Hour)

			if err != nil {
				t.Fatalf("create: %v", err)
			}

			// Spreading the failures over emails keeps them from reaching
			// the email limit when the client limit is under test.
			for index := range test.failures {
				failure := wrong

				if test.failures > emailpass.MaxAttemptsPerEmail {
					failure.Email = users.Email(fmt.Sprintf("user%d@example.com", index))
				}

				_, _ = authenticator.SignIn(ctx, failure)
			}

			server.FastForward(test.forward)

			verified = 0
			_, err = authenticator.SignIn(ctx, test.last)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if test.wantCheck != (verified == 1) {
				t.Fatalf("verified %d times, want verified %v", verified, test.wantCheck)
			}
		})
	}
}

func TestSignInForgetsEmailAttemptsOnSuccess(t *testing.T) {
	ctx := context.Background()
	snapshot := newSnapshot(t)
	attempts, _ := newAttempts(t)

	credentials := &mock.CredentialsRepository{
		FindByEmailFunc: func(context.Context, users.Email) (*users.CredentialsSnapshot, error) {
			return snapshot, nil
		},
	}

	authenticator, err := emailpass.NewAuthenticator(ctx, credentials, newHasher(new([]users.PasswordHash)), attempts, time.Hour)

	if err != nil {
		t.Fatalf("create: %v", err)
	}

	wrong := emailpass.Credentials{Email: testEmail, Password: "wrong password"}
	right := emailpass.Credentials{Email: testEmail, Password: testPassword}

	for range emailpass.MaxAttemptsPerEmail - 1 {
		_, _ = authenticator.SignIn(ctx, wrong)
	}

	if _, err = authenticator.SignIn(ctx, right); err != nil {
		t.Fatalf("sign in: %v", err)
	}

	for range emailpass.MaxAttemptsPerEmail - 1 {
		_, _ = authenticator.SignIn(ctx, wrong)
	}

	if _, err = authenticator.SignIn(ctx, right); err != nil {
		t.Fatalf("got %v after a successful sign-in reset the count, want nil", err)
	}
}

func TestSignInFailsClosedWhenAttemptStorageIsUnavailable(t *testing.T) {
	ctx := context.Background()
	snapshot := newSnapshot(t)
	attempts, server := newAttempts(t)
	verified := new([]users.PasswordHash)

	credentials := &mock.CredentialsRepository{
		FindByEmailFunc: func(context.Context, users.Email) (*users.CredentialsSnapshot, error) {
			return snapshot, nil
		},
	}

	authenticator, err := emailpass.NewAuthenticator(ctx, credentials, newHasher(verified), attempts, time.Hour)

	if err != nil {
		t.Fatalf("create: %v", err)
	}

	server.Close()

	session, err := authenticator.SignIn(ctx, emailpass.Credentials{Email: testEmail, Password: testPassword})

	if err == nil || session != nil || len(*verified) != 0 {
		t.Fatalf("got (%+v, %v) with %d verifications, want a storage failure and none", session, err, len(*verified))
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
		{"one session is not stored here", session, false, emailpass.ErrSessionNotStored, false},
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

			attempts, _ := newAttempts(t)
			authenticator, err := emailpass.NewAuthenticator(ctx, credentials, newHasher(new([]users.PasswordHash)), attempts, time.Hour)

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
