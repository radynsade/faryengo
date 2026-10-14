package handlers

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
	"github.com/radynsade/faryengo/web/admin/utils"
)

//
// Settings
//

// Every signed-in user may change their own profile and password, whatever
// their Role grants.

// Settings page

func (h *Handler) settings(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionSettings, "", func(request *http.Request, current *security.Identity, panel layouts.PanelProps) {
		profile, password := utils.SettingsProps(current, settingsURL(panel))

		// A saved profile redirects here, so the sidebar shows the new name.
		panel.RefreshAccount = true

		h.renderSettings(writer, request, http.StatusOK, pages.SettingsProps{Panel: panel, Profile: profile, Password: password}, "")
	})
}

// Changing the email replaces the credentials, so this device signs in again
// with the new email and the confirmed password; every other device is signed
// out. A change of names alone keeps every Session.

func (h *Handler) updateProfile(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionSettings, "", func(request *http.Request, current *security.Identity, panel layouts.PanelProps) {
		var user *users.User

		form, err := utils.ParseProfileForm(writer, request)

		if err == nil {
			user, err = h.users.UpdateProfile(request.Context(), input.UpdateProfileInput{
				FirstName:       form.FirstName,
				LastName:        form.LastName,
				Email:           form.Email,
				CurrentPassword: form.CurrentPassword,
				UpdatedAt:       form.UpdatedAt,
			})
		}

		if err == nil {
			password := ""

			if user.Email != current.User.Email {
				password = form.CurrentPassword
			}

			h.settingsSaved(writer, request, current, panel, user, password, "settings.profile_saved")
		} else {
			props := pages.SettingsProps{Panel: panel}
			props.Profile, props.Password = utils.SettingsProps(current, settingsURL(panel))
			props.Profile.FirstName, props.Profile.LastName, props.Profile.Email = form.FirstName, form.LastName, form.Email
			props.Profile.FieldErrors = utils.ProfileFieldErrors(request.Context(), err)

			if !form.UpdatedAt.IsZero() {
				props.Profile.UpdatedAt = utils.FormatUpdatedAt(form.UpdatedAt)
			}

			status, region, responded := http.StatusUnprocessableEntity, pages.ProfileFormID, false
			confirm, confirming := utils.PasswordConfirmation(request.Context(), err, form.Confirmed)

			if confirming {
				props.Profile.Confirm, region = confirm, pages.ProfileConfirmID
			} else if len(props.Profile.FieldErrors) == 0 {
				status, props.Profile.Errors, responded = h.settingsFailure(writer, request, err)
			}

			if !responded {
				h.renderSettings(writer, request, status, props, region)
			}
		}
	})
}

// A new password replaces the credentials, so this device signs in again with
// it; every other device is signed out.

func (h *Handler) changePassword(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionSettings, "", func(request *http.Request, current *security.Identity, panel layouts.PanelProps) {
		var user *users.User

		form, err := utils.ParsePasswordForm(writer, request)

		if err == nil {
			user, err = h.users.ChangePassword(request.Context(), input.ChangePasswordInput{
				CurrentPassword: form.CurrentPassword,
				NewPassword:     form.NewPassword,
			})
		}

		if err == nil {
			h.settingsSaved(writer, request, current, panel, user, form.NewPassword, "settings.password_changed")
		} else {
			props := pages.SettingsProps{Panel: panel}
			props.Profile, props.Password = utils.SettingsProps(current, settingsURL(panel))
			props.Password.FieldErrors = utils.PasswordFieldErrors(request.Context(), err)
			status, region, responded := http.StatusUnprocessableEntity, pages.PasswordFormID, false
			confirm, confirming := utils.PasswordConfirmation(request.Context(), err, form.Confirmed)

			if confirming {
				props.Password.Confirm, region = confirm, pages.PasswordConfirmID
			} else if len(props.Password.FieldErrors) == 0 {
				status, props.Password.Errors, responded = h.settingsFailure(writer, request, err)
			}

			if !responded {
				h.renderSettings(writer, request, status, props, region)
			}
		}
	})
}

//
// Helpers
//

func settingsURL(panel layouts.PanelProps) string {
	return panel.BasePath + "/settings"
}

// A saved change is confirmed in the success dialog of the settings page the
// redirect leads to. When a password is given, the change replaced the
// credentials and ended every Session, so this device signs in again with it
// and the confirmation waits under the new Session. If signing in again
// fails, the change stays committed and the user signs in themselves.

func (h *Handler) settingsSaved(
	writer http.ResponseWriter,
	request *http.Request,
	current *security.Identity,
	panel layouts.PanelProps,
	user *users.User,
	password string,
	messageID string,
) {
	var renewErr, flashErr error

	messages := []string{admini18n.T(request.Context(), messageID)}

	if password != "" {
		var session *security.Session

		session, renewErr = h.renewSession(writer, request, current.Session, user, password)
		messages = append(messages, admini18n.T(request.Context(), "settings.signed_out_elsewhere"))

		if renewErr == nil {
			utils.StateOf(request).Identity = &security.Identity{Session: session, User: user, Role: current.Role}
		}
	}

	for index := 0; renewErr == nil && flashErr == nil && index < len(messages); index++ {
		flashErr = h.flashes.Add(writer, request, flashmsg.Success, messages[index])
	}

	if renewErr != nil {
		slog.ErrorContext(request.Context(), "admin sign-in after a credentials change", "error", renewErr)
		http.Redirect(writer, request, utils.AdminPath(request)+"/sign-in", http.StatusSeeOther)
	} else if flashErr != nil {
		// The change is committed; only its confirmation is lost.
		utils.FlashUnavailable(writer, request, flashErr)
	} else {
		http.Redirect(writer, request, settingsURL(panel), http.StatusSeeOther)
	}
}

// The ended Session's record is removed once the new one is issued; it can no
// longer authenticate either way, so a failure to remove it is only logged.

func (h *Handler) renewSession(
	writer http.ResponseWriter,
	request *http.Request,
	ended *security.Session,
	user *users.User,
	password string,
) (*security.Session, error) {
	var id sessionid.ID

	session, err := h.passwords.SignIn(request.Context(), emailpass.Credentials{Email: user.Email, Password: password})

	if err == nil {
		id, err = h.sessions.Issue(request.Context(), session)
	}

	if err == nil {
		h.cookies.Set(writer, utils.SessionCookie, string(id), session.ExpiresAt)

		if signOutErr := h.sessions.SignOut(request.Context(), ended, false); signOutErr != nil {
			slog.WarnContext(request.Context(), "admin ended session cleanup", "error", signOutErr)
		}
	} else {
		h.cookies.Clear(writer, utils.SessionCookie)
		session = nil
	}

	return session, err
}

// A failure without a field error is shown above its form, through an error
// flash consumed in the same response. Without flash storage the response
// reports that instead, and the caller renders nothing.

func (h *Handler) settingsFailure(
	writer http.ResponseWriter,
	request *http.Request,
	err error,
) (int, []string, bool) {
	status, messageID := utils.SettingsError(err)

	if status >= http.StatusInternalServerError {
		slog.ErrorContext(request.Context(), "admin settings", "error", err)
	}

	messages, flashErr := h.flashes.ShowError(writer, request, admini18n.T(request.Context(), messageID))

	if flashErr != nil {
		utils.FlashUnavailable(writer, request, flashErr)
	}

	return status, messages, flashErr != nil
}

// A failed submission replaces only its form, or only the form's
// confirmation when the current password is missing or wrong, so the other
// fields keep what was typed; every other response is the whole page.

func (h *Handler) renderSettings(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	props pages.SettingsProps,
	region string,
) {
	if region != "" {
		utils.RenderFormRegion(writer, request, status, region, settingsRegion(region, props))
	} else {
		h.render(writer, request, status, props.Panel.Title, pages.Settings(props))
	}
}

func settingsRegion(region string, props pages.SettingsProps) templ.Component {
	var component templ.Component

	switch region {
	case pages.ProfileConfirmID:
		component = pages.ProfileConfirm(props.Profile)
	case pages.PasswordFormID:
		component = pages.PasswordForm(props.Password)
	case pages.PasswordConfirmID:
		component = pages.PasswordConfirm(props.Password)
	default:
		component = pages.ProfileForm(props.Profile)
	}

	return component
}
