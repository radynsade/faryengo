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

	"github.com/radynsade/faryengo/internal/accounts"
	accountsmock "github.com/radynsade/faryengo/internal/accounts/mock"
	"github.com/radynsade/faryengo/internal/app"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/budgets"
	budgetsmock "github.com/radynsade/faryengo/internal/budgets/mock"
	"github.com/radynsade/faryengo/internal/categories"
	categoriesmock "github.com/radynsade/faryengo/internal/categories/mock"
	"github.com/radynsade/faryengo/internal/currencies"
	currenciesmock "github.com/radynsade/faryengo/internal/currencies/mock"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/period"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	emailpassredis "github.com/radynsade/faryengo/internal/security/emailpass/redis"
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
	mux         *http.ServeMux
	user        *users.User
	role        *users.Role
	version     uuid.UUID
	workspaces  []*workspaces.Workspace
	members     map[workspaces.ID]workspaces.Memberships
	findErr     error
	createErr   error
	updateErr   error
	membersErr  error
	others      []*users.User
	categories  []*categories.Category
	categoryErr error
	accounts    []*accounts.Account
	accountErr  error
	budgets     []*budgets.Budget
	budgetErr   error
	currencies  []*currencies.Currency
	currencyErr error
}

// The fixture has one signed-in User with an ordinary Role, who owns two of
// three Workspaces, and another User who belongs to none. Its repositories
// keep them in memory behind the complete domain contracts.

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

	design := workspaces.NewWorkspace(workspaces.ID(uuid.MustParse(designID)), "Design")
	family := workspaces.NewWorkspace(workspaces.ID(uuid.MustParse(familyID)), "Family budget")
	other := workspaces.NewWorkspace(workspaces.ID(uuid.MustParse(otherID)), "Someone else's")
	f.workspaces = []*workspaces.Workspace{family, design, other}
	f.members = map[workspaces.ID]workspaces.Memberships{
		design.ID: {workspaces.NewMembership(design.ID, f.user.ID, workspaces.RoleOwner)},
		family.ID: {workspaces.NewMembership(family.ID, f.user.ID, workspaces.RoleOwner)},
	}
	f.currencies = []*currencies.Currency{
		currencies.NewCurrency("EUR", "€"),
		currencies.NewCurrency("USD", "$"),
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

	attempts, err := emailpassredis.NewAttemptStorage(client)
	must(t, err)

	passwords, err := emailpass.NewAuthenticator(t.Context(), f.credentials(), hasher, attempts, time.Hour)
	must(t, err)

	sessions, err := sessionid.NewAuthenticator(storage, f.credentials())
	must(t, err)

	identities, err := security.NewIdentityResolver(f.credentials(), f.users(), f.roles())
	must(t, err)

	workspaceService, err := app.NewWorkspaceService(&appmock.Transactor{}, f.workspaceRepository(), f.users())
	must(t, err)

	userService, err := app.NewUserService(&appmock.Transactor{}, f.users(), hasher)
	must(t, err)

	categoryService, err := app.NewCategoryService(&appmock.Transactor{}, f.categoryRepository(), f.workspaceRepository())
	must(t, err)

	accountService, err := app.NewAccountService(&appmock.Transactor{}, f.accountRepository(), f.workspaceRepository())
	must(t, err)

	budgetService, err := app.NewBudgetService(&appmock.Transactor{}, f.budgetRepository(), f.workspaceRepository())
	must(t, err)

	currencyService, err := app.NewCurrencyService(&appmock.Transactor{}, f.currencyRepository())
	must(t, err)

	flashes, err := flashredis.NewStore(client, "office", time.Minute)
	must(t, err)

	handler, err := NewHandler(passwords, sessions, identities, workspaceService, categoryService, accountService, budgetService, currencyService, userService, flashes, false)
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
			if filter.MemberID == nil || f.memberIndex(workspace.ID, *filter.MemberID) >= 0 {
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
		FindByIDFunc: func(_ context.Context, id workspaces.ID) (*workspaces.Workspace, error) {
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

			index := f.memberIndex(membership.WorkspaceID, membership.UserID)

			if index < 0 {
				err = workspacesmock.NewErrMembershipRemoveFailed(membership, workspaces.ErrMembershipNotFound)
			} else {
				f.members[membership.WorkspaceID] = slices.Delete(slices.Clone(f.members[membership.WorkspaceID]), index, index+1)
			}

			return err
		},
		UpdateMemberFunc: func(_ context.Context, membership workspaces.Membership) workspaces.ErrMembershipUpdateFailed {
			var err workspaces.ErrMembershipUpdateFailed

			index := f.memberIndex(membership.WorkspaceID, membership.UserID)

			if index < 0 {
				err = workspacesmock.NewErrMembershipUpdateFailed(membership, workspaces.ErrMembershipNotFound)
			} else {
				f.members[membership.WorkspaceID][index] = membership
			}

			return err
		},
		FindMembersFunc: func(_ context.Context, id workspaces.ID) (workspaces.Memberships, error) {
			var memberships workspaces.Memberships

			if f.membersErr == nil {
				memberships = slices.Clone(f.members[id])
			}

			return memberships, f.membersErr
		},
		FindMemberFunc: func(_ context.Context, id workspaces.ID, userID users.UserID) (workspaces.Membership, error) {
			var (
				membership workspaces.Membership
				err        = workspaces.ErrMembershipNotFound
			)

			if index := f.memberIndex(id, userID); index >= 0 {
				membership, err = f.members[id][index], nil
			}

			return membership, err
		},
		FindByIDForUpdateFunc: func(_ context.Context, id workspaces.ID) (*workspaces.Workspace, error) {
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
				f.members = make(map[workspaces.ID]workspaces.Memberships)
			}

			if f.memberIndex(membership.WorkspaceID, membership.UserID) >= 0 {
				err = workspacesmock.NewErrMembershipAddFailed(membership, workspaces.ErrMembershipAlreadyExists)
			} else {
				f.members[membership.WorkspaceID] = append(f.members[membership.WorkspaceID], membership)
			}

			return err
		},
	}
}

func (f *fixture) memberIndex(workspaceID workspaces.ID, userID users.UserID) int {
	return slices.IndexFunc(f.members[workspaceID], func(membership workspaces.Membership) bool {
		return membership.UserID == userID
	})
}

// The role is empty for a User who is not a member.

func (f *fixture) memberRole(workspaceID workspaces.ID, userID users.UserID) workspaces.Role {
	var role workspaces.Role

	if index := f.memberIndex(workspaceID, userID); index >= 0 {
		role = f.members[workspaceID][index].Role
	}

	return role
}

// A User who is not yet a member joins with the role.

func (f *fixture) setMemberRole(workspaceID workspaces.ID, userID users.UserID, role workspaces.Role) {
	if index := f.memberIndex(workspaceID, userID); index >= 0 {
		f.members[workspaceID][index].Role = role
	} else {
		f.members[workspaceID] = append(f.members[workspaceID], workspaces.NewMembership(workspaceID, userID, role))
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

// The categories are kept in memory in name order. Find honors the query's
// workspace and page, like the PostgreSQL repository; categoryErr fails every
// read and write.

func (f *fixture) categoryRepository() *categoriesmock.CategoryRepository {
	matching := func(filter categories.CategoryFilter) []*categories.Category {
		result := make([]*categories.Category, 0, len(f.categories))

		for _, category := range f.categories {
			if filter.WorkspaceID == nil || category.WorkspaceID == *filter.WorkspaceID {
				result = append(result, category)
			}
		}

		slices.SortFunc(result, func(a, b *categories.Category) int {
			return strings.Compare(strings.ToLower(string(a.Name)), strings.ToLower(string(b.Name)))
		})

		return result
	}

	index := func(id categories.ID) int {
		return slices.IndexFunc(f.categories, func(category *categories.Category) bool { return category.ID == id })
	}

	return &categoriesmock.CategoryRepository{
		CreateFunc: func(_ context.Context, category *categories.Category) categories.ErrCategoryCreateFailed {
			var err categories.ErrCategoryCreateFailed

			if f.categoryErr != nil {
				err = categoriesmock.NewErrCategoryCreateFailed(category, f.categoryErr)
			} else {
				f.categories = append(f.categories, category)
			}

			return err
		},
		UpdateFunc: func(_ context.Context, category *categories.Category) categories.ErrCategoryUpdateFailed {
			var err categories.ErrCategoryUpdateFailed

			if f.categoryErr != nil {
				err = categoriesmock.NewErrCategoryUpdateFailed(category, f.categoryErr)
			} else if found := index(category.ID); found < 0 {
				err = categoriesmock.NewErrCategoryUpdateFailed(category, categories.ErrCategoryNotFound)
			} else {
				f.categories[found] = category
			}

			return err
		},
		DeleteFunc: func(_ context.Context, id categories.ID) categories.ErrCategoryDeleteFailed {
			var err categories.ErrCategoryDeleteFailed

			if f.categoryErr != nil {
				err = categoriesmock.NewErrCategoryDeleteFailed(id, f.categoryErr)
			} else if found := index(id); found < 0 {
				err = categoriesmock.NewErrCategoryDeleteFailed(id, categories.ErrCategoryNotFound)
			} else if slices.ContainsFunc(f.budgets, func(budget *budgets.Budget) bool { return slices.Contains(budget.Categories, id) }) {
				err = categoriesmock.NewErrCategoryDeleteFailed(id, categories.ErrCategoryInUse)
			} else {
				f.categories = slices.Delete(f.categories, found, found+1)
			}

			return err
		},
		FindByIDFunc: func(_ context.Context, id categories.ID) (*categories.Category, error) {
			var (
				category *categories.Category
				err      = categories.ErrCategoryNotFound
			)

			if f.categoryErr != nil {
				err = f.categoryErr
			} else if found := index(id); found >= 0 {
				copied := *f.categories[found]
				category, err = &copied, nil
			}

			return category, err
		},
		FindFunc: func(_ context.Context, query categories.CategoryQuery) ([]*categories.Category, error) {
			var result []*categories.Category

			if f.categoryErr == nil {
				all := matching(query.Filter)
				start := min(int((query.Page-1)*query.Limit), len(all))
				result = all[start:min(start+int(query.Limit), len(all))]
			}

			return result, f.categoryErr
		},
		CountFunc: func(_ context.Context, filter categories.CategoryFilter) (int, error) {
			return len(matching(filter)), f.categoryErr
		},
	}
}

func (f *fixture) addCategory(workspaceID string, name string, icon string, color categories.Color) *categories.Category {
	var categoryIcon *categories.Icon

	if icon != "" {
		categoryIcon = new(categories.Icon(icon))
	}

	category := categories.NewCategory(
		categories.ID(uuid.Must(uuid.NewV7())),
		workspaces.ID(uuid.MustParse(workspaceID)),
		categories.Name(name),
		categoryIcon,
		color,
	)
	f.categories = append(f.categories, category)

	return category
}

// The accounts are kept in memory in name order. Find honors the query's
// workspace and page, like the PostgreSQL repository; accountErr fails every
// read and write.

func (f *fixture) accountRepository() *accountsmock.AccountRepository {
	matching := func(filter accounts.AccountFilter) []*accounts.Account {
		result := make([]*accounts.Account, 0, len(f.accounts))

		for _, account := range f.accounts {
			if filter.WorkspaceID == nil || account.WorkspaceID == *filter.WorkspaceID {
				result = append(result, account)
			}
		}

		slices.SortFunc(result, func(a, b *accounts.Account) int {
			return strings.Compare(strings.ToLower(string(a.Name)), strings.ToLower(string(b.Name)))
		})

		return result
	}

	index := func(id accounts.ID) int {
		return slices.IndexFunc(f.accounts, func(account *accounts.Account) bool { return account.ID == id })
	}

	return &accountsmock.AccountRepository{
		CreateFunc: func(_ context.Context, account *accounts.Account) accounts.ErrAccountCreateFailed {
			var err accounts.ErrAccountCreateFailed

			if f.accountErr != nil {
				err = accountsmock.NewErrAccountCreateFailed(account, f.accountErr)
			} else if !f.hasCurrency(account.CurrencyCode) {
				err = accountsmock.NewErrAccountCreateFailed(account, currencies.ErrCurrencyNotFound)
			} else {
				f.accounts = append(f.accounts, account)
			}

			return err
		},
		UpdateFunc: func(_ context.Context, account *accounts.Account) accounts.ErrAccountUpdateFailed {
			var err accounts.ErrAccountUpdateFailed

			if f.accountErr != nil {
				err = accountsmock.NewErrAccountUpdateFailed(account, f.accountErr)
			} else if found := index(account.ID); found < 0 {
				err = accountsmock.NewErrAccountUpdateFailed(account, accounts.ErrAccountNotFound)
			} else if !f.hasCurrency(account.CurrencyCode) {
				err = accountsmock.NewErrAccountUpdateFailed(account, currencies.ErrCurrencyNotFound)
			} else {
				f.accounts[found] = account
			}

			return err
		},
		DeleteFunc: func(_ context.Context, id accounts.ID) accounts.ErrAccountDeleteFailed {
			var err accounts.ErrAccountDeleteFailed

			if f.accountErr != nil {
				err = accountsmock.NewErrAccountDeleteFailed(id, f.accountErr)
			} else if found := index(id); found < 0 {
				err = accountsmock.NewErrAccountDeleteFailed(id, accounts.ErrAccountNotFound)
			} else {
				f.accounts = slices.Delete(f.accounts, found, found+1)
			}

			return err
		},
		FindByIDFunc: func(_ context.Context, id accounts.ID) (*accounts.Account, error) {
			var (
				account *accounts.Account
				err     = accounts.ErrAccountNotFound
			)

			if f.accountErr != nil {
				err = f.accountErr
			} else if found := index(id); found >= 0 {
				copied := *f.accounts[found]
				account, err = &copied, nil
			}

			return account, err
		},
		FindFunc: func(_ context.Context, query accounts.AccountQuery) ([]*accounts.Account, error) {
			var result []*accounts.Account

			if f.accountErr == nil {
				all := matching(query.Filter)
				start := min(int((query.Page-1)*query.Limit), len(all))
				result = all[start:min(start+int(query.Limit), len(all))]
			}

			return result, f.accountErr
		},
		CountFunc: func(_ context.Context, filter accounts.AccountFilter) (int, error) {
			return len(matching(filter)), f.accountErr
		},
	}
}

// Accounts added directly hold euros.

func (f *fixture) addAccount(workspaceID string, name string, userID *users.UserID) *accounts.Account {
	account := accounts.NewAccount(
		accounts.ID(uuid.Must(uuid.NewV7())),
		workspaces.ID(uuid.MustParse(workspaceID)),
		userID,
		"EUR",
		accounts.Name(name),
	)
	f.accounts = append(f.accounts, account)

	return account
}

// The budgets are kept in memory, the largest first. Find honors the query's
// workspace and page, like the PostgreSQL repository; budgetErr fails every
// read and write. A write must name an existing currency and categories of the
// budget's workspace, as the database's foreign keys keep them.

func (f *fixture) budgetRepository() *budgetsmock.BudgetRepository {
	matching := func(filter budgets.BudgetFilter) []*budgets.Budget {
		result := make([]*budgets.Budget, 0, len(f.budgets))

		for _, budget := range f.budgets {
			if filter.WorkspaceID == nil || budget.WorkspaceID == *filter.WorkspaceID {
				result = append(result, budget)
			}
		}

		slices.SortStableFunc(result, func(a, b *budgets.Budget) int {
			return int(b.Amount - a.Amount)
		})

		return result
	}

	index := func(id budgets.ID) int {
		return slices.IndexFunc(f.budgets, func(budget *budgets.Budget) bool { return budget.ID == id })
	}

	references := func(budget *budgets.Budget) error {
		var err error

		if !f.hasCurrency(budget.Currency) {
			err = currencies.ErrCurrencyNotFound
		}

		for _, id := range budget.Categories {
			if err == nil && !slices.ContainsFunc(f.categories, func(category *categories.Category) bool {
				return category.ID == id && category.WorkspaceID == budget.WorkspaceID
			}) {
				err = categories.ErrCategoryNotFound
			}
		}

		return err
	}

	return &budgetsmock.BudgetRepository{
		CreateFunc: func(_ context.Context, budget *budgets.Budget) budgets.ErrBudgetCreateFailed {
			var err budgets.ErrBudgetCreateFailed

			if f.budgetErr != nil {
				err = budgetsmock.NewErrBudgetCreateFailed(budget, f.budgetErr)
			} else if referenceErr := references(budget); referenceErr != nil {
				err = budgetsmock.NewErrBudgetCreateFailed(budget, referenceErr)
			} else {
				f.budgets = append(f.budgets, budget)
			}

			return err
		},
		UpdateFunc: func(_ context.Context, budget *budgets.Budget) budgets.ErrBudgetUpdateFailed {
			var err budgets.ErrBudgetUpdateFailed

			if f.budgetErr != nil {
				err = budgetsmock.NewErrBudgetUpdateFailed(budget, f.budgetErr)
			} else if found := index(budget.ID); found < 0 {
				err = budgetsmock.NewErrBudgetUpdateFailed(budget, budgets.ErrBudgetNotFound)
			} else if referenceErr := references(budget); referenceErr != nil {
				err = budgetsmock.NewErrBudgetUpdateFailed(budget, referenceErr)
			} else {
				f.budgets[found] = budget
			}

			return err
		},
		DeleteFunc: func(_ context.Context, id budgets.ID) budgets.ErrBudgetDeleteFailed {
			var err budgets.ErrBudgetDeleteFailed

			if f.budgetErr != nil {
				err = budgetsmock.NewErrBudgetDeleteFailed(id, f.budgetErr)
			} else if found := index(id); found < 0 {
				err = budgetsmock.NewErrBudgetDeleteFailed(id, budgets.ErrBudgetNotFound)
			} else {
				f.budgets = slices.Delete(f.budgets, found, found+1)
			}

			return err
		},
		FindByIDFunc: func(_ context.Context, id budgets.ID) (*budgets.Budget, error) {
			var (
				budget *budgets.Budget
				err    = budgets.ErrBudgetNotFound
			)

			if f.budgetErr != nil {
				err = f.budgetErr
			} else if found := index(id); found >= 0 {
				copied := *f.budgets[found]
				copied.Categories = slices.Clone(copied.Categories)
				budget, err = &copied, nil
			}

			return budget, err
		},
		FindFunc: func(_ context.Context, query budgets.BudgetQuery) ([]*budgets.Budget, error) {
			var result []*budgets.Budget

			if f.budgetErr == nil {
				all := matching(query.Filter)
				start := min(int((query.Page-1)*query.Limit), len(all))
				result = all[start:min(start+int(query.Limit), len(all))]
			}

			return result, f.budgetErr
		},
		CountFunc: func(_ context.Context, filter budgets.BudgetFilter) (int, error) {
			return len(matching(filter)), f.budgetErr
		},
	}
}

// Budgets added directly are set in euros and apply at any time.

func (f *fixture) addBudget(workspaceID string, amount budgets.Amount, categoryList ...*categories.Category) *budgets.Budget {
	ids := make(budgets.CategoryIDs, 0, len(categoryList))

	for _, category := range categoryList {
		ids = append(ids, category.ID)
	}

	budget := budgets.NewBudget(
		budgets.ID(uuid.Must(uuid.NewV7())),
		workspaces.ID(uuid.MustParse(workspaceID)),
		ids,
		"EUR",
		period.Period{},
		amount,
	)
	f.budgets = append(f.budgets, budget)

	return budget
}

// The currencies are kept in memory in code order; currencyErr fails listing
// them.

func (f *fixture) currencyRepository() *currenciesmock.CurrencyRepository {
	return &currenciesmock.CurrencyRepository{
		FindAllFunc: func(context.Context) ([]*currencies.Currency, error) {
			var result []*currencies.Currency

			if f.currencyErr == nil {
				result = f.currencies
			}

			return result, f.currencyErr
		},
	}
}

// An account may hold only a currency that exists, as the database's foreign
// key keeps it.

func (f *fixture) hasCurrency(code currencies.Code) bool {
	return slices.ContainsFunc(f.currencies, func(currency *currencies.Currency) bool { return currency.Code == code })
}
