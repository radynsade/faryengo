package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/radynsade/faryengo/internal/accounts"
	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Accounts
//

// A workspace's accounts belong to the workspace in the address, and only its
// members reach them. Every member sees them; only those whose role allows
// editing the workspace's content create, change, or delete them. An account
// may belong to one of the workspace's members, who are offered by name.

type accountsScope struct {
	current *security.Identity
	found   app.MemberWorkspace
	url     string
}

// The list shows one page of the accounts in name order.

func (h *Handler) listAccounts(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.accountsScope(writer, request, workspaces.ActionViewContent)

	if ok {
		notices, err := h.settingsNotices(request, scope.current.Session)

		if err != nil {
			slog.ErrorContext(request.Context(), "office flash storage", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.accounts_unavailable")
		} else {
			h.renderAccounts(writer, request, scope, utils.AccountsContentProps(request, scope.found, notices))
		}
	}
}

// Only members who may add an account reach its page.

func (h *Handler) newAccount(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.accountsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		members, err := h.workspaces.ListMembers(request.Context(), scope.found.Workspace.ID)

		if err == nil {
			h.renderNewAccount(writer, request, http.StatusOK, scope, utils.NewAccountFormProps(request, scope.found.Workspace.ID, members), false)
		} else {
			h.accountUnavailable(writer, request, scope, err)
		}
	}
}

// The form is posted from the new account's page, so a rejected one replaces
// the form there.

func (h *Handler) createAccount(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.accountsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var (
			account *accounts.Account
			userID  *users.UserID
		)

		form, err := utils.ParseAccountForm(writer, request)

		if err == nil {
			userID, err = utils.AccountUserID(form)
		}

		if err == nil {
			account, err = h.accounts.Create(request.Context(), input.CreateAccountInput{
				WorkspaceID: scope.found.Workspace.ID,
				UserID:      userID,
				Name:        form.Name,
			})
		}

		if err == nil {
			message := officei18n.T(request.Context(), "accounts.created", map[string]any{"Name": string(account.Name)})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if isWorkspaceAccessError(err) {
			h.settingsUnavailable(writer, request, err)
		} else {
			members, membersErr := h.workspaces.ListMembers(request.Context(), scope.found.Workspace.ID)

			if membersErr == nil {
				status, props := accountFormFailure(request, utils.NewAccountFormProps(request, scope.found.Workspace.ID, members), form, err)

				h.renderNewAccount(writer, request, status, scope, props, true)
			} else {
				h.accountUnavailable(writer, request, scope, membersErr)
			}
		}
	}
}

// Only members who may change an account reach its page.

func (h *Handler) editAccount(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.accountsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var members []app.WorkspaceMember

		account, err := h.findAccount(request)

		if err == nil {
			members, err = h.workspaces.ListMembers(request.Context(), scope.found.Workspace.ID)
		}

		if err == nil {
			h.renderEditAccount(writer, request, http.StatusOK, scope, account, utils.EditAccountContentProps(request, account, members), false)
		} else {
			h.accountUnavailable(writer, request, scope, err)
		}
	}
}

// The account keeps its workspace; only its name and member change.

func (h *Handler) updateAccount(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.accountsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var (
			form    utils.AccountForm
			userID  *users.UserID
			updated *accounts.Account
		)

		account, err := h.findAccount(request)

		if err == nil {
			form, err = utils.ParseAccountForm(writer, request)
		}

		if err == nil {
			userID, err = utils.AccountUserID(form)
		}

		if err == nil {
			updated, err = h.accounts.Update(request.Context(), input.UpdateAccountInput{
				ID:     account.ID,
				UserID: userID,
				Name:   form.Name,
			})
		}

		if err == nil {
			message := officei18n.T(request.Context(), "accounts.saved", map[string]any{"Name": string(updated.Name)})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if account == nil || errors.Is(err, accounts.ErrAccountNotFound) || isWorkspaceAccessError(err) {
			h.accountUnavailable(writer, request, scope, err)
		} else {
			members, membersErr := h.workspaces.ListMembers(request.Context(), scope.found.Workspace.ID)

			if membersErr == nil {
				var status int

				content := utils.EditAccountContentProps(request, account, members)
				status, content.Form = accountFormFailure(request, content.Form, form, err)

				h.renderEditAccount(writer, request, status, scope, account, content, true)
			} else {
				h.accountUnavailable(writer, request, scope, membersErr)
			}
		}
	}
}

// Only a request confirmed in the dialog deletes the account; an unconfirmed
// one leads back to the account's page.

func (h *Handler) deleteAccount(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.accountsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var confirmed bool

		account, err := h.findAccount(request)

		if err == nil {
			confirmed, err = utils.ParseDeleteAccountForm(writer, request)
		}

		if err == nil && confirmed {
			err = h.accounts.Delete(request.Context(), account.ID)
		}

		if err == nil && confirmed {
			message := officei18n.T(request.Context(), "accounts.deleted", map[string]any{"Name": string(account.Name)})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if err == nil {
			http.Redirect(writer, request, utils.AccountPath(request, account.WorkspaceID, account.ID), http.StatusSeeOther)
		} else if errors.Is(err, utils.ErrFormInvalid) {
			utils.Fail(writer, request, http.StatusBadRequest, "errors.form")
		} else {
			h.accountUnavailable(writer, request, scope, err)
		}
	}
}

//
// Helpers
//

// The response is written when the scope cannot be resolved, or when the
// member's role does not allow the action; otherwise the scope comes with the
// request whose context carries the User's identities.

func (h *Handler) accountsScope(
	writer http.ResponseWriter,
	request *http.Request,
	action workspaces.Action,
) (*http.Request, accountsScope, bool) {
	var scope accountsScope

	request, current, err := h.authenticate(writer, request)

	if err == nil {
		scope.current = current
		request, scope.found, err = h.enterWorkspace(request)
	}

	if err == nil && !scope.found.Role.Allows(action) {
		err = fmt.Errorf("%w: a workspace %s may not %s", workspaces.ErrRolePermissionDenied, scope.found.Role, action)
	}

	if err == nil {
		scope.url = utils.AccountsPath(request, scope.found.Workspace.ID)
	} else {
		h.settingsUnavailable(writer, request, err)
	}

	return request, scope, err == nil
}

// The account comes from the address, and only one of the workspace in the
// address is found.

func (h *Handler) findAccount(request *http.Request) (*accounts.Account, error) {
	var account *accounts.Account

	id, err := utils.ParseAccountID(request)

	if err == nil {
		account, err = h.accounts.FindByID(request.Context(), id)
	}

	return account, err
}

// An account that is missing, or that belongs to another workspace, is not
// found; a form posted to it, such as from a page left open after another
// member deleted it, leads back to the list.

func (h *Handler) accountUnavailable(
	writer http.ResponseWriter,
	request *http.Request,
	scope accountsScope,
	err error,
) {
	if errors.Is(err, accounts.ErrAccountNotFound) && request.Method == http.MethodGet {
		utils.Fail(writer, request, http.StatusNotFound, "errors.not_found")
	} else if errors.Is(err, accounts.ErrAccountNotFound) {
		http.Redirect(writer, request, scope.url, http.StatusSeeOther)
	} else if utils.IsUnauthenticated(err) || isWorkspaceAccessError(err) {
		h.settingsUnavailable(writer, request, err)
	} else {
		slog.ErrorContext(request.Context(), "office account", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.accounts_unavailable")
	}
}

// The form keeps what was submitted and explains what was rejected; a failure
// that belongs to no field is shown at the top of the form, with the status
// it calls for.

func accountFormFailure(
	request *http.Request,
	props pages.AccountFormProps,
	form utils.AccountForm,
	err error,
) (int, pages.AccountFormProps) {
	status := http.StatusUnprocessableEntity

	props.Name, props.User = form.Name, form.User
	props.FieldErrors = utils.AccountFieldErrors(request.Context(), err)

	if props.FieldErrors == nil {
		status, props.Error = utils.SettingsFormError(request.Context(), err)

		if status == http.StatusServiceUnavailable {
			slog.ErrorContext(request.Context(), "office account", "error", err)
			props.Error = officei18n.T(request.Context(), "errors.accounts_unavailable")
		}
	}

	return status, props
}

// The page shows one page of the workspace's accounts with their members; a
// list that cannot be loaded shows its error instead.

func (h *Handler) renderAccounts(
	writer http.ResponseWriter,
	request *http.Request,
	scope accountsScope,
	content pages.AccountsContentProps,
) {
	var members []app.WorkspaceMember

	title := officei18n.T(request.Context(), "nav."+utils.AccountsSection)
	query := utils.AccountsQuery(request)
	list, err := h.accounts.List(request.Context(), scope.found.Workspace.ID, query)

	if err == nil {
		members, err = h.workspaces.ListMembers(request.Context(), scope.found.Workspace.ID)
	}

	if err != nil {
		slog.ErrorContext(request.Context(), "office accounts", "error", err)
		content.ListError = officei18n.T(request.Context(), "errors.accounts_unavailable")
	} else {
		content.Items = utils.AccountItems(request, list.Accounts, members, content.NewURL != "")
		content.Pagination = utils.AccountsPagination(request, scope.found.Workspace.ID, query, list.Total)
	}

	dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.AccountsSection, title)
	dashboard.Narrow = true

	utils.Render(writer, request, http.StatusOK, title, pages.Accounts(dashboard, content))
}

// A failed submission replaces only its form, so the rest of the page keeps
// its state; every other response is the whole page.

func (h *Handler) renderNewAccount(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope accountsScope,
	form pages.AccountFormProps,
	formOnly bool,
) {
	if formOnly {
		utils.RenderFormRegion(writer, request, status, pages.AccountFormID, pages.SettingsFormFailure(pages.AccountForm(form)))
	} else {
		title := officei18n.T(request.Context(), "accounts.new")
		dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.AccountsSection, title)
		dashboard.Narrow = true

		utils.Render(writer, request, status, title, pages.NewAccount(dashboard, form))
	}
}

func (h *Handler) renderEditAccount(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope accountsScope,
	account *accounts.Account,
	content pages.EditAccountContentProps,
	formOnly bool,
) {
	if formOnly {
		utils.RenderFormRegion(writer, request, status, pages.AccountFormID, pages.SettingsFormFailure(pages.AccountForm(content.Form)))
	} else {
		title := string(account.Name)
		dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.AccountsSection, title)
		dashboard.Narrow = true

		utils.Render(writer, request, status, title, pages.EditAccount(dashboard, content))
	}
}
