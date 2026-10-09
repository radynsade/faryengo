package users

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/languages"
)

func TestRoleGrants(t *testing.T) {
	tests := []struct {
		name       string
		role       *Role
		permission Permission
		want       bool
	}{
		{"nil role", nil, PermissionViewRole, false},
		{"assigned permission", &Role{Permissions: Permissions{PermissionViewRole}}, PermissionViewRole, true},
		{"missing permission", &Role{Permissions: Permissions{PermissionManageRole}}, PermissionViewRole, false},
		{"no permissions", &Role{}, PermissionViewUser, false},
		{"super role", &Role{IsSuper: true}, PermissionManageUser, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.Grants(tt.permission); got != tt.want {
				t.Fatalf("Grants(%s) = %t, want %t", tt.permission, got, tt.want)
			}
		})
	}
}

func TestRoleNameValidateWithFallback(t *testing.T) {
	tests := []struct {
		name     string
		roleName RoleName
		fallback languages.Code
		want     error
	}{
		{"fallback translation only", RoleName{"en": "Editor"}, "en", nil},
		{"fallback and other translations", RoleName{"en": "Editor", "lv": "Redaktors"}, "en", nil},
		{"missing fallback translation", RoleName{"lv": "Redaktors"}, "en", ErrRoleNameFallbackMissing},
		{"empty name with a fallback language", RoleName{}, "en", ErrRoleNameFallbackMissing},
		{"blank fallback translation", RoleName{"en": " "}, "en", languages.ErrTranslationEmpty},
		{"too long other translation", RoleName{"en": "Editor", "lv": translationOfLength(MaxRoleNameLength + 1)}, "en", ErrRoleNameTooLong},
		{"no fallback language", RoleName{"lv": "Redaktors"}, "", nil},
		{"empty name without a fallback language", RoleName{}, "", languages.ErrTextWithoutTranslations},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.roleName.ValidateWithFallback(tt.fallback); !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("ValidateWithFallback(%q) = %v, want %v", tt.fallback, err, tt.want)
			}
		})
	}
}

func translationOfLength(length int) languages.Translation {
	return languages.Translation(strings.Repeat("a", length))
}
