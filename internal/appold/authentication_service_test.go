package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
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

func authFixture(t *testing.T) (*JWTAuthenticationService, *authRepository, *authHasher, *miniredis.Miniredis) {
	t.Helper()
	roleID := security.RoleID(newTestUUID(t))
	user, err := security.NewUser(security.UserID(newTestUUID(t)), roleID, "person@example.com", "+37123456789", "stored", "First", "Last")

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

	repository := &authRepository{credentials: security.Credentials{User: user, Version: newTestUUID(t)}, role: role}
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
	authentication, err := NewAuthenticationService(context.Background(), AuthenticationDependencies{Credentials: repository, Invalidator: repository, Roles: repository, Hasher: hasher, Revoker: sessions})

	if err != nil {
		t.Fatal(err)
	}

	service, err := NewJWTAuthenticationService(authentication, JWTAuthenticationDependencies{Tokens: tokens, Reissuer: tokens, Sessions: sessions, Rotator: sessions}, 24*time.Hour)

	if err != nil {
		t.Fatal(err)
	}

	return service, repository, hasher, mini
}

func signIn(t *testing.T, s *JWTAuthenticationService) security.TokenPair {
	t.Helper()
	pair, err := s.SignIn(context.Background(), input.SignInInput{Email: "person@example.com", Password: "correct"})

	if err != nil {
		t.Fatal(err)
	}

	claims, err := s.dependencies.Tokens.Verify(t.Context(), pair.AccessToken, security.AccessToken)

	if err != nil || claims.SessionID.Version() != 7 {
		t.Fatalf("JWT session ID must be UUIDv7: %v, %v", claims.SessionID, err)
	}

	return pair
}

func TestAuthenticationRevocation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		invalidate func(*testing.T, *JWTAuthenticationService, *authRepository, *miniredis.Miniredis, security.TokenPair)
	}{
		{"password change", func(t *testing.T, _ *JWTAuthenticationService, r *authRepository, _ *miniredis.Miniredis, _ security.TokenPair) {
			r.credentials.Version = newTestUUID(t)
		}},
		{"deleted user", func(_ *testing.T, _ *JWTAuthenticationService, r *authRepository, _ *miniredis.Miniredis, _ security.TokenPair) {
			r.deleted = true
		}},
		{"single logout", func(t *testing.T, s *JWTAuthenticationService, _ *authRepository, _ *miniredis.Miniredis, p security.TokenPair) {
			if err := s.SignOut(context.Background(), p.RefreshToken, security.RefreshToken); err != nil {
				t.Fatal(err)
			}
		}},
		{"logout all", func(t *testing.T, s *JWTAuthenticationService, _ *authRepository, _ *miniredis.Miniredis, p security.TokenPair) {
			if err := s.SignOutAll(context.Background(), p.AccessToken); err != nil {
				t.Fatal(err)
			}
		}},
		{"expired session", func(_ *testing.T, _ *JWTAuthenticationService, _ *authRepository, m *miniredis.Miniredis, _ security.TokenPair) {
			m.FastForward(24 * time.Hour)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, repository, _, mini := authFixture(t)
			pair := signIn(t, service)

			if _, err := service.Authenticate(context.Background(), pair.AccessToken); err != nil {
				t.Fatal(err)
			}

			// Populate the shared rotation result before invalidation, so a
			// duplicate cannot use the overlap window to bypass revocation.
			if _, err := service.Refresh(t.Context(), pair.RefreshToken); err != nil {
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
	service, _, _, mini := authFixture(t)
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

	mini.SetTime(time.Now().Add(securityredis.RefreshOverlapWindow))

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
	for _, tt := range []struct {
		name         string
		view, manage security.Permission
	}{
		{name: "users", view: security.PermissionViewUser, manage: security.PermissionManageUser},
		{name: "roles", view: security.PermissionViewRole, manage: security.PermissionManageRole},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, repository, _, _ := authFixture(t)

			if err := repository.role.SetPermissions([]security.Permission{tt.view}); err != nil {
				t.Fatal(err)
			}

			pair := signIn(t, service)

			if _, err := authorizeJWT(service, context.Background(), pair.AccessToken, tt.view); err != nil {
				t.Fatal(err)
			}

			if _, err := authorizeJWT(service, context.Background(), pair.AccessToken, tt.manage); !errors.Is(err, security.ErrPermissionDenied) {
				t.Fatal(err)
			}

			if err := repository.role.SetPermissions([]security.Permission{tt.manage}); err != nil {
				t.Fatal(err)
			}

			if _, err := authorizeJWT(service, context.Background(), pair.AccessToken, tt.view); !errors.Is(err, security.ErrPermissionDenied) {
				t.Fatal(err)
			}

			if _, err := authorizeJWT(service, context.Background(), pair.AccessToken, tt.manage); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAuthenticationCurrentSuperRole(t *testing.T) {
	service, repository, _, _ := authFixture(t)
	pair := signIn(t, service)

	for _, tt := range []struct {
		name    string
		isSuper bool
		want    error
	}{
		{name: "regular role denied", want: security.ErrPermissionDenied},
		{name: "super role allowed", isSuper: true},
		{name: "super flag cleared", want: security.ErrPermissionDenied},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository.role.SetIsSuper(tt.isSuper)
			principal, err := authorizeJWT(service, context.Background(), pair.AccessToken, security.PermissionManageUser)

			if !errors.Is(err, tt.want) || (err == nil && principal.IsSuper != tt.isSuper) {
				t.Fatalf("Authorize() = %v, %v, want %v", principal, err, tt.want)
			}
		})
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
	repository.credentials.Version = newTestUUID(t)
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
		r.credentials.Version, err = uuid.NewV7()

		if err != nil {
			err = fmt.Errorf("generate credential version: %w", err)
		}
	}

	return err
}

func TestDurableLogoutAllSurvivesRedisFailure(t *testing.T) {
	service, repository, _, mini := authFixture(t)
	pair := signIn(t, service)
	version := repository.credentials.Version
	mini.Close()

	if err := service.authentication.RevokeUserSessions(context.Background(), repository.credentials.User.ID()); err == nil {
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

			if !service.authentication.verifications.TryAcquire(4) {
				t.Fatal("cannot reserve verification capacity")
			}

			defer service.authentication.verifications.Release(4)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if tt.cancelled {
				cancel()
			}

			matches, err := service.authentication.verifyPassword(ctx, "correct", "stored")

			if matches || !errors.Is(err, tt.want) || hasher.calls.Load() != 0 {
				t.Fatalf("overload performed password work: matches = %v, error = %v, calls = %d", matches, err, hasher.calls.Load())
			}
		})
	}
}

func authorizeJWT(service *JWTAuthenticationService, ctx context.Context, raw string, permission security.Permission) (security.Principal, error) {
	principal, err := service.Authenticate(ctx, raw)

	if err == nil {
		err = Authorize(principal, permission)
	}

	return principal, err
}

func newTestUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()

	if err != nil {
		t.Fatalf("generate UUIDv7: %v", err)
	}

	return id
}
