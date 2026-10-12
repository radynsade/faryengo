package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/workspaces"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Dashboard
//

// Every signed-in visit starts by choosing a workspace.

func (h *Handler) root(writer http.ResponseWriter, request *http.Request) {
	http.Redirect(writer, request, utils.WorkspacesPath(request), http.StatusSeeOther)
}

// Sections are empty for now; each shows the workspace's dashboard with its
// own menu item active. A visitor who is not signed in is sent to the
// sign-in page, and a workspace that is missing or that the User does not
// belong to sends them back to choosing one.

func (h *Handler) section(key string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var workspace *workspaces.Workspace

		current, err := h.authenticate(writer, request)

		if err == nil {
			workspace, err = h.findWorkspace(request, current)
		}

		if err == nil {
			title := officei18n.T(request.Context(), "nav."+key)
			props := utils.DashboardProps(request, current, workspace, key, title)

			utils.Render(writer, request, http.StatusOK, title, pages.Section(props))
		} else if utils.IsUnauthenticated(err) {
			http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
		} else if errors.Is(err, workspaces.ErrWorkspaceNotFound) {
			http.Redirect(writer, request, utils.WorkspacesPath(request), http.StatusSeeOther)
		} else {
			slog.ErrorContext(request.Context(), "office workspace", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.workspaces_unavailable")
		}
	}
}

//
// Helpers
//

// A workspace ID that is not a UUID, or is nil, names no workspace, so it is
// reported like a missing one.

func (h *Handler) findWorkspace(
	request *http.Request,
	current *security.Identity,
) (*workspaces.Workspace, error) {
	var workspace *workspaces.Workspace

	id, err := uuid.Parse(request.PathValue("workspace"))

	if err != nil || id == uuid.Nil {
		err = fmt.Errorf("parse workspace ID %q: %w", request.PathValue("workspace"), workspaces.ErrWorkspaceNotFound)
	} else {
		workspace, err = h.workspaces.FindForMember(request.Context(), workspaces.WorkspaceID(id), current.User.ID)
	}

	return workspace, err
}
