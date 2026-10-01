package security

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
)

var (
	ErrInvalidRoleID = errors.New("invalid role ID")
	ErrRoleNotFound  = errors.New("role not found")
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
	permissions []Permission
}

func NewRole(id RoleID, permissions []Permission) (*Role, error) {
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}

	if err := validatePermissions(permissions); err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}

	return &Role{
		id:          id,
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

func (r *Role) Permissions() []Permission {
	return slices.Clone(r.permissions)
}

func (r *Role) SetPermissions(permissions []Permission) error {
	if err := validatePermissions(permissions); err != nil {
		return fmt.Errorf("set role permissions: %w", err)
	}

	r.permissions = slices.Clone(permissions)
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

// RoleRepository stores and retrieves roles by ID.
type RoleRepository interface {
	Save(ctx context.Context, role *Role) error
	FindByID(ctx context.Context, id RoleID) (*Role, error)
}
