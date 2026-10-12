package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Settings
//

// The settings exist outside any workspace, without the sidebar, and as a
// section of each workspace. Showing them consumes the confirmations a saved
// change left for them.

type settingsScope struct {
	current   *security.Identity
	workspace *workspaces.Workspace
	url       string
}

// Settings page

func (h *Handler) settings(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.settingsScope(writer, request)

	if ok {
		bag, err := h.flashes.Take(request.Context(), scope.current.Session)

		if err != nil {
			slog.ErrorContext(request.Context(), "office flash storage", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.settings_unavailable")
		} else {
			content := utils.SettingsContentProps(request, scope.current, scope.url, bag.Get(flashmsg.Success))

			h.renderSettings(writer, request, http.StatusOK, scope, content, "")
		}
	}
}

// Changing the email replaces the credentials, so this device signs in again
// with the new email and the confirmed password; every other device is signed
// out. A change of names alone keeps every Session.

func (h *Handler) updateProfile(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.settingsScope(writer, request)

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

			h.settingsSaved(writer, request, scope, user, password, messages)
		} else {
			content := utils.SettingsContentProps(request, scope.current, scope.url, nil)
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

			h.renderSettings(writer, request, status, scope, content, region)
		}
	}
}

// A new password replaces the credentials, so this device signs in again with
// it; every other device is signed out.

func (h *Handler) changePassword(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.settingsScope(writer, request)

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

			h.settingsSaved(writer, request, scope, user, form.NewPassword, messages)
		} else {
			content := utils.SettingsContentProps(request, scope.current, scope.url, nil)
			content.Password.FieldErrors = utils.PasswordFieldErrors(request.Context(), err)
			status, region := http.StatusUnprocessableEntity, pages.PasswordFormID
			confirm, confirming := utils.PasswordConfirmation(request.Context(), err, form.Confirmed)

			if confirming {
				content.Password.Confirm, region = confirm, pages.PasswordConfirmID
			} else if content.Password.FieldErrors == nil {
				status, content.Password.Error = utils.SettingsFormError(request.Context(), err)
				h.logSettingsFailure(request, status, err)
			}

			h.renderSettings(writer, request, status, scope, content, region)
		}
	}
}

//
// Helpers
//

// A visitor who is not signed in is sent to the sign-in page, and a workspace
// that is missing or that the User does not belong to sends them back to
// choosing one. The response is written when the scope cannot be resolved.

func (h *Handler) settingsScope(
	writer http.ResponseWriter,
	request *http.Request,
) (settingsScope, bool) {
	var scope settingsScope

	current, err := h.authenticate(writer, request)

	if err == nil {
		scope.current, scope.url = current, utils.OfficePath(request)+"/settings"

		if request.PathValue("workspace") != "" {
			scope.workspace, err = h.findWorkspace(request, current)

			if err == nil {
				scope.url = utils.WorkspacePath(request, scope.workspace.ID) + "/settings"
			}
		}
	}

	if err == nil {
		// The scope is resolved.
	} else if utils.IsUnauthenticated(err) {
		http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
	} else if errors.Is(err, workspaces.ErrWorkspaceNotFound) {
		http.Redirect(writer, request, utils.WorkspacesPath(request), http.StatusSeeOther)
	} else {
		slog.ErrorContext(request.Context(), "office settings", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.settings_unavailable")
	}

	return scope, err == nil
}

// A saved change is confirmed on the settings page the redirect leads to.
// When a password is given, the change replaced the credentials and ended
// every Session, so this device signs in again with it and the confirmation
// waits under the new Session. If signing in again fails, the change stays
// committed and the User signs in themselves.

func (h *Handler) settingsSaved(
	writer http.ResponseWriter,
	request *http.Request,
	scope settingsScope,
	user *users.User,
	password string,
	messages []string,
) {
	var (
		session            = scope.current.Session
		renewErr, flashErr error
	)

	if password != "" {
		session, renewErr = h.renewSession(writer, request, scope.current.Session, user, password)
		messages = append(messages, officei18n.T(request.Context(), "settings.signed_out_elsewhere"))
	}

	for index := 0; renewErr == nil && flashErr == nil && index < len(messages); index++ {
		flashErr = h.flashes.Add(request.Context(), session, flashmsg.Success, messages[index])
	}

	if renewErr != nil {
		slog.ErrorContext(request.Context(), "office sign-in after a credentials change", "error", renewErr)
		http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
	} else {
		if flashErr != nil {
			// The change is committed; only its confirmation is lost.
			slog.ErrorContext(request.Context(), "office flash storage", "error", flashErr)
		}

		http.Redirect(writer, request, scope.url, http.StatusSeeOther)
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

// Failures shown at the top of a form are logged unless they come from the
// request itself.

func (h *Handler) logSettingsFailure(request *http.Request, status int, err error) {
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(request.Context(), "office settings", "error", err)
	}
}

// A failed in-page submission replaces only its form, or only the form's
// confirmation when the current password is missing or wrong, so the other
// fields keep what was typed; every other response is the whole page, inside
// the workspace when the settings belong to one.

func (h *Handler) renderSettings(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope settingsScope,
	content pages.SettingsContentProps,
	region string,
) {
	title := officei18n.T(request.Context(), "nav.settings")

	if region != "" && utils.IsPartial(request) {
		utils.RenderFormRegion(writer, request, status, region, pages.SettingsFormFailure(settingsRegion(region, content)))
	} else if scope.workspace != nil {
		dashboard := utils.DashboardProps(request, scope.current, scope.workspace, "settings", title)
		dashboard.Narrow = true

		utils.Render(writer, request, status, title, pages.WorkspaceSettings(dashboard, content))
	} else {
		props := pages.SettingsProps{Layout: utils.SettingsLayoutProps(request, scope.current), Content: content}

		utils.Render(writer, request, status, title, pages.Settings(props))
	}
}

func settingsRegion(region string, content pages.SettingsContentProps) templ.Component {
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
