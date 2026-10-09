package input

import (
	"errors"

	"github.com/radynsade/faryengo/internal/users"
)

//
// Create role input
//

// Name maps language codes to translations.

var ErrCreateRoleInputInvalid = errors.New("invalid create role input")

type CreateRoleInput struct {
	Name        map[string]string
	Permissions []string
	IsSuper     bool
}

//
// Update role input
//

// The update replaces every field of the Role; an empty Permissions clears
// them.

var ErrUpdateRoleInputInvalid = errors.New("invalid update role input")

type UpdateRoleInput struct {
	ID          users.RoleID
	Name        map[string]string
	Permissions []string
	IsSuper     bool
}
