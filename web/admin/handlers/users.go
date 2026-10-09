package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	"github.com/radynsade/faryengo/pkg/domquery"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
	"github.com/radynsade/faryengo/web/admin/utils"
)

//
// User list
//

// Users page

// The page renders the list controls with a loading row; the table itself is
// loaded by a second request, so the page appears before the list query runs.

func (h *Handler) usersPage(writer http.ResponseWriter, request *http.Request) {
	h.userList(writer, request, false)
}

// Users table

func (h *Handler) usersTable(writer http.ResponseWriter, request *http.Request) {
	h.userList(writer, request, true)
}

//
// User form
//

// User create

func (h *Handler) userCreate(writer http.ResponseWriter, request *http.Request) {
	h.userForm(writer, request, false)
}

// User edit

func (h *Handler) userEdit(writer http.ResponseWriter, request *http.Request) {
	h.userForm(writer, request, true)
}

//
// User details
//

// User view

func (h *Handler) userView(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionUsers, users.PermissionViewUser, func(current *security.Identity, panel layouts.PanelProps) {
		props, err := h.userViewProps(request, current, panel)

		if err != nil {
			h.renderUserError(writer, request, current, err)
		} else {
			h.render(writer, request, http.StatusOK, props.User.FullName(request.Context()), pages.UserView(props))
		}
	})
}

// User delete

// A failed deletion keeps the confirmation dialog open with the error. With
// in-page updates only the dialog is replaced; otherwise the user page is
// rendered with the dialog already open.

func (h *Handler) userDelete(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionUsers, users.PermissionManageUser, func(current *security.Identity, panel layouts.PanelProps) {
		var id users.UserID

		props, err := h.userViewProps(request, current, panel)
		name := props.User.FullName(request.Context())

		if err == nil {
			err = utils.ParseDeleteForm(writer, request)
		}

		if err == nil {
			id, err = utils.ParseUserID(request)
		}

		if err == nil {
			err = h.users.DeleteAsUser(request.Context(), current.User.ID, id)
		}

		if err == nil {
			h.userDeleted(writer, request, panel, name)
		} else {
			status, messageID := utils.UserError(request, err)
			messages, flashErr := h.flashes.ShowError(writer, request, admini18n.T(request.Context(), messageID))

			if flashErr != nil {
				utils.FlashUnavailable(writer, request, flashErr)
			} else if utils.IsPartial(request) {
				dialog := pages.UserDeleteDialog(request.Context(), request.URL.Path, name, messages)

				utils.RenderFragment(writer, request, status, components.ConfirmDelete(dialog))
			} else if props.User.ID == "" {
				h.render(writer, request, status, panel.Title, pages.PanelSection(panel, strings.Join(messages, " ")))
			} else {
				props.DeleteErrors = messages

				h.render(writer, request, status, name, pages.UserView(props))
			}
		}
	})
}

//
// Helpers
//

// Role choices list every Role up to this many, ordered by name.

const maxRoleChoices = 1000

func (h *Handler) userList(writer http.ResponseWriter, request *http.Request, load bool) {
	h.withPanel(writer, request, layouts.SectionUsers, users.PermissionViewUser, func(current *security.Identity, panel layouts.PanelProps) {
		code := request.PathValue("language")
		query, err := utils.ParseUserQuery(request)
		status := http.StatusOK
		props := pages.UserListProps{
			Panel:        panel,
			Query:        query,
			CanManage:    current.Can(users.PermissionManageUser),
			CanViewRoles: current.Can(users.PermissionViewRole),
			Loading:      !load,
		}

		roles, catalog, rolesErr := h.roleChoices(request)

		if err == nil {
			err = rolesErr
		}

		if err == nil && load {
			var list app.UserList

			list, err = h.users.List(request.Context(), query)

			if err == nil {
				props.Total = list.Total

				for _, user := range list.Users {
					props.Rows = append(props.Rows, utils.UserRow(user, current, roles, code, catalog))
				}
			}
		}

		var flashErr error

		if err != nil {
			var messageID string

			status, messageID = utils.UserError(request, err)
			props.Loading, props.Rows, props.Total = false, nil, 0
			props.InvalidQuery = errors.Is(err, users.ErrInvalidUserQuery)
			props.Query = utils.SanitizeUserQuery(query)
			props.FieldErrors = utils.UserFilterFieldErrors(request, err)

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

		selected := ""

		if props.Query.Filter.RoleID != nil {
			selected = uuid.UUID(*props.Query.Filter.RoleID).String()
		}

		props.Roles = utils.RoleOptions(roles, selected, code, catalog)

		if flashErr != nil {
			utils.FlashUnavailable(writer, request, flashErr)
		} else if load && utils.IsPartial(request) {
			utils.RenderFragment(writer, request, status, pages.UsersTable(props))
		} else {
			h.render(writer, request, status, panel.Title, pages.Users(props))
		}
	})
}

func (h *Handler) userForm(writer http.ResponseWriter, request *http.Request, edit bool) {
	h.withPanel(writer, request, layouts.SectionUsers, users.PermissionManageUser, func(current *security.Identity, panel layouts.PanelProps) {
		var (
			user *users.User
			id   users.UserID
		)

		roles, catalog, err := h.roleChoices(request)

		if err == nil && edit {
			id, err = utils.ParseUserID(request)

			if err == nil {
				user, err = h.users.FindByID(request.Context(), id)
			}
		}

		if err != nil {
			h.renderUserError(writer, request, current, err)
		} else {
			code := request.PathValue("language")
			form := utils.UserForm{}
			props := pages.UserFormProps{
				Panel:  panel,
				Title:  admini18n.T(request.Context(), "users.create"),
				Action: panel.BasePath + "/users/create",
			}

			if edit {
				props.ID = uuid.UUID(user.ID).String()
				props.Name = admini18n.T(request.Context(), "common.full_name", map[string]any{
					"FirstName": string(user.FirstName),
					"LastName":  string(user.LastName),
				})
				props.Title = admini18n.T(request.Context(), "users.edit_title", map[string]any{"Name": props.Name})
				props.Action = panel.BasePath + "/users/" + props.ID + "/edit"
				props.Current = user.ID == current.User.ID
				form = utils.FormFromUser(user)
			}

			status, responded := http.StatusOK, false

			if request.Method == http.MethodPost {
				var saved *users.User

				form, err = utils.ParseUserForm(writer, request, edit)

				if err == nil {
					err = requestvalidation.Validate(request.Context(), form)
				}

				if err == nil && edit {
					var password *string

					// An empty password field keeps the current password.
					if form.Password != "" {
						password = &form.Password
					}

					saved, err = h.users.Update(request.Context(), input.UpdateUserInput{
						ID:        id,
						RoleID:    form.Role(),
						Email:     form.Email,
						Phone:     form.Phone,
						Password:  password,
						FirstName: form.FirstName,
						LastName:  form.LastName,
						UpdatedAt: form.UpdatedAt,
					})
				} else if err == nil {
					saved, err = h.users.Create(request.Context(), input.CreateUserInput{
						RoleID:    form.Role(),
						Email:     form.Email,
						Phone:     form.Phone,
						Password:  form.Password,
						FirstName: form.FirstName,
						LastName:  form.LastName,
					})
				}

				if err == nil {
					h.userSaved(writer, request, panel, saved, edit)
					responded = true
				} else {
					var messageID string

					status, messageID = utils.UserError(request, err)
					props.FieldErrors = utils.UserFormFieldErrors(request.Context(), err)

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
				props.FirstName = form.FirstName
				props.LastName = form.LastName
				props.Email = form.Email
				props.Phone = form.Phone
				props.Roles = utils.RoleOptions(roles, form.RoleID, code, catalog)

				if edit {
					props.UpdatedAt = utils.FormatUpdatedAt(form.UpdatedAt)
				}

				if request.Method == http.MethodPost && utils.IsPartial(request) {
					utils.RenderFormRegion(writer, request, status, pages.UserFormID, pages.UserFieldsForm(props))
				} else {
					h.render(writer, request, status, props.Title, pages.UserForm(props))
				}
			}
		}
	})
}

// A successful change is confirmed on the page the redirect leads to.

func (h *Handler) userSaved(
	writer http.ResponseWriter,
	request *http.Request,
	panel layouts.PanelProps,
	user *users.User,
	edit bool,
) {
	messageID := "users.created"

	if edit {
		messageID = "users.updated"
	}

	name := admini18n.T(request.Context(), "common.full_name", map[string]any{
		"FirstName": string(user.FirstName),
		"LastName":  string(user.LastName),
	})
	message := admini18n.T(request.Context(), messageID, map[string]any{"Name": name})

	if err := h.flashes.Add(writer, request, flashmsg.Success, message); err != nil {
		utils.FlashUnavailable(writer, request, err)
	} else {
		http.Redirect(writer, request, panel.BasePath+"/users/"+uuid.UUID(user.ID).String()+"/view", http.StatusSeeOther)
	}
}

// An in-page deletion is answered with the location to navigate to, because
// the request targeted the dialog rather than the page.

func (h *Handler) userDeleted(
	writer http.ResponseWriter,
	request *http.Request,
	panel layouts.PanelProps,
	name string,
) {
	message := admini18n.T(request.Context(), "users.deleted", map[string]any{"Name": name})
	location := panel.BasePath + "/users"

	if err := h.flashes.Add(writer, request, flashmsg.Success, message); err != nil {
		utils.FlashUnavailable(writer, request, err)
	} else if utils.IsPartial(request) {
		writer.Header().Set("HX-Location", utils.NavigationLocation(location))
		writer.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(writer, request, location, http.StatusSeeOther)
	}
}

func (h *Handler) userViewProps(
	request *http.Request,
	current *security.Identity,
	panel layouts.PanelProps,
) (pages.UserViewProps, error) {
	var (
		props   pages.UserViewProps
		user    *users.User
		role    *users.Role
		catalog []*languages.Language
	)

	id, err := utils.ParseUserID(request)

	if err == nil {
		user, err = h.users.FindByID(request.Context(), id)
	}

	if err == nil {
		catalog, err = h.languages.List(request.Context())
	}

	// A Role deleted since is shown by its identity.
	if err == nil {
		var roleErr error

		role, roleErr = h.roles.FindByID(request.Context(), user.RoleID)

		if roleErr != nil && !errors.Is(roleErr, users.ErrRoleNotFound) {
			err = roleErr
		}
	}

	if err == nil {
		var roles []*users.Role

		if role != nil {
			roles = append(roles, role)
		}

		props = pages.UserViewProps{
			Panel:        panel,
			User:         utils.UserRow(user, current, roles, request.PathValue("language"), catalog),
			CanManage:    current.Can(users.PermissionManageUser),
			CanViewRoles: current.Can(users.PermissionViewRole),
		}
	}

	return props, err
}

func (h *Handler) roleChoices(request *http.Request) ([]*users.Role, []*languages.Language, error) {
	var catalog []*languages.Language

	list, err := h.roles.List(request.Context(), users.RoleQuery{
		SortBy:    users.RoleSortName,
		SortOrder: domquery.SortOrderAsc,
		Limit:     maxRoleChoices,
		Page:      1,
	})

	if err == nil {
		catalog, err = h.languages.List(request.Context())
	}

	if err != nil {
		list = app.RoleList{}
		catalog = nil
	}

	return list.Roles, catalog, err
}

func (h *Handler) renderUserError(
	writer http.ResponseWriter,
	request *http.Request,
	current *security.Identity,
	err error,
) {
	status, messageID := utils.UserError(request, err)

	h.renderPanelError(writer, request, current, status, messageID)
}
