package security

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/google/uuid"
	"github.com/radynsade/faryengo/internal/languages"
)

//
// Permission
//

var (
	ErrInvalidPermission = errors.New("invalid permission")
	ErrPermissionDenied  = errors.New("permission denied")
)

type Permission string

const (
	PermissionManageUser Permission = "manage_user"
	PermissionViewUser   Permission = "view_user"
	PermissionManageRole Permission = "manage_role"
	PermissionViewRole   Permission = "view_role"
)

func AllPermissions() []Permission {
	return []Permission{
		PermissionManageUser,
		PermissionViewUser,
		PermissionManageRole,
		PermissionViewRole,
	}
}

func (p Permission) Validate() error {
	switch p {
	case PermissionManageUser, PermissionViewUser, PermissionManageRole, PermissionViewRole:
		return nil
	default:
		return ErrInvalidPermission
	}
}

//
// Role ID
//

var ErrInvalidRoleID = errors.New("invalid role ID")

type RoleID uuid.UUID

func (id RoleID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrInvalidRoleID
	}

	return err
}

//
// Role
//

var ErrInvalidRole = errors.New("invalid role")

type Role struct {
	id          RoleID
	name        languages.Text
	permissions []Permission
	isSuper     bool
}

func (r *Role) ID() RoleID {
	return r.id
}

func (r *Role) SetID(id RoleID) {
	r.id = id
}

func (r *Role) Name() languages.Text {
	return maps.Clone(r.name)
}

func (r *Role) SetName(name languages.Text) {
	r.name = maps.Clone(name)
}

func (r *Role) Permissions() []Permission {
	return slices.Clone(r.permissions)
}

func (r *Role) SetPermissions(permissions []Permission) {
	r.permissions = slices.Clone(permissions)
}

func (r *Role) IsSuper() bool {
	return r.isSuper
}

func (r *Role) SetIsSuper(isSuper bool) {
	r.isSuper = isSuper
}

func (r *Role) Validate() error {
	var err error

	err = r.id.Validate()

	if err == nil {
		err = r.name.Validate()
	}

	if err == nil {
		for _, permission := range r.permissions {
			err = permission.Validate()

			if err != nil {
				break
			}
		}
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidRole, err)
	}

	return err
}

//
// Role repository
//

var (
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleAlreadyExists = errors.New("role already exists")
	ErrRoleAlreadyInUse  = errors.New("role is assigned to users")
)

type ErrRoleCreateFailed interface {
	error
	Role() *Role
	Unwrap() error
}

type ErrRoleUpdateFailed interface {
	error
	Role() *Role
	Unwrap() error
}

type ErrRoleDeleteFailed interface {
	error
	RoleID() RoleID
	Unwrap() error
}

type RoleRepository interface {
	Create(ctx context.Context, role *Role) ErrRoleCreateFailed
	Update(ctx context.Context, role *Role) ErrRoleUpdateFailed
	Delete(ctx context.Context, id RoleID) ErrRoleDeleteFailed
	FindByID(ctx context.Context, id RoleID) (*Role, error)
	// Find(ctx context.Context, query RoleQuery) ([]*Role, error)
	// Count(ctx context.Context, filters RoleFilters) (int, error)
}
