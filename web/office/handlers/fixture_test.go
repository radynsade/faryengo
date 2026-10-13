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
	flashredis "github.com/radynsade/faryengo/pkg/flashmsg/redis"
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
	updateErr  error
	membersErr error
	others     []*users.User
}

// The fixture has one signed-in User with an ordinary Role, who belongs to two
// of three Workspaces, and another User who belongs to none. Its repositories keep them in memory behind the complete domain
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

	f.others = []*users.User{users.NewUser(
		users.UserID(uuid.Must(uuid.NewV7())),
		f.role.ID,
		"grace@example.com",
		time.Now(),
		"+37120000001",
		time.Now(),
		"hash:another horse",
		time.Now(),
		"Grace",
		"Hopper",
		time.Now(),
		time.Now(),
	)}

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

	workspaceService, err := app.NewWorkspaceService(&appmock.Transactor{}, f.workspaceRepository(), f.users())
	must(t, err)

	userService, err := app.NewUserService(&appmock.Transactor{}, f.users(), hasher)
	must(t, err)

	flashes, err := flashredis.NewStore(client, "office", time.Minute)
	must(t, err)

	handler, err := NewHandler(passwords, sessions, identities, workspaceService, userService, flashes, false)
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

// An update must carry the User's latest UpdatedAt, like the PostgreSQL
// repository, and a changed email or password replaces the credentials
// version, like its trigger.

func (f *fixture) users() *usersmock.UserRepository {
	return &usersmock.UserRepository{
		FindByIDFunc: func(_ context.Context, id users.UserID) (*users.User, error) {
			var (
				user *users.User
				err  = users.ErrUserNotFound
			)

			for _, candidate := range append([]*users.User{f.user}, f.others...) {
				if candidate.ID == id {
					copied := *candidate
					user, err = &copied, nil
				}
			}

			return user, err
		},
		FindByIDsFunc: func(_ context.Context, ids []users.UserID) ([]*users.User, error) {
			found := []*users.User{}

			for _, user := range append([]*users.User{f.user}, f.others...) {
				if slices.Contains(ids, user.ID) {
					copied := *user
					found = append(found, &copied)
				}
			}

			slices.SortFunc(found, func(a, b *users.User) int {
				return strings.Compare(strings.ToLower(string(a.FirstName)), strings.ToLower(string(b.FirstName)))
			})

			return found, nil
		},
		FindByEmailFunc: func(_ context.Context, email users.Email) (*users.User, error) {
			var (
				found *users.User
				err   = users.ErrUserNotFound
			)

			for _, user := range append([]*users.User{f.user}, f.others...) {
				if strings.EqualFold(string(user.Email), string(email)) {
					copied := *user
					found, err = &copied, nil
				}
			}

			return found, err
		},
		UpdateFunc: func(_ context.Context, user *users.User) users.ErrUserUpdateFailed {
			var err users.ErrUserUpdateFailed

			if user.ID != f.user.ID {
				err = usersmock.NewErrUserUpdateFailed(user, users.ErrUserNotFound)
			} else if !user.UpdatedAt.Equal(f.user.UpdatedAt) {
				err = usersmock.NewErrUserUpdateFailed(user, users.ErrUserConflict)
			} else {
				if user.Email != f.user.Email || user.PasswordHash != f.user.PasswordHash {
					f.version = uuid.Must(uuid.NewV7())
				}

				updated := *user
				updated.UpdatedAt = f.user.UpdatedAt.Add(time.Second)
				f.user = &updated
			}

			return err
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
// it ignores paging, which the fixture never needs. Created and renamed
// workspaces and added members join the in-memory state.

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

			if f.findErr != nil {
				found, err = nil, f.findErr
			}

			return found, err
		},
		RemoveMemberFunc: func(_ context.Context, membership workspaces.Membership) workspaces.ErrMembershipRemoveFailed {
			var err workspaces.ErrMembershipRemoveFailed

			members := f.members[membership.WorkspaceID]
			index := slices.Index(members, membership.UserID)

			if index < 0 {
				err = workspacesmock.NewErrMembershipRemoveFailed(membership, workspaces.ErrMembershipNotFound)
			} else {
				f.members[membership.WorkspaceID] = slices.Delete(slices.Clone(members), index, index+1)
			}

			return err
		},
		FindMemberIDsFunc: func(_ context.Context, id workspaces.WorkspaceID) ([]users.UserID, error) {
			var ids []users.UserID

			if f.membersErr == nil {
				ids = slices.Clone(f.members[id])
			}

			return ids, f.membersErr
		},
		HasMemberFunc: func(_ context.Context, membership workspaces.Membership) (bool, error) {
			return slices.Contains(f.members[membership.WorkspaceID], membership.UserID), nil
		},
		FindByIDForUpdateFunc: func(_ context.Context, id workspaces.WorkspaceID) (*workspaces.Workspace, error) {
			var (
				found *workspaces.Workspace
				err   = workspaces.ErrWorkspaceNotFound
			)

			for _, workspace := range f.workspaces {
				if workspace.ID == id {
					copied := *workspace
					found, err = &copied, nil
				}
			}

			return found, err
		},
		UpdateFunc: func(_ context.Context, workspace *workspaces.Workspace) workspaces.ErrWorkspaceUpdateFailed {
			var err workspaces.ErrWorkspaceUpdateFailed = workspacesmock.NewErrWorkspaceUpdateFailed(workspace, workspaces.ErrWorkspaceNotFound)

			if f.updateErr != nil {
				err = workspacesmock.NewErrWorkspaceUpdateFailed(workspace, f.updateErr)
			} else {
				for index, stored := range f.workspaces {
					if stored.ID == workspace.ID {
						copied := *workspace
						f.workspaces[index], err = &copied, nil
					}
				}
			}

			return err
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
			var err workspaces.ErrMembershipAddFailed

			if f.members == nil {
				f.members = make(map[workspaces.WorkspaceID][]users.UserID)
			}

			if slices.Contains(f.members[membership.WorkspaceID], membership.UserID) {
				err = workspacesmock.NewErrMembershipAddFailed(membership, workspaces.ErrMembershipAlreadyExists)
			} else {
				f.members[membership.WorkspaceID] = append(f.members[membership.WorkspaceID], membership.UserID)
			}

			return err
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
