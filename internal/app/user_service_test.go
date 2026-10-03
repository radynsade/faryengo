package app

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

type fakeUserStore struct {
	ctx         context.Context
	user        *security.User
	id          security.UserID
	stored      *security.User
	createErr   error
	updateErr   error
	deleteErr   error
	findErr     error
	createCalls int
	updateCalls int
	deleteCalls int
	findCalls   int
}

func (f *fakeUserStore) Create(ctx context.Context, user *security.User) error {
	f.ctx, f.user = ctx, user
	f.createCalls++
	return f.createErr
}

func (f *fakeUserStore) Update(ctx context.Context, user *security.User) error {
	f.ctx, f.user = ctx, user
	f.updateCalls++
	return f.updateErr
}

func (f *fakeUserStore) Delete(ctx context.Context, id security.UserID) error {
	f.ctx, f.id = ctx, id
	f.deleteCalls++
	return f.deleteErr
}

func (f *fakeUserStore) FindByID(ctx context.Context, id security.UserID) (*security.User, error) {
	f.ctx, f.id = ctx, id
	f.findCalls++
	return f.stored, f.findErr
}

type fakePasswordHasher struct {
	ctx      context.Context
	password string
	hash     security.PasswordHash
	err      error
	calls    int
}

func (f *fakePasswordHasher) Hash(ctx context.Context, password string) (security.PasswordHash, error) {
	f.ctx, f.password = ctx, password
	f.calls++
	return f.hash, f.err
}

func (f *fakePasswordHasher) Verify(context.Context, string, security.PasswordHash) (bool, error) {
	return false, nil
}

func validCreateUserInput() input.CreateUserInput {
	return input.CreateUserInput{RoleID: security.RoleID{1}, Email: "person@example.com", Phone: "+37123456789", Password: "secret-password", FirstName: "First", LastName: "Last"}
}

func validUpdateUserInput() input.UpdateUserInput {
	return input.UpdateUserInput{ID: security.UserID{2}, RoleID: security.RoleID{1}, Email: "new@example.com", Phone: "+37123456789", FirstName: "New", LastName: "Name"}
}

func TestUserServiceCreate(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name      string
		request   input.CreateUserInput
		hashErr   error
		createErr error
		wantErr   error
		wantWrite bool
	}{
		{name: "created", request: validCreateUserInput(), wantWrite: true},
		{name: "invalid", request: input.CreateUserInput{}, wantErr: input.ErrInvalidCreateUserInput},
		{name: "hash failure", request: validCreateUserInput(), hashErr: context.Canceled, wantErr: context.Canceled},
		{name: "duplicate", request: validCreateUserInput(), createErr: security.ErrUserAlreadyExists, wantErr: security.ErrUserAlreadyExists, wantWrite: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeUserStore{createErr: tt.createErr}
			hasher := &fakePasswordHasher{hash: "encoded", err: tt.hashErr}
			service, err := NewUserService(store, hasher)
			if err != nil {
				t.Fatal(err)
			}

			user, err := service.Create(ctx, tt.request)
			if !errors.Is(err, tt.wantErr) || (store.createCalls == 1) != tt.wantWrite {
				t.Fatalf("Create() = (%v, %v), writes = %d; want %v, write %v", user, err, store.createCalls, tt.wantErr, tt.wantWrite)
			}

			if tt.wantErr == nil {
				if user == nil || user != store.user || uuid.UUID(user.ID()) == uuid.Nil || user.PasswordHash() != "encoded" || user.Email() != security.Email(tt.request.Email) || hasher.ctx != ctx || hasher.password != "secret-password" || store.ctx != ctx {
					t.Fatalf("Create() user = %v, hasher = %+v, store = %+v", user, hasher, store)
				}
			} else if user != nil {
				t.Fatalf("Create() user = %v, want nil on error", user)
			}
		})
	}
}

func TestUserServiceUpdate(t *testing.T) {
	ctx := context.Background()
	stored, err := security.NewUser(security.UserID{2}, security.RoleID{1}, "old@example.com", "+37123456789", "old-hash", "Old", "Name")
	if err != nil {
		t.Fatal(err)
	}
	newPassword := "new-secret"
	withPassword := validUpdateUserInput()
	withPassword.Password = &newPassword

	for _, tt := range []struct {
		name        string
		request     input.UpdateUserInput
		stored      *security.User
		findErr     error
		hashErr     error
		updateErr   error
		wantErr     error
		wantHash    security.PasswordHash
		wantFind    int
		wantHashOps int
		wantWrite   int
	}{
		{name: "preserve password", request: validUpdateUserInput(), stored: stored, wantHash: "old-hash", wantFind: 1, wantWrite: 1},
		{name: "change password", request: withPassword, stored: stored, wantFind: 1, wantHash: "new-hash", wantHashOps: 1, wantWrite: 1},
		{name: "credentials changed during update", request: validUpdateUserInput(), stored: stored, updateErr: security.ErrUserConflict, wantErr: security.ErrUserConflict, wantFind: 1, wantWrite: 1},
		{name: "invalid", request: input.UpdateUserInput{}, wantErr: input.ErrInvalidUpdateUserInput},
		{name: "missing user", request: validUpdateUserInput(), findErr: security.ErrUserNotFound, wantErr: security.ErrUserNotFound, wantFind: 1},
		{name: "nil loaded user", request: validUpdateUserInput(), wantErr: security.ErrUserNotFound, wantFind: 1},
		{name: "hash failure", request: withPassword, stored: stored, wantFind: 1, hashErr: context.Canceled, wantErr: context.Canceled, wantHashOps: 1},
		{name: "write failure", request: withPassword, stored: stored, wantFind: 1, updateErr: security.ErrUserNotFound, wantErr: security.ErrUserNotFound, wantHashOps: 1, wantWrite: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeUserStore{stored: tt.stored, findErr: tt.findErr, updateErr: tt.updateErr}
			hasher := &fakePasswordHasher{hash: "new-hash", err: tt.hashErr}
			service, err := NewUserService(store, hasher)
			if err != nil {
				t.Fatal(err)
			}

			user, err := service.Update(ctx, tt.request)
			if !errors.Is(err, tt.wantErr) || store.findCalls != tt.wantFind || hasher.calls != tt.wantHashOps || store.updateCalls != tt.wantWrite {
				t.Fatalf("Update() = (%v, %v), find/hash/write = %d/%d/%d", user, err, store.findCalls, hasher.calls, store.updateCalls)
			}

			if tt.wantErr == nil {
				if user == nil || user != store.user || user.ID() != tt.request.ID || user.PasswordHash() != tt.wantHash || user.OriginalPasswordHash() != "old-hash" || user.Email() != security.Email(tt.request.Email) || store.ctx != ctx {
					t.Fatalf("Update() user = %v, store = %+v", user, store)
				}
			} else if user != nil {
				t.Fatalf("Update() user = %v, want nil on error", user)
			}
		})
	}
}

func TestUserServiceDelete(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name     string
		id       security.UserID
		storeErr error
		wantErr  error
		calls    int
	}{
		{name: "deleted", id: security.UserID{2}, calls: 1},
		{name: "invalid ID", wantErr: input.ErrInvalidUserID},
		{name: "not found", id: security.UserID{2}, storeErr: security.ErrUserNotFound, wantErr: security.ErrUserNotFound, calls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeUserStore{deleteErr: tt.storeErr}
			service, err := NewUserService(store, &fakePasswordHasher{})
			if err != nil {
				t.Fatal(err)
			}

			err = service.Delete(ctx, tt.id)
			if !errors.Is(err, tt.wantErr) || store.deleteCalls != tt.calls {
				t.Fatalf("Delete() error = %v, calls = %d; want %v and %d", err, store.deleteCalls, tt.wantErr, tt.calls)
			}
			if tt.calls == 1 && (store.id != tt.id || store.ctx != ctx) {
				t.Fatalf("Delete() ID = %v, context = %v", store.id, store.ctx)
			}
		})
	}
}

func TestUserServiceRejectsNilDependencies(t *testing.T) {
	store := &fakeUserStore{}
	hasher := &fakePasswordHasher{}
	for _, tt := range []struct {
		name   string
		store  security.UserRepository
		hasher security.PasswordHasher
		want   error
	}{
		{name: "repository", hasher: hasher, want: ErrNilUserRepository},
		{name: "hasher", store: store, want: ErrNilPasswordHasher},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewUserService(tt.store, tt.hasher)
			if service != nil || !errors.Is(err, tt.want) {
				t.Fatalf("NewUserService() = (%v, %v), want %v", service, err, tt.want)
			}
		})
	}
}
