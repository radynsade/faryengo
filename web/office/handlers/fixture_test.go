package handlers

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	sessionidredis "github.com/radynsade/faryengo/internal/security/sessionid/redis"
	"github.com/radynsade/faryengo/internal/users"
	usersmock "github.com/radynsade/faryengo/internal/users/mock"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

//
// Fixture
//

const (
	testEmail    = "ada@example.com"
	testPassword = "correct horse"
)

type fixture struct {
	mux     *http.ServeMux
	user    *users.User
	role    *users.Role
	version uuid.UUID
}

// The fixture has one User with an ordinary Role. Its repositories keep them
// in memory behind the complete domain contracts.

func newFixture(t *testing.T) *fixture {
	t.Helper()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr(), MaxRetries: -1})

	t.Cleanup(func() { _ = client.Close() })

	f := &fixture{version: uuid.Must(uuid.NewV7())}
	f.role = users.NewRole(
		users.RoleID(uuid.Must(uuid.NewV7())),
		users.RoleName{"en": languages.Translation("Customer")},
		nil,
		false,
	)
	f.user = users.NewUser(
		users.UserID(uuid.Must(uuid.NewV7())),
		f.role.ID,
		testEmail,
		time.Now(),
		"+37120000000",
		time.Now(),
		"hash:"+testPassword,
		time.Now(),
		"Ada",
		"Lovelace",
		time.Now(),
		time.Now(),
	)

	hasher := &usersmock.PasswordHasher{
		HashFunc: func(_ context.Context, password string) (users.PasswordHash, error) {
			return users.PasswordHash("hash:" + password), nil
		},
		VerifyFunc: func(_ context.Context, password string, hash users.PasswordHash) (bool, error) {
			return string(hash) == "hash:"+password, nil
		},
	}

	storage, err := sessionidredis.NewSessionStorage(client)
	must(t, err)

	passwords, err := emailpass.NewAuthenticator(t.Context(), f.credentials(), hasher, time.Hour)
	must(t, err)

	sessions, err := sessionid.NewAuthenticator(storage, f.credentials())
	must(t, err)

	identities, err := security.NewIdentityResolver(f.credentials(), f.users(), f.roles())
	must(t, err)

	handler, err := NewHandler(passwords, sessions, identities, false)
	must(t, err)

	f.mux = http.NewServeMux()
	must(t, handler.RegisterHandlers(f.mux))

	return f
}

//
// Requests
//

type request struct {
	method  string
	path    string
	body    string
	cookies []*http.Cookie
	headers map[string]string
}

func (f *fixture) do(r request) *httptest.ResponseRecorder {
	if r.method == "" {
		r.method = http.MethodGet
	}

	httpRequest := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))

	if r.method == http.MethodPost {
		httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	for _, key := range slices.Sorted(maps.Keys(r.headers)) {
		httpRequest.Header.Set(key, r.headers[key])
	}

	for _, cookie := range r.cookies {
		httpRequest.AddCookie(cookie)
	}

	recorder := httptest.NewRecorder()
	f.mux.ServeHTTP(recorder, httpRequest)

	return recorder
}

func (f *fixture) signIn(t *testing.T) []*http.Cookie {
	t.Helper()

	response := f.do(request{
		method: http.MethodPost,
		path:   "/office/en/sign-in",
		body:   "email=" + testEmail + "&password=" + strings.ReplaceAll(testPassword, " ", "+"),
	})

	if response.Code != http.StatusSeeOther {
		t.Fatalf("sign in = %d: %s", response.Code, response.Body.String())
	}

	return response.Result().Cookies()
}

//
// Domain contracts
//

func (f *fixture) credentials() *usersmock.CredentialsRepository {
	return &usersmock.CredentialsRepository{
		FindByEmailFunc: func(_ context.Context, email users.Email) (*users.CredentialsSnapshot, error) {
			var (
				snapshot *users.CredentialsSnapshot
				err      = users.ErrUserNotFound
			)

			if email == f.user.Email {
				snapshot, err = users.NewCredentialsSnapshot(f.user, f.version), nil
			}

			return snapshot, err
		},
		FindVersionByUserIDFunc: func(_ context.Context, id users.UserID) (uuid.UUID, error) {
			var (
				version uuid.UUID
				err     = users.ErrUserNotFound
			)

			if id == f.user.ID {
				version, err = f.version, nil
			}

			return version, err
		},
		RotateVersionFunc: func(_ context.Context, _ users.UserID) error {
			f.version = uuid.Must(uuid.NewV7())

			return nil
		},
	}
}

func (f *fixture) users() *usersmock.UserRepository {
	return &usersmock.UserRepository{
		FindByIDFunc: func(_ context.Context, id users.UserID) (*users.User, error) {
			var (
				user *users.User
				err  = users.ErrUserNotFound
			)

			if id == f.user.ID {
				user, err = f.user, nil
			}

			return user, err
		},
	}
}

func (f *fixture) roles() *usersmock.RoleRepository {
	return &usersmock.RoleRepository{
		FindByIDFunc: func(_ context.Context, id users.RoleID) (*users.Role, error) {
			var (
				role *users.Role
				err  = users.ErrRoleNotFound
			)

			if id == f.role.ID {
				role, err = f.role, nil
			}

			return role, err
		},
	}
}

//
// Helpers
//

func must(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}
