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

	"github.com/radynsade/faryengo/internal/app"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/languages"
	languagesmock "github.com/radynsade/faryengo/internal/languages/mock"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	sessionidredis "github.com/radynsade/faryengo/internal/security/sessionid/redis"
	"github.com/radynsade/faryengo/internal/users"
	usersmock "github.com/radynsade/faryengo/internal/users/mock"
	flashredis "github.com/radynsade/faryengo/pkg/flashmsg/redis"
	"github.com/radynsade/faryengo/web/admin/utils"
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
	mux      *http.ServeMux
	redis    *miniredis.Miniredis
	user     *users.User
	role     *users.Role
	roles    map[users.RoleID]*users.Role
	version  uuid.UUID
	writes   int
	listErr  error
	lastFind users.RoleQuery
}

// The fixture signs in as a User whose Role has the given authority. Its
// repositories keep Roles in memory behind the complete domain contracts.

func newFixture(t *testing.T, permissions users.Permissions, isSuper bool) *fixture {
	t.Helper()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr(), MaxRetries: -1})

	t.Cleanup(func() { _ = client.Close() })

	f := &fixture{
		redis:   server,
		roles:   make(map[users.RoleID]*users.Role),
		version: uuid.Must(uuid.NewV7()),
	}

	f.role = f.addRole(t, "Owner", permissions, isSuper)
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

	transactor := &appmock.Transactor{}
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

	identities, err := security.NewIdentityResolver(f.credentials(), f.users(), f.roleRepository())
	must(t, err)

	roleService, err := app.NewRoleService(transactor, f.roleRepository())
	must(t, err)

	languageService, err := app.NewLanguageService(transactor, catalogRepository())
	must(t, err)

	flashes, err := flashredis.NewStore(client, "admin", utils.FlashTTL)
	must(t, err)

	handler, err := NewHandler(passwords, sessions, identities, roleService, languageService, flashes, false)
	must(t, err)

	f.mux = http.NewServeMux()
	must(t, handler.RegisterHandlers(f.mux))

	return f
}

func (f *fixture) addRole(t *testing.T, name string, permissions users.Permissions, isSuper bool) *users.Role {
	t.Helper()

	role := users.NewRole(
		users.RoleID(uuid.Must(uuid.NewV7())),
		users.RoleName{"en": languages.Translation(name)},
		permissions,
		isSuper,
	)

	f.roles[role.ID] = role

	return role
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
		path:   "/admin/en/sign-in",
		body:   "email=" + testEmail + "&password=" + strings.ReplaceAll(testPassword, " ", "+"),
	})

	if response.Code != http.StatusSeeOther {
		t.Fatalf("sign in = %d: %s", response.Code, response.Body.String())
	}

	return response.Result().Cookies()
}

var partial = map[string]string{"HX-Request": "true"}

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
		RotateVersionFunc: func(_ context.Context, id users.UserID) error {
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

func (f *fixture) roleRepository() *usersmock.RoleRepository {
	find := func(_ context.Context, id users.RoleID) (*users.Role, error) {
		var err error

		role, found := f.roles[id]

		if !found {
			err = users.ErrRoleNotFound
		}

		return role, err
	}

	return &usersmock.RoleRepository{
		CreateFunc: func(_ context.Context, role *users.Role) users.ErrRoleCreateFailed {
			f.writes++
			f.roles[role.ID] = role

			return nil
		},
		UpdateFunc: func(_ context.Context, role *users.Role) users.ErrRoleUpdateFailed {
			f.writes++
			f.roles[role.ID] = role

			return nil
		},
		DeleteFunc: func(_ context.Context, id users.RoleID) users.ErrRoleDeleteFailed {
			var err users.ErrRoleDeleteFailed

			f.writes++

			if id == f.user.RoleID {
				err = usersmock.NewErrRoleDeleteFailed(id, users.ErrRoleAlreadyInUse)
			} else {
				delete(f.roles, id)
			}

			return err
		},
		FindByIDFunc:          find,
		FindByIDForUpdateFunc: find,
		FindFunc: func(_ context.Context, query users.RoleQuery) ([]*users.Role, error) {
			f.lastFind = query
			matching := f.matching(query.Filter)
			start := min(len(matching), int((query.Page-1)*query.Limit))
			end := min(len(matching), start+int(query.Limit))

			return matching[start:end], f.listErr
		},
		CountFunc: func(_ context.Context, filter users.RoleFilter) (int, error) {
			return len(f.matching(filter)), f.listErr
		},
	}
}

func (f *fixture) matching(filter users.RoleFilter) []*users.Role {
	var result []*users.Role

	for _, id := range slices.SortedFunc(maps.Keys(f.roles), func(a, b users.RoleID) int {
		return strings.Compare(uuid.UUID(a).String(), uuid.UUID(b).String())
	}) {
		role := f.roles[id]

		if strings.Contains(strings.ToLower(string(role.Name["en"])), strings.ToLower(filter.NameLike)) {
			result = append(result, role)
		}
	}

	return result
}

func catalogRepository() *languagesmock.LanguageRepository {
	return &languagesmock.LanguageRepository{
		FindAllFunc: func(context.Context) ([]*languages.Language, error) {
			return []*languages.Language{
				languages.NewLanguage("en", "English", "English", true),
				languages.NewLanguage("lv", "Latvian", "Latviešu", false),
				languages.NewLanguage("ru", "Russian", "Русский", false),
			}, nil
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

func roleID(role *users.Role) string {
	return uuid.UUID(role.ID).String()
}
