package app

import (
	"errors"
	"testing"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

func TestCreateRoleServiceDomainInputs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		request input.CreateRoleInput
		want    []error
	}{
		{name: "translated role", request: input.CreateRoleInput{Name: map[string]string{"en": "Administrator", "lv": "Administrators"}, Permissions: []security.Permission{security.PermissionViewUser}}},
		{name: "empty permissions", request: input.CreateRoleInput{Name: map[string]string{"en": "Guest"}}},
		{name: "role permissions", request: input.CreateRoleInput{Name: map[string]string{"en": "Role manager"}, Permissions: []security.Permission{security.PermissionManageRole, security.PermissionViewRole}}},
		{name: "super", request: input.CreateRoleInput{Name: map[string]string{"en": "Super"}, IsSuper: true}},
		{name: "missing name", want: []error{security.ErrInvalidRoleName}},
		{name: "invalid translation", request: input.CreateRoleInput{Name: map[string]string{"EN": "Name", "lv": " "}, Permissions: []security.Permission{"unknown"}}, want: []error{languages.ErrInvalidCodeCharacters, languages.ErrInvalidTranslationContentCharacters, security.ErrInvalidPermission}},
		{name: "invalid UTF8", request: input.CreateRoleInput{Name: map[string]string{"en": "\xff"}}, want: []error{languages.ErrInvalidTranslationContentCharacters}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored, _ := roleFromInput(security.RoleID{1}, map[string]string{"en": "Old"}, nil, false)
			service, _ := NewRoleService(&fakeRoleStore{stored: stored})

			_, err := service.Create(t.Context(), tt.request)

			if (err == nil) != (len(tt.want) == 0) || (err != nil && !errors.Is(err, input.ErrInvalidCreateRoleInput)) {
				t.Fatalf("Service() = %v, want %v", err, tt.want)
			}

			for _, want := range tt.want {
				if !errors.Is(err, want) {
					t.Fatalf("Service() = %v, want %v", err, want)
				}
			}
		})
	}
}

func TestUpdateRoleServiceDomainInputs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		request input.UpdateRoleInput
		want    error
	}{
		{name: "unchanged fields", request: input.UpdateRoleInput{ID: security.RoleID{1}}},
		{name: "clear permissions", request: input.UpdateRoleInput{ID: security.RoleID{1}, Permissions: []security.Permission{}}},
		{name: "role permissions", request: input.UpdateRoleInput{ID: security.RoleID{1}, Permissions: []security.Permission{security.PermissionManageRole, security.PermissionViewRole}}},
		{name: "replace name", request: input.UpdateRoleInput{ID: security.RoleID{1}, Name: map[string]string{"lv": "Loma"}}},
		{name: "invalid ID", want: security.ErrInvalidRoleID},
		{name: "empty replacement", request: input.UpdateRoleInput{ID: security.RoleID{1}, Name: map[string]string{}}, want: security.ErrInvalidRoleName},
		{name: "invalid permission", request: input.UpdateRoleInput{ID: security.RoleID{1}, Permissions: []security.Permission{"unknown"}}, want: security.ErrInvalidPermission},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored, _ := roleFromInput(security.RoleID{1}, map[string]string{"en": "Old"}, nil, false)
			service, _ := NewRoleService(&fakeRoleStore{stored: stored})

			_, err := service.Update(t.Context(), tt.request)

			if !errors.Is(err, tt.want) || (err != nil && !errors.Is(err, input.ErrInvalidUpdateRoleInput)) {
				t.Fatalf("Service() = %v, want %v", err, tt.want)
			}
		})
	}
}
