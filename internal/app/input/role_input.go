package input

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/radynsade/faryengo/internal/languages"
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

func (request CreateRoleInput) Validate() error {
	validationErrors := validateRoleFields(request.Name, request.Permissions)

	if len(validationErrors) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidCreateRoleInput, errors.Join(validationErrors...))
	}

	return nil
}

// UpdateRoleInput leaves fields unchanged when nil. A non-nil Name replaces
// all translations, and a non-nil empty Permissions slice clears permissions.
type UpdateRoleInput struct {
	ID          security.RoleID
	Name        map[string]string
	Permissions []security.Permission
	IsSuper     *bool
}

func (request UpdateRoleInput) Validate() error {
	var validationErrors []error

	if err := request.ID.Validate(); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("ID: %w", err))
	}

	if request.Name != nil {
		validationErrors = append(validationErrors, validateRoleFields(request.Name, nil)...)
	}

	validationErrors = append(validationErrors, validateRolePermissions(request.Permissions)...)

	if len(validationErrors) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidUpdateRoleInput, errors.Join(validationErrors...))
	}

	return nil
}

func validateRoleFields(name map[string]string, permissions []security.Permission) []error {
	validationErrors := validateRolePermissions(permissions)

	if len(name) == 0 {
		validationErrors = append(validationErrors, fmt.Errorf("name: %w", security.ErrInvalidRoleName))
	}

	for _, code := range slices.Sorted(maps.Keys(name)) {
		if _, err := languages.NewTranslation(languages.LanguageCode(code), name[code]); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("name translation %q: %w", code, err))
		}
	}

	return validationErrors
}

func validateRolePermissions(permissions []security.Permission) []error {
	var validationErrors []error

	for _, permission := range permissions {
		if err := permission.Validate(); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("permission %q: %w", permission, err))
		}
	}

	return validationErrors
}
