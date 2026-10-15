package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/forms"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Workspace settings
//

// A workspace's settings belong to the workspace in the address, and only its
// members reach them. The member's role decides what they may change there.

type workspaceSettingsScope struct {
	current *security.Identity
	found   app.MemberWorkspace
	url     string
}

// Every member sees the settings; only those whose role allows a change see
// its form.

func (h *Handler) workspaceSettings(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.workspaceSettingsScope(writer, request, workspaces.ActionViewContent)

	if ok {
		notices, err := h.settingsNotices(request, scope.current.Session)

		if err != nil {
			slog.ErrorContext(request.Context(), "office flash storage", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.settings_unavailable")
		} else {
			content := utils.WorkspaceSettingsContentProps(request.Context(), scope.found, scope.url, notices)

			h.renderWorkspaceSettings(writer, request, http.StatusOK, scope, content, "")
		}
	}
}

// A workspace's name is the same for every member, so only an owner may
// change it.

func (h *Handler) renameWorkspace(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.workspaceSettingsScope(writer, request, workspaces.ActionRename)

	if ok {
		form, err := utils.ParseWorkspaceForm(writer, request)

		if err == nil {
			_, err = h.workspaces.Rename(
				request.Context(),
				input.RenameWorkspaceInput{ID: scope.found.Workspace.ID, Name: form.Name},
			)
		}

		if err == nil {
			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{officei18n.T(request.Context(), "settings.workspace_saved")})
		} else if isWorkspaceAccessError(err) {
			h.settingsUnavailable(writer, request, err)
		} else {
			content := utils.WorkspaceSettingsContentProps(request.Context(), scope.found, scope.url, nil)
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

// An owner may add another existing User by email, with the role they get in
// the workspace. The confirmation names the added User.

func (h *Handler) addWorkspaceMember(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.workspaceSettingsScope(writer, request, workspaces.ActionManageMembers)

	if ok {
		var member *users.User

		form, err := utils.ParseMemberForm(writer, request)

		if err == nil {
			member, err = h.workspaces.AddMemberByEmail(request.Context(), input.AddMemberByEmailInput{
				WorkspaceID: scope.found.Workspace.ID,
				Email:       form.Email,
				Role:        form.Role,
			})
		}

		if err == nil {
			message := officei18n.T(request.Context(), "settings.member_added", map[string]any{
				"Name":  utils.FullName(member),
				"Email": string(member.Email),
			})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if isWorkspaceAccessError(err) {
			h.settingsUnavailable(writer, request, err)
		} else {
			content := utils.WorkspaceSettingsContentProps(request.Context(), scope.found, scope.url, nil)
			content.Member.Email = form.Email
			content.Member.Role = form.Role
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

// An owner may change any other member's role, as long as the workspace keeps
// an owner. The signed-in User does not change their own role, so such a
// request, and one for a User who is no longer a member, changes nothing and
// shows the current members.

func (h *Handler) changeWorkspaceMemberRole(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.workspaceSettingsScope(writer, request, workspaces.ActionManageMembers)

	if ok {
		var member *users.User

		form, err := utils.ParseRoleForm(writer, request)

		if err == nil {
			member, err = h.findMember(request, scope, form.UserID)
		}

		if err == nil {
			err = h.workspaces.ChangeMemberRole(request.Context(), input.ChangeMemberRoleInput{
				WorkspaceID: scope.found.Workspace.ID,
				UserID:      form.UserID,
				Role:        form.Role,
			})
		}

		if err == nil {
			message := officei18n.T(request.Context(), "settings.member_role_changed", map[string]any{
				"Name": utils.FullName(member),
				"Role": utils.RoleLabel(request.Context(), workspaces.Role(form.Role)),
			})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else {
			h.memberChangeFailed(writer, request, scope, err)
		}
	}
}

// An owner may remove another member, who can be added again by email. The
// signed-in User does not remove themselves from here, so such a request, and
// one for a User who is no longer a member, changes nothing and shows the
// current members.

func (h *Handler) removeWorkspaceMember(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.workspaceSettingsScope(writer, request, workspaces.ActionManageMembers)

	if ok {
		var member *users.User

		id, err := utils.ParseRemoveMemberForm(writer, request)

		if err == nil && id != scope.current.User.ID {
			member, err = h.findMember(request, scope, id)

			if err == nil {
				err = h.workspaces.RemoveMember(request.Context(), scope.found.Workspace.ID, id)
			}
		}

		if err == nil && member != nil {
			message := officei18n.T(request.Context(), "settings.member_removed", map[string]any{"Name": utils.FullName(member)})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if err == nil {
			http.Redirect(writer, request, scope.url, http.StatusSeeOther)
		} else {
			h.memberChangeFailed(writer, request, scope, err)
		}
	}
}

//
// Helpers
//

// The response is written when the scope cannot be resolved, or when the
// member's role does not allow the action; otherwise the scope comes with the
// request whose context carries the User's identities.

func (h *Handler) workspaceSettingsScope(
	writer http.ResponseWriter,
	request *http.Request,
	action workspaces.Action,
) (*http.Request, workspaceSettingsScope, bool) {
	var scope workspaceSettingsScope

	request, current, err := h.authenticate(writer, request)

	if err == nil {
		scope.current = current
		request, scope.found, err = h.enterWorkspace(request)
	}

	if err == nil && !scope.found.Role.Allows(action) {
		err = fmt.Errorf("%w: a workspace %s may not %s", workspaces.ErrRolePermissionDenied, scope.found.Role, action)
	}

	if err == nil {
		scope.url = utils.WorkspacePath(request, scope.found.Workspace.ID) + "/settings"
	} else {
		h.settingsUnavailable(writer, request, err)
	}

	return request, scope, err == nil
}

// A member is named from the workspace's members, so a User who is not one is
// reported as ErrMembershipNotFound.

func (h *Handler) findMember(
	request *http.Request,
	scope workspaceSettingsScope,
	id users.UserID,
) (*users.User, error) {
	var member *users.User

	members, err := h.workspaces.ListMembers(request.Context(), scope.found.Workspace.ID)

	if err == nil {
		for _, found := range members {
			if found.User.ID == id {
				member = found.User
				break
			}
		}
	}

	if err == nil && member == nil {
		err = fmt.Errorf("find member %s: %w", uuid.UUID(id), workspaces.ErrMembershipNotFound)
	}

	return member, err
}

// A change to a member who is gone, or to the signed-in User's own role,
// changes nothing and shows the current members; one that would leave the workspace without an owner conflicts
// with it.

func (h *Handler) memberChangeFailed(
	writer http.ResponseWriter,
	request *http.Request,
	scope workspaceSettingsScope,
	err error,
) {
	if errors.Is(err, users.ErrUserNotFound) ||
		errors.Is(err, workspaces.ErrMembershipNotFound) ||
		errors.Is(err, workspaces.ErrMembershipOwnRole) {
		http.Redirect(writer, request, scope.url, http.StatusSeeOther)
	} else if isWorkspaceAccessError(err) {
		h.settingsUnavailable(writer, request, err)
	} else if errors.Is(err, forms.ErrInvalid) || errors.Is(err, input.ErrChangeMemberRoleInputInvalid) {
		utils.Fail(writer, request, http.StatusBadRequest, "errors.form")
	} else if errors.Is(err, workspaces.ErrMembershipLastOwner) {
		utils.Fail(writer, request, http.StatusConflict, "errors.last_owner")
	} else {
		slog.ErrorContext(request.Context(), "office workspace member change", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.settings_unavailable")
	}
}

// The membership or role may have changed since the scope was resolved.

func isWorkspaceAccessError(err error) bool {
	return errors.Is(err, workspaces.ErrWorkspaceNotFound) || errors.Is(err, workspaces.ErrRolePermissionDenied)
}

// A failed submission replaces only its form, so the other fields keep what
// was typed; every other response is the whole page, inside the workspace
// with its members. A member list that cannot be loaded shows its
// error instead.

func (h *Handler) renderWorkspaceSettings(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope workspaceSettingsScope,
	content pages.WorkspaceSettingsContentProps,
	region string,
) {
	if region != "" {
		utils.RenderFormRegion(writer, request, status, region, pages.SettingsFormFailure(workspaceSettingsRegion(region, content)))
	} else {
		title := officei18n.T(request.Context(), "settings.workspace_title")
		members, err := h.workspaces.ListMembers(request.Context(), scope.found.Workspace.ID)

		if err != nil {
			slog.ErrorContext(request.Context(), "office workspace members", "error", err)
			content.MembersError = officei18n.T(request.Context(), "errors.members_unavailable")
		} else {
			content.Members = utils.MemberItems(request.Context(), members, scope.current, content.CanManageMembers, scope.url)
		}

		dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.WorkspaceSettingsSection, title)
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
