package users

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/languages"
)

// Every constructor returns the value it was given, even when the value is
// invalid, together with the error of its validation.

func TestValueConstructors(t *testing.T) {
	id := uuid.MustParse("01920000-0000-7000-8000-000000000001")

	tests := []struct {
		name      string
		construct func() (any, error)
		want      any
		wantErr   error
	}{
		{"user ID", func() (any, error) { return constructed(NewUserID(id)) }, UserID(id), nil},
		{"nil user ID", func() (any, error) { return constructed(NewUserID(uuid.Nil)) }, UserID{}, ErrUserIDInvalid},
		{"email", func() (any, error) { return constructed(NewEmail("ada@example.com")) }, Email("ada@example.com"), nil},
		{"invalid email", func() (any, error) { return constructed(NewEmail("Ada <ada@example.com>")) }, Email("Ada <ada@example.com>"), ErrEmailInvalid},
		{"phone", func() (any, error) { return constructed(NewPhone("+15551234567")) }, Phone("+15551234567"), nil},
		{"invalid phone", func() (any, error) { return constructed(NewPhone("5551234567")) }, Phone("5551234567"), ErrPhoneInvalid},
		{"first name", func() (any, error) { return constructed(NewFirstName("Ada")) }, FirstName("Ada"), nil},
		{"blank first name", func() (any, error) { return constructed(NewFirstName(" ")) }, FirstName(" "), ErrFirstNameEmpty},
		{"last name", func() (any, error) { return constructed(NewLastName("Lovelace")) }, LastName("Lovelace"), nil},
		{"blank last name", func() (any, error) { return constructed(NewLastName("")) }, LastName(""), ErrLastNameEmpty},
		{"password", func() (any, error) { return constructed(NewPassword("secret1")) }, Password("secret1"), nil},
		{"short password", func() (any, error) { return constructed(NewPassword("abc")) }, Password("abc"), ErrPasswordTooShort},
		{"password hash", func() (any, error) { return constructed(NewPasswordHash("hash")) }, PasswordHash("hash"), nil},
		{"empty password hash", func() (any, error) { return constructed(NewPasswordHash("")) }, PasswordHash(""), ErrPasswordHashEmpty},
		{"role ID", func() (any, error) { return constructed(NewRoleID(id)) }, RoleID(id), nil},
		{"nil role ID", func() (any, error) { return constructed(NewRoleID(uuid.Nil)) }, RoleID{}, ErrRoleIDInvalid},
		{"permission", func() (any, error) { return constructed(NewPermission("view_role")) }, PermissionViewRole, nil},
		{"unknown permission", func() (any, error) { return constructed(NewPermission("fly")) }, Permission("fly"), ErrPermissionInvalid},
		{"permissions", func() (any, error) {
			return constructed(NewPermissions([]string{"view_role", "view_user"}))
		}, Permissions{PermissionViewRole, PermissionViewUser}, nil},
		{"no permissions", func() (any, error) { return constructed(NewPermissions(nil)) }, Permissions(nil), nil},
		{"unknown permission among permissions", func() (any, error) {
			return constructed(NewPermissions([]string{"view_role", "fly"}))
		}, Permissions{PermissionViewRole, "fly"}, ErrPermissionInvalid},
		{"role name", func() (any, error) {
			return constructed(NewRoleName(map[string]string{"en": "Editor", "lv": "Redaktors"}))
		}, RoleName{"en": "Editor", "lv": "Redaktors"}, nil},
		{"missing role name", func() (any, error) { return constructed(NewRoleName(nil)) }, RoleName(nil), languages.ErrTextNil},
		{"role name with an invalid code", func() (any, error) {
			return constructed(NewRoleName(map[string]string{"english": "Editor"}))
		}, RoleName{"english": "Editor"}, languages.ErrCodeInvalid},
		{"role sort", func() (any, error) { return constructed(NewRoleSort("name")) }, RoleSortName, nil},
		{"unknown role sort", func() (any, error) { return constructed(NewRoleSort("email")) }, RoleSort("email"), ErrInvalidRoleQuery},
		{"user sort", func() (any, error) { return constructed(NewUserSort("email")) }, UserSortEmail, nil},
		{"unknown user sort", func() (any, error) { return constructed(NewUserSort("name")) }, UserSort("name"), ErrInvalidUserQuery},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.construct()

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) {
				t.Fatalf("got error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func constructed[T any](value T, err error) (any, error) {
	return value, err
}

func TestUserValidate(t *testing.T) {
	valid := func() *User {
		return NewUser(
			UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001")),
			RoleID(uuid.MustParse("01920000-0000-7000-8000-000000000002")),
			"ada@example.com",
			time.Now(),
			"+15551234567",
			time.Now(),
			"hash",
			time.Now(),
			"Ada",
			"Lovelace",
			time.Now(),
			time.Now(),
		)
	}

	tests := []struct {
		name     string
		change   func(user *User)
		wantErrs []error
	}{
		{"valid", func(*User) {}, nil},
		{"nil ID", func(user *User) { user.ID = UserID{} }, []error{ErrUserIDInvalid}},
		{"invalid email", func(user *User) { user.Email = "ada" }, []error{ErrEmailInvalid}},
		{"invalid email and blank last name", func(user *User) {
			user.Email = "ada"
			user.LastName = " "
		}, []error{ErrEmailInvalid, ErrLastNameEmpty}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := valid()
			tt.change(user)
			err := user.Validate()

			if (err == nil) != (len(tt.wantErrs) == 0) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErrs)
			}

			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want %v", err, want)
				}
			}

			if err != nil && !errors.Is(err, ErrUserInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrUserInvalid)
			}
		})
	}
}

func TestUserValidateNil(t *testing.T) {
	err := (*User)(nil).Validate()

	if !errors.Is(err, ErrUserNil) || !errors.Is(err, ErrUserInvalid) {
		t.Fatalf("got %v, want %v wrapped in %v", err, ErrUserNil, ErrUserInvalid)
	}
}
