package security_test

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

func TestRoleValidate(t *testing.T) {
	validID := security.RoleID(uuid.MustParse("01971a62-51dd-7000-8000-000000000001"))
	validName := languages.Text{"en": "Administrator"}

	for _, tt := range []struct {
		name        string
		id          security.RoleID
		roleName    languages.Text
		permissions []security.Permission
		isSuper     bool
		want        []error
		exclude     error
	}{
		{name: "valid", id: validID, roleName: validName, permissions: security.AllPermissions()},
		{name: "nil permissions", id: validID, roleName: validName},
		{name: "empty permissions", id: validID, roleName: validName, permissions: []security.Permission{}},
		{name: "super role", id: validID, roleName: validName, isSuper: true},
		{name: "zero values", want: []error{security.ErrInvalidRoleID}, exclude: languages.ErrInvalidText},
		{name: "nil name", id: validID, want: []error{languages.ErrInvalidText, languages.ErrTextHasNoTranslations}},
		{name: "empty name", id: validID, roleName: languages.Text{}, want: []error{languages.ErrInvalidText, languages.ErrTextHasNoTranslations}},
		{name: "invalid language code", id: validID, roleName: languages.Text{"EN": "Administrator"}, want: []error{languages.ErrInvalidText, languages.ErrInvalidCode}},
		{name: "blank translation", id: validID, roleName: languages.Text{"en": " \t"}, want: []error{languages.ErrInvalidText, languages.ErrInvalidTranslationContent}},
		{name: "invalid UTF-8 translation", id: validID, roleName: languages.Text{"en": "\xff"}, want: []error{languages.ErrInvalidText, languages.ErrInvalidTranslationContent}},
		{name: "invalid later permission", id: validID, roleName: validName, permissions: []security.Permission{security.PermissionViewRole, "unknown"}, want: []error{security.ErrInvalidPermission}},
		{name: "super role with invalid permission", id: validID, roleName: validName, permissions: []security.Permission{"unknown"}, isSuper: true, want: []error{security.ErrInvalidPermission}},
		{name: "name validated before permissions", id: validID, permissions: []security.Permission{"unknown"}, want: []error{languages.ErrInvalidText, languages.ErrTextHasNoTranslations}, exclude: security.ErrInvalidPermission},
	} {
		t.Run(tt.name, func(t *testing.T) {
			role := security.NewRole(tt.id, maps.Clone(tt.roleName), slices.Clone(tt.permissions), tt.isSuper)

			if role == nil {
				t.Fatal("NewRole() returned nil before validation")
			}

			err := role.Validate()

			if len(tt.want) == 0 {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, security.ErrInvalidRole) {
					t.Errorf("Validate() = %v, want ErrInvalidRole", err)
				}

				for _, target := range tt.want {
					if !errors.Is(err, target) {
						t.Errorf("Validate() = %v, want errors.Is(_, %v)", err, target)
					}
				}
			}

			if tt.exclude != nil && errors.Is(err, tt.exclude) {
				t.Errorf("Validate() = %v, must stop before %v", err, tt.exclude)
			}

			if role.ID != tt.id || !maps.Equal(role.Name, tt.roleName) || !slices.Equal(role.Permissions, tt.permissions) || role.IsSuper != tt.isSuper {
				t.Fatal("Validate() changed the role state")
			}

			if (role.Name == nil) != (tt.roleName == nil) || (role.Permissions == nil) != (tt.permissions == nil) {
				t.Fatal("Validate() changed the role state")
			}
		})
	}
}

func TestRoleValidateFieldChanges(t *testing.T) {
	validID := security.RoleID(uuid.MustParse("01971a62-51dd-7000-8000-000000000001"))

	for _, tt := range []struct {
		name   string
		change func(*security.Role)
		want   error
	}{
		{name: "zero ID", change: func(role *security.Role) { role.ID = security.RoleID{} }, want: security.ErrInvalidRoleID},
		{name: "nil name", change: func(role *security.Role) { role.Name = nil }, want: languages.ErrTextHasNoTranslations},
		{name: "blank translation", change: func(role *security.Role) { role.Name["en"] = "" }, want: languages.ErrEmptyTranslationContent},
		{name: "invalid permissions", change: func(role *security.Role) { role.Permissions = []security.Permission{"unknown"} }, want: security.ErrInvalidPermission},
		{name: "invalid permission element", change: func(role *security.Role) { role.Permissions[0] = "unknown" }, want: security.ErrInvalidPermission},
		{name: "super role", change: func(role *security.Role) { role.IsSuper = true }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			role := security.NewRole(validID, languages.Text{"en": "Administrator"}, []security.Permission{security.PermissionViewRole}, false)
			tt.change(role)
			before := *role
			before.Name = maps.Clone(role.Name)
			before.Permissions = slices.Clone(role.Permissions)
			err := role.Validate()

			if !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want errors.Is(_, %v)", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, security.ErrInvalidRole) {
				t.Errorf("Validate() = %v, want ErrInvalidRole", err)
			}

			if role.ID != before.ID || !maps.Equal(role.Name, before.Name) || !slices.Equal(role.Permissions, before.Permissions) || role.IsSuper != before.IsSuper {
				t.Fatal("Validate() changed the role state")
			}

			if (role.Name == nil) != (before.Name == nil) || (role.Permissions == nil) != (before.Permissions == nil) {
				t.Fatal("Validate() changed the role state")
			}
		})
	}
}
