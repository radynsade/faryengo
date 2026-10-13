package utils

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
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
	Email           string    `form:"email" validate:"required,office_mailbox,maxbytes=254"`
	CurrentPassword string    `form:"current_password" validate:"maxbytes=4096"`
	Confirmed       bool      `form:"confirm"`
	UpdatedAt       time.Time `form:"updated_at"`
}

func ParseProfileForm(writer http.ResponseWriter, request *http.Request) (ProfileForm, error) {
	var form ProfileForm

	err := parseForm(writer, request, maxSettingsFormBytes, "first_name", "last_name", "email", "current_password", "confirm", "updated_at")

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

// ProfileFieldErrors maps a rejected profile to translated messages keyed by
// form field name; a failure that belongs to no field yields none.

func ProfileFieldErrors(ctx context.Context, err error) map[string]string {
	fields := make(map[string]string)

	addTransportErrors(ctx, fields, err)

	for _, check := range []struct {
		field string
		err   error
		empty error
		long  error
		limit int
	}{
		{"first_name", users.ErrFirstNameInvalid, users.ErrFirstNameEmpty, users.ErrFirstNameTooLong, users.MaxFirstNameLength},
		{"last_name", users.ErrLastNameInvalid, users.ErrLastNameEmpty, users.ErrLastNameTooLong, users.MaxLastNameLength},
	} {
		if _, found := fields[check.field]; !found && errors.Is(err, check.err) {
			switch {
			case errors.Is(err, check.empty):
				fields[check.field] = officei18n.T(ctx, "validation.required")
			case errors.Is(err, check.long):
				fields[check.field] = officei18n.Count(ctx, "validation.too_long_chars", check.limit)
			default:
				fields[check.field] = officei18n.T(ctx, "validation.invalid")
			}
		}
	}

	if _, found := fields["email"]; !found {
		if errors.Is(err, users.ErrEmailInvalid) {
			fields["email"] = officei18n.T(ctx, "validation.email")
		} else if errors.Is(err, users.ErrUserAlreadyExists) {
			fields["email"] = officei18n.T(ctx, "validation.email_taken")
		}
	}

	return nilIfEmpty(fields)
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

	err := parseForm(writer, request, maxSettingsFormBytes, "current_password", "new_password", "new_password_repeat", "confirm")

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

// PasswordFieldErrors maps a rejected password change to translated messages
// keyed by form field name.

func PasswordFieldErrors(ctx context.Context, err error) map[string]string {
	fields := make(map[string]string)

	addTransportErrors(ctx, fields, err)

	if _, found := fields["new_password"]; !found && errors.Is(err, users.ErrPasswordInvalid) {
		switch {
		case errors.Is(err, users.ErrPasswordEmpty):
			fields["new_password"] = officei18n.T(ctx, "validation.required")
		case errors.Is(err, users.ErrPasswordTooShort):
			fields["new_password"] = officei18n.Count(ctx, "validation.too_short_chars", users.MinPasswordLength)
		case errors.Is(err, users.ErrPasswordTooLong):
			fields["new_password"] = officei18n.T(ctx, "validation.too_long")
		default:
			fields["new_password"] = officei18n.T(ctx, "validation.invalid")
		}
	}

	return nilIfEmpty(fields)
}

//
// Member form
//

type MemberForm struct {
	Email string `form:"email" validate:"required,office_mailbox,maxbytes=254"`
}

func ParseMemberForm(writer http.ResponseWriter, request *http.Request) (MemberForm, error) {
	var form MemberForm

	err := parseForm(writer, request, maxSettingsFormBytes, "email")

	if err == nil {
		form = MemberForm{Email: request.PostForm.Get("email")}
		err = requestvalidation.Validate(request.Context(), form)
	}

	return form, err
}

// An email that names no User, or a User who already belongs to the
// workspace, is a problem with the email field.

func MemberFieldErrors(ctx context.Context, err error) map[string]string {
	fields := make(map[string]string)

	addTransportErrors(ctx, fields, err)

	if _, found := fields["email"]; !found {
		switch {
		case errors.Is(err, users.ErrEmailInvalid):
			fields["email"] = officei18n.T(ctx, "validation.email")
		case errors.Is(err, users.ErrUserNotFound):
			fields["email"] = officei18n.T(ctx, "validation.member_unknown")
		case errors.Is(err, workspaces.ErrMembershipAlreadyExists):
			fields["email"] = officei18n.T(ctx, "validation.member_exists")
		}
	}

	return nilIfEmpty(fields)
}

//
// Members
//

func MemberItems(
	members []*users.User,
	current *security.Identity,
	settingsURL string,
) []pages.MemberItem {
	items := make([]pages.MemberItem, 0, len(members))

	for _, member := range members {
		item := pages.MemberItem{
			ID:        uuid.UUID(member.ID).String(),
			Name:      FullName(member),
			Email:     string(member.Email),
			Initials:  initial(string(member.FirstName)) + initial(string(member.LastName)),
			IsCurrent: member.ID == current.User.ID,
		}

		if !item.IsCurrent {
			item.RemoveURL = settingsURL + "/members/" + item.ID + "/remove"
		}

		items = append(items, item)
	}

	return items
}

func FullName(user *users.User) string {
	return strings.TrimSpace(string(user.FirstName) + " " + string(user.LastName))
}

// Removal carries the member in its address, so its form has no fields.

func ParseRemoveMemberForm(writer http.ResponseWriter, request *http.Request) (users.UserID, error) {
	var id users.UserID

	err := parseForm(writer, request, maxSettingsFormBytes)

	if err == nil {
		parsed, parseErr := uuid.Parse(request.PathValue("user"))

		if parseErr == nil {
			id, parseErr = users.NewUserID(parsed)
		}

		if parseErr != nil {
			id = users.UserID{}
			err = errors.Join(ErrFormInvalid, users.ErrUserIDInvalid)
		}
	}

	return id, err
}

//
// Password confirmation
//

// A missing or wrong current password reopens the confirmation instead of
// failing the form. A missing one is an error only when it was confirmed
// empty; otherwise the confirmation simply asks for it.

func PasswordConfirmation(ctx context.Context, err error, confirmed bool) (pages.ConfirmProps, bool) {
	var props pages.ConfirmProps

	if errors.Is(err, input.ErrCurrentPasswordRequired) {
		props.Open = true

		if confirmed {
			props.Error = officei18n.T(ctx, "validation.required")
		}
	} else if errors.Is(err, input.ErrCurrentPasswordIncorrect) {
		props = pages.ConfirmProps{Open: true, Error: officei18n.T(ctx, "validation.password_incorrect")}
	}

	return props, props.Open
}

//
// Settings
//

// The profile form starts from the User's current details; a failed
// submission shows the submitted values instead. Passwords are never
// rendered back. The forms post to the address the page was shown at.

func ProfileSettingsContentProps(
	current *security.Identity,
	settingsURL string,
	notices []string,
) pages.ProfileSettingsContentProps {
	user := current.User

	return pages.ProfileSettingsContentProps{
		Notices: notices,
		Profile: pages.ProfileFormProps{
			Action:    settingsURL + "/profile",
			CancelURL: settingsURL,
			FirstName: string(user.FirstName),
			LastName:  string(user.LastName),
			Email:     string(user.Email),
			UpdatedAt: user.UpdatedAt.Format(time.RFC3339Nano),
		},
		Password: pages.PasswordFormProps{
			Action:    settingsURL + "/password",
			CancelURL: settingsURL,
		},
	}
}

// The workspace form starts from the workspace's current name. The members
// are loaded only for a whole page.

func WorkspaceSettingsContentProps(
	workspace *workspaces.Workspace,
	settingsURL string,
	notices []string,
) pages.WorkspaceSettingsContentProps {
	return pages.WorkspaceSettingsContentProps{
		Notices:   notices,
		Workspace: pages.WorkspaceFormProps{Action: settingsURL + "/workspace", Name: string(workspace.Name)},
		Member:    pages.MemberFormProps{Action: settingsURL + "/members"},
	}
}

// A profile that changed after the form was built cannot be saved over the
// newer change; any other failure without a field error is operational.

func SettingsFormError(ctx context.Context, err error) (int, string) {
	status, messageID := http.StatusServiceUnavailable, "errors.settings_unavailable"

	if errors.Is(err, ErrFormInvalid) {
		status, messageID = http.StatusBadRequest, "errors.form"
	} else if errors.Is(err, users.ErrUserConflict) {
		status, messageID = http.StatusConflict, "errors.profile_conflict"
	}

	return status, officei18n.T(ctx, messageID)
}

//
// Helpers
//

func addTransportErrors(ctx context.Context, fields map[string]string, err error) {
	var failures *requestvalidation.Errors

	if errors.As(err, &failures) {
		for _, field := range failures.Fields {
			messageID := "validation.invalid"

			switch field.Rule {
			case "required":
				messageID = "validation.required"
			case "office_mailbox":
				messageID = "validation.email"
			case "maxbytes":
				messageID = "validation.too_long"
			case "eqfield":
				messageID = "validation.passwords_differ"
			}

			if _, found := fields[field.Field]; !found {
				fields[field.Field] = officei18n.T(ctx, messageID)
			}
		}
	}
}

func nilIfEmpty(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		fields = nil
	}

	return fields
}
