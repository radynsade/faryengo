package pages

import (
	"context"

	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
)

//
// Currency values
//

type CurrencyRow struct {
	Code   string
	Symbol string
}

//
// Currency list
//

type CurrencyListProps struct {
	Panel  layouts.PanelProps
	Rows   []CurrencyRow
	Errors []string
}

//
// Currency form
//

// CurrencyFormID names the region a failed in-page submission replaces.

const CurrencyFormID = "currency-form"

// A Currency is identified by its code, so only creation submits one; editing
// shows the code and changes only the symbol.

type CurrencyFormProps struct {
	Panel       layouts.PanelProps
	Edit        bool
	Code        string
	Symbol      string
	Title       string
	Action      string
	Errors      []string
	FieldErrors components.FieldErrors
}

//
// Currency view
//

type CurrencyViewProps struct {
	Panel    layouts.PanelProps
	Currency CurrencyRow
}

//
// Helpers
//

func CurrencyDeleteDialog(ctx context.Context, action, code string, errors []string) components.ConfirmDeleteProps {
	return components.ConfirmDeleteProps{
		Title:   admini18n.T(ctx, "currencies.delete_title", map[string]any{"Code": code}),
		Message: admini18n.T(ctx, "currencies.delete_message"),
		Action:  action,
		Errors:  errors,
	}
}
