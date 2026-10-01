package security

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestNewRole(t *testing.T) {
	id := RoleID(uuid.UUID{1})

	for _, tt := range []struct {
		name        string
		id          RoleID
		permissions []Permission
		wantErr     error
	}{
		{name: "empty permissions", id: id},
		{name: "valid permissions", id: id, permissions: []Permission{PermissionViewUser, PermissionManageUser}},
		{name: "missing ID", permissions: []Permission{PermissionViewUser}, wantErr: ErrInvalidRoleID},
		{name: "invalid permission", id: id, permissions: []Permission{"unknown"}, wantErr: ErrInvalidPermission},
	} {
		t.Run(tt.name, func(t *testing.T) {
			role, err := NewRole(tt.id, tt.permissions)
			if tt.wantErr != nil {
				if role != nil || !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewRole() = (%v, %v), want (nil, %v)", role, err, tt.wantErr)
				}

				return
			}

			if err != nil || role == nil {
				t.Fatalf("NewRole() = (%v, %v), want role and nil error", role, err)
			}

			if role.ID() != id || !slices.Equal(role.Permissions(), tt.permissions) {
				t.Fatalf("NewRole() = (%v, %v), want ID %v and permissions %v", role, err, id, tt.permissions)
			}
		})
	}
}

func TestRoleChangesPreserveValidState(t *testing.T) {
	originalID := RoleID(uuid.UUID{1})
	permissions := []Permission{PermissionViewUser}
	role, err := NewRole(originalID, permissions)
	if err != nil {
		t.Fatalf("NewRole() error = %v", err)
	}

	permissions[0] = PermissionManageUser
	if !slices.Equal(role.Permissions(), []Permission{PermissionViewUser}) {
		t.Fatal("caller changed role permissions through constructor input")
	}

	returned := role.Permissions()
	returned[0] = PermissionManageUser
	if !slices.Equal(role.Permissions(), []Permission{PermissionViewUser}) {
		t.Fatal("caller changed role permissions through getter result")
	}

	if err := role.SetPermissions([]Permission{"unknown"}); !errors.Is(err, ErrInvalidPermission) {
		t.Fatalf("SetPermissions(invalid) = %v, want %v", err, ErrInvalidPermission)
	}

	if err := role.SetID(RoleID{}); !errors.Is(err, ErrInvalidRoleID) {
		t.Fatalf("SetID(zero) = %v, want %v", err, ErrInvalidRoleID)
	}

	if role.ID() != originalID || !slices.Equal(role.Permissions(), []Permission{PermissionViewUser}) {
		t.Fatal("invalid setters changed role")
	}

	newPermissions := []Permission{PermissionManageUser}
	if err := role.SetPermissions(newPermissions); err != nil {
		t.Fatalf("SetPermissions(valid) error = %v", err)
	}

	newPermissions[0] = PermissionViewUser
	newID := RoleID(uuid.UUID{2})
	if err := role.SetID(newID); err != nil {
		t.Fatalf("SetID(valid) error = %v", err)
	}

	if role.ID() != newID || !slices.Equal(role.Permissions(), []Permission{PermissionManageUser}) {
		t.Fatal("valid setters did not preserve role state")
	}
}
