package utils

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"mime"
	"net/http"
	"slices"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
)

//
// Validation rules
//

// The domain's mailbox syntax accepts addresses the built-in email rule
// rejects, so the transport delegates to the domain value.

func init() {
	if err := requestvalidation.RegisterStringRule("office_mailbox", func(value string) bool {
		return users.Email(value).Validate() == nil
	}); err != nil {
		panic(err)
	}
}

//
// Sign-in form
//

const maxSignInFormBytes = 32 << 10

// A form with an unexpected content type, an oversized body, an unknown field
// or a repeated field is malformed rather than invalid, so it is rejected
// without field errors.

var ErrFormInvalid = errors.New("invalid form")

type SignInForm struct {
	Email    string `form:"email" validate:"required,office_mailbox,maxbytes=254"`
	Password string `form:"password" validate:"required,maxbytes=4096"`
}

func ParseSignInForm(writer http.ResponseWriter, request *http.Request) (SignInForm, error) {
	var form SignInForm

	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))

	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		err = ErrFormInvalid
	} else {
		request.Body = http.MaxBytesReader(writer, request.Body, maxSignInFormBytes)

		if parseErr := request.ParseForm(); parseErr != nil {
			err = fmt.Errorf("%w: %w", ErrFormInvalid, parseErr)
		}
	}

	if err == nil {
		for _, key := range slices.Sorted(maps.Keys(request.PostForm)) {
			if (key != "email" && key != "password") || len(request.PostForm[key]) > 1 {
				err = ErrFormInvalid
			}
		}
	}

	if err == nil {
		form = SignInForm{Email: request.PostForm.Get("email"), Password: request.PostForm.Get("password")}
	}

	return form, err
}

//
// Field errors
//

// FieldErrors maps validation failures to translated messages keyed by form
// field name.

func FieldErrors(ctx context.Context, err error) map[string]string {
	var (
		failures *requestvalidation.Errors
		fields   map[string]string
	)

	if errors.As(err, &failures) {
		fields = make(map[string]string)

		for _, field := range failures.Fields {
			key, messageID := field.Field, "validation.invalid"

			switch field.Field {
			case "Email":
				key = "email"
			case "Password":
				key = "password"
			}

			switch field.Rule {
			case "required":
				messageID = "validation.required"
			case "office_mailbox":
				messageID = "validation.email"
			case "maxbytes":
				messageID = "validation.too_long"
			}

			if _, found := fields[key]; !found {
				fields[key] = officei18n.T(ctx, messageID)
			}
		}
	}

	return fields
}
