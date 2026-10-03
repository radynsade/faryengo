package security

import (
	"testing"
)

func TestSuperRolePermissions(t *testing.T) {
	role, err := NewRole(RoleID{1}, testName(t), nil)

	if err != nil {
		t.Fatal(err)
	}

	if role.IsSuper() {
		t.Fatal("new role defaults to super")
	}

	for _, tt := range []struct {
		name       string
		isSuper    bool
		permission Permission
		want       bool
	}{
		{name: "regular role", permission: PermissionManageUser},
		{name: "super manage", isSuper: true, permission: PermissionManageUser, want: true},
		{name: "super view", isSuper: true, permission: PermissionViewUser, want: true},
		{name: "super rejects unknown", isSuper: true, permission: "unknown"},
		{name: "super cleared", permission: PermissionManageUser},
	} {
		t.Run(tt.name, func(t *testing.T) {
			role.SetIsSuper(tt.isSuper)
			principal := Principal{IsSuper: role.IsSuper(), Permissions: role.Permissions()}

			if principal.HasPermission(tt.permission) != tt.want || role.IsSuper() != tt.isSuper {
				t.Fatalf("permission %q = %v, want %v", tt.permission, principal.HasPermission(tt.permission), tt.want)
			}
		})
	}
}
