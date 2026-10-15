package utils

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	"github.com/radynsade/faryengo/pkg/forms"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

//
// Currency form
//

const maxCurrencyFormBytes = 4 << 10

// The transport only bounds sizes and encodings; the domain reports every
// invalid field at once. A code is compared in upper case, so one typed in
// lower case is accepted.

type CurrencyForm struct {
	Code   string `form:"code" validate:"utf8,maxbytes=12"`
	Symbol string `form:"symbol" validate:"utf8,maxbytes=40"`
}

// A Currency is identified by its code, so editing submits only the symbol
// and takes the code from the route.

func ParseCurrencyForm(writer http.ResponseWriter, request *http.Request, edit bool) (CurrencyForm, error) {
	var form CurrencyForm

	fields := []string{"code", "symbol"}

	if edit {
		fields = []string{"symbol"}
	}

	err := forms.Parse(writer, request, maxCurrencyFormBytes)

	if err == nil {
		err = forms.CheckFields(request.PostForm, fields, nil)
	}

	if err == nil {
		form = CurrencyForm{
			Code:   strings.ToUpper(strings.TrimSpace(request.PostForm.Get("code"))),
			Symbol: strings.TrimSpace(request.PostForm.Get("symbol")),
		}
	}

	if err == nil && edit {
		form.Code = request.PathValue("currency")
	}

	return form, err
}

//
// Currency code
//

func ParseCurrencyCode(request *http.Request) (currencies.Code, error) {
	code, err := currencies.NewCode(request.PathValue("currency"))

	if err != nil {
		code = ""
		err = fmt.Errorf("parse a currency code: %w", err)
	}

	return code, err
}

//
// Currency errors
//

// Invalid input is checked before an invalid code, because the input errors
// wrap the domain's code errors while a code in the route is not input.

func CurrencyError(request *http.Request, err error) (int, string) {
	status, messageID := http.StatusInternalServerError, "errors.currencies"

	switch {
	case errors.Is(err, currencies.ErrCurrencyNotFound):
		status, messageID = http.StatusNotFound, "errors.currency_not_found"
	case errors.Is(err, currencies.ErrCurrencyAlreadyExists):
		status, messageID = http.StatusConflict, "errors.currency_exists"
	case errors.Is(err, currencies.ErrCurrencyInUse):
		status, messageID = http.StatusConflict, "errors.currency_used"
	case errors.Is(err, requestvalidation.ErrInvalidRequest),
		errors.Is(err, input.ErrCreateCurrencyInputInvalid),
		errors.Is(err, input.ErrUpdateCurrencyInputInvalid):
		status, messageID = http.StatusUnprocessableEntity, "errors.currency_values"
	case errors.Is(err, currencies.ErrCodeInvalid), errors.Is(err, forms.ErrInvalid):
		status, messageID = http.StatusBadRequest, "errors.currency_form"
	}

	if status == http.StatusInternalServerError {
		slog.ErrorContext(request.Context(), "admin currency operation", "error", err)
	}

	return status, messageID
}

//
// Currency values
//

func CurrencyRow(currency *currencies.Currency) pages.CurrencyRow {
	return pages.CurrencyRow{Code: string(currency.Code), Symbol: string(currency.Symbol)}
}

// A code taken by another Currency belongs beside the code, so a duplicate is
// both a field error and a conflict.

var currencyFieldErrors = []fieldErrorMapping{
	{key: "code", messageID: "validation.currency_code", causes: []error{currencies.ErrCodeInvalid}},
	{key: "code", messageID: "validation.currency_taken", causes: []error{currencies.ErrCurrencyAlreadyExists}},
	{key: "symbol", messageID: "validation.required", causes: []error{currencies.ErrSymbolEmpty}},
	{key: "symbol", messageID: "validation.too_long", causes: []error{currencies.ErrSymbolTooLong}},
	{key: "symbol", messageID: "validation.currency_symbol", causes: []error{currencies.ErrSymbolInvalidChars, currencies.ErrSymbolNoSign}},
}

func CurrencyFormFieldErrors(ctx context.Context, err error) components.FieldErrors {
	return fieldErrors(ctx, err, currencyFieldErrors)
}
