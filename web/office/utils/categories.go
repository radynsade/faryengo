package utils

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
)

//
// Paths
//

// CategoriesSection is the menu item of the workspace's categories.

const CategoriesSection = "categories"

func CategoriesPath(request *http.Request, workspaceID workspaces.WorkspaceID) string {
	return WorkspacePath(request, workspaceID) + "/categories"
}

func CategoryPath(request *http.Request, workspaceID workspaces.WorkspaceID, id categories.CategoryID) string {
	return CategoriesPath(request, workspaceID) + "/" + uuid.UUID(id).String()
}

//
// Category form
//

// The name's limits and the known icons and colors belong to the domain; the
// transport bounds only the size of each field and of the whole form.

const maxCategoryFormBytes = 8 << 10

type CategoryForm struct {
	Name  string `form:"name" validate:"maxbytes=400"`
	Icon  string `form:"icon" validate:"maxbytes=64"`
	Color string `form:"color" validate:"maxbytes=32"`
}

func ParseCategoryForm(writer http.ResponseWriter, request *http.Request) (CategoryForm, error) {
	var form CategoryForm

	err := parseForm(writer, request, maxCategoryFormBytes, "name", "icon", "color")

	if err == nil {
		form = CategoryForm{
			Name:  request.PostForm.Get("name"),
			Icon:  request.PostForm.Get("icon"),
			Color: request.PostForm.Get("color"),
		}

		err = requestvalidation.Validate(request.Context(), form)
	}

	return form, err
}

// CategoryFieldErrors maps a rejected category to translated messages keyed by
// form field name; a failure that belongs to no field yields none. A missing
// color is required rather than invalid, because the form always offers one.

func CategoryFieldErrors(ctx context.Context, err error) map[string]string {
	fields := make(map[string]string)

	addTransportErrors(ctx, fields, err)

	if _, found := fields["name"]; !found && errors.Is(err, categories.ErrCategoryNameInvalid) {
		switch {
		case errors.Is(err, categories.ErrCategoryNameEmpty):
			fields["name"] = officei18n.T(ctx, "validation.required")
		case errors.Is(err, categories.ErrCategoryNameTooLong):
			fields["name"] = officei18n.Count(ctx, "validation.too_long_chars", categories.MaxCategoryNameLength)
		default:
			fields["name"] = officei18n.T(ctx, "validation.invalid")
		}
	}

	if _, found := fields["icon"]; !found && errors.Is(err, categories.ErrIconInvalid) {
		fields["icon"] = officei18n.T(ctx, "validation.invalid")
	}

	if _, found := fields["color"]; !found && errors.Is(err, categories.ErrColorInvalid) {
		fields["color"] = officei18n.T(ctx, "validation.choose_one")
	}

	return nilIfEmpty(fields)
}

// A deletion carries the category in its address; its only field is the
// dialog's confirmation, without which nothing is deleted.

func ParseDeleteCategoryForm(writer http.ResponseWriter, request *http.Request) (bool, error) {
	var confirmed bool

	err := parseForm(writer, request, maxCategoryFormBytes, "confirm")

	if err == nil {
		confirmed = request.PostForm.Get("confirm") == "1"
	}

	return confirmed, err
}

// A category ID that is not a UUID, or is nil, names no category, so it is
// reported like a missing one.

func ParseCategoryID(request *http.Request) (categories.CategoryID, error) {
	var id categories.CategoryID

	parsed, err := uuid.Parse(request.PathValue("category"))

	if err == nil {
		id, err = categories.NewCategoryID(parsed)
	}

	if err != nil {
		id = categories.CategoryID{}
		err = errors.Join(categories.ErrCategoryNotFound, err)
	}

	return id, err
}

//
// Choices
//

// Colors and icons are offered in the domain's order; the first choice of
// icons is none.

func CategoryColorChoices(ctx context.Context) []pages.CategoryChoice {
	colors := categories.Colors()
	choices := make([]pages.CategoryChoice, 0, len(colors))

	for _, color := range colors {
		choices = append(choices, pages.CategoryChoice{Value: string(color), Label: ColorLabel(ctx, color)})
	}

	return choices
}

func CategoryIconChoices(ctx context.Context) []pages.CategoryChoice {
	icons := categories.Icons()
	choices := make([]pages.CategoryChoice, 0, len(icons)+1)
	choices = append(choices, pages.CategoryChoice{Value: "", Label: officei18n.T(ctx, "categories.no_icon")})

	for _, icon := range icons {
		choices = append(choices, pages.CategoryChoice{Value: string(icon), Label: IconLabel(ctx, icon)})
	}

	return choices
}

func ColorLabel(ctx context.Context, color categories.Color) string {
	return officei18n.T(ctx, "category_colors."+string(color))
}

func IconLabel(ctx context.Context, icon categories.Icon) string {
	return officei18n.T(ctx, "category_icons."+string(icon))
}

//
// Categories
//

// CategoriesPageSize bounds one page of the list.

const CategoriesPageSize = 50

// The page comes from the address; a missing or malformed one is the first.

func CategoriesQuery(request *http.Request) categories.CategoryQuery {
	page, err := strconv.ParseUint(request.URL.Query().Get("page"), 10, 32)

	if err != nil || page == 0 {
		page = 1
	}

	return categories.NewCategoryQuery(
		categories.NewCategoryFilter("", "", nil),
		true,
		categories.CategorySortName,
		CategoriesPageSize,
		uint(page),
	)
}

// Members who may edit the content are offered the page that adds a
// category.

func CategoriesContentProps(
	request *http.Request,
	found app.MemberWorkspace,
	notices []string,
) pages.CategoriesContentProps {
	props := pages.CategoriesContentProps{Notices: notices}

	if found.Role.Allows(workspaces.ActionEditContent) {
		props.NewURL = CategoriesPath(request, found.Workspace.ID) + "/new"
	}

	return props
}

// A new category's form starts from the first color and no icon, and posts to
// the list, which it leads back to.

func NewCategoryFormProps(request *http.Request, workspaceID workspaces.WorkspaceID) pages.CategoryFormProps {
	ctx := request.Context()

	return pages.CategoryFormProps{
		Action:    CategoriesPath(request, workspaceID),
		Color:     string(categories.Colors()[0]),
		Colors:    CategoryColorChoices(ctx),
		Icons:     CategoryIconChoices(ctx),
		Submit:    officei18n.T(ctx, "categories.create_submit"),
		CancelURL: CategoriesPath(request, workspaceID),
	}
}

func EditCategoryContentProps(
	request *http.Request,
	category *categories.Category,
) pages.EditCategoryContentProps {
	ctx := request.Context()
	url := CategoryPath(request, category.WorkspaceID, category.ID)
	icon := ""

	if category.Icon != nil {
		icon = string(*category.Icon)
	}

	return pages.EditCategoryContentProps{
		Form: pages.CategoryFormProps{
			Action:    url,
			Name:      string(category.Name),
			Icon:      icon,
			Color:     string(category.Color),
			Colors:    CategoryColorChoices(ctx),
			Icons:     CategoryIconChoices(ctx),
			Submit:    officei18n.T(ctx, "categories.save_submit"),
			CancelURL: CategoriesPath(request, category.WorkspaceID),
		},
		DeleteURL: url + "/delete",
		Name:      string(category.Name),
	}
}

func CategoryItems(
	request *http.Request,
	found []*categories.Category,
	canEdit bool,
) []pages.CategoryItem {
	ctx := request.Context()
	items := make([]pages.CategoryItem, 0, len(found))

	for _, category := range found {
		item := pages.CategoryItem{
			ID:    uuid.UUID(category.ID).String(),
			Name:  string(category.Name),
			Color: string(category.Color),
		}

		if category.Icon != nil {
			item.Icon = string(*category.Icon)
			item.IconLabel = IconLabel(ctx, *category.Icon)
		}

		if canEdit {
			item.Href = CategoryPath(request, category.WorkspaceID, category.ID)
		}

		items = append(items, item)
	}

	return items
}

// The pages are numbered from one; a list that fits on one page has no
// pagination.

func CategoriesPagination(
	request *http.Request,
	workspaceID workspaces.WorkspaceID,
	query categories.CategoryQuery,
	total int,
) pages.PaginationProps {
	var props pages.PaginationProps

	pageCount := (total + int(query.Limit) - 1) / int(query.Limit)
	page := int(query.Page)

	if pageCount > 1 {
		base := CategoriesPath(request, workspaceID)
		props.Summary = officei18n.T(request.Context(), "pagination.summary", map[string]any{
			"Page":  page,
			"Pages": pageCount,
		})

		if page > 1 {
			props.PreviousURL = pageURL(base, min(page-1, pageCount))
		}

		if page < pageCount {
			props.NextURL = pageURL(base, page+1)
		}
	}

	return props
}

// The first page has the list's own address.

func pageURL(base string, page int) string {
	result := base

	if page > 1 {
		result += "?" + url.Values{"page": {strconv.Itoa(page)}}.Encode()
	}

	return result
}
