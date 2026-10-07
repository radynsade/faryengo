package security_test

import (
	"errors"
	"github.com/radynsade/faryengo/internal/security"
	"testing"
)

func TestPermissionValidate(t *testing.T) {
	for _, tt := range []struct {
		name       string
		permission security.Permission
		wantErr    error
	}{
		{name: "manage user", permission: security.PermissionManageUser},
		{name: "view user", permission: security.PermissionViewUser},
		{name: "manage role", permission: security.PermissionManageRole},
		{name: "view role", permission: security.PermissionViewRole},
		{name: "empty", wantErr: security.ErrInvalidPermission},
		{name: "unknown", permission: security.Permission("delete_user"), wantErr: security.ErrInvalidPermission},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.permission.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("security.Permission.Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
