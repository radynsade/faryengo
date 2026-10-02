package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
	securityjwt "github.com/radynsade/faryengo/internal/security/jwt"
	securityredis "github.com/radynsade/faryengo/internal/security/redis"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type authRepository struct {
	credentials security.Credentials
	role        *security.Role
	failure     error
	deleted     bool
}

func (r *authRepository) FindByEmail(ctx context.Context, email security.Email) (*security.Credentials, error) {
	var result *security.Credentials
	err := ctx.Err()

	if err == nil {
		if r.failure != nil {
			err = r.failure
		} else if r.deleted || !strings.EqualFold(string(email), string(r.credentials.User.Email())) {
			err = security.ErrUserNotFound
		} else {
			copy := r.credentials
			result = &copy
		}
	}

	return result, err
}

func (r *authRepository) FindByUserID(ctx context.Context, id security.UserID) (*security.Credentials, error) {
	var result *security.Credentials
	err := ctx.Err()

	if err == nil {
		if r.failure != nil {
			err = r.failure
		} else if r.deleted || id != r.credentials.User.ID() {
			err = security.ErrUserNotFound
		} else {
			copy := r.credentials
			result = &copy
		}
	}

	return result, err
}

func (r *authRepository) FindByID(ctx context.Context, id security.RoleID) (*security.Role, error) {
	var role *security.Role
	err := ctx.Err()

	if err == nil {
		if r.failure != nil {
			err = r.failure
		} else if r.role == nil || r.role.ID() != id {
			err = security.ErrRoleNotFound
		} else {
			role = r.role
		}
	}

	return role, err
}

type authHasher struct {
	calls    atomic.Int32
	onVerify func()
}

func (h *authHasher) Hash(context.Context, string) (security.PasswordHash, error) {
	return "dummy", nil
}
func (h *authHasher) Verify(ctx context.Context, password string, hash security.PasswordHash) (bool, error) {
	h.calls.Add(1)

	if h.onVerify != nil {
		h.onVerify()
	}

	return password == "correct" && hash == "stored", ctx.Err()
}

func authFixture(t *testing.T) (*AuthenticationService, *authRepository, *authHasher, *miniredis.Miniredis) {
	t.Helper()
	roleID := security.RoleID(uuid.New())
	user, err := security.NewUser(security.UserID(uuid.New()), roleID, "person@example.com", "+37123456789", "stored", "First", "Last")

	if err != nil {
		t.Fatal(err)
	}

	translation, err := languages.NewTranslation("en", "Admin")

	if err != nil {
		t.Fatal(err)
	}

	role, err := security.NewRole(roleID, languages.Text{"en": translation}, []security.Permission{security.PermissionViewUser})

	if err != nil {
		t.Fatal(err)
	}

	repository := &authRepository{credentials: security.Credentials{User: user, Version: uuid.New()}, role: role}
	mini := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: mini.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	sessions, err := securityredis.NewSessionStore(client)

	if err != nil {
		t.Fatal(err)
	}

	_, key, err := ed25519.GenerateKey(rand.Reader)

	if err != nil {
		t.Fatal(err)
	}

	tokens, err := securityjwt.NewManager(securityjwt.Config{PrivateKey: key, KeyID: "test", Issuer: "faryen", AccessAudience: "admin", RefreshAudience: "refresh", AccessTTL: 5 * time.Minute})

	if err != nil {
		t.Fatal(err)
	}

	hasher := &authHasher{}
	service, err := NewAuthenticationService(context.Background(), AuthenticationDependencies{Credentials: repository, Invalidator: repository, Roles: repository, Hasher: hasher, Tokens: tokens, Sessions: sessions, Rotator: sessions, Revoker: sessions}, 24*time.Hour)

	if err != nil {
		t.Fatal(err)
	}

	return service, repository, hasher, mini
}

func signIn(t *testing.T, s *AuthenticationService) security.TokenPair {
	t.Helper()
	pair, err := s.SignIn(context.Background(), input.SignInInput{Email: "person@example.com", Password: "correct"})

	if err != nil {
		t.Fatal(err)
	}

	return pair
}

func TestAuthenticationRevocation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		invalidate func(*testing.T, *AuthenticationService, *authRepository, *miniredis.Miniredis, security.TokenPair)
	}{
		{"password change", func(_ *testing.T, _ *AuthenticationService, r *authRepository, _ *miniredis.Miniredis, _ security.TokenPair) {
			r.credentials.Version = uuid.New()
		}},
		{"deleted user", func(_ *testing.T, _ *AuthenticationService, r *authRepository, _ *miniredis.Miniredis, _ security.TokenPair) {
			r.deleted = true
		}},
		{"single logout", func(t *testing.T, s *AuthenticationService, _ *authRepository, _ *miniredis.Miniredis, p security.TokenPair) {
			if err := s.SignOut(context.Background(), p.RefreshToken, security.RefreshToken); err != nil {
				t.Fatal(err)
			}
		}},
		{"logout all", func(t *testing.T, s *AuthenticationService, _ *authRepository, _ *miniredis.Miniredis, p security.TokenPair) {
			if err := s.SignOutAll(context.Background(), p.AccessToken); err != nil {
				t.Fatal(err)
			}
		}},
		{"expired session", func(_ *testing.T, _ *AuthenticationService, _ *authRepository, m *miniredis.Miniredis, _ security.TokenPair) {
			m.FastForward(24 * time.Hour)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, repository, _, mini := authFixture(t)
			pair := signIn(t, service)

			if _, err := service.Authenticate(context.Background(), pair.AccessToken); err != nil {
				t.Fatal(err)
			}

			tt.invalidate(t, service, repository, mini, pair)

			if _, err := service.Authenticate(context.Background(), pair.AccessToken); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatal(err)
			}

			if _, err := service.Refresh(context.Background(), pair.RefreshToken); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatal(err)
			}
		})
	}
}

func TestAuthenticationRotationAndReplay(t *testing.T) {
	service, _, _, _ := authFixture(t)
	old := signIn(t, service)
	next, err := service.Refresh(context.Background(), old.RefreshToken)

	if err != nil {
		t.Fatal(err)
	}

	if old.RefreshToken == next.RefreshToken || !old.RefreshExpiresAt.Equal(next.RefreshExpiresAt) {
		t.Fatal("refresh was not rotated with absolute expiry")
	}

	if _, err := service.Authenticate(context.Background(), next.AccessToken); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Refresh(context.Background(), old.RefreshToken); !errors.Is(err, security.ErrRefreshTokenReused) {
		t.Fatal(err)
	}

	for _, token := range []string{old.AccessToken, next.AccessToken} {
		if _, err := service.Authenticate(context.Background(), token); !errors.Is(err, security.ErrSessionRevoked) {
			t.Fatal(err)
		}
	}

	if _, err := service.Refresh(context.Background(), next.RefreshToken); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}
}

func TestAuthenticationCurrentPermissions(t *testing.T) {
	service, repository, _, _ := authFixture(t)
	pair := signIn(t, service)

	if _, err := service.Authorize(context.Background(), pair.AccessToken, security.PermissionViewUser); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Authorize(context.Background(), pair.AccessToken, security.PermissionManageUser); !errors.Is(err, security.ErrPermissionDenied) {
		t.Fatal(err)
	}

	if err := repository.role.SetPermissions([]security.Permission{security.PermissionManageUser}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Authorize(context.Background(), pair.AccessToken, security.PermissionViewUser); !errors.Is(err, security.ErrPermissionDenied) {
		t.Fatal(err)
	}

	if _, err := service.Authorize(context.Background(), pair.AccessToken, security.PermissionManageUser); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticationInvalidCredentials(t *testing.T) {
	for _, tt := range []struct {
		name, email, password string
		want                  error
		verifications         int32
	}{
		{"wrong password", "person@example.com", "incorrect", security.ErrInvalidCredentials, 1},
		{"unknown account", "unknown@example.com", "correct", security.ErrInvalidCredentials, 1},
		{"invalid email", "bad", "correct", input.ErrInvalidSignInInput, 0},
		{"empty password", "person@example.com", "", input.ErrInvalidSignInInput, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, _, hasher, _ := authFixture(t)
			pair, err := service.SignIn(context.Background(), input.SignInInput{Email: tt.email, Password: tt.password})

			if !errors.Is(err, tt.want) || pair.AccessToken != "" || hasher.calls.Load() != tt.verifications {
				t.Fatalf("error = %v, verification count = %d", err, hasher.calls.Load())
			}
		})
	}
}

func TestAuthenticationStorageFailures(t *testing.T) {
	for _, tt := range []struct {
		name string
		fail func(*authRepository, *miniredis.Miniredis)
	}{
		{"PostgreSQL", func(r *authRepository, _ *miniredis.Miniredis) { r.failure = errors.New("database unavailable") }},
		{"Redis", func(_ *authRepository, m *miniredis.Miniredis) { m.Close() }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, repository, _, mini := authFixture(t)
			pair := signIn(t, service)
			tt.fail(repository, mini)

			if _, err := service.Authenticate(context.Background(), pair.AccessToken); err == nil {
				t.Fatal("storage outage authorized token")
			}

			if _, err := service.Refresh(context.Background(), pair.RefreshToken); err == nil {
				t.Fatal("storage outage refreshed token")
			}
		})
	}
}

func TestPasswordChangeCannotResurrectOtherDevices(t *testing.T) {
	service, repository, _, _ := authFixture(t)
	first, second := signIn(t, service), signIn(t, service)
	repository.credentials.Version = uuid.New()
	third := signIn(t, service)

	for _, old := range []security.TokenPair{first, second} {
		if _, err := service.Authenticate(context.Background(), old.AccessToken); !errors.Is(err, security.ErrSessionRevoked) {
			t.Fatal(err)
		}

		if _, err := service.Refresh(context.Background(), old.RefreshToken); !errors.Is(err, security.ErrSessionRevoked) {
			t.Fatal(err)
		}
	}

	if _, err := service.Authenticate(context.Background(), third.AccessToken); err != nil {
		t.Fatal(err)
	}
}

func (r *authRepository) Invalidate(ctx context.Context, id security.UserID) error {
	var err error

	if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	} else if id != r.credentials.User.ID() {
		err = security.ErrUserNotFound
	} else {
		r.credentials.Version = uuid.New()
	}

	return err
}

func TestDurableLogoutAllSurvivesRedisFailure(t *testing.T) {
	service, repository, _, mini := authFixture(t)
	pair := signIn(t, service)
	version := repository.credentials.Version
	mini.Close()

	if err := service.RevokeUserSessions(context.Background(), repository.credentials.User.ID()); err == nil {
		t.Fatal("Redis cleanup failure was hidden")
	}

	if repository.credentials.Version == version {
		t.Fatal("durable revocation was not applied")
	}

	if err := mini.Restart(); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Authenticate(context.Background(), pair.AccessToken); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}

	if _, err := service.Refresh(context.Background(), pair.RefreshToken); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatal(err)
	}
}

func TestPasswordVerificationCapacity(t *testing.T) {
	for _, tt := range []struct {
		name      string
		cancelled bool
		want      error
	}{
		{name: "full capacity", want: ErrAuthenticationBusy},
		{name: "cancelled request", cancelled: true, want: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, _, hasher, _ := authFixture(t)

			if !service.verifications.TryAcquire(4) {
				t.Fatal("cannot reserve verification capacity")
			}

			defer service.verifications.Release(4)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if tt.cancelled {
				cancel()
			}

			matches, err := service.verifyPassword(ctx, "correct", "stored")

			if matches || !errors.Is(err, tt.want) || hasher.calls.Load() != 0 {
				t.Fatalf("overload performed password work: matches = %v, error = %v, calls = %d", matches, err, hasher.calls.Load())
			}
		})
	}
}
