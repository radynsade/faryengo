package admin

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
)

func requestFieldErrors(ctx context.Context, err error) components.FieldErrors {
	var failures *requestvalidation.Errors
	var fields components.FieldErrors

	if errors.As(err, &failures) {
		fields = make(components.FieldErrors)

		for _, field := range failures.Fields {
			id := "invalid"

			switch field.Rule {
			case "required", "notblank":
				id = "required"
			case "email", "mailbox":
				id = "email"
			case "uuid", "uuid_input":
				id = "uuid"
			case "e164":
				id = "phone"
			case "max", "maxbytes":
				id = "too_long"
			}

			if field.Field == "name" && (field.Rule == "required" || field.Rule == "min") {
				id = "role_name"
			}

			key := field.Field

			// All selected values belong to the one permissions control.
			if strings.HasPrefix(key, "permissions[") {
				key = "permissions"
			}

			message := admini18n.T(ctx, "validation."+id)

			if !slices.Contains(fields[key], message) {
				fields[key] = append(fields[key], message)
			}
		}
	} else {
		// Domain/application errors with an unambiguous editable field also stay
		// beside that field. Operational failures remain form-wide notifications.
		switch {
		case errors.Is(err, users.ErrInvalidRoleName):
			fields = components.FieldErrors{"name": {admini18n.T(ctx, "validation.role_name")}}
		case errors.Is(err, languages.ErrTranslationInvalidChars):
			fields = components.FieldErrors{"name": {admini18n.T(ctx, "validation.invalid")}}
		case errors.Is(err, languages.ErrLanguageNotFound):
			fields = components.FieldErrors{"name": {admini18n.T(ctx, "errors.language_missing")}}
		case errors.Is(err, users.ErrPermissionInvalid):
			fields = components.FieldErrors{"permissions": {admini18n.T(ctx, "validation.invalid")}}
		}
	}

	return fields
}
