package utils

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	"github.com/radynsade/faryengo/pkg/domquery"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

//
// Role query
//

const DefaultRolePageSize = 20

// The query string carries the filters, sorting, and page, so every list
// state has its own address. The limits match the domain's role filter.

type roleQueryParameters struct {
	IDLike      string   `form:"uuid" validate:"utf8,maxbytes=36,excludesall=0x00"`
	NameLike    string   `form:"name" validate:"utf8,maxbytes=100,excludesall=0x00"`
	Permissions []string `form:"permissions" validate:"dive,oneof=manage_user view_user manage_role view_role"`
	Super       string   `form:"super" validate:"omitempty,oneof=true false"`
	Sort        string   `form:"sort" validate:"oneof=id name is_super"`
	Order       string   `form:"order" validate:"oneof=asc desc"`
	Page        int      `form:"page" validate:"min=1,max=1000000"`
	Size        int      `form:"size" validate:"min=1,max=100"`
}

var roleFilterFields = []string{"uuid", "name", "permissions", "super"}

// The parsed query keeps the submitted filters even when validation fails, so
// the filter form shows them beside their errors.

func ParseRoleQuery(request *http.Request) (users.RoleQuery, error) {
	parameters := roleQueryParameters{
		Sort:  string(users.RoleSortID),
		Order: "asc",
		Page:  1,
		Size:  DefaultRolePageSize,
	}

	values, err := url.ParseQuery(request.URL.RawQuery)

	if err == nil {
		err = checkFields(values, []string{"uuid", "name", "super", "sort", "order", "page", "size"}, []string{"permissions"})
	}

	if err == nil {
		parameters.IDLike = strings.TrimSpace(values.Get("uuid"))
		parameters.NameLike = strings.TrimSpace(values.Get("name"))
		parameters.Permissions = values["permissions"]
		parameters.Super = values.Get("super")

		for _, parameter := range []struct {
			key    string
			target *string
		}{{"sort", &parameters.Sort}, {"order", &parameters.Order}} {
			if value := values.Get(parameter.key); value != "" {
				*parameter.target = value
			}
		}

		for _, parameter := range []struct {
			key    string
			target *int
		}{{"page", &parameters.Page}, {"size", &parameters.Size}} {
			if value := values.Get(parameter.key); value != "" && err == nil {
				*parameter.target, err = strconv.Atoi(value)
			}
		}
	}

	if err == nil {
		err = requestvalidation.Validate(request.Context(), parameters)
	}

	query := users.RoleQuery{
		Filter: users.RoleFilter{
			IDLike:   parameters.IDLike,
			NameLike: parameters.NameLike,
		},
		SortBy:    users.RoleSort(parameters.Sort),
		SortOrder: domquery.SortOrder(parameters.Order == "asc"),
		Limit:     uint(max(parameters.Size, 0)),
		Page:      uint(max(parameters.Page, 0)),
	}

	for _, permission := range parameters.Permissions {
		query.Filter.Permissions = append(query.Filter.Permissions, users.Permission(permission))
	}

	if parameters.Super == "true" || parameters.Super == "false" {
		isSuper := parameters.Super == "true"
		query.Filter.IsSuper = &isSuper
	}

	if err != nil {
		err = fmt.Errorf("parse a role query: %w: %w", users.ErrInvalidRoleQuery, err)
	}

	return query, err
}

// Links built from a rejected query must still be valid, so its sorting and
// paging fall back to the defaults while its filters stay.

func SanitizeRoleQuery(query users.RoleQuery) users.RoleQuery {
	if query.SortBy.Validate() != nil {
		query.SortBy = users.RoleSortID
	}

	if query.Limit == 0 || query.Limit > 100 {
		query.Limit = DefaultRolePageSize
	}

	query.Page = 1
	query.Filter.Permissions = slices.DeleteFunc(slices.Clone(query.Filter.Permissions), func(permission users.Permission) bool {
		return permission.Validate() != nil
	})

	return query
}

// Only the request's own validation produces filter field errors; a failure
// to load the list says nothing about the submitted filters.

func FilterFieldErrors(ctx context.Context, err error) components.FieldErrors {
	var (
		fields   components.FieldErrors
		all      components.FieldErrors
		failures *requestvalidation.Errors
	)

	if errors.As(err, &failures) {
		all = FieldErrors(ctx, failures)
	}

	for _, key := range roleFilterFields {
		if len(all[key]) > 0 {
			if fields == nil {
				fields = make(components.FieldErrors)
			}

			fields[key] = all[key]
		}
	}

	// A filter failure outside the filter fields is reported as a whole.
	if len(fields) != len(all) {
		fields = nil
	}

	return fields
}

//
// Role errors
//

func RoleError(request *http.Request, err error) (int, string) {
	status, messageID := http.StatusInternalServerError, "errors.roles"

	switch {
	case errors.Is(err, users.ErrRoleNotFound):
		status, messageID = http.StatusNotFound, "errors.role_not_found"
	case errors.Is(err, users.ErrRoleAlreadyInUse):
		status, messageID = http.StatusConflict, "errors.role_used"
	case errors.Is(err, users.ErrRoleAlreadyExists):
		status, messageID = http.StatusConflict, "errors.role_exists"
	case errors.Is(err, users.ErrRoleIDInvalid), errors.Is(err, ErrFormInvalid):
		status, messageID = http.StatusBadRequest, "errors.role_form"
	case errors.Is(err, users.ErrInvalidRoleQuery), errors.Is(err, users.ErrRoleFilterInvalid):
		status, messageID = http.StatusBadRequest, "errors.role_query"
	case errors.Is(err, languages.ErrLanguageNotFound):
		status, messageID = http.StatusUnprocessableEntity, "errors.language_missing"
	case errors.Is(err, requestvalidation.ErrInvalidRequest),
		errors.Is(err, input.ErrCreateRoleInputInvalid),
		errors.Is(err, input.ErrUpdateRoleInputInvalid):
		status, messageID = http.StatusUnprocessableEntity, "errors.role_values"
	}

	if status == http.StatusInternalServerError {
		slog.ErrorContext(request.Context(), "admin role operation", "error", err)
	}

	return status, messageID
}

//
// Role values
//

// A Role's name is shown in the interface language, then in the fallback
// language, then in the first language by code.

func RoleName(role *users.Role, code string, catalog []*languages.Language) string {
	translation, found := role.Name[languages.Code(code)]

	for _, language := range catalog {
		if !found && language.IsFallback {
			translation, found = role.Name[language.Code]
		}
	}

	if codes := slices.Sorted(maps.Keys(role.Name)); !found && len(codes) > 0 {
		translation = role.Name[codes[0]]
	}

	return string(translation)
}

func RoleRow(
	ctx context.Context,
	role *users.Role,
	current *security.Identity,
	code string,
	catalog []*languages.Language,
) pages.RoleRow {
	row := pages.RoleRow{
		ID:      uuid.UUID(role.ID).String(),
		Name:    RoleName(role, code, catalog),
		IsSuper: role.IsSuper,
		Current: current != nil && role.ID == current.Role.ID,
	}

	for _, permission := range users.AllPermissions() {
		if slices.Contains(role.Permissions, permission) {
			row.Permissions = append(row.Permissions, permissionLabel(ctx, permission))
		}
	}

	return row
}

func FormFromRole(role *users.Role) RoleForm {
	form := RoleForm{Name: make(map[string]string, len(role.Name)), Permissions: []string{}}

	for code, translation := range role.Name {
		form.Name[string(code)] = string(translation)
	}

	for _, permission := range role.Permissions {
		form.Permissions = append(form.Permissions, string(permission))
	}

	if role.IsSuper {
		form.IsSuper = "1"
	}

	return form
}

func PermissionsOf(values []string) users.Permissions {
	permissions := make(users.Permissions, 0, len(values))

	for _, value := range values {
		permissions = append(permissions, users.Permission(value))
	}

	return permissions
}

func permissionLabel(ctx context.Context, permission users.Permission) string {
	return admini18n.T(ctx, "permissions."+string(permission))
}

func PermissionOptions(ctx context.Context, selected users.Permissions) []pages.PermissionOption {
	options := make([]pages.PermissionOption, 0, len(users.AllPermissions()))

	for _, permission := range users.AllPermissions() {
		options = append(options, pages.PermissionOption{
			Value:    permission,
			Label:    permissionLabel(ctx, permission),
			Selected: slices.Contains(selected, permission),
		})
	}

	return options
}

func RoleNameTranslations(
	ctx context.Context,
	catalog []*languages.Language,
	names map[string]string,
	code string,
	fields components.FieldErrors,
) components.TranslationsInputProps {
	props := components.TranslationsInputProps{
		ID:        "role-name",
		Name:      "name",
		Label:     admini18n.T(ctx, "fields.name"),
		Language:  code,
		Help:      admini18n.T(ctx, "roles.name_help"),
		MaxLength: users.MaxRoleNameLength,
		Values:    make([]components.TranslationInputValue, 0, len(catalog)),
		Errors:    fields["name"],
	}

	for _, language := range catalog {
		languageCode := string(language.Code)

		if language.IsFallback {
			props.Help = admini18n.T(ctx, "roles.name_help_fallback", map[string]any{"Language": string(language.NativeName)})
		}

		props.Values = append(props.Values, components.TranslationInputValue{
			Code:     languageCode,
			Language: string(language.NativeName),
			Value:    names[languageCode],
			Required: language.IsFallback,
			Errors:   fields["name["+languageCode+"]"],
		})
	}

	return props
}

// A missing fallback translation belongs to the fallback language's field,
// which only the catalog can name, so it is added to the generic field errors.

func RoleFormFieldErrors(
	ctx context.Context,
	err error,
	catalog []*languages.Language,
) components.FieldErrors {
	fields := FieldErrors(ctx, err)

	for _, language := range catalog {
		if language.IsFallback && errors.Is(err, users.ErrRoleNameFallbackMissing) {
			if fields == nil {
				fields = make(components.FieldErrors)
			}

			fields["name["+string(language.Code)+"]"] = []string{admini18n.T(ctx, "validation.fallback_translation")}
		}
	}

	return fields
}
