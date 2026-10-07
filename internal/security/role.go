package security

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/radynsade/faryengo/internal/languages"
)

//
// Permission
//

var (
	ErrPermissionInvalid = errors.New("invalid permission")
	ErrPermissionDenied  = errors.New("permission denied")
)

type Permission string

const (
	PermissionManageUser Permission = "manage_user"
	PermissionViewUser   Permission = "view_user"
	PermissionManageRole Permission = "manage_role"
	PermissionViewRole   Permission = "view_role"
)

func (p Permission) Validate() error {
	switch p {
	case PermissionManageUser, PermissionViewUser, PermissionManageRole, PermissionViewRole:
		return nil
	default:
		return ErrPermissionInvalid
	}
}

type Permissions []Permission

func (p Permissions) Validate() error {
	var err error

	for _, permission := range p {
		err = permission.Validate()

		if err != nil {
			break
		}
	}

	return err
}

func AllPermissions() Permissions {
	return Permissions{
		PermissionManageUser,
		PermissionViewUser,
		PermissionManageRole,
		PermissionViewRole,
	}
}

//
// Role ID
//

var ErrRoleIDInvalid = errors.New("invalid role ID")

type RoleID uuid.UUID

func (id RoleID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrRoleIDInvalid
	}

	return err
}

//
// Role
//

var (
	ErrRoleInvalid = errors.New("invalid role")
	ErrRoleNil     = errors.New("is nil")
)

type Role struct {
	ID          RoleID
	Name        languages.Text
	Permissions Permissions
	IsSuper     bool
}

func NewRole(id RoleID, name languages.Text, permissions Permissions, isSuper bool) *Role {
	return &Role{
		ID:          id,
		Name:        name,
		Permissions: permissions,
		IsSuper:     isSuper,
	}
}

func (r *Role) Validate() error {
	var err error

	if r == nil {
		err = ErrRoleNil
	} else {
		err = r.ID.Validate()
	}

	if err == nil {
		err = r.Name.Validate()
	}

	if err == nil {
		err = r.Permissions.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrRoleInvalid, err)
	}

	return err
}
