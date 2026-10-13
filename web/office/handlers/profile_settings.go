package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Profile settings
//

// The profile settings belong to the signed-in User, whatever workspace they
// work in. They exist outside any workspace and inside each of the User's
// workspaces, where the workspace's dashboard stays around them; the changes
// are the same either way.

type profileSettingsScope struct {
	current   *security.Identity
	workspace *workspaces.Workspace
	url       string
}

// Profile settings page

func (h *Handler) profileSettings(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.profileSettingsScope(writer, request)

	if ok {
		notices, err := h.settingsNotices(request, scope.current.Session)

		if err != nil {
			slog.ErrorContext(request.Context(), "office flash storage", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.settings_unavailable")
		} else {
			content := utils.ProfileSettingsContentProps(scope.current, scope.url, notices)

			h.renderProfileSettings(writer, request, http.StatusOK, scope, content, "")
		}
	}
}

// Changing the email replaces the credentials, so this device signs in again
// with the new email and the confirmed password; every other device is signed
// out. A change of names alone keeps every Session.

func (h *Handler) updateProfile(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.profileSettingsScope(writer, request)

	if ok {
		var user *users.User

		form, err := utils.ParseProfileForm(writer, request)

		if err == nil {
			user, err = h.users.UpdateProfile(request.Context(), input.UpdateProfileInput{
				ID:              scope.current.User.ID,
				FirstName:       form.FirstName,
				LastName:        form.LastName,
				Email:           form.Email,
				CurrentPassword: form.CurrentPassword,
				UpdatedAt:       form.UpdatedAt,
			})
		}

		if err == nil {
			messages := []string{officei18n.T(request.Context(), "settings.profile_saved")}
			password := ""

			if user.Email != scope.current.User.Email {
				password = form.CurrentPassword
			}

			h.profileSaved(writer, request, scope, user, password, messages)
		} else {
			content := utils.ProfileSettingsContentProps(scope.current, scope.url, nil)
			content.Profile.FirstName, content.Profile.LastName, content.Profile.Email = form.FirstName, form.LastName, form.Email
			content.Profile.FieldErrors = utils.ProfileFieldErrors(request.Context(), err)

			if !form.UpdatedAt.IsZero() {
				content.Profile.UpdatedAt = form.UpdatedAt.Format(time.RFC3339Nano)
			}

			status, region := http.StatusUnprocessableEntity, pages.ProfileFormID
			confirm, confirming := utils.PasswordConfirmation(request.Context(), err, form.Confirmed)

			if confirming {
				content.Profile.Confirm, region = confirm, pages.ProfileConfirmID
			} else if content.Profile.FieldErrors == nil {
				status, content.Profile.Error = utils.SettingsFormError(request.Context(), err)
				h.logSettingsFailure(request, status, err)
			}

			h.renderProfileSettings(writer, request, status, scope, content, region)
		}
	}
}

// A new password replaces the credentials, so this device signs in again with
// it; every other device is signed out.

func (h *Handler) changePassword(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.profileSettingsScope(writer, request)

	if ok {
		var user *users.User

		form, err := utils.ParsePasswordForm(writer, request)

		if err == nil {
			user, err = h.users.ChangePassword(request.Context(), input.ChangePasswordInput{
				ID:              scope.current.User.ID,
				CurrentPassword: form.CurrentPassword,
				NewPassword:     form.NewPassword,
			})
		}

		if err == nil {
			messages := []string{officei18n.T(request.Context(), "settings.password_changed")}

			h.profileSaved(writer, request, scope, user, form.NewPassword, messages)
		} else {
			content := utils.ProfileSettingsContentProps(scope.current, scope.url, nil)
			content.Password.FieldErrors = utils.PasswordFieldErrors(request.Context(), err)
			status, region := http.StatusUnprocessableEntity, pages.PasswordFormID
			confirm, confirming := utils.PasswordConfirmation(request.Context(), err, form.Confirmed)

			if confirming {
				content.Password.Confirm, region = confirm, pages.PasswordConfirmID
			} else if content.Password.FieldErrors == nil {
				status, content.Password.Error = utils.SettingsFormError(request.Context(), err)
				h.logSettingsFailure(request, status, err)
			}

			h.renderProfileSettings(writer, request, status, scope, content, region)
		}
	}
}

//
// Helpers
//

// The settings inside a workspace are shown only to its members. The response
// is written when the scope cannot be resolved.

func (h *Handler) profileSettingsScope(
	writer http.ResponseWriter,
	request *http.Request,
) (profileSettingsScope, bool) {
	var scope profileSettingsScope

	current, err := h.authenticate(writer, request)

	if err == nil {
		scope.current, scope.url = current, utils.ProfileSettingsPath(request)

		if request.PathValue("workspace") != "" {
			var found app.MemberWorkspace

			found, err = h.findWorkspace(request, current)
			scope.workspace = found.Workspace
		}
	}

	if err != nil {
		h.settingsUnavailable(writer, request, err)
	} else if scope.workspace != nil {
		scope.url = utils.WorkspaceProfileSettingsPath(request, scope.workspace.ID)
	}

	return scope, err == nil
}

// When a password is given, the change replaced the credentials and ended
// every Session, so this device signs in again with it and the confirmation
// waits under the new Session. If signing in again fails, the change stays
// committed and the User signs in themselves.

func (h *Handler) profileSaved(
	writer http.ResponseWriter,
	request *http.Request,
	scope profileSettingsScope,
	user *users.User,
	password string,
	messages []string,
) {
	var (
		session = scope.current.Session
		err     error
	)

	if password != "" {
		session, err = h.renewSession(writer, request, scope.current.Session, user, password)
		messages = append(messages, officei18n.T(request.Context(), "settings.signed_out_elsewhere"))
	}

	if err != nil {
		slog.ErrorContext(request.Context(), "office sign-in after a credentials change", "error", err)
		http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
	} else {
		h.settingsSaved(writer, request, session, scope.url, messages)
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
			slog.WarnContext(request.Context(), "office ended session cleanup", "error", signOutErr)
		}
	} else {
		h.cookies.Clear(writer, utils.SessionCookie)
		session = nil
	}

	return session, err
}

// A failed in-page submission replaces only its form, or only the form's
// confirmation when the current password is missing or wrong, so the other
// fields keep what was typed; every other response is the whole page, inside
// the workspace when the settings are shown in one.

func (h *Handler) renderProfileSettings(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope profileSettingsScope,
	content pages.ProfileSettingsContentProps,
	region string,
) {
	title := officei18n.T(request.Context(), "nav.settings")

	if region != "" && utils.IsPartial(request) {
		utils.RenderFormRegion(writer, request, status, region, pages.SettingsFormFailure(profileSettingsRegion(region, content)))
	} else if scope.workspace != nil {
		dashboard := utils.DashboardProps(request, scope.current, scope.workspace, utils.ProfileSettingsSection, title)
		dashboard.Narrow = true

		utils.Render(writer, request, status, title, pages.WorkspaceProfileSettings(dashboard, content))
	} else {
		props := pages.ProfileSettingsProps{Layout: utils.ProfileSettingsLayoutProps(request, scope.current), Content: content}

		utils.Render(writer, request, status, title, pages.ProfileSettings(props))
	}
}

func profileSettingsRegion(region string, content pages.ProfileSettingsContentProps) templ.Component {
	var component templ.Component

	switch region {
	case pages.ProfileConfirmID:
		component = pages.ProfileConfirm(content.Profile)
	case pages.PasswordFormID:
		component = pages.PasswordForm(content.Password)
	case pages.PasswordConfirmID:
		component = pages.PasswordConfirm(content.Password)
	default:
		component = pages.ProfileForm(content.Profile)
	}

	return component
}
