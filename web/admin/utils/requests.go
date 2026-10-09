package utils

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
)

//
// Validation rules
//

// The domain's mailbox syntax accepts addresses the built-in email rule
// rejects, so the transport delegates to the domain value. UUIDs accept every
// encoding uuid.Parse does; the domain rejects nil identities separately.

func init() {
	for _, rule := range []struct {
		name  string
		check func(string) bool
	}{
		{"mailbox", func(value string) bool { return users.Email(value).Validate() == nil }},
		{"uuid_input", func(value string) bool { _, err := uuid.Parse(value); return err == nil }},
	} {
		if err := requestvalidation.RegisterStringRule(rule.name, rule.check); err != nil {
			panic(err)
		}
	}
}

//
// Form bodies
//

const (
	maxSignInFormBytes = 32 << 10
	maxRoleFormBytes   = 64 << 10
)

// A form with an unexpected content type, an oversized body, an unknown field
// or a repeated single-valued field is malformed rather than invalid, so it
// is rejected without field errors.

var ErrFormInvalid = errors.New("invalid form")

func parseForm(writer http.ResponseWriter, request *http.Request, maxBytes int64) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))

	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		err = ErrFormInvalid
	} else {
		request.Body = http.MaxBytesReader(writer, request.Body, maxBytes)

		if parseErr := request.ParseForm(); parseErr != nil {
			err = fmt.Errorf("%w: %w", ErrFormInvalid, parseErr)
		}
	}

	return err
}

func checkFields(values url.Values, single []string, multiple []string) error {
	var err error

	for _, key := range slices.Sorted(maps.Keys(values)) {
		if !slices.Contains(single, key) && !slices.Contains(multiple, key) {
			err = ErrFormInvalid
		} else if slices.Contains(single, key) && len(values[key]) > 1 {
			err = ErrFormInvalid
		}

		if err != nil {
			break
		}
	}

	return err
}

//
// Sign-in form
//

type SignInForm struct {
	Email    string `form:"email" validate:"required,mailbox,maxbytes=254"`
	Password string `form:"password" validate:"required,maxbytes=4096"`
}

func ParseSignInForm(writer http.ResponseWriter, request *http.Request) (SignInForm, error) {
	var form SignInForm

	err := parseForm(writer, request, maxSignInFormBytes)

	if err == nil {
		err = checkFields(request.PostForm, []string{"email", "password"}, nil)
	}

	if err == nil {
		form = SignInForm{Email: request.PostForm.Get("email"), Password: request.PostForm.Get("password")}
	}

	return form, err
}

//
// Role form
//

type RoleForm struct {
	Name        map[string]string `form:"name" validate:"dive,keys,len=2,ascii,lowercase,endkeys,notblank,utf8"`
	Permissions []string          `form:"permissions" validate:"dive,oneof=manage_user view_user manage_role view_role"`
	IsSuper     string            `form:"is_super" validate:"omitempty,eq=1"`
}

// Name translations arrive as name[<code>] fields, one per catalog language.
// A blank translation is left out of the Role's name.

func ParseRoleForm(
	writer http.ResponseWriter,
	request *http.Request,
	catalog []*languages.Language,
) (RoleForm, error) {
	form := RoleForm{Name: make(map[string]string), Permissions: []string{}}
	fields := []string{"is_super"}

	for _, language := range catalog {
		fields = append(fields, "name["+string(language.Code)+"]")
	}

	err := parseForm(writer, request, maxRoleFormBytes)

	if err == nil {
		err = checkFields(request.PostForm, fields, []string{"permissions"})
	}

	if err == nil {
		for _, language := range catalog {
			code := string(language.Code)

			if name := strings.TrimSpace(request.PostForm.Get("name[" + code + "]")); name != "" {
				form.Name[code] = name
			}
		}

		form.Permissions = append(form.Permissions, request.PostForm["permissions"]...)
		form.IsSuper = request.PostForm.Get("is_super")
	}

	if err == nil && form.IsSuper != "" && form.IsSuper != "1" {
		err = ErrFormInvalid
	}

	return form, err
}

//
// Role delete form
//

func ParseRoleDeleteForm(writer http.ResponseWriter, request *http.Request) error {
	err := parseForm(writer, request, maxRoleFormBytes)

	if err == nil {
		err = checkFields(request.PostForm, []string{"confirm"}, nil)
	}

	if err == nil && request.PostForm.Get("confirm") != "delete" {
		err = ErrFormInvalid
	}

	return err
}

//
// Role ID
//

type roleIDParameter struct {
	ID string `form:"role" validate:"required,uuid_input"`
}

func ParseRoleID(request *http.Request) (users.RoleID, error) {
	var id users.RoleID

	parameter := roleIDParameter{ID: request.PathValue("role")}
	err := requestvalidation.Validate(request.Context(), parameter)

	if err == nil {
		var parsed uuid.UUID

		parsed, err = uuid.Parse(parameter.ID)
		id = users.RoleID(parsed)
	}

	if err == nil {
		err = id.Validate()
	}

	if err != nil {
		id = users.RoleID{}
		err = fmt.Errorf("%w: %w", users.ErrRoleIDInvalid, err)
	}

	return id, err
}

//
// Field errors
//

// The application layer joins every invalid field into one error, so each
// mapping is checked independently and the first match for a control wins.

var domainFieldErrors = []struct {
	key       string
	messageID string
	causes    []error
}{
	{"name", "validation.role_name", []error{languages.ErrTextNil, languages.ErrTextWithoutTranslations}},
	{"name", "validation.too_long", []error{users.ErrRoleNameTooLong}},
	{"name", "validation.invalid", []error{languages.ErrTranslationInvalid, languages.ErrCodeInvalid}},
	{"name", "errors.language_missing", []error{languages.ErrLanguageNotFound}},
	{"permissions", "validation.invalid", []error{users.ErrPermissionInvalid}},
}

// Field errors map transport rules and domain validation errors that belong
// to one editable control to localized messages beside that control.
// Operational failures are not field errors.

func FieldErrors(ctx context.Context, err error) components.FieldErrors {
	var (
		failures *requestvalidation.Errors
		fields   components.FieldErrors
	)

	if errors.As(err, &failures) {
		fields = make(components.FieldErrors)

		for _, field := range failures.Fields {
			key, messageID := field.Field, "validation.invalid"

			switch field.Rule {
			case "required", "notblank":
				messageID = "validation.required"
			case "email", "mailbox":
				messageID = "validation.email"
			case "uuid", "uuid_input":
				messageID = "validation.uuid"
			case "max", "maxbytes":
				messageID = "validation.too_long"
			}

			// Every selected value belongs to the one permissions control.
			if strings.HasPrefix(key, "permissions[") {
				key = "permissions"
			}

			message := admini18n.T(ctx, messageID)

			if !slices.Contains(fields[key], message) {
				fields[key] = append(fields[key], message)
			}
		}
	} else {
		for _, mapping := range domainFieldErrors {
			matches := slices.ContainsFunc(mapping.causes, func(cause error) bool { return errors.Is(err, cause) })

			if matches && fields == nil {
				fields = make(components.FieldErrors)
			}

			if matches && fields[mapping.key] == nil {
				fields[mapping.key] = []string{admini18n.T(ctx, mapping.messageID)}
			}
		}
	}

	return fields
}
