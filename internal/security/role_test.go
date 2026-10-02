package security

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/languages"
)

func TestNewRole(t *testing.T) {
	id := RoleID(uuid.UUID{1})
	name := testName(t)

	for _, tt := range []struct {
		name        string
		id          RoleID
		permissions []Permission
		roleName    languages.Text
		wantErr     error
	}{
		{name: "empty permissions", id: id, roleName: name},
		{name: "valid permissions", id: id, roleName: name, permissions: []Permission{PermissionViewUser, PermissionManageUser}},
		{name: "missing ID", roleName: name, permissions: []Permission{PermissionViewUser}, wantErr: ErrInvalidRoleID},
		{name: "invalid permission", id: id, roleName: name, permissions: []Permission{"unknown"}, wantErr: ErrInvalidPermission},
		{name: "missing name", id: id, wantErr: ErrInvalidRoleName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			role, err := NewRole(tt.id, tt.roleName, tt.permissions)
			if tt.wantErr != nil {
				if role != nil || !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewRole() = (%v, %v), want (nil, %v)", role, err, tt.wantErr)
				}

				return
			}

			if err != nil || role == nil {
				t.Fatalf("NewRole() = (%v, %v), want role and nil error", role, err)
			}

			if role.ID() != id || len(role.Name()) != len(tt.roleName) || !slices.Equal(role.Permissions(), tt.permissions) {
				t.Fatalf("NewRole() = (%v, %v), want ID %v and permissions %v", role, err, id, tt.permissions)
			}
		})
	}
}

func TestRoleChangesPreserveValidState(t *testing.T) {
	originalID := RoleID(uuid.UUID{1})
	permissions := []Permission{PermissionViewUser}
	role, err := NewRole(originalID, testName(t), permissions)
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

func testName(t *testing.T) languages.Text {
	t.Helper()
	translation, err := languages.NewTranslation("en", "Administrator")
	if err != nil {
		t.Fatalf("NewTranslation() error = %v", err)
	}

	name, err := languages.NewText([]languages.Translation{translation})
	if err != nil {
		t.Fatalf("NewText() error = %v", err)
	}

	return name
}

func TestRoleNamePreservesValidState(t *testing.T) {
	name := testName(t)
	role, err := NewRole(RoleID{1}, name, nil)
	if err != nil {
		t.Fatalf("NewRole() error = %v", err)
	}

	delete(name, "en")
	if len(role.Name()) != 1 {
		t.Fatal("caller changed role name through constructor input")
	}

	returned := role.Name()
	delete(returned, "en")
	if len(role.Name()) != 1 {
		t.Fatal("caller changed role name through getter result")
	}

	if err := role.SetName(nil); !errors.Is(err, ErrInvalidRoleName) {
		t.Fatalf("SetName(nil) error = %v, want %v", err, ErrInvalidRoleName)
	}

	if err := role.SetName(languages.Text{"EN": {}}); !errors.Is(err, languages.ErrInvalidLanguageCode) {
		t.Fatalf("SetName(invalid) error = %v, want invalid language code", err)
	}

	if len(role.Name()) != 1 {
		t.Fatal("invalid name setter changed role")
	}

	newName := testName(t)
	if err := role.SetName(newName); err != nil {
		t.Fatalf("SetName(valid) error = %v", err)
	}

	delete(newName, "en")
	if len(role.Name()) != 1 {
		t.Fatal("caller changed role name through setter input")
	}
}
