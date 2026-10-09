package users

import "testing"

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
