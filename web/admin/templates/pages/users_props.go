package pages

import (
	"context"
	"net/url"
	"strconv"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/pkg/domquery"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
)

//
// User values
//

type RoleOption struct {
	Value    string
	Label    string
	Selected bool
}

type UserRow struct {
	ID        string
	FirstName string
	LastName  string
	Email     string
	Phone     string
	RoleID    string
	RoleName  string
	CreatedAt string
	UpdatedAt string
	Current   bool
}

func (r UserRow) FullName(ctx context.Context) string {
	return admini18n.T(ctx, "common.full_name", map[string]any{"FirstName": r.FirstName, "LastName": r.LastName})
}

//
// User list
//

type UserListProps struct {
	Panel        layouts.PanelProps
	Rows         []UserRow
	Query        users.UserQuery
	Total        int
	Roles        []RoleOption
	CanManage    bool
	CanViewRoles bool
	InvalidQuery bool
	Loading      bool
	Errors       []string
	FieldErrors  components.FieldErrors
}

func (p UserListProps) Pages() int {
	pages := 1

	if p.Query.Limit > 0 && p.Total > 0 {
		pages = (p.Total + int(p.Query.Limit) - 1) / int(p.Query.Limit)
	}

	return pages
}

func (p UserListProps) URL(page int, sort users.UserSort, order domquery.SortOrder) string {
	return p.Panel.BasePath + "/users?" + p.values(page, sort, order).Encode()
}

func (p UserListProps) CurrentURL() string {
	return p.URL(int(p.Query.Page), p.Query.SortBy, p.Query.SortOrder)
}

func (p UserListProps) TableURL() string {
	return p.Panel.BasePath + "/users/table?" + p.values(int(p.Query.Page), p.Query.SortBy, p.Query.SortOrder).Encode()
}

// Sorting by the current column again reverses the order; another column
// starts ascending.

func (p UserListProps) SortURL(sort users.UserSort) string {
	order := domquery.SortOrder(domquery.SortOrderAsc)

	if p.Query.SortBy == sort && p.Query.SortOrder.IsAsc() {
		order = domquery.SortOrderDesc
	}

	return p.URL(1, sort, order)
}

func (p UserListProps) SortDirection(sort users.UserSort) string {
	direction := "none"

	if p.Query.SortBy == sort && p.Query.SortOrder.IsAsc() {
		direction = "ascending"
	} else if p.Query.SortBy == sort {
		direction = "descending"
	}

	return direction
}

func (p UserListProps) values(page int, sort users.UserSort, order domquery.SortOrder) url.Values {
	values := url.Values{
		"page":  {strconv.Itoa(page)},
		"size":  {strconv.FormatUint(uint64(p.Query.Limit), 10)},
		"sort":  {string(sort)},
		"order": {SortOrderValue(order)},
	}

	if p.Query.Filter.IDLike != "" {
		values.Set("uuid", p.Query.Filter.IDLike)
	}

	if p.Query.Filter.EmailLike != "" {
		values.Set("email", p.Query.Filter.EmailLike)
	}

	if p.Query.Filter.NameLike != "" {
		values.Set("name", p.Query.Filter.NameLike)
	}

	if p.Query.Filter.RoleID != nil {
		values.Set("role", uuid.UUID(*p.Query.Filter.RoleID).String())
	}

	return values
}

//
// User form
//

// UserFormID names the region a failed in-page submission replaces.

const UserFormID = "user-form"

type UserFormProps struct {
	Panel       layouts.PanelProps
	ID          string
	Name        string
	Title       string
	Action      string
	FirstName   string
	LastName    string
	Email       string
	Phone       string
	Roles       []RoleOption
	UpdatedAt   string
	Current     bool
	Errors      []string
	FieldErrors components.FieldErrors
}

//
// User view
//

type UserViewProps struct {
	Panel        layouts.PanelProps
	User         UserRow
	CanManage    bool
	CanViewRoles bool
	DeleteErrors []string
}

//
// Helpers
//

func UserDeleteDialog(ctx context.Context, action, name string, errors []string) components.ConfirmDeleteProps {
	return components.ConfirmDeleteProps{
		Title:   admini18n.T(ctx, "users.delete_title", map[string]any{"Name": name}),
		Message: admini18n.T(ctx, "users.delete_message"),
		Action:  action,
		Errors:  errors,
	}
}

func hasSelectedRole(options []RoleOption) bool {
	selected := false

	for _, option := range options {
		selected = selected || option.Selected
	}

	return selected
}
