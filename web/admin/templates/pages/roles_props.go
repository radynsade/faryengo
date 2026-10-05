package pages

import (
	"context"
	"net/url"
	"strconv"

	"github.com/radynsade/faryengo/internal/security"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
)

type PermissionOption struct {
	Value    security.Permission
	Label    string
	Selected bool
}

type RoleRow struct {
	ID          string
	Name        string
	IsSuper     bool
	Permissions []string
	Current     bool
}

type RoleListProps struct {
	Panel        layouts.PanelProps
	Rows         []RoleRow
	Query        security.RoleQuery
	Page         security.RolePage
	Permissions  []PermissionOption
	InvalidQuery bool
}

func (p RoleListProps) URL(page int, sort security.RoleSort, descending bool) string {
	values := url.Values{"page": {strconv.Itoa(page)}, "size": {strconv.Itoa(p.Query.PageSize)}, "sort": {string(sort)}, "order": {"asc"}}

	if descending {
		values.Set("order", "desc")
	}

	if p.Query.Filters.IDLike != "" {
		values.Set("uuid", p.Query.Filters.IDLike)
	}

	if p.Query.Filters.NameLike != "" {
		values.Set("name", p.Query.Filters.NameLike)
	}

	if p.Query.Filters.IsSuper != nil {
		values.Set("super", strconv.FormatBool(*p.Query.Filters.IsSuper))
	}

	for _, permission := range p.Query.Filters.Permissions {
		values.Add("permissions", string(permission))
	}

	return p.Panel.BasePath + "/roles?" + values.Encode()
}

func (p RoleListProps) SortURL(sort security.RoleSort) string {
	return p.URL(1, sort, p.Query.Sort == sort && !p.Query.Descending)
}

func (p RoleListProps) SortDirection(sort security.RoleSort) string {
	direction := "none"

	if p.Query.Sort == sort {
		direction = "ascending"

		if p.Query.Descending {
			direction = "descending"
		}
	}

	return direction
}

type RoleNameField struct {
	Code  string
	Label string
	Value string
}

type RoleFormProps struct {
	Panel            layouts.PanelProps
	ID               string
	Name             string
	Title            string
	Action           string
	NameTranslations components.TranslationsInputProps
	Permissions      []PermissionOption
	IsSuper          bool
}

type RoleViewProps struct {
	Panel        layouts.PanelProps
	Role         RoleRow
	Names        []RoleNameField
	DeleteErrors []string
}

func roleDeleteDialog(ctx context.Context, action, name string, errors []string) components.ConfirmDeleteProps {
	return components.ConfirmDeleteProps{
		ID: "confirm-delete", Title: admini18n.T(ctx, "roles.delete"), Name: name, Action: action, Errors: errors,
		Message: admini18n.T(ctx, "roles.delete_message"),
	}
}

func permissionSelectProps(ctx context.Context, id, label string, options []PermissionOption) components.MultiSelectProps {
	values := make([]components.MultiSelectOption, 0, len(options))

	for _, option := range options {
		values = append(values, components.MultiSelectOption{
			Value: string(option.Value), Label: option.Label, Selected: option.Selected,
		})
	}

	return components.MultiSelectProps{
		ID: id, Name: "permissions", Label: label, Placeholder: admini18n.T(ctx, "fields.select_permissions"), Options: values,
	}
}
