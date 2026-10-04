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

var (
	ErrInvalidRoleID     = errors.New("invalid role ID")
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleAlreadyExists = errors.New("role already exists")
	ErrRoleAlreadyInUse  = errors.New("role is assigned to users")
	ErrInvalidRoleName   = errors.New("invalid role name")
)

type RoleID uuid.UUID

func (id RoleID) Validate() error {
	if uuid.UUID(id) == uuid.Nil {
		return ErrInvalidRoleID
	}

	return nil
}

type Role struct {
	id          RoleID
	name        languages.Text
	permissions []Permission
	isSuper     bool
}

func NewRole(id RoleID, name languages.Text, permissions []Permission) (*Role, error) {
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}

	if err := validateRoleName(name); err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}

	if err := validatePermissions(permissions); err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}

	return &Role{
		id:          id,
		name:        maps.Clone(name),
		permissions: slices.Clone(permissions),
	}, nil
}

func (r *Role) ID() RoleID {
	return r.id
}

func (r *Role) SetID(id RoleID) error {
	if err := id.Validate(); err != nil {
		return fmt.Errorf("set role ID: %w", err)
	}

	r.id = id
	return nil
}

func (r *Role) Name() languages.Text {
	return maps.Clone(r.name)
}

func (r *Role) SetName(name languages.Text) error {
	if err := validateRoleName(name); err != nil {
		return fmt.Errorf("set role name: %w", err)
	}

	r.name = maps.Clone(name)
	return nil
}

func (r *Role) Permissions() []Permission {
	return slices.Clone(r.permissions)
}

func (r *Role) IsSuper() bool {
	return r.isSuper
}

func (r *Role) SetIsSuper(isSuper bool) {
	r.isSuper = isSuper
}

func (r *Role) SetPermissions(permissions []Permission) error {
	if err := validatePermissions(permissions); err != nil {
		return fmt.Errorf("set role permissions: %w", err)
	}

	r.permissions = slices.Clone(permissions)
	return nil
}

func validateRoleName(name languages.Text) error {
	if len(name) == 0 {
		return ErrInvalidRoleName
	}

	if err := name.Validate(); err != nil {
		return fmt.Errorf("validate role name: %w", err)
	}

	return nil
}

func validatePermissions(permissions []Permission) error {
	for _, permission := range permissions {
		if err := permission.Validate(); err != nil {
			return fmt.Errorf("permission %q: %w", permission, err)
		}
	}

	return nil
}

// RoleRepository stores and retrieves roles.
type RoleRepository interface {
	Create(ctx context.Context, role *Role) error
	Update(ctx context.Context, role *Role) error
	Delete(ctx context.Context, id RoleID) error
	FindByID(ctx context.Context, id RoleID) (*Role, error)
	Find(ctx context.Context, query RoleQuery) ([]*Role, error)
	Count(ctx context.Context, filters RoleFilters) (int, error)
}
