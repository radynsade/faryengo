package input

import (
	"errors"

	"github.com/radynsade/faryengo/internal/security"
)

var (
	ErrInvalidCreateRoleInput = errors.New("invalid create role input")
	ErrInvalidUpdateRoleInput = errors.New("invalid update role input")
)

type CreateRoleInput struct {
	Name        map[string]string
	Permissions []security.Permission
	IsSuper     bool
}

// UpdateRoleInput leaves fields unchanged when nil. A non-nil Name replaces
// all translations, and a non-nil empty Permissions slice clears permissions.
type UpdateRoleInput struct {
	ID          security.RoleID
	Name        map[string]string
	Permissions []security.Permission
	IsSuper     *bool
}
