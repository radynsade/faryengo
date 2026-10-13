package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Workspace settings
//

// A workspace's settings belong to the workspace in the address, and only its
// members reach them.

type workspaceSettingsScope struct {
	current   *security.Identity
	workspace *workspaces.Workspace
	url       string
}

// Workspace settings page

func (h *Handler) workspaceSettings(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.workspaceSettingsScope(writer, request)

	if ok {
		notices, err := h.settingsNotices(request, scope.current.Session)

		if err != nil {
			slog.ErrorContext(request.Context(), "office flash storage", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.settings_unavailable")
		} else {
			content := utils.WorkspaceSettingsContentProps(scope.workspace, scope.url, notices)

			h.renderWorkspaceSettings(writer, request, http.StatusOK, scope, content, "")
		}
	}
}

// A workspace's name is the same for every member, so any member may change
// it; the scope has already checked the membership.

func (h *Handler) renameWorkspace(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.workspaceSettingsScope(writer, request)

	if ok {
		form, err := utils.ParseWorkspaceForm(writer, request)

		if err == nil {
			_, err = h.workspaces.Rename(request.Context(), input.RenameWorkspaceInput{ID: scope.workspace.ID, Name: form.Name})
		}

		if err == nil {
			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{officei18n.T(request.Context(), "settings.workspace_saved")})
		} else if errors.Is(err, workspaces.ErrWorkspaceNotFound) {
			http.Redirect(writer, request, utils.WorkspacesPath(request), http.StatusSeeOther)
		} else {
			content := utils.WorkspaceSettingsContentProps(scope.workspace, scope.url, nil)
			content.Workspace.Name = form.Name
			content.Workspace.FieldErrors = utils.WorkspaceFieldErrors(request.Context(), err)
			status := http.StatusUnprocessableEntity

			if content.Workspace.FieldErrors == nil {
				status, content.Workspace.Error = utils.SettingsFormError(request.Context(), err)
				h.logSettingsFailure(request, status, err)
			}

			h.renderWorkspaceSettings(writer, request, status, scope, content, pages.WorkspaceFormID)
		}
	}
}

// Any member may add another existing User by email; the scope has already
// checked the membership. The confirmation names the added User.

func (h *Handler) addWorkspaceMember(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.workspaceSettingsScope(writer, request)

	if ok {
		var member *users.User

		form, err := utils.ParseMemberForm(writer, request)

		if err == nil {
			member, err = h.workspaces.AddMemberByEmail(request.Context(), input.AddMemberByEmailInput{
				WorkspaceID: scope.workspace.ID,
				Email:       form.Email,
			})
		}

		if err == nil {
			message := officei18n.T(request.Context(), "settings.member_added", map[string]any{
				"Name":  utils.FullName(member),
				"Email": string(member.Email),
			})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if errors.Is(err, workspaces.ErrWorkspaceNotFound) {
			http.Redirect(writer, request, utils.WorkspacesPath(request), http.StatusSeeOther)
		} else {
			content := utils.WorkspaceSettingsContentProps(scope.workspace, scope.url, nil)
			content.Member.Email = form.Email
			content.Member.FieldErrors = utils.MemberFieldErrors(request.Context(), err)
			status := http.StatusUnprocessableEntity

			if content.Member.FieldErrors == nil {
				status, content.Member.Error = utils.SettingsFormError(request.Context(), err)
				h.logSettingsFailure(request, status, err)
			}

			h.renderWorkspaceSettings(writer, request, status, scope, content, pages.MemberFormID)
		}
	}
}

// Any member may remove another one, who can be added again by email. The
// signed-in User does not remove themselves from here, so such a request, and
// one for a User who is no longer a member, changes nothing and shows the
// current members.

func (h *Handler) removeWorkspaceMember(writer http.ResponseWriter, request *http.Request) {
	scope, ok := h.workspaceSettingsScope(writer, request)

	if ok {
		var member *users.User

		id, err := utils.ParseRemoveMemberForm(writer, request)

		if err == nil && id != scope.current.User.ID {
			member, err = h.users.FindByID(request.Context(), id)

			if err == nil {
				err = h.workspaces.RemoveMember(request.Context(), scope.workspace.ID, id)
			}
		}

		if err == nil && member != nil {
			message := officei18n.T(request.Context(), "settings.member_removed", map[string]any{"Name": utils.FullName(member)})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if err == nil || errors.Is(err, users.ErrUserNotFound) || errors.Is(err, workspaces.ErrMembershipNotFound) {
			http.Redirect(writer, request, scope.url, http.StatusSeeOther)
		} else if errors.Is(err, utils.ErrFormInvalid) {
			utils.Fail(writer, request, http.StatusBadRequest, "errors.form")
		} else {
			slog.ErrorContext(request.Context(), "office workspace member removal", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.settings_unavailable")
		}
	}
}

//
// Helpers
//

// The response is written when the scope cannot be resolved.

func (h *Handler) workspaceSettingsScope(
	writer http.ResponseWriter,
	request *http.Request,
) (workspaceSettingsScope, bool) {
	var scope workspaceSettingsScope

	current, err := h.authenticate(writer, request)

	if err == nil {
		scope.current = current
		scope.workspace, err = h.findWorkspace(request, current)
	}

	if err == nil {
		scope.url = utils.WorkspacePath(request, scope.workspace.ID) + "/settings"
	} else {
		h.settingsUnavailable(writer, request, err)
	}

	return scope, err == nil
}

// A failed in-page submission replaces only its form, so the other fields
// keep what was typed; every other response is the whole page, inside the
// workspace with its members. A member list that cannot be loaded shows its
// error instead.

func (h *Handler) renderWorkspaceSettings(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope workspaceSettingsScope,
	content pages.WorkspaceSettingsContentProps,
	region string,
) {
	if region != "" && utils.IsPartial(request) {
		utils.RenderFormRegion(writer, request, status, region, pages.SettingsFormFailure(workspaceSettingsRegion(region, content)))
	} else {
		title := officei18n.T(request.Context(), "settings.workspace_title")
		members, err := h.workspaces.ListMembers(request.Context(), scope.workspace.ID)

		if err != nil {
			slog.ErrorContext(request.Context(), "office workspace members", "error", err)
			content.MembersError = officei18n.T(request.Context(), "errors.members_unavailable")
		} else {
			content.Members = utils.MemberItems(members, scope.current, scope.url)
		}

		dashboard := utils.DashboardProps(request, scope.current, scope.workspace, utils.WorkspaceSettingsSection, title)
		dashboard.Narrow = true

		utils.Render(writer, request, status, title, pages.WorkspaceSettings(dashboard, content))
	}
}

func workspaceSettingsRegion(region string, content pages.WorkspaceSettingsContentProps) templ.Component {
	var component templ.Component

	if region == pages.MemberFormID {
		component = pages.MemberForm(content.Member)
	} else {
		component = pages.WorkspaceForm(content.Workspace)
	}

	return component
}
