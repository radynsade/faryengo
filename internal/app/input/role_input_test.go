package input

import (
	"errors"
	"testing"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

func TestCreateRoleInputValidate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		request CreateRoleInput
		want    []error
	}{
		{name: "translated role", request: CreateRoleInput{Name: map[string]string{"en": "Administrator", "lv": "Administrators"}, Permissions: []security.Permission{security.PermissionViewUser}}},
		{name: "empty permissions", request: CreateRoleInput{Name: map[string]string{"en": "Guest"}}},
		{name: "role permissions", request: CreateRoleInput{Name: map[string]string{"en": "Role manager"}, Permissions: []security.Permission{security.PermissionManageRole, security.PermissionViewRole}}},
		{name: "super", request: CreateRoleInput{Name: map[string]string{"en": "Super"}, IsSuper: true}},
		{name: "missing name", want: []error{security.ErrInvalidRoleName}},
		{name: "invalid translation", request: CreateRoleInput{Name: map[string]string{"EN": "Name", "lv": " "}, Permissions: []security.Permission{"unknown"}}, want: []error{languages.ErrInvalidLanguageCode, languages.ErrInvalidTranslationContent, security.ErrInvalidPermission}},
		{name: "invalid UTF8", request: CreateRoleInput{Name: map[string]string{"en": "\xff"}}, want: []error{languages.ErrInvalidTranslationContent}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()

			if (err == nil) != (len(tt.want) == 0) || (err != nil && !errors.Is(err, ErrInvalidCreateRoleInput)) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			for _, want := range tt.want {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want %v", err, want)
				}
			}
		})
	}
}

func TestUpdateRoleInputValidate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		request UpdateRoleInput
		want    error
	}{
		{name: "unchanged fields", request: UpdateRoleInput{ID: security.RoleID{1}}},
		{name: "clear permissions", request: UpdateRoleInput{ID: security.RoleID{1}, Permissions: []security.Permission{}}},
		{name: "role permissions", request: UpdateRoleInput{ID: security.RoleID{1}, Permissions: []security.Permission{security.PermissionManageRole, security.PermissionViewRole}}},
		{name: "replace name", request: UpdateRoleInput{ID: security.RoleID{1}, Name: map[string]string{"lv": "Loma"}}},
		{name: "invalid ID", want: security.ErrInvalidRoleID},
		{name: "empty replacement", request: UpdateRoleInput{ID: security.RoleID{1}, Name: map[string]string{}}, want: security.ErrInvalidRoleName},
		{name: "invalid permission", request: UpdateRoleInput{ID: security.RoleID{1}, Permissions: []security.Permission{"unknown"}}, want: security.ErrInvalidPermission},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()

			if !errors.Is(err, tt.want) || (err != nil && !errors.Is(err, ErrInvalidUpdateRoleInput)) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}
