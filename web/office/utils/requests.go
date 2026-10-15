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
	"github.com/radynsade/faryengo/internal/workspaces"
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
		_, err := users.NewEmail(value)

		return err == nil
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

	err := parseForm(writer, request, maxSignInFormBytes, "email", "password")

	if err == nil {
		form = SignInForm{Email: request.PostForm.Get("email"), Password: request.PostForm.Get("password")}
	}

	return form, err
}

//
// Workspace form
//

// The name's limits belong to the domain, which validates it after trimming;
// the transport bounds only the size of the whole form.

const maxWorkspaceFormBytes = 8 << 10

type WorkspaceForm struct {
	Name string
}

func ParseWorkspaceForm(writer http.ResponseWriter, request *http.Request) (WorkspaceForm, error) {
	var form WorkspaceForm

	err := parseForm(writer, request, maxWorkspaceFormBytes, "name")

	if err == nil {
		form = WorkspaceForm{Name: request.PostForm.Get("name")}
	}

	return form, err
}

// WorkspaceFieldErrors maps a rejected workspace name to a translated message
// keyed by the form field name.

func WorkspaceFieldErrors(ctx context.Context, err error) map[string]string {
	var fields map[string]string

	switch {
	case errors.Is(err, workspaces.ErrNameEmpty):
		fields = map[string]string{"name": officei18n.T(ctx, "validation.required")}
	case errors.Is(err, workspaces.ErrNameTooLong):
		fields = map[string]string{
			"name": officei18n.Count(ctx, "validation.too_long_chars", workspaces.MaxNameLength),
		}
	case errors.Is(err, workspaces.ErrNameInvalid):
		fields = map[string]string{"name": officei18n.T(ctx, "validation.invalid")}
	}

	return fields
}

//
// Form parsing
//

// A form with an unexpected content type, an oversized body, a field other
// than the allowed ones, or a repeated field is malformed.

func parseForm(
	writer http.ResponseWriter,
	request *http.Request,
	maxBytes int64,
	allowed ...string,
) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))

	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		err = ErrFormInvalid
	} else {
		request.Body = http.MaxBytesReader(writer, request.Body, maxBytes)

		if parseErr := request.ParseForm(); parseErr != nil {
			err = fmt.Errorf("%w: %w", ErrFormInvalid, parseErr)
		}
	}

	if err == nil {
		for _, key := range slices.Sorted(maps.Keys(request.PostForm)) {
			if !slices.Contains(allowed, key) || len(request.PostForm[key]) > 1 {
				err = ErrFormInvalid
			}
		}
	}

	return err
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
