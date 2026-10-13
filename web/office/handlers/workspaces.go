package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"

	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/domquery"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Workspaces
//

// The selection lists one page of the User's workspaces in name order; a
// User with more than maxListedWorkspaces sees the first ones only. When the
// list cannot be loaded, the page still renders, with the error in place of
// the list, so the User keeps the page's controls and can try again.

const maxListedWorkspaces = 100

// Select a workspace

func (h *Handler) selectWorkspace(writer http.ResponseWriter, request *http.Request) {
	current, err := h.authenticate(writer, request)

	if err == nil {
		query := workspaces.WorkspaceQuery{
			SortOrder: domquery.SortOrderAsc,
			SortBy:    workspaces.WorkspaceSortName,
			Limit:     maxListedWorkspaces,
			Page:      1,
		}

		status := http.StatusOK
		title := officei18n.T(request.Context(), "workspaces.select_title")
		result, listErr := h.workspaces.ListByMember(request.Context(), current.User.ID, query)
		props := utils.SelectWorkspaceProps(request, current, result.Workspaces)

		if listErr != nil {
			slog.ErrorContext(request.Context(), "office workspace selection", "error", listErr)
			status = http.StatusServiceUnavailable
			props.Error = officei18n.T(request.Context(), "errors.workspaces_unavailable")
		}

		utils.Render(writer, request, status, title, pages.SelectWorkspace(props))
	} else if utils.IsUnauthenticated(err) {
		http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
	} else {
		slog.ErrorContext(request.Context(), "office authentication", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.sign_in_unavailable")
	}
}

// Creating a workspace page

func (h *Handler) newWorkspacePage(writer http.ResponseWriter, request *http.Request) {
	current, err := h.authenticate(writer, request)

	if err == nil {
		h.renderCreateWorkspace(writer, request, http.StatusOK, utils.CreateWorkspaceProps(request, current, ""))
	} else if utils.IsUnauthenticated(err) {
		http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
	} else {
		slog.ErrorContext(request.Context(), "office authentication", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.sign_in_unavailable")
	}
}

// The creator becomes the workspace's first member, and a successful creation
// opens the new workspace's overview.

func (h *Handler) createWorkspace(writer http.ResponseWriter, request *http.Request) {
	current, err := h.authenticate(writer, request)

	if err == nil {
		h.submitWorkspace(writer, request, current)
	} else if utils.IsUnauthenticated(err) {
		http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
	} else {
		slog.ErrorContext(request.Context(), "office authentication", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.sign_in_unavailable")
	}
}

//
// Helpers
//

func (h *Handler) submitWorkspace(
	writer http.ResponseWriter,
	request *http.Request,
	current *security.Identity,
) {
	var workspace *workspaces.Workspace

	form, err := utils.ParseWorkspaceForm(writer, request)

	if err == nil {
		workspace, err = h.workspaces.CreateAsUser(request.Context(), current.User.ID, input.CreateWorkspaceInput{Name: form.Name})
	}

	props := utils.CreateWorkspaceProps(request, current, form.Name)

	if err == nil {
		http.Redirect(writer, request, utils.WorkspacePath(request, workspace.ID), http.StatusSeeOther)
	} else if errors.Is(err, utils.ErrFormInvalid) {
		props.Error = officei18n.T(request.Context(), "errors.form")
		h.renderCreateWorkspace(writer, request, http.StatusBadRequest, props)
	} else if errors.Is(err, input.ErrCreateWorkspaceInputInvalid) {
		props.FieldErrors = utils.WorkspaceFieldErrors(request.Context(), err)
		h.renderCreateWorkspace(writer, request, http.StatusUnprocessableEntity, props)
	} else {
		slog.ErrorContext(request.Context(), "office workspace creation", "error", err)
		props.Error = officei18n.T(request.Context(), "errors.workspace_create_unavailable")
		h.renderCreateWorkspace(writer, request, http.StatusServiceUnavailable, props)
	}
}

// A failed submission replaces only the form; every other response is the
// whole page.

func (h *Handler) renderCreateWorkspace(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	props pages.CreateWorkspaceProps,
) {
	if request.Method == http.MethodPost {
		utils.RenderFormRegion(writer, request, status, pages.CreateWorkspaceFormID, pages.CreateWorkspaceForm(props))
	} else {
		utils.Render(writer, request, status, officei18n.T(request.Context(), "workspaces.create"), pages.CreateWorkspace(props))
	}
}
