package security

import (
	"errors"
	"testing"
)

func TestPermissionValidate(t *testing.T) {
	for _, tt := range []struct {
		name       string
		permission Permission
		wantErr    error
	}{
		{name: "manage user", permission: PermissionManageUser},
		{name: "view user", permission: PermissionViewUser},
		{name: "empty", wantErr: ErrInvalidPermission},
		{name: "unknown", permission: Permission("delete_user"), wantErr: ErrInvalidPermission},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.permission.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Permission.Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
