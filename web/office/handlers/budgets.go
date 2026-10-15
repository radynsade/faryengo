package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/budgets"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/forms"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Budgets
//

// A workspace's budgets belong to the workspace in the address, and only its
// members reach them. Every member sees them; only those whose role allows
// editing the workspace's content create, change, or delete them. A budget
// covers some of the workspace's categories, which are offered by name, and
// is set in one of the currencies, which are offered by code, during a period
// chosen with the period selector. Budgets have no names of their own, so
// each is named by its categories.

type budgetsScope struct {
	current *security.Identity
	found   app.MemberWorkspace
	url     string
}

// The list shows one page of the budgets, the largest first.

func (h *Handler) listBudgets(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.budgetsScope(writer, request, workspaces.ActionViewContent)

	if ok {
		notices, err := h.settingsNotices(request, scope.current.Session)

		if err != nil {
			slog.ErrorContext(request.Context(), "office flash storage", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.budgets_unavailable")
		} else {
			h.renderBudgets(writer, request, scope, utils.BudgetsContentProps(request, scope.found, notices))
		}
	}
}

// Only members who may add a budget reach its page.

func (h *Handler) newBudget(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.budgetsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		categoryList, currencyList, err := h.budgetChoices(request, scope.found.Workspace.ID)

		if err == nil {
			form := utils.NewBudgetFormProps(request, scope.found.Workspace.ID, categoryList, currencyList)

			h.renderNewBudget(writer, request, http.StatusOK, scope, form, false)
		} else {
			h.budgetUnavailable(writer, request, scope, err)
		}
	}
}

// The form is posted from the new budget's page, so a rejected one replaces
// the form there.

func (h *Handler) createBudget(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.budgetsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var (
			budget       *budgets.Budget
			categoryIDs  []categories.ID
			budgetPeriod input.PeriodInput
			amount       int64
		)

		form, err := utils.ParseBudgetForm(writer, request)

		if err == nil {
			categoryIDs, budgetPeriod, amount, err = utils.BudgetValues(form)
		}

		if err == nil {
			budget, err = h.budgets.Create(request.Context(), input.CreateBudgetInput{
				WorkspaceID:  scope.found.Workspace.ID,
				CategoryIDs:  categoryIDs,
				CurrencyCode: form.Currency,
				Period:       budgetPeriod,
				Amount:       amount,
			})
		}

		if err == nil {
			h.budgetSaved(writer, request, scope, "budgets.created", budget)
		} else if isWorkspaceAccessError(err) {
			h.settingsUnavailable(writer, request, err)
		} else {
			categoryList, currencyList, choicesErr := h.budgetChoices(request, scope.found.Workspace.ID)

			if choicesErr == nil {
				props := utils.NewBudgetFormProps(request, scope.found.Workspace.ID, categoryList, currencyList)
				status, props := budgetFormFailure(request, props, categoryList, form, err)

				h.renderNewBudget(writer, request, status, scope, props, true)
			} else {
				h.budgetUnavailable(writer, request, scope, choicesErr)
			}
		}
	}
}

// Only members who may change a budget reach its page.

func (h *Handler) editBudget(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.budgetsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var (
			categoryList []*categories.Category
			currencyList []*currencies.Currency
		)

		budget, err := h.findBudget(request)

		if err == nil {
			categoryList, currencyList, err = h.budgetChoices(request, scope.found.Workspace.ID)
		}

		if err == nil {
			content := utils.EditBudgetContentProps(request, budget, categoryList, currencyList)

			h.renderEditBudget(writer, request, http.StatusOK, scope, content, false)
		} else {
			h.budgetUnavailable(writer, request, scope, err)
		}
	}
}

// The budget keeps its workspace; its categories, currency, period, and
// amount are replaced.

func (h *Handler) updateBudget(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.budgetsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var (
			form         utils.BudgetForm
			categoryIDs  []categories.ID
			budgetPeriod input.PeriodInput
			amount       int64
			updated      *budgets.Budget
		)

		budget, err := h.findBudget(request)

		if err == nil {
			form, err = utils.ParseBudgetForm(writer, request)
		}

		if err == nil {
			categoryIDs, budgetPeriod, amount, err = utils.BudgetValues(form)
		}

		if err == nil {
			updated, err = h.budgets.Update(request.Context(), input.UpdateBudgetInput{
				ID:           budget.ID,
				CategoryIDs:  categoryIDs,
				CurrencyCode: form.Currency,
				Period:       budgetPeriod,
				Amount:       amount,
			})
		}

		if err == nil {
			h.budgetSaved(writer, request, scope, "budgets.saved", updated)
		} else if budget == nil || errors.Is(err, budgets.ErrBudgetNotFound) || isWorkspaceAccessError(err) {
			h.budgetUnavailable(writer, request, scope, err)
		} else {
			categoryList, currencyList, choicesErr := h.budgetChoices(request, scope.found.Workspace.ID)

			if choicesErr == nil {
				var status int

				content := utils.EditBudgetContentProps(request, budget, categoryList, currencyList)
				status, content.Form = budgetFormFailure(request, content.Form, categoryList, form, err)

				h.renderEditBudget(writer, request, status, scope, content, true)
			} else {
				h.budgetUnavailable(writer, request, scope, choicesErr)
			}
		}
	}
}

// Only a request confirmed in the dialog deletes the budget; an unconfirmed
// one leads back to the budget's page.

func (h *Handler) deleteBudget(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.budgetsScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var confirmed bool

		budget, err := h.findBudget(request)

		if err == nil {
			confirmed, err = utils.ParseDeleteBudgetForm(writer, request)
		}

		if err == nil && confirmed {
			err = h.budgets.Delete(request.Context(), budget.ID)
		}

		if err == nil && confirmed {
			h.budgetSaved(writer, request, scope, "budgets.deleted", budget)
		} else if err == nil {
			http.Redirect(writer, request, utils.BudgetPath(request, budget.WorkspaceID, budget.ID), http.StatusSeeOther)
		} else if errors.Is(err, forms.ErrInvalid) {
			utils.Fail(writer, request, http.StatusBadRequest, "errors.form")
		} else {
			h.budgetUnavailable(writer, request, scope, err)
		}
	}
}

//
// Helpers
//

// The response is written when the scope cannot be resolved, or when the
// member's role does not allow the action; otherwise the scope comes with the
// request whose context carries the User's identities.

func (h *Handler) budgetsScope(
	writer http.ResponseWriter,
	request *http.Request,
	action workspaces.Action,
) (*http.Request, budgetsScope, bool) {
	var scope budgetsScope

	request, current, err := h.authenticate(writer, request)

	if err == nil {
		scope.current = current
		request, scope.found, err = h.enterWorkspace(request)
	}

	if err == nil && !scope.found.Role.Allows(action) {
		err = fmt.Errorf("%w: a workspace %s may not %s", workspaces.ErrRolePermissionDenied, scope.found.Role, action)
	}

	if err == nil {
		scope.url = utils.BudgetsPath(request, scope.found.Workspace.ID)
	} else {
		h.settingsUnavailable(writer, request, err)
	}

	return request, scope, err == nil
}

// The budget comes from the address, and only one of the workspace in the
// address is found.

func (h *Handler) findBudget(request *http.Request) (*budgets.Budget, error) {
	var budget *budgets.Budget

	id, err := utils.ParseBudgetID(request)

	if err == nil {
		budget, err = h.budgets.FindByID(request.Context(), id)
	}

	return budget, err
}

// A budget's form offers every category of the workspace and every currency.

func (h *Handler) budgetChoices(
	request *http.Request,
	workspaceID workspaces.ID,
) ([]*categories.Category, []*currencies.Currency, error) {
	var currencyList []*currencies.Currency

	categoryList, err := h.workspaceCategories(request, workspaceID)

	if err == nil {
		currencyList, err = h.currencies.List(request.Context())
	}

	if err != nil {
		categoryList, currencyList = nil, nil
	}

	return categoryList, currencyList, err
}

// Every page of the workspace's categories is read, so the form and the list
// know each category a budget may cover.

func (h *Handler) workspaceCategories(
	request *http.Request,
	workspaceID workspaces.ID,
) ([]*categories.Category, error) {
	var (
		all  []*categories.Category
		list app.CategoryList
		err  error
	)

	for page := uint(1); err == nil && (page == 1 || len(all) < list.Total); page++ {
		list, err = h.categories.List(request.Context(), workspaceID, utils.BudgetCategoriesQuery(page))

		if err == nil && len(list.Categories) == 0 {
			break
		}

		all = append(all, list.Categories...)
	}

	if err != nil {
		all = nil
	}

	return all, err
}

// The confirmation names the budget by its categories. The change is
// committed before they are read, so when they cannot be, only the
// confirmation is lost.

func (h *Handler) budgetSaved(
	writer http.ResponseWriter,
	request *http.Request,
	scope budgetsScope,
	messageID string,
	budget *budgets.Budget,
) {
	var messages []string

	categoryList, err := h.workspaceCategories(request, scope.found.Workspace.ID)

	if err == nil {
		messages = []string{officei18n.T(request.Context(), messageID, map[string]any{
			"Categories": utils.BudgetTitle(request.Context(), budget, categoryList),
		})}
	} else {
		slog.ErrorContext(request.Context(), "office budget categories", "error", err)
	}

	h.settingsSaved(writer, request, scope.current.Session, scope.url, messages)
}

// A budget that is missing, or that belongs to another workspace, is not
// found; a form posted to it, such as from a page left open after another
// member deleted it, leads back to the list.

func (h *Handler) budgetUnavailable(
	writer http.ResponseWriter,
	request *http.Request,
	scope budgetsScope,
	err error,
) {
	if errors.Is(err, budgets.ErrBudgetNotFound) && request.Method == http.MethodGet {
		utils.Fail(writer, request, http.StatusNotFound, "errors.not_found")
	} else if errors.Is(err, budgets.ErrBudgetNotFound) {
		http.Redirect(writer, request, scope.url, http.StatusSeeOther)
	} else if utils.IsUnauthenticated(err) || isWorkspaceAccessError(err) {
		h.settingsUnavailable(writer, request, err)
	} else {
		slog.ErrorContext(request.Context(), "office budget", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.budgets_unavailable")
	}
}

// The form keeps what was submitted and explains what was rejected. A failure
// that belongs to no field is shown at the top of the form, with the status it
// calls for.

func budgetFormFailure(
	request *http.Request,
	props pages.BudgetFormProps,
	categoryList []*categories.Category,
	form utils.BudgetForm,
	err error,
) (int, pages.BudgetFormProps) {
	status := http.StatusUnprocessableEntity

	props.Categories = utils.BudgetCategoryChoices(categoryList, form.Categories)
	props.Currency, props.Amount = form.Currency, form.Amount
	props.Period = utils.SubmittedPeriodSelectorProps(request.Context(), props.Period, form.Period)
	props.FieldErrors = utils.BudgetFieldErrors(request.Context(), form, err)

	if props.FieldErrors == nil {
		status, props.Error = utils.SettingsFormError(request.Context(), err)

		if status == http.StatusServiceUnavailable {
			slog.ErrorContext(request.Context(), "office budget", "error", err)
			props.Error = officei18n.T(request.Context(), "errors.budgets_unavailable")
		}
	}

	return status, props
}

// The page shows one page of the workspace's budgets with their categories; a
// list that cannot be loaded shows its error instead.

func (h *Handler) renderBudgets(
	writer http.ResponseWriter,
	request *http.Request,
	scope budgetsScope,
	content pages.BudgetsContentProps,
) {
	var categoryList []*categories.Category

	title := officei18n.T(request.Context(), "nav."+utils.BudgetsSection)
	query := utils.BudgetsQuery(request)
	list, err := h.budgets.List(request.Context(), scope.found.Workspace.ID, query)

	if err == nil {
		categoryList, err = h.workspaceCategories(request, scope.found.Workspace.ID)
	}

	if err != nil {
		slog.ErrorContext(request.Context(), "office budgets", "error", err)
		content.ListError = officei18n.T(request.Context(), "errors.budgets_unavailable")
	} else {
		content.Items = utils.BudgetItems(request, list.Budgets, categoryList, content.NewURL != "")
		content.Pagination = utils.BudgetsPagination(request, scope.found.Workspace.ID, query, list.Total)
	}

	dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.BudgetsSection, title)
	dashboard.Narrow = true

	utils.Render(writer, request, http.StatusOK, title, pages.Budgets(dashboard, content))
}

// A failed submission replaces only its form, so the rest of the page keeps
// its state; every other response is the whole page.

func (h *Handler) renderNewBudget(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope budgetsScope,
	form pages.BudgetFormProps,
	formOnly bool,
) {
	if formOnly {
		utils.RenderFormRegion(writer, request, status, pages.BudgetFormID, pages.SettingsFormFailure(pages.BudgetForm(form)))
	} else {
		title := officei18n.T(request.Context(), "budgets.new")
		dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.BudgetsSection, title)
		dashboard.Narrow = true

		utils.Render(writer, request, status, title, pages.NewBudget(dashboard, form))
	}
}

// The page is titled by the budget's categories.

func (h *Handler) renderEditBudget(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope budgetsScope,
	content pages.EditBudgetContentProps,
	formOnly bool,
) {
	if formOnly {
		utils.RenderFormRegion(writer, request, status, pages.BudgetFormID, pages.SettingsFormFailure(pages.BudgetForm(content.Form)))
	} else {
		title := content.Title
		dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.BudgetsSection, title)
		dashboard.Narrow = true

		utils.Render(writer, request, status, title, pages.EditBudget(dashboard, content))
	}
}
