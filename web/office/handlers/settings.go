package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Settings
//

// The profile settings belong to the User and are shown outside any
// workspace or inside one; a workspace's settings are a section of that
// workspace. Both confirm a saved change on the page the redirect leads to.

func (h *Handler) settingsSaved(
	writer http.ResponseWriter,
	request *http.Request,
	session *security.Session,
	settingsURL string,
	messages []string,
) {
	var err error

	for index := 0; err == nil && index < len(messages); index++ {
		err = h.flashes.Add(request.Context(), session, flashmsg.Success, messages[index])
	}

	if err != nil {
		// The change is committed; only its confirmation is lost.
		slog.ErrorContext(request.Context(), "office flash storage", "error", err)
	}

	http.Redirect(writer, request, settingsURL, http.StatusSeeOther)
}

// A visitor who is not signed in is sent to the sign-in page. A change the
// member's workspace role does not allow is forbidden. Settings inside a
// workspace that is missing or that the User does not belong to are not
// found; a form posted to them, such as from a page left open after the User
// was removed, leads back to choosing a workspace.

func (h *Handler) settingsUnavailable(writer http.ResponseWriter, request *http.Request, err error) {
	if utils.IsUnauthenticated(err) {
		http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
	} else if errors.Is(err, workspaces.ErrRolePermissionDenied) {
		utils.Fail(writer, request, http.StatusForbidden, "errors.permission_denied")
	} else if errors.Is(err, workspaces.ErrWorkspaceNotFound) && request.Method == http.MethodGet {
		utils.Fail(writer, request, http.StatusNotFound, "errors.not_found")
	} else if errors.Is(err, workspaces.ErrWorkspaceNotFound) {
		http.Redirect(writer, request, utils.WorkspacesPath(request), http.StatusSeeOther)
	} else {
		slog.ErrorContext(request.Context(), "office settings", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.settings_unavailable")
	}
}

// Showing either page consumes the confirmations a saved change left for it.

func (h *Handler) settingsNotices(request *http.Request, session *security.Session) ([]string, error) {
	var notices []string

	bag, err := h.flashes.Take(request.Context(), session)

	if err == nil {
		notices = bag.Get(flashmsg.Success)
	}

	return notices, err
}

// Failures shown at the top of a form are logged unless they come from the
// request itself.

func (h *Handler) logSettingsFailure(request *http.Request, status int, err error) {
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(request.Context(), "office settings", "error", err)
	}
}
