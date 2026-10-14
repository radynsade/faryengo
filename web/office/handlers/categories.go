package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/workspaces"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Categories
//

// A workspace's categories belong to the workspace in the address, and only
// its members reach them. Every member sees them; only those whose role
// allows editing the workspace's content create, change, or delete them.

type categoriesScope struct {
	current *security.Identity
	found   app.MemberWorkspace
	url     string
}

// The list shows one page of the categories in name order.

func (h *Handler) listCategories(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.categoriesScope(writer, request, workspaces.ActionViewContent)

	if ok {
		notices, err := h.settingsNotices(request, scope.current.Session)

		if err != nil {
			slog.ErrorContext(request.Context(), "office flash storage", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.categories_unavailable")
		} else {
			h.renderCategories(writer, request, scope, utils.CategoriesContentProps(request, scope.found, notices))
		}
	}
}

// Only members who may add a category reach its page.

func (h *Handler) newCategory(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.categoriesScope(writer, request, workspaces.ActionEditContent)

	if ok {
		h.renderNewCategory(writer, request, http.StatusOK, scope, utils.NewCategoryFormProps(request, scope.found.Workspace.ID), false)
	}
}

// The form is posted from the new category's page, so a rejected one replaces
// the form there.

func (h *Handler) createCategory(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.categoriesScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var category *categories.Category

		form, err := utils.ParseCategoryForm(writer, request)

		if err == nil {
			category, err = h.categories.Create(request.Context(), input.CreateCategoryInput{
				WorkspaceID: scope.found.Workspace.ID,
				Name:        form.Name,
				Icon:        form.Icon,
				Color:       form.Color,
			})
		}

		if err == nil {
			message := officei18n.T(request.Context(), "categories.created", map[string]any{"Name": string(category.Name)})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if isWorkspaceAccessError(err) {
			h.settingsUnavailable(writer, request, err)
		} else {
			status, props := categoryFormFailure(request, utils.NewCategoryFormProps(request, scope.found.Workspace.ID), form, err)

			h.renderNewCategory(writer, request, status, scope, props, true)
		}
	}
}

// Only members who may change a category reach its page.

func (h *Handler) editCategory(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.categoriesScope(writer, request, workspaces.ActionEditContent)

	if ok {
		category, err := h.findCategory(request)

		if err == nil {
			h.renderEditCategory(writer, request, http.StatusOK, scope, category, utils.EditCategoryContentProps(request, category), false)
		} else {
			h.categoryUnavailable(writer, request, scope, err)
		}
	}
}

// The category keeps its workspace; only its name, icon, and color change.

func (h *Handler) updateCategory(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.categoriesScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var (
			form    utils.CategoryForm
			updated *categories.Category
		)

		category, err := h.findCategory(request)

		if err == nil {
			form, err = utils.ParseCategoryForm(writer, request)
		}

		if err == nil {
			updated, err = h.categories.Update(request.Context(), input.UpdateCategoryInput{
				ID:    category.ID,
				Name:  form.Name,
				Icon:  form.Icon,
				Color: form.Color,
			})
		}

		if err == nil {
			message := officei18n.T(request.Context(), "categories.saved", map[string]any{"Name": string(updated.Name)})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if category == nil || errors.Is(err, categories.ErrCategoryNotFound) || isWorkspaceAccessError(err) {
			h.categoryUnavailable(writer, request, scope, err)
		} else {
			var status int

			content := utils.EditCategoryContentProps(request, category)
			status, content.Form = categoryFormFailure(request, content.Form, form, err)

			h.renderEditCategory(writer, request, status, scope, category, content, true)
		}
	}
}

// Only a request confirmed in the dialog deletes the category; an
// unconfirmed one leads back to the category's page.

func (h *Handler) deleteCategory(writer http.ResponseWriter, request *http.Request) {
	request, scope, ok := h.categoriesScope(writer, request, workspaces.ActionEditContent)

	if ok {
		var confirmed bool

		category, err := h.findCategory(request)

		if err == nil {
			confirmed, err = utils.ParseDeleteCategoryForm(writer, request)
		}

		if err == nil && confirmed {
			err = h.categories.Delete(request.Context(), category.ID)
		}

		if err == nil && confirmed {
			message := officei18n.T(request.Context(), "categories.deleted", map[string]any{"Name": string(category.Name)})

			h.settingsSaved(writer, request, scope.current.Session, scope.url, []string{message})
		} else if err == nil {
			http.Redirect(writer, request, utils.CategoryPath(request, category.WorkspaceID, category.ID), http.StatusSeeOther)
		} else if errors.Is(err, utils.ErrFormInvalid) {
			utils.Fail(writer, request, http.StatusBadRequest, "errors.form")
		} else {
			h.categoryUnavailable(writer, request, scope, err)
		}
	}
}

//
// Helpers
//

// The response is written when the scope cannot be resolved, or when the
// member's role does not allow the action; otherwise the scope comes with the
// request whose context carries the User's identities.

func (h *Handler) categoriesScope(
	writer http.ResponseWriter,
	request *http.Request,
	action workspaces.Action,
) (*http.Request, categoriesScope, bool) {
	var scope categoriesScope

	request, current, err := h.authenticate(writer, request)

	if err == nil {
		scope.current = current
		request, scope.found, err = h.enterWorkspace(request)
	}

	if err == nil && !scope.found.Role.Allows(action) {
		err = fmt.Errorf("%w: a workspace %s may not %s", workspaces.ErrRolePermissionDenied, scope.found.Role, action)
	}

	if err == nil {
		scope.url = utils.CategoriesPath(request, scope.found.Workspace.ID)
	} else {
		h.settingsUnavailable(writer, request, err)
	}

	return request, scope, err == nil
}

// The category comes from the address, and only one of the workspace in the
// address is found.

func (h *Handler) findCategory(request *http.Request) (*categories.Category, error) {
	var category *categories.Category

	id, err := utils.ParseCategoryID(request)

	if err == nil {
		category, err = h.categories.FindByID(request.Context(), id)
	}

	return category, err
}

// A category that is missing, or that belongs to another workspace, is not
// found; a form posted to it, such as from a page left open after another
// member deleted it, leads back to the list.

func (h *Handler) categoryUnavailable(
	writer http.ResponseWriter,
	request *http.Request,
	scope categoriesScope,
	err error,
) {
	if errors.Is(err, categories.ErrCategoryNotFound) && request.Method == http.MethodGet {
		utils.Fail(writer, request, http.StatusNotFound, "errors.not_found")
	} else if errors.Is(err, categories.ErrCategoryNotFound) {
		http.Redirect(writer, request, scope.url, http.StatusSeeOther)
	} else if utils.IsUnauthenticated(err) || isWorkspaceAccessError(err) {
		h.settingsUnavailable(writer, request, err)
	} else {
		slog.ErrorContext(request.Context(), "office category", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.categories_unavailable")
	}
}

// The form keeps what was submitted and explains what was rejected; a failure
// that belongs to no field is shown at the top of the form, with the status
// it calls for.

func categoryFormFailure(
	request *http.Request,
	props pages.CategoryFormProps,
	form utils.CategoryForm,
	err error,
) (int, pages.CategoryFormProps) {
	status := http.StatusUnprocessableEntity

	props.Name, props.Icon, props.Color = form.Name, form.Icon, form.Color
	props.FieldErrors = utils.CategoryFieldErrors(request.Context(), err)

	if props.FieldErrors == nil {
		status, props.Error = utils.SettingsFormError(request.Context(), err)

		if status == http.StatusServiceUnavailable {
			slog.ErrorContext(request.Context(), "office category", "error", err)
			props.Error = officei18n.T(request.Context(), "errors.categories_unavailable")
		}
	}

	return status, props
}

// The page shows one page of the workspace's categories; a list that cannot be
// loaded shows its error instead.

func (h *Handler) renderCategories(
	writer http.ResponseWriter,
	request *http.Request,
	scope categoriesScope,
	content pages.CategoriesContentProps,
) {
	title := officei18n.T(request.Context(), "nav."+utils.CategoriesSection)
	query := utils.CategoriesQuery(request)
	list, err := h.categories.List(request.Context(), scope.found.Workspace.ID, query)

	if err != nil {
		slog.ErrorContext(request.Context(), "office categories", "error", err)
		content.ListError = officei18n.T(request.Context(), "errors.categories_unavailable")
	} else {
		content.Items = utils.CategoryItems(request, list.Categories, content.NewURL != "")
		content.Pagination = utils.CategoriesPagination(request, scope.found.Workspace.ID, query, list.Total)
	}

	dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.CategoriesSection, title)
	dashboard.Narrow = true

	utils.Render(writer, request, http.StatusOK, title, pages.Categories(dashboard, content))
}

// A failed submission replaces only its form, so the rest of the page keeps
// its state; every other response is the whole page.

func (h *Handler) renderNewCategory(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope categoriesScope,
	form pages.CategoryFormProps,
	formOnly bool,
) {
	if formOnly {
		utils.RenderFormRegion(writer, request, status, pages.CategoryFormID, pages.SettingsFormFailure(pages.CategoryForm(form)))
	} else {
		title := officei18n.T(request.Context(), "categories.new")
		dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.CategoriesSection, title)
		dashboard.Narrow = true

		utils.Render(writer, request, status, title, pages.NewCategory(dashboard, form))
	}
}

func (h *Handler) renderEditCategory(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	scope categoriesScope,
	category *categories.Category,
	content pages.EditCategoryContentProps,
	formOnly bool,
) {
	if formOnly {
		utils.RenderFormRegion(writer, request, status, pages.CategoryFormID, pages.SettingsFormFailure(pages.CategoryForm(content.Form)))
	} else {
		title := string(category.Name)
		dashboard := utils.DashboardProps(request, scope.current, scope.found.Workspace, utils.CategoriesSection, title)
		dashboard.Narrow = true

		utils.Render(writer, request, status, title, pages.EditCategory(dashboard, content))
	}
}
