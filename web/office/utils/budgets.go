package utils

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/budgets"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	"github.com/radynsade/faryengo/pkg/domquery"
	"github.com/radynsade/faryengo/pkg/forms"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/components"
	"github.com/radynsade/faryengo/web/office/templates/pages"
)

//
// Paths
//

// BudgetsSection is the menu item of the workspace's budgets.

const BudgetsSection = "budgets"

func BudgetsPath(request *http.Request, workspaceID workspaces.ID) string {
	return WorkspacePath(request, workspaceID) + "/budgets"
}

func BudgetPath(request *http.Request, workspaceID workspaces.ID, id budgets.ID) string {
	return BudgetsPath(request, workspaceID) + "/" + uuid.UUID(id).String()
}

//
// Amounts
//

// An amount is typed in the currency's major units with at most two decimal
// places, after a point or a comma, and is kept in minor units, such as cents.
// Spaces that group the digits are ignored. The whole part is bounded so the
// minor units fit in the domain's amount.

const (
	amountMinorUnits     = 100
	maxAmountWholeDigits = 15
)

var (
	ErrBudgetAmountMissing = errors.New("budget amount is missing")
	ErrBudgetAmountInvalid = errors.New("budget amount is not a number of at most two decimal places")
	amountPattern          = regexp.MustCompile(`^([0-9]{1,15})(?:[.,]([0-9]{1,2}))?$`)
	amountSpaces           = strings.NewReplacer(" ", "", " ", "", " ", "")
)

func ParseAmount(value string) (int64, error) {
	var (
		amount int64
		err    error
	)

	compact := amountSpaces.Replace(strings.TrimSpace(value))
	match := amountPattern.FindStringSubmatch(compact)

	if compact == "" {
		err = ErrBudgetAmountMissing
	} else if match == nil {
		err = ErrBudgetAmountInvalid
	} else {
		whole, _ := strconv.ParseInt(match[1], 10, 64)
		fraction, _ := strconv.ParseInt((match[2] + "00")[:2], 10, 64)
		amount = whole*amountMinorUnits + fraction
	}

	return amount, err
}

// FormatAmount writes an amount in major units with two decimal places,
// separated by the interface language's decimal separator, so the form shows
// a stored amount the way it is typed.

func FormatAmount(ctx context.Context, amount budgets.Amount) string {
	whole := int64(amount) / amountMinorUnits
	fraction := int64(amount) % amountMinorUnits

	return strconv.FormatInt(whole, 10) + officei18n.T(ctx, "budgets.decimal_separator") + strconv.FormatInt(100+fraction, 10)[1:]
}

//
// Budget form
//

// Which categories a budget may cover, which currencies exist, the rules of a
// period, and the amount's sign belong to the domain; the transport bounds the
// size of each field and of the whole form, and reads the period's times and
// the amount. Each chosen category is a repeated field.

const maxBudgetFormBytes = 16 << 10

type BudgetForm struct {
	Categories []string `form:"categories"`
	Currency   string   `form:"currency" validate:"maxbytes=12"`
	Period     PeriodForm
	Amount     string `form:"amount" validate:"maxbytes=64"`
}

var ErrBudgetCategoryInvalid = errors.New("budget category is not a category ID")

func ParseBudgetForm(writer http.ResponseWriter, request *http.Request) (BudgetForm, error) {
	var form BudgetForm

	err := forms.Parse(writer, request, maxBudgetFormBytes)

	if err == nil {
		err = forms.CheckFields(request.PostForm, append([]string{"currency", "amount"}, periodFields...), []string{"categories"})
	}

	if err == nil {
		form = BudgetForm{
			Categories: request.PostForm["categories"],
			Currency:   request.PostForm.Get("currency"),
			Period:     readPeriodForm(request.PostForm),
			Amount:     request.PostForm.Get("amount"),
		}

		err = requestvalidation.Validate(request.Context(), form)
	}

	return form, err
}

// BudgetValues reads the form's categories, period, and amount. A category
// that is not a UUID names none. A form that cannot be read never reaches the
// service, so the categories and the currency are then checked here too, and
// every problem with the form is reported at once.

func BudgetValues(form BudgetForm) ([]categories.ID, input.PeriodInput, int64, error) {
	var (
		ids         = make([]categories.ID, 0, len(form.Categories))
		categoryErr error
	)

	for _, value := range form.Categories {
		parsed, parseErr := uuid.Parse(value)

		if parseErr != nil {
			categoryErr = errors.Join(ErrBudgetCategoryInvalid, parseErr)
		} else {
			ids = append(ids, categories.ID(parsed))
		}
	}

	budgetPeriod, periodErr := PeriodValues(form.Period)
	amount, amountErr := ParseAmount(form.Amount)
	err := errors.Join(categoryErr, periodErr, amountErr)

	if err != nil {
		if categoryErr == nil {
			categoryErr = budgets.NewCategoryIDs(ids).Validate()
		}

		_, currencyErr := currencies.NewCode(form.Currency)
		err = errors.Join(categoryErr, currencyErr, periodErr, amountErr)
		ids, budgetPeriod, amount = nil, input.PeriodInput{}, 0
	}

	return ids, budgetPeriod, amount, err
}

// BudgetFieldErrors maps a rejected budget to translated messages keyed by
// form field name; a failure that belongs to no field yields none. A category
// that is malformed, repeated, or no longer in the workspace is a problem with
// the categories, because the form offers only the workspace's categories. A
// currency that is missing, malformed, or no longer exists is a problem with
// the currency, because the form offers only existing ones.

func BudgetFieldErrors(ctx context.Context, form BudgetForm, err error) map[string]string {
	fields := make(map[string]string)

	addTransportErrors(ctx, fields, err)
	PeriodFieldErrors(ctx, fields, form.Period, err)

	if _, found := fields["categories"]; !found {
		if errors.Is(err, budgets.ErrCategoryIDsEmpty) {
			fields["categories"] = officei18n.T(ctx, "budgets.categories_required")
		} else if errors.Is(err, budgets.ErrCategoryIDsInvalid) || errors.Is(err, ErrBudgetCategoryInvalid) ||
			errors.Is(err, categories.ErrCategoryNotFound) {
			fields["categories"] = officei18n.T(ctx, "validation.invalid")
		}
	}

	if _, found := fields["currency"]; !found &&
		(errors.Is(err, currencies.ErrCodeInvalid) || errors.Is(err, currencies.ErrCurrencyNotFound)) {
		fields["currency"] = officei18n.T(ctx, "validation.choose_one")
	}

	if _, found := fields["amount"]; !found {
		if errors.Is(err, ErrBudgetAmountMissing) {
			fields["amount"] = officei18n.T(ctx, "validation.required")
		} else if errors.Is(err, ErrBudgetAmountInvalid) || errors.Is(err, budgets.ErrAmountInvalid) {
			fields["amount"] = officei18n.T(ctx, "budgets.amount_invalid")
		}
	}

	return nilIfEmpty(fields)
}

// A deletion carries the budget in its address; its only field is the
// dialog's confirmation, without which nothing is deleted.

func ParseDeleteBudgetForm(writer http.ResponseWriter, request *http.Request) (bool, error) {
	var confirmed bool

	err := parseForm(writer, request, maxBudgetFormBytes, "confirm")

	if err == nil {
		confirmed = request.PostForm.Get("confirm") == "1"
	}

	return confirmed, err
}

// A budget ID that is not a UUID, or is nil, names no budget, so it is
// reported like a missing one.

func ParseBudgetID(request *http.Request) (budgets.ID, error) {
	var id budgets.ID

	parsed, err := uuid.Parse(request.PathValue("budget"))

	if err == nil {
		id, err = budgets.NewID(parsed)
	}

	if err != nil {
		id = budgets.ID{}
		err = errors.Join(budgets.ErrBudgetNotFound, err)
	}

	return id, err
}

//
// Categories
//

// BudgetCategoriesQuery reads one page of the workspace's categories in name
// order; the form and the list read every page.

const budgetCategoriesPageSize = 200

func BudgetCategoriesQuery(page uint) categories.CategoryQuery {
	return categories.NewCategoryQuery(
		categories.NewCategoryFilter("", "", nil),
		domquery.SortOrderAsc,
		categories.CategorySortName,
		budgetCategoriesPageSize,
		page,
	)
}

// The workspace's categories are offered in name order; those in selected
// start checked.

func BudgetCategoryChoices(
	list []*categories.Category,
	selected []string,
) []pages.BudgetCategoryChoice {
	choices := make([]pages.BudgetCategoryChoice, 0, len(list))

	for _, category := range list {
		tag := budgetCategoryTag(category)
		choices = append(choices, pages.BudgetCategoryChoice{
			Value:   uuid.UUID(category.ID).String(),
			Tag:     tag,
			Checked: slices.Contains(selected, uuid.UUID(category.ID).String()),
		})
	}

	return choices
}

// BudgetTitle names a budget by its categories, in the order it lists them. A
// category that is not among the known ones, such as one deleted since, is
// named as unknown.

func BudgetTitle(ctx context.Context, budget *budgets.Budget, known []*categories.Category) string {
	names := make([]string, 0, len(budget.Categories))

	for _, tag := range budgetTags(ctx, budget, known) {
		names = append(names, tag.Name)
	}

	return strings.Join(names, officei18n.T(ctx, "budgets.category_separator"))
}

func budgetTags(ctx context.Context, budget *budgets.Budget, known []*categories.Category) []pages.BudgetCategoryTag {
	tags := make([]pages.BudgetCategoryTag, 0, len(budget.Categories))

	for _, id := range budget.Categories {
		index := slices.IndexFunc(known, func(category *categories.Category) bool { return category.ID == id })

		if index >= 0 {
			tags = append(tags, budgetCategoryTag(known[index]))
		} else {
			tags = append(tags, pages.BudgetCategoryTag{Name: officei18n.T(ctx, "budgets.unknown_category")})
		}
	}

	return tags
}

func budgetCategoryTag(category *categories.Category) pages.BudgetCategoryTag {
	tag := pages.BudgetCategoryTag{
		Name:  string(category.Name),
		Color: string(category.Color),
	}

	if category.Icon != nil {
		tag.Icon = string(*category.Icon)
	}

	return tag
}

//
// Currencies
//

// BudgetCurrencyOptions asks for a currency first, so a new budget never
// takes one by chance, then offers every currency by code with its symbol.

func BudgetCurrencyOptions(ctx context.Context, list []*currencies.Currency) []components.SelectOption {
	options := make([]components.SelectOption, 0, len(list)+1)
	options = append(options, components.SelectOption{Value: "", Label: officei18n.T(ctx, "budgets.choose_currency")})

	for _, currency := range list {
		options = append(options, components.SelectOption{
			Value: string(currency.Code),
			Label: officei18n.T(ctx, "budgets.currency_option", map[string]any{
				"Code":   string(currency.Code),
				"Symbol": string(currency.Symbol),
			}),
		})
	}

	return options
}

//
// Budgets
//

// BudgetsPageSize bounds one page of the list.

const BudgetsPageSize = 50

// The page comes from the address; a missing or malformed one is the first.
// The largest budgets come first.

func BudgetsQuery(request *http.Request) budgets.BudgetQuery {
	page, err := strconv.ParseUint(request.URL.Query().Get("page"), 10, 32)

	if err != nil || page == 0 {
		page = 1
	}

	return budgets.NewBudgetQuery(
		budgets.NewBudgetFilter("", nil, nil, nil),
		domquery.SortOrderDesc,
		budgets.BudgetSortAmount,
		BudgetsPageSize,
		uint(page),
	)
}

// Members who may edit the content are offered the page that adds a budget.

func BudgetsContentProps(
	request *http.Request,
	found app.MemberWorkspace,
	notices []string,
) pages.BudgetsContentProps {
	props := pages.BudgetsContentProps{Notices: notices}

	if found.Role.Allows(workspaces.ActionEditContent) {
		props.NewURL = BudgetsPath(request, found.Workspace.ID) + "/new"
	}

	return props
}

// A new budget's form starts with no categories, no currency, and no amount,
// and posts to the list, which it leads back to.

func NewBudgetFormProps(
	request *http.Request,
	workspaceID workspaces.ID,
	categoryList []*categories.Category,
	currencyList []*currencies.Currency,
) pages.BudgetFormProps {
	ctx := request.Context()

	return pages.BudgetFormProps{
		Action:     BudgetsPath(request, workspaceID),
		Categories: BudgetCategoryChoices(categoryList, nil),
		Currencies: BudgetCurrencyOptions(ctx, currencyList),
		Period:     NewPeriodSelectorProps(ctx),
		Submit:     officei18n.T(ctx, "budgets.create_submit"),
		CancelURL:  BudgetsPath(request, workspaceID),
	}
}

func EditBudgetContentProps(
	request *http.Request,
	budget *budgets.Budget,
	categoryList []*categories.Category,
	currencyList []*currencies.Currency,
) pages.EditBudgetContentProps {
	ctx := request.Context()
	url := BudgetPath(request, budget.WorkspaceID, budget.ID)
	selected := make([]string, 0, len(budget.Categories))

	for _, id := range budget.Categories {
		selected = append(selected, uuid.UUID(id).String())
	}

	return pages.EditBudgetContentProps{
		Form: pages.BudgetFormProps{
			Action:     url,
			Categories: BudgetCategoryChoices(categoryList, selected),
			Currency:   string(budget.Currency),
			Currencies: BudgetCurrencyOptions(ctx, currencyList),
			Period:     PeriodSelectorProps(ctx, budget.Period),
			Amount:     FormatAmount(ctx, budget.Amount),
			Submit:     officei18n.T(ctx, "budgets.save_submit"),
			CancelURL:  BudgetsPath(request, budget.WorkspaceID),
		},
		DeleteURL: url + "/delete",
		Title:     BudgetTitle(ctx, budget, categoryList),
	}
}

// Each budget shows its categories, its period, and its amount with its
// currency's code.

func BudgetItems(
	request *http.Request,
	found []*budgets.Budget,
	categoryList []*categories.Category,
	canEdit bool,
) []pages.BudgetItem {
	ctx := request.Context()
	items := make([]pages.BudgetItem, 0, len(found))

	for _, budget := range found {
		item := pages.BudgetItem{
			ID:         uuid.UUID(budget.ID).String(),
			Title:      BudgetTitle(ctx, budget, categoryList),
			Categories: budgetTags(ctx, budget, categoryList),
			Period:     PeriodSummary(ctx, budget.Period),
			Amount: officei18n.T(ctx, "budgets.amount", map[string]any{
				"Amount": FormatAmount(ctx, budget.Amount),
				"Code":   string(budget.Currency),
			}),
		}

		if canEdit {
			item.Href = BudgetPath(request, budget.WorkspaceID, budget.ID)
		}

		items = append(items, item)
	}

	return items
}

// The pages are numbered from one; a list that fits on one page has no
// pagination.

func BudgetsPagination(
	request *http.Request,
	workspaceID workspaces.ID,
	query budgets.BudgetQuery,
	total int,
) pages.PaginationProps {
	var props pages.PaginationProps

	pageCount := (total + int(query.Limit) - 1) / int(query.Limit)
	page := int(query.Page)

	if pageCount > 1 {
		base := BudgetsPath(request, workspaceID)
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
