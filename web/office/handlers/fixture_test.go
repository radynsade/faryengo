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
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	sessionidredis "github.com/radynsade/faryengo/internal/security/sessionid/redis"
	"github.com/radynsade/faryengo/internal/users"
	usersmock "github.com/radynsade/faryengo/internal/users/mock"
	"github.com/radynsade/faryengo/internal/workspaces"
	workspacesmock "github.com/radynsade/faryengo/internal/workspaces/mock"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

//
// Fixture
//

const (
	testEmail    = "ada@example.com"
	testPassword = "correct horse"

	// The User belongs to Design and Family budget, not to the other one.
	designID   = "01920000-0000-7000-8000-0000000000d1"
	familyID   = "01920000-0000-7000-8000-0000000000f1"
	otherID    = "01920000-0000-7000-8000-0000000000e1"
	designPath = "/office/en/workspaces/" + designID
)

type fixture struct {
	mux        *http.ServeMux
	user       *users.User
	role       *users.Role
	version    uuid.UUID
	workspaces []*workspaces.Workspace
	members    map[workspaces.WorkspaceID][]users.UserID
	findErr    error
	createErr  error
}

// The fixture has one User with an ordinary Role, who belongs to two of three
// Workspaces. Its repositories keep them in memory behind the complete domain
// contracts.

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

	design := workspaces.NewWorkspace(workspaces.WorkspaceID(uuid.MustParse(designID)), "Design")
	family := workspaces.NewWorkspace(workspaces.WorkspaceID(uuid.MustParse(familyID)), "Family budget")
	other := workspaces.NewWorkspace(workspaces.WorkspaceID(uuid.MustParse(otherID)), "Someone else's")
	f.workspaces = []*workspaces.Workspace{family, design, other}
	f.members = map[workspaces.WorkspaceID][]users.UserID{
		design.ID: {f.user.ID},
		family.ID: {f.user.ID},
	}

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

	workspaceService, err := app.NewWorkspaceService(&appmock.Transactor{}, f.workspaceRepository())
	must(t, err)

	handler, err := NewHandler(passwords, sessions, identities, workspaceService, false)
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

// Find returns every Workspace the filter's member belongs to, in name order;
// it ignores paging, which the fixture never needs. Created workspaces and
// added members join the in-memory state.

func (f *fixture) workspaceRepository() *workspacesmock.WorkspaceRepository {
	matching := func(filter workspaces.WorkspaceFilter) []*workspaces.Workspace {
		result := make([]*workspaces.Workspace, 0, len(f.workspaces))

		for _, workspace := range f.workspaces {
			if filter.MemberID == nil || slices.Contains(f.members[workspace.ID], *filter.MemberID) {
				result = append(result, workspace)
			}
		}

		slices.SortFunc(result, func(a, b *workspaces.Workspace) int {
			return strings.Compare(string(a.Name), string(b.Name))
		})

		return result
	}

	return &workspacesmock.WorkspaceRepository{
		FindFunc: func(_ context.Context, query workspaces.WorkspaceQuery) ([]*workspaces.Workspace, error) {
			var result []*workspaces.Workspace

			if f.findErr == nil {
				result = matching(query.Filter)
			}

			return result, f.findErr
		},
		CountFunc: func(_ context.Context, filter workspaces.WorkspaceFilter) (int, error) {
			return len(matching(filter)), nil
		},
		FindByIDFunc: func(_ context.Context, id workspaces.WorkspaceID) (*workspaces.Workspace, error) {
			var (
				found *workspaces.Workspace
				err   = workspaces.ErrWorkspaceNotFound
			)

			for _, workspace := range f.workspaces {
				if workspace.ID == id {
					found, err = workspace, nil
				}
			}

			return found, err
		},
		HasMemberFunc: func(_ context.Context, membership workspaces.Membership) (bool, error) {
			return slices.Contains(f.members[membership.WorkspaceID], membership.UserID), nil
		},
		CreateFunc: func(_ context.Context, workspace *workspaces.Workspace) workspaces.ErrWorkspaceCreateFailed {
			var err workspaces.ErrWorkspaceCreateFailed

			if f.createErr != nil {
				err = workspacesmock.NewErrWorkspaceCreateFailed(workspace, f.createErr)
			} else {
				f.workspaces = append(f.workspaces, workspace)
			}

			return err
		},
		AddMemberFunc: func(_ context.Context, membership workspaces.Membership) workspaces.ErrMembershipAddFailed {
			if f.members == nil {
				f.members = make(map[workspaces.WorkspaceID][]users.UserID)
			}

			f.members[membership.WorkspaceID] = append(f.members[membership.WorkspaceID], membership.UserID)

			return nil
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
