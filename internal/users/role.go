package users

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"unicode/utf8"

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
// Role name
//

const MaxRoleNameLength = 100

var (
	ErrRoleNameInvalid         = errors.New("invalid role name")
	ErrRoleNameTooLong         = fmt.Errorf("exceeds the limit of %d characters", MaxRoleNameLength)
	ErrRoleNameFallbackMissing = errors.New("lacks the fallback language translation")
)

type RoleName languages.Text

func (r RoleName) Validate() error {
	var err error
	err = languages.Text(r).Validate()

	if err == nil {
		for _, translation := range slices.Sorted(maps.Values(r)) {
			if utf8.RuneCountInString(string(translation)) > MaxRoleNameLength {
				err = ErrRoleNameTooLong
			}

			if err != nil {
				break
			}
		}
	}

	return err
}

// When the catalog has a fallback language, a Role name requires its
// translation and every other translation is optional. Without a fallback
// language, any one translation is enough. An empty fallback code means the
// catalog has none.

func (r RoleName) ValidateWithFallback(fallback languages.Code) error {
	var err error

	if _, found := r[fallback]; fallback != "" && !found {
		err = fmt.Errorf("%w: %w", ErrRoleNameInvalid, ErrRoleNameFallbackMissing)
	} else {
		err = r.Validate()
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
	Name        RoleName
	Permissions Permissions
	IsSuper     bool
}

func NewRole(id RoleID, name RoleName, permissions Permissions, isSuper bool) *Role {
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

// A super Role grants every permission; an ordinary Role grants only its
// assigned permissions.

func (r *Role) Grants(permission Permission) bool {
	return r != nil && (r.IsSuper || slices.Contains(r.Permissions, permission))
}
