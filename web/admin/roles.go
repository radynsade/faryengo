package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

var errInvalidRoleForm = errors.New("invalid role form")

func (h *Handler) roles(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, "roles", "", func(actor security.Principal, panel layouts.PanelProps) {
		query, err := parseRoleQuery(request)
		props := pages.RoleListProps{Panel: panel, Query: query,
			Page: security.RolePage{Page: 1, PageSize: security.DefaultRolePageSize}}
		status := http.StatusOK

		if err == nil {
			props.Page, err = h.roleService.List(request.Context(), query)
		}

		if err == nil {
			props.Query.Page = props.Page.Page
			catalog, catalogErr := h.languages.List(request.Context())
			err = catalogErr

			if err == nil {
				for _, role := range props.Page.Roles {
					props.Rows = append(props.Rows, roleRow(request.Context(), role, actor, query.Language, catalog))
				}
			}
		}

		var flashErr error

		if err != nil {
			var message string
			status, message = roleError(request, err)
			flashErr = h.addFlash(request.Context(), writer, request, flashmsg.Error, message)
			props.InvalidQuery = true
			props.Page = security.RolePage{Page: 1, PageSize: security.DefaultRolePageSize}
			props.Rows = nil
		}

		props.Permissions = permissionOptions(request.Context(), query.Filters.Permissions)

		if flashErr != nil {
			h.flashUnavailable(writer, request, flashErr)
		} else {
			h.renderPage(writer, request, admini18n.T(request.Context(), "navigation.roles")+" · Faryen "+admini18n.T(request.Context(), "common.admin"), pages.Roles(props), templ.WithStatus(status))
		}
	})
}

func parseRoleQuery(request *http.Request) (security.RoleQuery, error) {
	query := security.RoleQuery{Sort: security.RoleSortID, Page: 1, PageSize: security.DefaultRolePageSize, Language: languages.LanguageCode(request.PathValue("language"))}
	values, err := url.ParseQuery(request.URL.RawQuery)

	if err == nil {
		query.Filters.IDLike = strings.TrimSpace(values.Get("uuid"))
		query.Filters.NameLike = strings.TrimSpace(values.Get("name"))

		for _, permission := range values["permissions"] {
			query.Filters.Permissions = append(query.Filters.Permissions, security.Permission(permission))
		}

		for _, key := range []string{"uuid", "name", "super", "sort", "order", "page", "size"} {
			if len(values[key]) > 1 {
				err = security.ErrInvalidRoleQuery
			}
		}

		if value := values.Get("super"); value != "" {
			if value == "true" || value == "false" {
				isSuper := value == "true"
				query.Filters.IsSuper = &isSuper
			} else {
				err = security.ErrInvalidRoleQuery
			}
		}

		if value := values.Get("sort"); value != "" {
			query.Sort = security.RoleSort(value)
		}

		if value := values.Get("order"); value != "" {
			query.Descending = value == "desc"

			if value != "asc" && value != "desc" {
				err = security.ErrInvalidRoleQuery
			}
		}

		for _, parameter := range []struct {
			key    string
			target *int
		}{{"page", &query.Page}, {"size", &query.PageSize}} {
			if value := values.Get(parameter.key); value != "" {
				number, parseErr := strconv.Atoi(value)

				if parseErr != nil {
					err = security.ErrInvalidRoleQuery
				} else {
					*parameter.target = number
				}
			}
		}

		if err == nil {
			err = query.Validate()
		}
	}

	if err != nil {
		err = fmt.Errorf("parse roles filters: %w: %w", security.ErrInvalidRoleQuery, err)
	}

	return query, err
}

func (h *Handler) roleCreate(writer http.ResponseWriter, request *http.Request) {
	h.roleForm(writer, request, false)
}

func (h *Handler) roleEdit(writer http.ResponseWriter, request *http.Request) {
	h.roleForm(writer, request, true)
}

func (h *Handler) roleForm(writer http.ResponseWriter, request *http.Request, edit bool) {
	h.withPanel(writer, request, "roles", "", func(actor security.Principal, panel layouts.PanelProps) {
		catalog, err := h.languages.List(request.Context())
		var role *security.Role
		var id security.RoleID

		if err == nil && edit {
			id, err = roleID(request)

			if err == nil {
				role, err = h.roleService.FindByID(request.Context(), id)
			}
		}

		if err != nil {
			h.renderRoleError(writer, request, panel, err)
		} else {
			props := pages.RoleFormProps{Panel: panel, Title: admini18n.T(request.Context(), "roles.create"), Action: panel.BasePath + "/roles/create"}
			values := input.CreateRoleInput{Name: make(map[string]string), Permissions: []security.Permission{}}

			if edit {
				props.ID = uuid.UUID(id).String()
				props.Name = roleRow(request.Context(), role, actor, languages.LanguageCode(request.PathValue("language")), catalog).Name
				props.Title, props.Action = admini18n.T(request.Context(), "roles.edit"), panel.BasePath+"/roles/"+props.ID+"/edit"
				values.IsSuper, values.Permissions = role.IsSuper(), role.Permissions()

				for _, translation := range role.Name().Translations() {
					values.Name[string(translation.LanguageCode())] = translation.Content()
				}
			}

			status := http.StatusOK
			responded := false

			if request.Method == http.MethodPost {
				values, err = parseRoleForm(writer, request, catalog)

				if err == nil {
					if edit {
						role, err = h.roleService.Update(request.Context(), input.UpdateRoleInput{ID: id, Name: values.Name, Permissions: values.Permissions, IsSuper: &values.IsSuper})
					} else {
						role, err = h.roleService.Create(request.Context(), values)
					}
				}

				if err == nil {
					operation := "roles.created"

					if edit {
						operation = "roles.updated"
					}

					row := roleRow(request.Context(), role, actor, languages.LanguageCode(request.PathValue("language")), catalog)
					message := admini18n.T(request.Context(), operation, map[string]any{"Name": row.Name})

					if flashErr := h.addFlash(request.Context(), writer, request, flashmsg.Success, message); flashErr != nil {
						h.flashUnavailable(writer, request, flashErr)
					} else {
						http.Redirect(writer, request, panel.BasePath+"/roles/"+uuid.UUID(role.ID()).String()+"/view", http.StatusSeeOther)
					}

					responded = true
				} else {
					var message string
					status, message = roleError(request, err)

					if flashErr := h.addFlash(request.Context(), writer, request, flashmsg.Error, message); flashErr != nil {
						h.flashUnavailable(writer, request, flashErr)
						responded = true
					}
				}
			}

			if !responded {
				props.NameTranslations = roleNameTranslations(request.Context(), catalog, values.Name, request.PathValue("language"))
				props.Permissions = permissionOptions(request.Context(), values.Permissions)
				props.IsSuper = values.IsSuper
				h.renderPage(writer, request, props.Title+" · Faryen "+admini18n.T(request.Context(), "common.admin"), pages.RoleForm(props), templ.WithStatus(status))
			}
		}
	})
}

func parseRoleForm(writer http.ResponseWriter, request *http.Request, catalog []*languages.Language) (input.CreateRoleInput, error) {
	values := input.CreateRoleInput{Name: make(map[string]string), Permissions: []security.Permission{}}
	err := parseRolePost(writer, request)

	if err == nil {
		known := make(map[string]bool, len(catalog))

		for _, language := range catalog {
			known[string(language.Code())] = true
		}

		for _, key := range slices.Sorted(maps.Keys(request.PostForm)) {
			if strings.HasPrefix(key, "name[") && strings.HasSuffix(key, "]") {
				code := strings.TrimSuffix(strings.TrimPrefix(key, "name["), "]")

				if !known[code] || len(request.PostForm[key]) != 1 {
					err = errInvalidRoleForm
				} else if name := strings.TrimSpace(request.PostForm.Get(key)); name != "" {
					values.Name[code] = name
				}
			} else if key != "permissions" && key != "is_super" {
				err = errInvalidRoleForm
			}
		}

		if len(request.PostForm["is_super"]) > 1 || (request.PostForm.Get("is_super") != "" && request.PostForm.Get("is_super") != "1") {
			err = errInvalidRoleForm
		}

		values.IsSuper = request.PostForm.Get("is_super") == "1"

		for _, permission := range request.PostForm["permissions"] {
			values.Permissions = append(values.Permissions, security.Permission(permission))
		}

		if err == nil {
			err = values.Validate()
		}
	}

	return values, err
}

func parseRolePost(writer http.ResponseWriter, request *http.Request) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))

	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		err = errInvalidRoleForm
	} else {
		request.Body = http.MaxBytesReader(writer, request.Body, 65536)

		if parseErr := request.ParseForm(); parseErr != nil {
			err = fmt.Errorf("%w: %w", errInvalidRoleForm, parseErr)
		}
	}

	return err
}

func (h *Handler) roleView(writer http.ResponseWriter, request *http.Request) {
	h.roleDetails(writer, request, false)
}

func (h *Handler) roleDelete(writer http.ResponseWriter, request *http.Request) {
	h.roleDetails(writer, request, true)
}

func (h *Handler) roleDetails(writer http.ResponseWriter, request *http.Request, deleteRole bool) {
	h.withPanel(writer, request, "roles", "", func(actor security.Principal, panel layouts.PanelProps) {
		id, err := roleID(request)
		var role *security.Role
		var catalog []*languages.Language

		if err == nil {
			role, err = h.roleService.FindByID(request.Context(), id)
		}

		if err == nil {
			catalog, err = h.languages.List(request.Context())
		}

		if err != nil {
			if deleteRole && request.Header.Get("HX-Request") == "true" {
				status, message := roleError(request, err)
				h.renderRoleDeleteError(writer, request, "", status, message)
			} else {
				h.renderRoleError(writer, request, panel, err)
			}
		} else {
			props := pages.RoleViewProps{Panel: panel, Role: roleRow(request.Context(), role, actor, languages.LanguageCode(request.PathValue("language")), catalog)}
			status, responded := http.StatusOK, false
			var deleteMessage string

			if deleteRole && request.Method == http.MethodPost {
				err = parseRolePost(writer, request)

				if err == nil && (len(request.PostForm) != 1 || len(request.PostForm["confirm"]) != 1 || request.PostForm.Get("confirm") != "delete") {
					err = errInvalidRoleForm
				}

				if err == nil {
					err = h.roleService.Delete(request.Context(), id)
				}

				if err == nil {
					message := admini18n.T(request.Context(), "roles.deleted", map[string]any{"Name": props.Role.Name})

					if flashErr := h.addFlash(request.Context(), writer, request, flashmsg.Success, message); flashErr != nil {
						h.flashUnavailable(writer, request, flashErr)
					} else if request.Header.Get("HX-Request") == "true" {
						writer.Header().Set("HX-Redirect", panel.BasePath+"/roles")
					} else {
						http.Redirect(writer, request, panel.BasePath+"/roles", http.StatusSeeOther)
					}

					responded = true
				} else {
					status, deleteMessage = roleError(request, err)
				}
			}

			if !responded {
				if deleteRole && request.Header.Get("HX-Request") == "true" {
					h.renderRoleDeleteError(writer, request, props.Role.Name, status, deleteMessage)
				} else {
					if deleteMessage != "" {
						err = h.addFlash(request.Context(), writer, request, flashmsg.Error, deleteMessage)

						if err == nil {
							var bag *flashmsg.Bag
							bag, err = h.readFlashes(request.Context(), request, flashmsg.Error)

							if err == nil {
								props.DeleteErrors = bag.Get(flashmsg.Error)
							}
						}
					}

					if err != nil {
						h.flashUnavailable(writer, request, err)
					} else {
						h.renderPage(writer, request, props.Role.Name+" · Faryen "+admini18n.T(request.Context(), "common.admin"), pages.RoleView(props), templ.WithStatus(status))
					}
				}
			}
		}
	})
}

func (h *Handler) renderRoleDeleteError(writer http.ResponseWriter, request *http.Request, name string, status int, message string) {
	err := h.addFlash(request.Context(), writer, request, flashmsg.Error, message)
	var bag *flashmsg.Bag

	if err == nil {
		bag, err = h.readFlashes(request.Context(), request, flashmsg.Error)
	}

	if err != nil {
		h.flashUnavailable(writer, request, err)
	} else {
		writer.Header().Add("Vary", "HX-Request")
		writer.Header().Set("HX-Retarget", "#confirm-delete")
		writer.Header().Set("HX-Reswap", "outerHTML")
		templ.Handler(components.ConfirmDelete(components.ConfirmDeleteProps{
			ID: "confirm-delete", Title: admini18n.T(request.Context(), "roles.delete"), Name: name, Action: request.URL.Path, Errors: bag.Get(flashmsg.Error),
			Message: admini18n.T(request.Context(), "roles.delete_message"),
		}), templ.WithStatus(status)).ServeHTTP(writer, request)
	}
}

func roleID(request *http.Request) (security.RoleID, error) {
	id, err := uuid.Parse(request.PathValue("role"))

	if err != nil || id == uuid.Nil {
		err = security.ErrInvalidRoleID
	}

	return security.RoleID(id), err
}

func roleNameTranslations(ctx context.Context, catalog []*languages.Language, names map[string]string, language string) components.TranslationsInputProps {
	props := components.TranslationsInputProps{
		ID: "role-name", Name: "name", Label: admini18n.T(ctx, "fields.name"), Language: language, MaxLength: 500,
		Help:   admini18n.T(ctx, "roles.name_help"),
		Values: make([]components.TranslationInputValue, 0, len(catalog)),
	}

	for _, item := range catalog {
		code := string(item.Code())
		props.Values = append(props.Values, components.TranslationInputValue{Code: code, Language: string(item.NativeName()), Value: names[code]})
	}

	return props
}

func permissionLabel(ctx context.Context, permission security.Permission) string {
	return admini18n.T(ctx, "permissions."+string(permission))
}

func permissionOptions(ctx context.Context, selected []security.Permission) []pages.PermissionOption {
	options := make([]pages.PermissionOption, 0, len(security.AllPermissions()))

	for _, permission := range security.AllPermissions() {
		options = append(options, pages.PermissionOption{Value: permission, Label: permissionLabel(ctx, permission), Selected: slices.Contains(selected, permission)})
	}

	return options
}

func roleRow(ctx context.Context, role *security.Role, actor security.Principal, code languages.LanguageCode, catalog []*languages.Language) pages.RoleRow {
	name := role.Name()
	translation, found := name.Translation(code)

	if !found {
		for _, language := range catalog {
			if language.IsFallback() {
				translation, found = name.Translation(language.Code())
			}
		}
	}

	if !found && len(name) > 0 {
		translation = name.Translations()[0]
	}

	row := pages.RoleRow{ID: uuid.UUID(role.ID()).String(), Name: translation.Content(), IsSuper: role.IsSuper(), Current: role.ID() == actor.RoleID}

	for _, permission := range security.AllPermissions() {
		if slices.Contains(role.Permissions(), permission) {
			row.Permissions = append(row.Permissions, permissionLabel(ctx, permission))
		}
	}

	return row
}

func (h *Handler) renderRoleError(writer http.ResponseWriter, request *http.Request, panel layouts.PanelProps, err error) {
	status, message := roleError(request, err)

	if flashErr := h.addFlash(request.Context(), writer, request, flashmsg.Error, message); flashErr != nil {
		h.flashUnavailable(writer, request, flashErr)
	} else {
		h.renderPage(writer, request, admini18n.T(request.Context(), "navigation.roles")+" · Faryen "+admini18n.T(request.Context(), "common.admin"), pages.PanelSection(panel, ""), templ.WithStatus(status))
	}
}

func roleError(request *http.Request, err error) (int, string) {
	status, message := http.StatusInternalServerError, admini18n.T(request.Context(), "errors.roles")

	switch {
	case errors.Is(err, security.ErrRoleNotFound):
		status, message = http.StatusNotFound, admini18n.T(request.Context(), "errors.role_not_found")
	case errors.Is(err, security.ErrRoleAlreadyInUse):
		status, message = http.StatusConflict, admini18n.T(request.Context(), "errors.role_used")
	case errors.Is(err, security.ErrRoleAlreadyExists):
		status, message = http.StatusConflict, admini18n.T(request.Context(), "errors.role_exists")
	case errors.Is(err, security.ErrInvalidRoleID), errors.Is(err, errInvalidRoleForm):
		status, message = http.StatusBadRequest, admini18n.T(request.Context(), "errors.role_form")
	case errors.Is(err, security.ErrInvalidRoleQuery):
		status, message = http.StatusBadRequest, admini18n.T(request.Context(), "errors.role_query")
	case errors.Is(err, input.ErrInvalidCreateRoleInput), errors.Is(err, input.ErrInvalidUpdateRoleInput):
		status, message = http.StatusUnprocessableEntity, admini18n.T(request.Context(), "errors.role_values")
	case errors.Is(err, languages.ErrLanguageNotFound):
		status, message = http.StatusUnprocessableEntity, admini18n.T(request.Context(), "errors.language_missing")
	}

	if status == http.StatusInternalServerError {
		slog.ErrorContext(request.Context(), "admin role operation", "error", err)
	}

	return status, message
}
