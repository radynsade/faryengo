package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/radynsade/faryengo/internal/security"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
	"github.com/radynsade/faryengo/web/admin/utils"
)

//
// Role list
//

// Roles page

// The page renders the list controls with a loading row; the table itself is
// loaded by a second request, so the page appears before the list query runs.

func (h *Handler) rolesPage(writer http.ResponseWriter, request *http.Request) {
	h.roleList(writer, request, false)
}

// Roles table

func (h *Handler) rolesTable(writer http.ResponseWriter, request *http.Request) {
	h.roleList(writer, request, true)
}

//
// Role form
//

// Role create

func (h *Handler) roleCreate(writer http.ResponseWriter, request *http.Request) {
	h.roleForm(writer, request, false)
}

// Role edit

func (h *Handler) roleEdit(writer http.ResponseWriter, request *http.Request) {
	h.roleForm(writer, request, true)
}

//
// Role details
//

// Role view

func (h *Handler) roleView(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionRoles, users.PermissionViewRole, func(current *security.Identity, panel layouts.PanelProps) {
		props, err := h.roleViewProps(request, current, panel)

		if err != nil {
			h.renderRoleError(writer, request, current, err)
		} else {
			h.render(writer, request, http.StatusOK, props.Role.Name, pages.RoleView(props))
		}
	})
}

// Role delete

// A failed deletion keeps the confirmation dialog open with the error. With
// in-page updates only the dialog is replaced; otherwise the role page is
// rendered with the dialog already open.

func (h *Handler) roleDelete(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionRoles, users.PermissionManageRole, func(current *security.Identity, panel layouts.PanelProps) {
		var id users.RoleID

		props, err := h.roleViewProps(request, current, panel)

		if err == nil {
			err = utils.ParseRoleDeleteForm(writer, request)
		}

		if err == nil {
			id, err = utils.ParseRoleID(request)
		}

		if err == nil {
			err = h.roles.Delete(request.Context(), id)
		}

		if err == nil {
			h.roleDeleted(writer, request, panel, props.Role.Name)
		} else {
			status, messageID := utils.RoleError(request, err)
			messages, flashErr := h.flashes.ShowError(writer, request, admini18n.T(request.Context(), messageID))

			if flashErr != nil {
				utils.FlashUnavailable(writer, request, flashErr)
			} else if utils.IsPartial(request) {
				dialog := pages.RoleDeleteDialog(request.Context(), request.URL.Path, props.Role.Name, messages)

				utils.RenderFragment(writer, request, status, components.ConfirmDelete(dialog))
			} else if props.Role.ID == "" {
				h.render(writer, request, status, panel.Title, pages.PanelSection(panel, strings.Join(messages, " ")))
			} else {
				props.DeleteErrors = messages

				h.render(writer, request, status, props.Role.Name, pages.RoleView(props))
			}
		}
	})
}

//
// Helpers
//

func (h *Handler) roleList(writer http.ResponseWriter, request *http.Request, load bool) {
	h.withPanel(writer, request, layouts.SectionRoles, users.PermissionViewRole, func(current *security.Identity, panel layouts.PanelProps) {
		query, err := utils.ParseRoleQuery(request)
		status := http.StatusOK
		props := pages.RoleListProps{
			Panel:     panel,
			Query:     query,
			CanManage: current.Can(users.PermissionManageRole),
			Loading:   !load,
		}

		if err == nil && load {
			var (
				list    app.RoleList
				catalog []*languages.Language
			)

			list, err = h.roles.List(request.Context(), query)

			if err == nil {
				catalog, err = h.languages.List(request.Context())
			}

			if err == nil {
				props.Total = list.Total

				for _, role := range list.Roles {
					props.Rows = append(props.Rows, utils.RoleRow(request.Context(), role, current, request.PathValue("language"), catalog))
				}
			}
		}

		var flashErr error

		if err != nil {
			var messageID string

			status, messageID = utils.RoleError(request, err)
			props.Loading, props.Rows, props.Total = false, nil, 0
			props.InvalidQuery = errors.Is(err, users.ErrInvalidRoleQuery)
			props.Query = utils.SanitizeRoleQuery(query)
			props.FieldErrors = utils.FilterFieldErrors(request.Context(), err)

			// Filter values are shown beside their fields; any other failure
			// is reported as a notification.
			if len(props.FieldErrors) == 0 {
				flashErr = h.flashes.Add(writer, request, flashmsg.Error, admini18n.T(request.Context(), messageID))
			}

			if flashErr == nil && load && utils.IsPartial(request) {
				var bag *flashmsg.Bag

				bag, flashErr = h.flashes.Take(request, flashmsg.Error)

				if flashErr == nil {
					props.Errors = bag.Get(flashmsg.Error)
				}
			}
		}

		props.Permissions = utils.PermissionOptions(request.Context(), props.Query.Filter.Permissions)

		if flashErr != nil {
			utils.FlashUnavailable(writer, request, flashErr)
		} else if load && utils.IsPartial(request) {
			utils.RenderFragment(writer, request, status, pages.RolesTable(props))
		} else {
			h.render(writer, request, status, panel.Title, pages.Roles(props))
		}
	})
}

func (h *Handler) roleForm(writer http.ResponseWriter, request *http.Request, edit bool) {
	h.withPanel(writer, request, layouts.SectionRoles, users.PermissionManageRole, func(current *security.Identity, panel layouts.PanelProps) {
		var (
			role *users.Role
			id   users.RoleID
		)

		catalog, err := h.languages.List(request.Context())

		if err == nil && edit {
			id, err = utils.ParseRoleID(request)

			if err == nil {
				role, err = h.roles.FindByID(request.Context(), id)
			}
		}

		if err != nil {
			h.renderRoleError(writer, request, current, err)
		} else {
			code := request.PathValue("language")
			form := utils.RoleForm{Name: make(map[string]string), Permissions: []string{}}
			props := pages.RoleFormProps{
				Panel:  panel,
				Title:  admini18n.T(request.Context(), "roles.create"),
				Action: panel.BasePath + "/roles/create",
			}

			if edit {
				props.ID = uuid.UUID(role.ID).String()
				props.Name = utils.RoleName(role, code, catalog)
				props.Title = admini18n.T(request.Context(), "roles.edit_title", map[string]any{"Name": props.Name})
				props.Action = panel.BasePath + "/roles/" + props.ID + "/edit"
				form = utils.FormFromRole(role)
			}

			status, responded := http.StatusOK, false

			if request.Method == http.MethodPost {
				var saved *users.Role

				form, err = utils.ParseRoleForm(writer, request, catalog)

				if err == nil {
					err = requestvalidation.Validate(request.Context(), form)
				}

				if err == nil && edit {
					saved, err = h.roles.Update(request.Context(), input.UpdateRoleInput{
						ID:          id,
						Name:        form.Name,
						Permissions: form.Permissions,
						IsSuper:     form.IsSuper == "1",
					})
				} else if err == nil {
					saved, err = h.roles.Create(request.Context(), input.CreateRoleInput{
						Name:        form.Name,
						Permissions: form.Permissions,
						IsSuper:     form.IsSuper == "1",
					})
				}

				if err == nil {
					h.roleSaved(writer, request, panel, saved, utils.RoleName(saved, code, catalog), edit)
					responded = true
				} else {
					var messageID string

					status, messageID = utils.RoleError(request, err)
					props.FieldErrors = utils.RoleFormFieldErrors(request.Context(), err, catalog)

					// Field errors stand beside their controls; any other failure
					// is shown at the top of the form.
					if len(props.FieldErrors) == 0 {
						var flashErr error

						props.Errors, flashErr = h.flashes.ShowError(writer, request, admini18n.T(request.Context(), messageID))

						if flashErr != nil {
							utils.FlashUnavailable(writer, request, flashErr)
							responded = true
						}
					}
				}
			}

			if !responded {
				props.NameTranslations = utils.RoleNameTranslations(request.Context(), catalog, form.Name, code, props.FieldErrors)
				props.Permissions = utils.PermissionOptions(request.Context(), utils.PermissionsOf(form.Permissions))
				props.IsSuper = form.IsSuper == "1"

				if request.Method == http.MethodPost && utils.IsPartial(request) {
					utils.RenderFormRegion(writer, request, status, pages.RoleFormID, pages.RoleFieldsForm(props))
				} else {
					h.render(writer, request, status, props.Title, pages.RoleForm(props))
				}
			}
		}
	})
}

// A successful change is confirmed on the page the redirect leads to.

func (h *Handler) roleSaved(
	writer http.ResponseWriter,
	request *http.Request,
	panel layouts.PanelProps,
	role *users.Role,
	name string,
	edit bool,
) {
	messageID := "roles.created"

	if edit {
		messageID = "roles.updated"
	}

	message := admini18n.T(request.Context(), messageID, map[string]any{"Name": name})

	if err := h.flashes.Add(writer, request, flashmsg.Success, message); err != nil {
		utils.FlashUnavailable(writer, request, err)
	} else {
		http.Redirect(writer, request, panel.BasePath+"/roles/"+uuid.UUID(role.ID).String()+"/view", http.StatusSeeOther)
	}
}

// An in-page deletion is answered with the location to navigate to, because
// the request targeted the dialog rather than the page.

func (h *Handler) roleDeleted(
	writer http.ResponseWriter,
	request *http.Request,
	panel layouts.PanelProps,
	name string,
) {
	message := admini18n.T(request.Context(), "roles.deleted", map[string]any{"Name": name})
	location := panel.BasePath + "/roles"

	if err := h.flashes.Add(writer, request, flashmsg.Success, message); err != nil {
		utils.FlashUnavailable(writer, request, err)
	} else if utils.IsPartial(request) {
		writer.Header().Set("HX-Location", utils.NavigationLocation(location))
		writer.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(writer, request, location, http.StatusSeeOther)
	}
}

func (h *Handler) roleViewProps(
	request *http.Request,
	current *security.Identity,
	panel layouts.PanelProps,
) (pages.RoleViewProps, error) {
	var (
		props   pages.RoleViewProps
		role    *users.Role
		catalog []*languages.Language
	)

	id, err := utils.ParseRoleID(request)

	if err == nil {
		role, err = h.roles.FindByID(request.Context(), id)
	}

	if err == nil {
		catalog, err = h.languages.List(request.Context())
	}

	if err == nil {
		props = pages.RoleViewProps{
			Panel:     panel,
			Role:      utils.RoleRow(request.Context(), role, current, request.PathValue("language"), catalog),
			CanManage: current.Can(users.PermissionManageRole),
		}
	}

	return props, err
}

func (h *Handler) renderRoleError(
	writer http.ResponseWriter,
	request *http.Request,
	current *security.Identity,
	err error,
) {
	status, messageID := utils.RoleError(request, err)

	h.renderPanelError(writer, request, current, status, messageID)
}
