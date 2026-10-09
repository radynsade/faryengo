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
	"time"

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
	maxUserFormBytes   = 32 << 10
	maxDeleteFormBytes = 1 << 10
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
// User form
//

// The transport only bounds sizes and encodings; the domain reports every
// invalid field at once. A submitted password is never rendered back.

type UserForm struct {
	FirstName string `form:"first_name" validate:"utf8,maxbytes=400"`
	LastName  string `form:"last_name" validate:"utf8,maxbytes=400"`
	Email     string `form:"email" validate:"utf8,maxbytes=254"`
	Phone     string `form:"phone" validate:"utf8,maxbytes=16"`
	RoleID    string `form:"role" validate:"omitempty,uuid_input"`
	Password  string `form:"password" validate:"utf8,maxbytes=4096"`
	UpdatedAt time.Time
}

// Editing carries the moment of the User's latest change the form was based
// on, so an update over a newer change is rejected as a conflict.

func ParseUserForm(writer http.ResponseWriter, request *http.Request, edit bool) (UserForm, error) {
	var form UserForm

	fields := []string{"first_name", "last_name", "email", "phone", "role", "password"}

	if edit {
		fields = append(fields, "updated_at")
	}

	err := parseForm(writer, request, maxUserFormBytes)

	if err == nil {
		err = checkFields(request.PostForm, fields, nil)
	}

	if err == nil {
		form = UserForm{
			FirstName: strings.TrimSpace(request.PostForm.Get("first_name")),
			LastName:  strings.TrimSpace(request.PostForm.Get("last_name")),
			Email:     strings.TrimSpace(request.PostForm.Get("email")),
			Phone:     strings.TrimSpace(request.PostForm.Get("phone")),
			RoleID:    strings.TrimSpace(request.PostForm.Get("role")),
			Password:  request.PostForm.Get("password"),
		}
	}

	if err == nil && edit {
		var parseErr error

		form.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, request.PostForm.Get("updated_at"))

		if parseErr != nil {
			err = fmt.Errorf("%w: %w", ErrFormInvalid, parseErr)
		}
	}

	return form, err
}

// A role value that is not a UUID was rejected by the transport rules, so
// only an absent role reaches the domain as the nil identity.

func (f UserForm) Role() users.RoleID {
	parsed, _ := uuid.Parse(f.RoleID)

	return users.RoleID(parsed)
}

//
// Delete form
//

func ParseDeleteForm(writer http.ResponseWriter, request *http.Request) error {
	err := parseForm(writer, request, maxDeleteFormBytes)

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
// User ID
//

type userIDParameter struct {
	ID string `form:"user" validate:"required,uuid_input"`
}

func ParseUserID(request *http.Request) (users.UserID, error) {
	var id users.UserID

	parameter := userIDParameter{ID: request.PathValue("user")}
	err := requestvalidation.Validate(request.Context(), parameter)

	if err == nil {
		var parsed uuid.UUID

		parsed, err = uuid.Parse(parameter.ID)
		id = users.UserID(parsed)
	}

	if err == nil {
		err = id.Validate()
	}

	if err != nil {
		id = users.UserID{}
		err = fmt.Errorf("%w: %w", users.ErrUserIDInvalid, err)
	}

	return id, err
}

//
// Field errors
//

// The application layer joins every invalid field into one error, so each
// mapping is checked independently and the first match for a control wins.
// A mapping with a count names a limit, which the message pluralizes.

type fieldErrorMapping struct {
	key       string
	messageID string
	count     int
	causes    []error
}

var roleFieldErrors = []fieldErrorMapping{
	{key: "name", messageID: "validation.role_name", causes: []error{languages.ErrTextNil, languages.ErrTextWithoutTranslations}},
	{key: "name", messageID: "validation.too_long", causes: []error{users.ErrRoleNameTooLong}},
	{key: "name", messageID: "validation.invalid", causes: []error{languages.ErrTranslationInvalid, languages.ErrCodeInvalid}},
	{key: "name", messageID: "errors.language_missing", causes: []error{languages.ErrLanguageNotFound}},
	{key: "permissions", messageID: "validation.invalid", causes: []error{users.ErrPermissionInvalid}},
}

var userFieldErrors = []fieldErrorMapping{
	{key: "first_name", messageID: "validation.required", causes: []error{users.ErrFirstNameEmpty}},
	{key: "first_name", messageID: "validation.too_long", causes: []error{users.ErrFirstNameTooLong}},
	{key: "first_name", messageID: "validation.invalid", causes: []error{users.ErrFirstNameInvalidChars}},
	{key: "last_name", messageID: "validation.required", causes: []error{users.ErrLastNameEmpty}},
	{key: "last_name", messageID: "validation.too_long", causes: []error{users.ErrLastNameTooLong}},
	{key: "last_name", messageID: "validation.invalid", causes: []error{users.ErrLastNameInvalidChars}},
	{key: "email", messageID: "validation.email", causes: []error{users.ErrEmailInvalid}},
	{key: "email", messageID: "validation.email_taken", causes: []error{users.ErrUserAlreadyExists}},
	{key: "phone", messageID: "validation.phone", causes: []error{users.ErrPhoneInvalid}},
	{key: "role", messageID: "validation.role", causes: []error{users.ErrRoleIDInvalid}},
	{key: "role", messageID: "validation.role_missing", causes: []error{users.ErrRoleNotFound}},
	{key: "password", messageID: "validation.required", causes: []error{users.ErrPasswordEmpty}},
	{key: "password", messageID: "validation.password_short", count: users.MinPasswordLength, causes: []error{users.ErrPasswordTooShort}},
	{key: "password", messageID: "validation.too_long", causes: []error{users.ErrPasswordTooLong}},
	{key: "password", messageID: "validation.invalid", causes: []error{users.ErrPasswordInvalidChars}},
}

// Field errors map transport rules and domain validation errors that belong
// to one editable control to localized messages beside that control.
// Operational failures are not field errors.

func FieldErrors(ctx context.Context, err error) components.FieldErrors {
	return fieldErrors(ctx, err, roleFieldErrors)
}

func UserFormFieldErrors(ctx context.Context, err error) components.FieldErrors {
	return fieldErrors(ctx, err, userFieldErrors)
}

func fieldErrors(ctx context.Context, err error, mappings []fieldErrorMapping) components.FieldErrors {
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
		for _, mapping := range mappings {
			matches := slices.ContainsFunc(mapping.causes, func(cause error) bool { return errors.Is(err, cause) })

			if matches && fields == nil {
				fields = make(components.FieldErrors)
			}

			if matches && fields[mapping.key] == nil && mapping.count > 0 {
				fields[mapping.key] = []string{admini18n.Count(ctx, mapping.messageID, mapping.count)}
			} else if matches && fields[mapping.key] == nil {
				fields[mapping.key] = []string{admini18n.T(ctx, mapping.messageID)}
			}
		}
	}

	return fields
}
