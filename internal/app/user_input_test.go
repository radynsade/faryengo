package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

func TestCreateUserServiceDomainInputs(t *testing.T) {
	valid := input.CreateUserInput{RoleID: security.RoleID{1}, Email: "person@example.com", Phone: "+37123456789", Password: "secret-password", FirstName: "First", LastName: "Last"}
	withPassword := func(password string) input.CreateUserInput {
		request := valid
		request.Password = password

		return request
	}

	for _, tt := range []struct {
		name    string
		request input.CreateUserInput
		wantErr error
	}{
		{name: "valid", request: valid},
		{name: "invalid fields", request: input.CreateUserInput{}, wantErr: input.ErrInvalidCreateUserInput},
		{name: "empty password", request: input.CreateUserInput{RoleID: valid.RoleID, Email: valid.Email, Phone: valid.Phone, FirstName: valid.FirstName, LastName: valid.LastName}, wantErr: security.ErrInvalidPassword},
		{name: "short password", request: withPassword("1234567"), wantErr: security.ErrInvalidPassword},
		{name: "blank password", request: withPassword("        "), wantErr: security.ErrInvalidPassword},
		{name: "minimum password length", request: withPassword("12345678")},
		{name: "short Unicode password", request: withPassword("āāāāāāā"), wantErr: security.ErrInvalidPassword},
		{name: "Unicode password", request: withPassword("āāāāāāāā")},
		{name: "maximum password bytes", request: withPassword(strings.Repeat("x", security.MaxPasswordBytes))},
		{name: "oversized password", request: withPassword(strings.Repeat("x", security.MaxPasswordBytes+1)), wantErr: security.ErrInvalidPassword},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, _ := NewUserService(&fakeUserStore{stored: validStoredUser(t)}, &fakePasswordHasher{hash: "hash"})

			_, err := service.Create(t.Context(), tt.request)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Service() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && !errors.Is(err, input.ErrInvalidCreateUserInput) {
				t.Fatalf("Service() error = %v, want input.ErrInvalidCreateUserInput", err)
			}
		})
	}
}

func TestUpdateUserServiceDomainInputs(t *testing.T) {
	valid := input.UpdateUserInput{ID: security.UserID{2}, RoleID: security.RoleID{1}, Email: "person@example.com", Phone: "+37123456789", FirstName: "First", LastName: "Last"}
	empty := ""
	invalidPassword := valid
	invalidPassword.Password = &empty

	for _, tt := range []struct {
		name    string
		request input.UpdateUserInput
		wantErr error
	}{
		{name: "valid without password change", request: valid},
		{name: "invalid ID", request: input.UpdateUserInput{RoleID: valid.RoleID, Email: valid.Email, Phone: valid.Phone, FirstName: valid.FirstName, LastName: valid.LastName}, wantErr: input.ErrInvalidUserID},
		{name: "empty new password", request: invalidPassword, wantErr: security.ErrInvalidPassword},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, _ := NewUserService(&fakeUserStore{stored: validStoredUser(t)}, &fakePasswordHasher{hash: "hash"})

			_, err := service.Update(t.Context(), tt.request)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Service() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && !errors.Is(err, input.ErrInvalidUpdateUserInput) {
				t.Fatalf("Service() error = %v, want input.ErrInvalidUpdateUserInput", err)
			}
		})
	}
}

func validStoredUser(t *testing.T) *security.User {
	t.Helper()
	user, err := security.NewUser(security.UserID{2}, security.RoleID{1}, "old@example.com", "+37123456789", "hash", "Old", "Name")

	if err != nil {
		t.Fatal(err)
	}

	return user
}
