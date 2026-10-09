package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/users/mock"
)

var (
	testUserID   = users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001"))
	testRoleID   = users.RoleID(uuid.MustParse("01920000-0000-7000-8000-000000000002"))
	testMoment   = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	testReloaded = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
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
	ctx := t.Context()

	var service *UserService

	created, createErr := service.Create(ctx, input.CreateUserInput{})
	found, findErr := service.FindByID(ctx, testUserID)
	updated, updateErr := service.Update(ctx, input.UpdateUserInput{})
	deleteErr := service.Delete(ctx, testUserID)

	for _, err := range []error{createErr, findErr, updateErr, deleteErr} {
		if !errors.Is(err, ErrUserRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrUserRepositoryNil", err)
		}
	}

	if created != nil || found != nil || updated != nil {
		t.Fatalf("nil receiver returned users: %v, %v, %v", created, found, updated)
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
			ctx := t.Context()
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
			ctx := t.Context()
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
			ctx := t.Context()
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
			ctx := t.Context()
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
