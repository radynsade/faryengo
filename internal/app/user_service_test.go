package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/mock"
	"github.com/radynsade/faryengo/pkg/domquery"
)

var (
	testUserID   = users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001"))
	testRoleID   = users.RoleID(uuid.MustParse("01920000-0000-7000-8000-000000000002"))
	testMoment   = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	testReloaded = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	testAdminID  = users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
)

func TestNewUserService(t *testing.T) {
	for _, tt := range []struct {
		name       string
		transactor Transactor
		repository users.UserRepository
		hasher     users.PasswordHasher
		wantErr    error
	}{
		{
			name:       "created",
			transactor: &appmock.Transactor{},
			repository: &mock.UserRepository{},
			hasher:     &mock.PasswordHasher{},
		},
		{name: "nil transactor", repository: &mock.UserRepository{}, hasher: &mock.PasswordHasher{}, wantErr: ErrTransactorNil},
		{name: "nil repository", transactor: &appmock.Transactor{}, hasher: &mock.PasswordHasher{}, wantErr: ErrUserRepositoryNil},
		{name: "nil hasher", transactor: &appmock.Transactor{}, repository: &mock.UserRepository{}, wantErr: ErrPasswordHasherNil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewUserService(tt.transactor, tt.repository, tt.hasher)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || (service == nil) != (tt.wantErr != nil) {
				t.Fatalf("NewUserService() = (%v, %v), want error %v", service, err, tt.wantErr)
			}
		})
	}
}

func TestUserServiceNilReceiver(t *testing.T) {
	ctx := systemContext(t)

	var service *UserService

	created, createErr := service.Create(ctx, input.CreateUserInput{})
	found, findErr := service.FindByID(ctx, testUserID)
	updated, updateErr := service.Update(ctx, input.UpdateUserInput{})
	deleteErr := service.Delete(ctx, testUserID)
	profile, profileErr := service.UpdateProfile(ctx, input.UpdateProfileInput{})
	changed, changeErr := service.ChangePassword(ctx, input.ChangePasswordInput{})

	for _, err := range []error{createErr, findErr, updateErr, deleteErr, profileErr, changeErr} {
		if !errors.Is(err, ErrUserRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrUserRepositoryNil", err)
		}
	}

	if created != nil || found != nil || updated != nil || profile != nil || changed != nil {
		t.Fatalf("nil receiver returned users: %v, %v, %v, %v, %v", created, found, updated, profile, changed)
	}
}

func TestUserServiceCreate(t *testing.T) {
	valid := input.CreateUserInput{
		RoleID:    testRoleID,
		Email:     "anna@example.com",
		Phone:     "+37120000000",
		Password:  "correct horse",
		FirstName: "Anna",
		LastName:  "Bērziņa",
	}

	for _, tt := range []struct {
		name           string
		request        input.CreateUserInput
		hashErr        error
		createErr      error
		reloadErr      error
		wantErrs       []error
		wantHashCalls  int
		wantWriteCalls int
	}{
		{name: "created", request: valid, wantHashCalls: 1, wantWriteCalls: 1},
		{
			name:    "all fields invalid",
			request: input.CreateUserInput{Password: "short"},
			wantErrs: []error{
				input.ErrCreateUserInputInvalid,
				users.ErrRoleIDInvalid,
				users.ErrEmailInvalid,
				users.ErrPhoneInvalid,
				users.ErrPasswordTooShort,
				users.ErrFirstNameEmpty,
				users.ErrLastNameEmpty,
			},
		},
		{
			name:          "hash failure",
			request:       valid,
			hashErr:       context.DeadlineExceeded,
			wantErrs:      []error{context.DeadlineExceeded},
			wantHashCalls: 1,
		},
		{
			name:           "missing role",
			request:        valid,
			createErr:      users.ErrRoleNotFound,
			wantErrs:       []error{users.ErrRoleNotFound},
			wantHashCalls:  1,
			wantWriteCalls: 1,
		},
		{
			name:           "duplicate email",
			request:        valid,
			createErr:      users.ErrUserAlreadyExists,
			wantErrs:       []error{users.ErrUserAlreadyExists},
			wantHashCalls:  1,
			wantWriteCalls: 1,
		},
		{
			name:           "reload failure",
			request:        valid,
			reloadErr:      context.DeadlineExceeded,
			wantErrs:       []error{context.DeadlineExceeded},
			wantHashCalls:  1,
			wantWriteCalls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := userContext(t, testAdminID, users.PermissionManageUser)
			hashCalls, writeCalls := 0, 0

			var stored *users.User

			hasher := &mock.PasswordHasher{
				HashFunc: func(_ context.Context, password string) (users.PasswordHash, error) {
					hashCalls++

					if password != tt.request.Password {
						t.Errorf("Hash() password = %q, want %q", password, tt.request.Password)
					}

					return "hash:" + users.PasswordHash(password), tt.hashErr
				},
			}

			repository := &mock.UserRepository{
				CreateFunc: func(callCtx context.Context, user *users.User) users.ErrUserCreateFailed {
					var err users.ErrUserCreateFailed

					writeCalls++
					stored = user

					if !appmock.InTransaction(callCtx) {
						t.Errorf("Create() ran outside a transaction")
					}

					if tt.createErr != nil {
						err = mock.NewErrUserCreateFailed(user, tt.createErr)
					}

					return err
				},
				FindByIDFunc: reloadFrom(&stored, tt.reloadErr),
			}

			service, err := NewUserService(&appmock.Transactor{}, repository, hasher)

			if err != nil {
				t.Fatal(err)
			}

			user, err := service.Create(ctx, tt.request)

			if hashCalls != tt.wantHashCalls || writeCalls != tt.wantWriteCalls {
				t.Fatalf("Create() calls = (hash %d, write %d), want (%d, %d)", hashCalls, writeCalls, tt.wantHashCalls, tt.wantWriteCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if user != nil {
					t.Fatalf("Create() user = %v, want nil on error", user)
				}
			} else {
				if uuid.UUID(stored.ID).Version() != 7 || !stored.CreatedAt.IsZero() || !stored.UpdatedAt.IsZero() {
					t.Fatalf("stored user = %+v, want a UUIDv7 and database-owned timestamps", stored)
				}

				if user == nil || user.ID != stored.ID || user.RoleID != tt.request.RoleID ||
					user.Email != users.Email(tt.request.Email) ||
					user.PasswordHash != users.PasswordHash("hash:"+tt.request.Password) ||
					user.UpdatedAt != testReloaded {
					t.Fatalf("Create() user = %+v, want the reloaded user", user)
				}
			}
		})
	}
}

func TestUserServiceFindByID(t *testing.T) {
	existing := testUser()

	for _, tt := range []struct {
		name      string
		id        users.UserID
		findErr   error
		wantErrs  []error
		wantCalls int
	}{
		{name: "found", id: testUserID, wantCalls: 1},
		{name: "invalid ID", wantErrs: []error{users.ErrUserIDInvalid}},
		{
			name:      "not found",
			id:        testUserID,
			findErr:   users.ErrUserNotFound,
			wantErrs:  []error{users.ErrUserNotFound},
			wantCalls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := userContext(t, testAdminID, users.PermissionViewUser)
			calls := 0

			repository := &mock.UserRepository{
				FindByIDFunc: func(callCtx context.Context, id users.UserID) (*users.User, error) {
					var user *users.User

					calls++

					if callCtx != ctx || id != tt.id {
						t.Errorf("FindByID() called with (%v, %v)", callCtx, id)
					}

					if tt.findErr == nil {
						user = existing
					}

					return user, tt.findErr
				},
			}

			service, err := NewUserService(&appmock.Transactor{}, repository, &mock.PasswordHasher{})

			if err != nil {
				t.Fatal(err)
			}

			user, err := service.FindByID(ctx, tt.id)

			if calls != tt.wantCalls {
				t.Fatalf("FindByID() repository calls = %d, want %d", calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && user != nil {
				t.Fatalf("FindByID() user = %v, want nil on error", user)
			} else if len(tt.wantErrs) == 0 && user != existing {
				t.Fatalf("FindByID() user = %v, want %v", user, existing)
			}
		})
	}
}

func TestUserServiceList(t *testing.T) {
	found := []*users.User{testUser()}
	withoutPage := validUserQuery()
	withoutPage.Page = 0
	unknownSort := validUserQuery()
	unknownSort.SortBy = "phone"
	longFilter := validUserQuery()
	longFilter.Filter.EmailLike = strings.Repeat("a", users.MaxUserFilterEmailLikeLength+1)
	nilRole := validUserQuery()
	nilRole.Filter.RoleID = &users.RoleID{}

	for _, tt := range []struct {
		name       string
		query      users.UserQuery
		countErr   error
		findErr    error
		wantErrs   []error
		wantCalls  int
		wantResult UserList
	}{
		{name: "listed", query: validUserQuery(), wantCalls: 2, wantResult: UserList{Users: found, Total: 7}},
		{name: "missing page", query: withoutPage, wantErrs: []error{users.ErrInvalidUserQuery}},
		{name: "unknown sort", query: unknownSort, wantErrs: []error{users.ErrInvalidUserQuery}},
		{name: "invalid filter", query: longFilter, wantErrs: []error{users.ErrUserFilterInvalid, users.ErrUserFilterEmailLikeTooLong}},
		{name: "nil role filter", query: nilRole, wantErrs: []error{users.ErrUserFilterInvalid, users.ErrRoleIDInvalid}},
		{
			name:      "count failure",
			query:     validUserQuery(),
			countErr:  context.DeadlineExceeded,
			wantErrs:  []error{context.DeadlineExceeded},
			wantCalls: 1,
		},
		{
			name:      "find failure",
			query:     validUserQuery(),
			findErr:   context.DeadlineExceeded,
			wantErrs:  []error{context.DeadlineExceeded},
			wantCalls: 2,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := userContext(t, testAdminID, users.PermissionViewUser)
			calls := 0

			repository := &mock.UserRepository{
				CountFunc: func(callCtx context.Context, filter users.UserFilter) (int, error) {
					calls++

					if callCtx != ctx || filter.EmailLike != tt.query.Filter.EmailLike {
						t.Errorf("Count() called with (%v, %+v)", callCtx, filter)
					}

					return 7, tt.countErr
				},
				FindFunc: func(callCtx context.Context, query users.UserQuery) ([]*users.User, error) {
					calls++

					if callCtx != ctx || query.Page != tt.query.Page || query.SortBy != tt.query.SortBy {
						t.Errorf("Find() called with (%v, %+v)", callCtx, query)
					}

					return found, tt.findErr
				},
			}

			service, err := NewUserService(&appmock.Transactor{}, repository, &mock.PasswordHasher{})

			if err != nil {
				t.Fatal(err)
			}

			list, err := service.List(ctx, tt.query)

			if calls != tt.wantCalls {
				t.Fatalf("List() repository calls = %d, want %d", calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if list.Total != tt.wantResult.Total || len(list.Users) != len(tt.wantResult.Users) {
				t.Fatalf("List() = %+v, want %+v", list, tt.wantResult)
			}
		})
	}
}

func TestUserServiceUpdate(t *testing.T) {
	newPassword := "new password"
	shortPassword := "short"
	newRoleID := users.RoleID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
	valid := input.UpdateUserInput{
		ID:        testUserID,
		RoleID:    newRoleID,
		Email:     "anna.new@example.com",
		Phone:     "+37129999999",
		FirstName: "Anna",
		LastName:  "Ozola",
		UpdatedAt: testMoment,
	}
	withPassword := valid
	withPassword.Password = &newPassword
	withShortPassword := valid
	withShortPassword.Password = &shortPassword

	for _, tt := range []struct {
		name           string
		request        input.UpdateUserInput
		loadErr        error
		hashErr        error
		updateErr      error
		wantErrs       []error
		wantHash       users.PasswordHash
		wantHashCalls  int
		wantWriteCalls int
	}{
		{name: "updated keeping password", request: valid, wantHash: "existing hash", wantWriteCalls: 1},
		{
			name:           "updated with password",
			request:        withPassword,
			wantHash:       "hash:new password",
			wantHashCalls:  1,
			wantWriteCalls: 1,
		},
		{
			name:     "invalid fields",
			request:  input.UpdateUserInput{Email: "not an email", Password: &shortPassword},
			wantErrs: []error{input.ErrUpdateUserInputInvalid, users.ErrUserIDInvalid, users.ErrRoleIDInvalid, users.ErrEmailInvalid, users.ErrPasswordTooShort},
		},
		{
			name:     "short password",
			request:  withShortPassword,
			wantErrs: []error{input.ErrUpdateUserInputInvalid, users.ErrPasswordTooShort},
		},
		{name: "not found", request: valid, loadErr: users.ErrUserNotFound, wantErrs: []error{users.ErrUserNotFound}},
		{
			name:          "hash failure",
			request:       withPassword,
			hashErr:       context.DeadlineExceeded,
			wantErrs:      []error{context.DeadlineExceeded},
			wantHashCalls: 1,
		},
		{
			name:           "outdated change",
			request:        valid,
			updateErr:      users.ErrUserConflict,
			wantErrs:       []error{users.ErrUserConflict},
			wantWriteCalls: 1,
		},
		{
			name:           "duplicate email",
			request:        valid,
			updateErr:      users.ErrUserAlreadyExists,
			wantErrs:       []error{users.ErrUserAlreadyExists},
			wantWriteCalls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := userContext(t, testAdminID, users.PermissionManageUser)
			hashCalls, writeCalls := 0, 0
			stored := testUser()

			var written *users.User

			hasher := &mock.PasswordHasher{
				HashFunc: func(_ context.Context, password string) (users.PasswordHash, error) {
					hashCalls++

					return "hash:" + users.PasswordHash(password), tt.hashErr
				},
			}

			repository := &mock.UserRepository{
				FindByIDFunc: func(callCtx context.Context, _ users.UserID) (*users.User, error) {
					var (
						user *users.User
						err  = tt.loadErr
					)

					if !appmock.InTransaction(callCtx) {
						t.Error("FindByID() ran outside a transaction")
					}

					if err == nil {
						copied := *stored
						user = &copied
					}

					return user, err
				},
				UpdateFunc: func(callCtx context.Context, user *users.User) users.ErrUserUpdateFailed {
					var err users.ErrUserUpdateFailed

					writeCalls++
					written = user

					if !appmock.InTransaction(callCtx) {
						t.Errorf("Update() ran outside a transaction")
					}

					if tt.updateErr != nil {
						err = mock.NewErrUserUpdateFailed(user, tt.updateErr)
					} else {
						copied := *user
						copied.UpdatedAt = testReloaded
						stored = &copied
					}

					return err
				},
			}

			service, err := NewUserService(&appmock.Transactor{}, repository, hasher)

			if err != nil {
				t.Fatal(err)
			}

			user, err := service.Update(ctx, tt.request)

			if hashCalls != tt.wantHashCalls || writeCalls != tt.wantWriteCalls {
				t.Fatalf("Update() calls = (hash %d, write %d), want (%d, %d)", hashCalls, writeCalls, tt.wantHashCalls, tt.wantWriteCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if written != nil {
				if written.UpdatedAt != tt.request.UpdatedAt || written.CreatedAt != testMoment || written.EmailChangedAt != testMoment {
					t.Fatalf("written user = %+v, want the request's UpdatedAt and the loaded timestamps", written)
				}
			}

			if len(tt.wantErrs) > 0 {
				if user != nil {
					t.Fatalf("Update() user = %v, want nil on error", user)
				}
			} else if user == nil || user.RoleID != newRoleID || user.Email != users.Email(tt.request.Email) ||
				user.LastName != users.LastName(tt.request.LastName) || user.PasswordHash != tt.wantHash ||
				user.UpdatedAt != testReloaded {
				t.Fatalf("Update() user = %+v, want the reloaded user with hash %q", user, tt.wantHash)
			}
		})
	}
}

func TestUserServiceDelete(t *testing.T) {
	for _, tt := range []struct {
		name      string
		id        users.UserID
		deleteErr error
		wantErrs  []error
		wantCalls int
	}{
		{name: "deleted", id: testUserID, wantCalls: 1},
		{name: "invalid ID", wantErrs: []error{users.ErrUserIDInvalid}},
		{
			name:      "not found",
			id:        testUserID,
			deleteErr: users.ErrUserNotFound,
			wantErrs:  []error{users.ErrUserNotFound},
			wantCalls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := userContext(t, testAdminID, users.PermissionManageUser)
			calls := 0

			repository := &mock.UserRepository{
				DeleteFunc: func(callCtx context.Context, id users.UserID) users.ErrUserDeleteFailed {
					var err users.ErrUserDeleteFailed

					calls++

					if !appmock.InTransaction(callCtx) || id != tt.id {
						t.Errorf("Delete() called with (%v, %v), want a transaction", callCtx, id)
					}

					if tt.deleteErr != nil {
						err = mock.NewErrUserDeleteFailed(id, tt.deleteErr)
					}

					return err
				},
			}

			service, err := NewUserService(&appmock.Transactor{}, repository, &mock.PasswordHasher{})

			if err != nil {
				t.Fatal(err)
			}

			err = service.Delete(ctx, tt.id)

			if calls != tt.wantCalls {
				t.Fatalf("Delete() repository calls = %d, want %d", calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)
		})
	}
}

// A User cannot delete their own account. The system identity acts for no
// account, so it deletes any User.

func TestUserServiceDeleteOwnAccount(t *testing.T) {
	for _, tt := range []struct {
		name      string
		ctx       context.Context
		wantErrs  []error
		wantCalls int
	}{
		{name: "another user", ctx: userContext(t, testAdminID, users.PermissionManageUser), wantCalls: 1},
		{name: "own account", ctx: userContext(t, testUserID, users.PermissionManageUser), wantErrs: []error{input.ErrDeleteOwnUser}},
		{name: "system identity", ctx: systemContext(t), wantCalls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			repository := &mock.UserRepository{
				DeleteFunc: func(_ context.Context, id users.UserID) users.ErrUserDeleteFailed {
					calls++

					if id != testUserID {
						t.Errorf("Delete() called with %v, want %v", id, testUserID)
					}

					return nil
				},
			}

			service, err := NewUserService(&appmock.Transactor{}, repository, &mock.PasswordHasher{})

			if err != nil {
				t.Fatal(err)
			}

			err = service.Delete(tt.ctx, testUserID)

			if calls != tt.wantCalls {
				t.Fatalf("Delete() repository calls = %d, want %d", calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)
		})
	}
}

// Every operation is refused before any repository call to a caller without
// an identity, and to one whose Role lacks the permission it needs. A User
// always finds themselves and changes their own profile and password; the
// system identity has no account to change.

func TestUserServiceAccess(t *testing.T) {
	operations := []struct {
		name string
		call func(ctx context.Context, service *UserService) error
	}{
		{"create", func(ctx context.Context, service *UserService) error {
			_, err := service.Create(ctx, input.CreateUserInput{})
			return err
		}},
		{"find another user", func(ctx context.Context, service *UserService) error {
			_, err := service.FindByID(ctx, testAdminID)
			return err
		}},
		{"list", func(ctx context.Context, service *UserService) error {
			_, err := service.List(ctx, validUserQuery())
			return err
		}},
		{"update", func(ctx context.Context, service *UserService) error {
			_, err := service.Update(ctx, input.UpdateUserInput{})
			return err
		}},
		{"delete", func(ctx context.Context, service *UserService) error {
			return service.Delete(ctx, testAdminID)
		}},
		{"update profile", func(ctx context.Context, service *UserService) error {
			_, err := service.UpdateProfile(ctx, input.UpdateProfileInput{})
			return err
		}},
		{"change password", func(ctx context.Context, service *UserService) error {
			_, err := service.ChangePassword(ctx, input.ChangePasswordInput{})
			return err
		}},
	}

	for _, tt := range []struct {
		name     string
		ctx      context.Context
		wantErrs []error
		skip     []string
	}{
		{name: "missing identity", ctx: t.Context(), wantErrs: []error{ErrUnauthenticated, security.ErrIdentityMissing}},
		{
			name:     "insufficient permission",
			ctx:      userContext(t, testUserID, users.PermissionViewRole, users.PermissionManageRole),
			wantErrs: []error{users.ErrPermissionDenied},
			skip:     []string{"update profile", "change password"},
		},
		{
			name:     "managing users without viewing them",
			ctx:      userContext(t, testUserID, users.PermissionManageUser),
			wantErrs: []error{users.ErrPermissionDenied},
			skip:     []string{"create", "find another user", "update", "delete", "update profile", "change password"},
		},
		{
			name:     "system identity has no account",
			ctx:      systemContext(t),
			wantErrs: []error{ErrUnauthenticated},
			skip:     []string{"create", "find another user", "list", "update", "delete"},
		},
	} {
		for _, operation := range operations {
			if slices.Contains(tt.skip, operation.name) {
				continue
			}

			t.Run(tt.name+" "+operation.name, func(t *testing.T) {
				calls := 0
				repository := &mock.UserRepository{
					FindByIDFunc: func(context.Context, users.UserID) (*users.User, error) {
						calls++
						return testUser(), nil
					},
				}

				service, err := NewUserService(&appmock.Transactor{}, repository, &mock.PasswordHasher{})

				if err != nil {
					t.Fatal(err)
				}

				err = operation.call(tt.ctx, service)

				if calls != 0 {
					t.Fatalf("%s repository calls = %d, want none", operation.name, calls)
				}

				assertErrors(t, err, tt.wantErrs)
			})
		}
	}

	for _, tt := range []struct {
		name string
		ctx  context.Context
		id   users.UserID
	}{
		{"themselves without permissions", userContext(t, testUserID), testUserID},
		{"another user with permission to manage users", userContext(t, testUserID, users.PermissionManageUser), testAdminID},
	} {
		t.Run("find "+tt.name, func(t *testing.T) {
			repository := &mock.UserRepository{
				FindByIDFunc: func(context.Context, users.UserID) (*users.User, error) { return testUser(), nil },
			}

			service, err := NewUserService(&appmock.Transactor{}, repository, &mock.PasswordHasher{})

			if err != nil {
				t.Fatal(err)
			}

			if _, err = service.FindByID(tt.ctx, tt.id); err != nil {
				t.Fatalf("FindByID() = %v, want it allowed", err)
			}
		})
	}
}

func validUserQuery() users.UserQuery {
	return users.UserQuery{
		Filter:    users.UserFilter{EmailLike: "example"},
		SortOrder: domquery.SortOrderAsc,
		SortBy:    users.UserSortEmail,
		Limit:     20,
		Page:      1,
	}
}

func testUser() *users.User {
	return users.NewUser(
		testUserID,
		testRoleID,
		"anna@example.com",
		testMoment,
		"+37120000000",
		testMoment,
		"existing hash",
		testMoment,
		"Anna",
		"Bērziņa",
		testMoment,
		testMoment,
	)
}

func reloadFrom(
	stored **users.User,
	reloadErr error,
) func(ctx context.Context, id users.UserID) (*users.User, error) {
	return func(ctx context.Context, id users.UserID) (*users.User, error) {
		var (
			user *users.User
			err  = reloadErr
		)

		if !appmock.InTransaction(ctx) {
			err = errors.New("reload outside a transaction")
		}

		if err == nil && *stored != nil && (*stored).ID == id {
			copied := **stored
			copied.UpdatedAt = testReloaded
			copied.CreatedAt = testReloaded
			user = &copied
		} else if err == nil {
			err = users.ErrUserNotFound
		}

		return user, err
	}
}

// The mock hasher accepts only "correct horse" for the stored hash.

func profileHasher(hashErr error, hashCalls *int) *mock.PasswordHasher {
	return &mock.PasswordHasher{
		HashFunc: func(_ context.Context, password string) (users.PasswordHash, error) {
			*hashCalls++

			return "hash:" + users.PasswordHash(password), hashErr
		},
		VerifyFunc: func(_ context.Context, password string, hash users.PasswordHash) (bool, error) {
			return hash == "existing hash" && password == "correct horse", nil
		},
	}
}

// The User is loaded outside the transaction for the password check, then
// written and reloaded inside it.

func profileRepository(
	t *testing.T,
	stored **users.User,
	updateErr error,
	written **users.User,
	writes *int,
) *mock.UserRepository {
	reload := reloadFrom(stored, nil)

	return &mock.UserRepository{
		FindByIDFunc: func(ctx context.Context, id users.UserID) (*users.User, error) {
			var (
				user *users.User
				err  error
			)

			if appmock.InTransaction(ctx) {
				user, err = reload(ctx, id)
			} else if *stored == nil {
				err = users.ErrUserNotFound
			} else {
				copied := **stored
				user = &copied
			}

			return user, err
		},
		UpdateFunc: func(ctx context.Context, user *users.User) users.ErrUserUpdateFailed {
			var err users.ErrUserUpdateFailed

			*writes++
			*written = user

			if !appmock.InTransaction(ctx) {
				t.Error("Update() ran outside a transaction")
			}

			if updateErr != nil {
				err = mock.NewErrUserUpdateFailed(user, updateErr)
			} else {
				*stored = user
			}

			return err
		},
	}
}

func TestUserServiceUpdateProfile(t *testing.T) {
	names := input.UpdateProfileInput{
		FirstName: "Anna",
		LastName:  "Ozola",
		Email:     "anna@example.com",
		UpdatedAt: testMoment,
	}
	email := names
	email.Email = "anna.new@example.com"
	email.CurrentPassword = "correct horse"
	emailWithoutPassword := email
	emailWithoutPassword.CurrentPassword = ""
	emailWithWrongPassword := email
	emailWithWrongPassword.CurrentPassword = "wrong horse"

	for _, tt := range []struct {
		name       string
		request    input.UpdateProfileInput
		missing    bool
		updateErr  error
		wantErrs   []error
		wantWrites int
		wantEmail  users.Email
	}{
		{name: "names without a password", request: names, wantWrites: 1, wantEmail: "anna@example.com"},
		{name: "email with the current password", request: email, wantWrites: 1, wantEmail: "anna.new@example.com"},
		{name: "email without a password", request: emailWithoutPassword, wantErrs: []error{input.ErrCurrentPasswordRequired}},
		{name: "email with a wrong password", request: emailWithWrongPassword, wantErrs: []error{input.ErrCurrentPasswordIncorrect}},
		{
			name:     "invalid fields",
			request:  input.UpdateProfileInput{FirstName: " ", Email: "Anna <anna@example.com>"},
			wantErrs: []error{input.ErrUpdateProfileInputInvalid, users.ErrFirstNameEmpty, users.ErrLastNameEmpty, users.ErrEmailInvalid},
		},
		{name: "missing user", request: names, missing: true, wantErrs: []error{users.ErrUserNotFound}},
		{name: "outdated change", request: names, updateErr: users.ErrUserConflict, wantErrs: []error{users.ErrUserConflict}, wantWrites: 1},
		{name: "taken email", request: email, updateErr: users.ErrUserAlreadyExists, wantErrs: []error{users.ErrUserAlreadyExists}, wantWrites: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hashCalls, writes := 0, 0
			stored := testUser()

			var written *users.User

			if tt.missing {
				stored = nil
			}

			service, err := NewUserService(&appmock.Transactor{}, profileRepository(t, &stored, tt.updateErr, &written, &writes), profileHasher(nil, &hashCalls))

			if err != nil {
				t.Fatal(err)
			}

			user, err := service.UpdateProfile(userContext(t, testUserID), tt.request)

			if writes != tt.wantWrites || hashCalls != 0 {
				t.Fatalf("calls = (write %d, hash %d), want (%d, 0)", writes, hashCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if user != nil {
					t.Fatalf("UpdateProfile() user = %v, want nil on error", user)
				}
			} else {
				original := testUser()

				if user.Email != tt.wantEmail || user.FirstName != "Anna" || user.LastName != "Ozola" || user.UpdatedAt != testReloaded {
					t.Fatalf("UpdateProfile() user = %+v, want the reloaded profile", user)
				}

				if written.RoleID != original.RoleID || written.Phone != original.Phone || written.PasswordHash != original.PasswordHash || written.UpdatedAt != tt.request.UpdatedAt {
					t.Fatalf("written user = %+v, want the role, phone, and password kept and the request's UpdatedAt", written)
				}
			}
		})
	}
}

func TestUserServiceChangePassword(t *testing.T) {
	valid := input.ChangePasswordInput{CurrentPassword: "correct horse", NewPassword: "battery staple"}
	withoutCurrent := valid
	withoutCurrent.CurrentPassword = ""
	wrongCurrent := valid
	wrongCurrent.CurrentPassword = "wrong horse"
	short := valid
	short.NewPassword = "short"

	for _, tt := range []struct {
		name       string
		request    input.ChangePasswordInput
		hashErr    error
		updateErr  error
		wantErrs   []error
		wantHashes int
		wantWrites int
	}{
		{name: "changed", request: valid, wantHashes: 1, wantWrites: 1},
		{name: "without the current password", request: withoutCurrent, wantErrs: []error{input.ErrCurrentPasswordRequired}},
		{name: "with a wrong current password", request: wrongCurrent, wantErrs: []error{input.ErrCurrentPasswordIncorrect}},
		{name: "short new password", request: short, wantErrs: []error{input.ErrChangePasswordInputInvalid, users.ErrPasswordTooShort}},
		{name: "hash failure", request: valid, hashErr: context.DeadlineExceeded, wantErrs: []error{context.DeadlineExceeded}, wantHashes: 1},
		{name: "outdated change", request: valid, updateErr: users.ErrUserConflict, wantErrs: []error{users.ErrUserConflict}, wantHashes: 1, wantWrites: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hashCalls, writes := 0, 0
			stored := testUser()

			var written *users.User

			service, err := NewUserService(&appmock.Transactor{}, profileRepository(t, &stored, tt.updateErr, &written, &writes), profileHasher(tt.hashErr, &hashCalls))

			if err != nil {
				t.Fatal(err)
			}

			user, err := service.ChangePassword(userContext(t, testUserID), tt.request)

			if hashCalls != tt.wantHashes || writes != tt.wantWrites {
				t.Fatalf("calls = (hash %d, write %d), want (%d, %d)", hashCalls, writes, tt.wantHashes, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if user != nil {
					t.Fatalf("ChangePassword() user = %v, want nil on error", user)
				}
			} else if user.PasswordHash != "hash:battery staple" || written.Email != testUser().Email || written.UpdatedAt != testMoment {
				t.Fatalf("ChangePassword() user = %+v, written = %+v, want the new hash with the rest kept", user, written)
			}
		})
	}
}
