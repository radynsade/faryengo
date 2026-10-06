package security

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/radynsade/faryengo/internal/languages"
)

// NewRoleName constructs a nonempty translated role name from primitive values.
func NewRoleName(values map[string]string) (languages.Text, error) {
	translations := make([]languages.Translation, 0, len(values))
	var problems []error
	var name languages.Text

	for _, rawCode := range slices.Sorted(maps.Keys(values)) {
		code, codeErr := languages.NewLanguageCode(rawCode)

		if codeErr != nil {
			problems = append(problems, fmt.Errorf("name translation %q: %w", rawCode, codeErr))
		} else {
			translation, err := languages.NewTranslation(code, values[rawCode])

			if err != nil {
				problems = append(problems, fmt.Errorf("name translation %q: %w", rawCode, err))
			} else {
				translations = append(translations, translation)
			}
		}
	}

	err := errors.Join(problems...)

	if err == nil {
		name, err = languages.NewText(translations)
	}

	if err == nil {
		err = validateRoleName(name)
	}

	if err != nil {
		name = nil
	}

	return name, err
}

func NewPermission(value string) (Permission, error) {
	permission := Permission(value)
	err := permission.Validate()

	if err != nil {
		permission = ""
	}

	return permission, err
}

func NewPermissions(values []Permission) ([]Permission, error) {
	permissions := slices.Clone(values)
	err := validatePermissions(permissions)

	if err != nil {
		permissions = nil
	}

	return permissions, err
}
