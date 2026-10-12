package utils

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

//
// Profile form
//

// The names' and email's limits belong to the domain; the transport bounds
// the form's size and the email's syntax, and requires the moment of the
// latest change the form was built from.

const maxSettingsFormBytes = 32 << 10

type ProfileForm struct {
	FirstName       string    `form:"first_name" validate:"maxbytes=400"`
	LastName        string    `form:"last_name" validate:"maxbytes=400"`
	Email           string    `form:"email" validate:"required,mailbox,maxbytes=254"`
	CurrentPassword string    `form:"current_password" validate:"maxbytes=4096"`
	Confirmed       bool      `form:"confirm"`
	UpdatedAt       time.Time `form:"updated_at"`
}

func ParseProfileForm(writer http.ResponseWriter, request *http.Request) (ProfileForm, error) {
	var form ProfileForm

	err := parseForm(writer, request, maxSettingsFormBytes)

	if err == nil {
		err = checkFields(request.PostForm, []string{"first_name", "last_name", "email", "current_password", "confirm", "updated_at"}, nil)
	}

	if err == nil {
		form = ProfileForm{
			FirstName:       request.PostForm.Get("first_name"),
			LastName:        request.PostForm.Get("last_name"),
			Email:           request.PostForm.Get("email"),
			CurrentPassword: request.PostForm.Get("current_password"),
			Confirmed:       request.PostForm.Has("confirm"),
		}

		form.UpdatedAt, err = time.Parse(time.RFC3339Nano, request.PostForm.Get("updated_at"))

		if err != nil {
			err = errors.Join(ErrFormInvalid, err)
		}
	}

	if err == nil {
		err = requestvalidation.Validate(request.Context(), form)
	}

	return form, err
}

func ProfileFieldErrors(ctx context.Context, err error) components.FieldErrors {
	return fieldErrors(ctx, err, userFieldErrors)
}

//
// Password form
//

// The current password comes from the confirmation, which the server asks
// for when it is missing, so the transport does not require it.

type PasswordForm struct {
	CurrentPassword   string `form:"current_password" validate:"maxbytes=4096"`
	NewPassword       string `form:"new_password" validate:"required,maxbytes=4096"`
	NewPasswordRepeat string `form:"new_password_repeat" validate:"required,eqfield=NewPassword"`
	Confirmed         bool   `form:"confirm"`
}

func ParsePasswordForm(writer http.ResponseWriter, request *http.Request) (PasswordForm, error) {
	var form PasswordForm

	err := parseForm(writer, request, maxSettingsFormBytes)

	if err == nil {
		err = checkFields(request.PostForm, []string{"current_password", "new_password", "new_password_repeat", "confirm"}, nil)
	}

	if err == nil {
		form = PasswordForm{
			CurrentPassword:   request.PostForm.Get("current_password"),
			NewPassword:       request.PostForm.Get("new_password"),
			NewPasswordRepeat: request.PostForm.Get("new_password_repeat"),
			Confirmed:         request.PostForm.Has("confirm"),
		}

		err = requestvalidation.Validate(request.Context(), form)
	}

	return form, err
}

var passwordFieldErrors = []fieldErrorMapping{
	{key: "new_password", messageID: "validation.required", causes: []error{users.ErrPasswordEmpty}},
	{key: "new_password", messageID: "validation.password_short", count: users.MinPasswordLength, causes: []error{users.ErrPasswordTooShort}},
	{key: "new_password", messageID: "validation.too_long", causes: []error{users.ErrPasswordTooLong}},
	{key: "new_password", messageID: "validation.invalid", causes: []error{users.ErrPasswordInvalidChars}},
}

func PasswordFieldErrors(ctx context.Context, err error) components.FieldErrors {
	return fieldErrors(ctx, err, passwordFieldErrors)
}

//
// Password confirmation
//

// A missing or wrong current password reopens the confirmation instead of
// failing the form. A missing one is an error only when it was confirmed
// empty; otherwise the confirmation simply asks for it.

func PasswordConfirmation(ctx context.Context, err error, confirmed bool) (pages.PasswordConfirmProps, bool) {
	var props pages.PasswordConfirmProps

	if errors.Is(err, input.ErrCurrentPasswordRequired) {
		props.Open = true

		if confirmed {
			props.Errors = []string{admini18n.T(ctx, "validation.required")}
		}
	} else if errors.Is(err, input.ErrCurrentPasswordIncorrect) {
		props = pages.PasswordConfirmProps{Open: true, Errors: []string{admini18n.T(ctx, "validation.password_incorrect")}}
	}

	return props, props.Open
}

//
// Settings
//

// The profile form starts from the User's current details; a failed
// submission shows the submitted values instead. Passwords are never
// rendered back.

func SettingsProps(current *security.Identity, settingsURL string) (pages.ProfileFormProps, pages.PasswordFormProps) {
	user := current.User

	profile := pages.ProfileFormProps{
		Action:    settingsURL + "/profile",
		CancelURL: settingsURL,
		FirstName: string(user.FirstName),
		LastName:  string(user.LastName),
		Email:     string(user.Email),
		UpdatedAt: FormatUpdatedAt(user.UpdatedAt),
	}
	password := pages.PasswordFormProps{
		Action:    settingsURL + "/password",
		CancelURL: settingsURL,
	}

	return profile, password
}

// A profile that changed after the form was built cannot be saved over the
// newer change; any other failure without a field error is operational.

func SettingsError(err error) (int, string) {
	status, messageID := http.StatusServiceUnavailable, "errors.settings_unavailable"

	if errors.Is(err, ErrFormInvalid) {
		status, messageID = http.StatusBadRequest, "errors.settings_form"
	} else if errors.Is(err, users.ErrUserConflict) {
		status, messageID = http.StatusConflict, "errors.profile_conflict"
	}

	return status, messageID
}
