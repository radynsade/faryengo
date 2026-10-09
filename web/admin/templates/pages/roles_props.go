package pages

import (
	"context"
	"net/url"
	"strconv"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/pkg/domquery"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
)

//
// Role values
//

type PermissionOption struct {
	Value    users.Permission
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

//
// Role list
//

type RoleListProps struct {
	Panel        layouts.PanelProps
	Rows         []RoleRow
	Query        users.RoleQuery
	Total        int
	Permissions  []PermissionOption
	CanManage    bool
	InvalidQuery bool
	Loading      bool
	Errors       []string
	FieldErrors  components.FieldErrors
}

func (p RoleListProps) Pages() int {
	pages := 1

	if p.Query.Limit > 0 && p.Total > 0 {
		pages = (p.Total + int(p.Query.Limit) - 1) / int(p.Query.Limit)
	}

	return pages
}

func (p RoleListProps) URL(page int, sort users.RoleSort, order domquery.SortOrder) string {
	return p.Panel.BasePath + "/roles?" + p.values(page, sort, order).Encode()
}

func (p RoleListProps) CurrentURL() string {
	return p.URL(int(p.Query.Page), p.Query.SortBy, p.Query.SortOrder)
}

func (p RoleListProps) TableURL() string {
	return p.Panel.BasePath + "/roles/table?" + p.values(int(p.Query.Page), p.Query.SortBy, p.Query.SortOrder).Encode()
}

// Sorting by the current column again reverses the order; another column
// starts ascending.

func (p RoleListProps) SortURL(sort users.RoleSort) string {
	order := domquery.SortOrder(domquery.SortOrderAsc)

	if p.Query.SortBy == sort && p.Query.SortOrder.IsAsc() {
		order = domquery.SortOrderDesc
	}

	return p.URL(1, sort, order)
}

func (p RoleListProps) SortDirection(sort users.RoleSort) string {
	direction := "none"

	if p.Query.SortBy == sort && p.Query.SortOrder.IsAsc() {
		direction = "ascending"
	} else if p.Query.SortBy == sort {
		direction = "descending"
	}

	return direction
}

func (p RoleListProps) values(page int, sort users.RoleSort, order domquery.SortOrder) url.Values {
	values := url.Values{
		"page":  {strconv.Itoa(page)},
		"size":  {strconv.FormatUint(uint64(p.Query.Limit), 10)},
		"sort":  {string(sort)},
		"order": {SortOrderValue(order)},
	}

	if p.Query.Filter.IDLike != "" {
		values.Set("uuid", p.Query.Filter.IDLike)
	}

	if p.Query.Filter.NameLike != "" {
		values.Set("name", p.Query.Filter.NameLike)
	}

	if p.Query.Filter.IsSuper != nil {
		values.Set("super", strconv.FormatBool(*p.Query.Filter.IsSuper))
	}

	for _, permission := range p.Query.Filter.Permissions {
		values.Add("permissions", string(permission))
	}

	return values
}

func SortOrderValue(order domquery.SortOrder) string {
	value := "desc"

	if order.IsAsc() {
		value = "asc"
	}

	return value
}

//
// Role form
//

// RoleFormID names the region a failed in-page submission replaces.

const RoleFormID = "role-form"

type RoleFormProps struct {
	Panel            layouts.PanelProps
	ID               string
	Name             string
	Title            string
	Action           string
	NameTranslations components.TranslationsInputProps
	Permissions      []PermissionOption
	IsSuper          bool
	Errors           []string
	FieldErrors      components.FieldErrors
}

//
// Role view
//

type RoleViewProps struct {
	Panel        layouts.PanelProps
	Role         RoleRow
	CanManage    bool
	DeleteErrors []string
}

//
// Helpers
//

func RoleDeleteDialog(ctx context.Context, action, name string, errors []string) components.ConfirmDeleteProps {
	return components.ConfirmDeleteProps{
		Title:   admini18n.T(ctx, "roles.delete_title", map[string]any{"Name": name}),
		Message: admini18n.T(ctx, "roles.delete_message"),
		Action:  action,
		Errors:  errors,
	}
}

func permissionSelectProps(
	ctx context.Context,
	id string,
	label string,
	options []PermissionOption,
	errors []string,
) components.MultiSelectProps {
	values := make([]components.MultiSelectOption, 0, len(options))

	for _, option := range options {
		values = append(values, components.MultiSelectOption{
			Value:    string(option.Value),
			Label:    option.Label,
			Selected: option.Selected,
		})
	}

	return components.MultiSelectProps{
		ID:          id,
		Name:        "permissions",
		Label:       label,
		Placeholder: admini18n.T(ctx, "fields.select_permissions"),
		Options:     values,
		Errors:      errors,
	}
}
