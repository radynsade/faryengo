package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/radynsade/faryengo/middleware/requestvalidation"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
)

func requestFieldMessages(ctx context.Context, err error) string {
	var fields *requestvalidation.Errors
	var messages []string

	if errors.As(err, &fields) {
		for _, field := range fields.Fields {
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

			messages = append(messages, admini18n.T(ctx, "validation."+id, map[string]any{"Field": field.Field}))
		}
	}

	return strings.Join(messages, " ")
}
