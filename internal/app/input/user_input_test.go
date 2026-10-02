package input

import (
	"errors"
	"testing"

	"github.com/radynsade/faryengo/internal/security"
)

func TestCreateUserInputValidate(t *testing.T) {
	valid := CreateUserInput{RoleID: security.RoleID{1}, Email: "person@example.com", Phone: "+37123456789", Password: "secret", FirstName: "First", LastName: "Last"}

	for _, tt := range []struct {
		name    string
		input   CreateUserInput
		wantErr error
	}{
		{name: "valid", input: valid},
		{name: "invalid fields", input: CreateUserInput{}, wantErr: ErrInvalidCreateUserInput},
		{name: "empty password", input: CreateUserInput{RoleID: valid.RoleID, Email: valid.Email, Phone: valid.Phone, FirstName: valid.FirstName, LastName: valid.LastName}, wantErr: security.ErrInvalidPassword},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && !errors.Is(err, ErrInvalidCreateUserInput) {
				t.Fatalf("Validate() error = %v, want ErrInvalidCreateUserInput", err)
			}
		})
	}
}

func TestUpdateUserInputValidate(t *testing.T) {
	valid := UpdateUserInput{ID: security.UserID{2}, RoleID: security.RoleID{1}, Email: "person@example.com", Phone: "+37123456789", FirstName: "First", LastName: "Last"}
	empty := ""
	invalidPassword := valid
	invalidPassword.Password = &empty

	for _, tt := range []struct {
		name    string
		input   UpdateUserInput
		wantErr error
	}{
		{name: "valid without password change", input: valid},
		{name: "invalid ID", input: UpdateUserInput{RoleID: valid.RoleID, Email: valid.Email, Phone: valid.Phone, FirstName: valid.FirstName, LastName: valid.LastName}, wantErr: ErrInvalidUserID},
		{name: "empty new password", input: invalidPassword, wantErr: security.ErrInvalidPassword},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && !errors.Is(err, ErrInvalidUpdateUserInput) {
				t.Fatalf("Validate() error = %v, want ErrInvalidUpdateUserInput", err)
			}
		})
	}
}
