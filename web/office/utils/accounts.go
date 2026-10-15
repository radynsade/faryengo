package utils

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/accounts"
	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	"github.com/radynsade/faryengo/pkg/domquery"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/components"
	"github.com/radynsade/faryengo/web/office/templates/pages"
)

//
// Paths
//

// AccountsSection is the menu item of the workspace's accounts.

const AccountsSection = "accounts"

func AccountsPath(request *http.Request, workspaceID workspaces.ID) string {
	return WorkspacePath(request, workspaceID) + "/accounts"
}

func AccountPath(request *http.Request, workspaceID workspaces.ID, id accounts.ID) string {
	return AccountsPath(request, workspaceID) + "/" + uuid.UUID(id).String()
}

//
// Account form
//

// The name's limits, which members may hold an account, and which currencies
// exist belong to the domain; the transport bounds only the size of each field
// and of the whole form. An empty user means the account belongs to no
// member. A currency is required when the account is created and never
// changes, so editing does not submit one.

const maxAccountFormBytes = 8 << 10

type AccountForm struct {
	Name     string `form:"name" validate:"maxbytes=400"`
	User     string `form:"user" validate:"maxbytes=36"`
	Currency string `form:"currency" validate:"maxbytes=12"`
}

func ParseAccountForm(writer http.ResponseWriter, request *http.Request, edit bool) (AccountForm, error) {
	var form AccountForm

	fields := []string{"name", "user", "currency"}

	if edit {
		fields = []string{"name", "user"}
	}

	err := parseForm(writer, request, maxAccountFormBytes, fields...)

	if err == nil {
		form = AccountForm{
			Name:     request.PostForm.Get("name"),
			User:     request.PostForm.Get("user"),
			Currency: request.PostForm.Get("currency"),
		}

		err = requestvalidation.Validate(request.Context(), form)
	}

	return form, err
}

// AccountUserID reads the form's member; a value that is not a UUID names no
// User and is reported as an invalid user ID.

func AccountUserID(form AccountForm) (*users.UserID, error) {
	var (
		userID *users.UserID
		err    error
	)

	if form.User != "" {
		var parsed uuid.UUID

		parsed, err = uuid.Parse(form.User)

		if err == nil {
			userID = new(users.UserID(parsed))
			err = userID.Validate()
		} else {
			err = errors.Join(users.ErrUserIDInvalid, err)
		}

		if err != nil {
			userID = nil
		}
	}

	return userID, err
}

// AccountFieldErrors maps a rejected account to translated messages keyed by
// form field name; a failure that belongs to no field yields none. A member
// who is unknown or has left the workspace is a problem with the member
// field, because the form offers only current members. A currency that is
// missing, malformed, or no longer exists is a problem with the currency
// field, because the form offers only existing currencies.

func AccountFieldErrors(ctx context.Context, err error) map[string]string {
	fields := make(map[string]string)

	addTransportErrors(ctx, fields, err)

	if _, found := fields["name"]; !found && errors.Is(err, accounts.ErrNameInvalid) {
		switch {
		case errors.Is(err, accounts.ErrNameEmpty):
			fields["name"] = officei18n.T(ctx, "validation.required")
		case errors.Is(err, accounts.ErrNameTooLong):
			fields["name"] = officei18n.Count(ctx, "validation.too_long_chars", accounts.MaxNameLength)
		default:
			fields["name"] = officei18n.T(ctx, "validation.invalid")
		}
	}

	if _, found := fields["user"]; !found && (errors.Is(err, users.ErrUserIDInvalid) || errors.Is(err, users.ErrUserNotFound)) {
		fields["user"] = officei18n.T(ctx, "validation.choose_one")
	}

	if _, found := fields["currency"]; !found &&
		(errors.Is(err, currencies.ErrCodeInvalid) || errors.Is(err, currencies.ErrCurrencyNotFound)) {
		fields["currency"] = officei18n.T(ctx, "validation.choose_one")
	}

	return nilIfEmpty(fields)
}

// A deletion carries the account in its address; its only field is the
// dialog's confirmation, without which nothing is deleted.

func ParseDeleteAccountForm(writer http.ResponseWriter, request *http.Request) (bool, error) {
	var confirmed bool

	err := parseForm(writer, request, maxAccountFormBytes, "confirm")

	if err == nil {
		confirmed = request.PostForm.Get("confirm") == "1"
	}

	return confirmed, err
}

// An account ID that is not a UUID, or is nil, names no account, so it is
// reported like a missing one.

func ParseAccountID(request *http.Request) (accounts.ID, error) {
	var id accounts.ID

	parsed, err := uuid.Parse(request.PathValue("account"))

	if err == nil {
		id, err = accounts.NewID(parsed)
	}

	if err != nil {
		id = accounts.ID{}
		err = errors.Join(accounts.ErrAccountNotFound, err)
	}

	return id, err
}

//
// Members
//

// AccountUserOptions offers no member first, then the workspace's members in
// name order. An account may keep a User who has left the workspace; that
// User is offered too, as a former member, so saving the account keeps them.

func AccountUserOptions(
	ctx context.Context,
	members []app.WorkspaceMember,
	kept *users.UserID,
) []components.SelectOption {
	options := make([]components.SelectOption, 0, len(members)+2)
	options = append(options, components.SelectOption{Value: "", Label: officei18n.T(ctx, "accounts.no_member")})

	if kept != nil && memberName(members, *kept) == "" {
		options = append(options, components.SelectOption{
			Value: uuid.UUID(*kept).String(),
			Label: officei18n.T(ctx, "accounts.former_member"),
		})
	}

	for _, member := range members {
		options = append(options, components.SelectOption{
			Value: uuid.UUID(member.User.ID).String(),
			Label: FullName(member.User),
		})
	}

	return options
}

//
// Currencies
//

// AccountCurrencyOptions asks for a currency first, so a new account never
// takes one by chance, then offers every currency by code with its symbol.

func AccountCurrencyOptions(ctx context.Context, list []*currencies.Currency) []components.SelectOption {
	options := make([]components.SelectOption, 0, len(list)+1)
	options = append(options, components.SelectOption{Value: "", Label: officei18n.T(ctx, "accounts.choose_currency")})

	for _, currency := range list {
		options = append(options, components.SelectOption{
			Value: string(currency.Code),
			Label: officei18n.T(ctx, "accounts.currency_option", map[string]any{
				"Code":   string(currency.Code),
				"Symbol": string(currency.Symbol),
			}),
		})
	}

	return options
}

// The name of the member among members, or empty for a User who is not one.

func memberName(members []app.WorkspaceMember, userID users.UserID) string {
	name := ""

	for _, member := range members {
		if member.User.ID == userID {
			name = FullName(member.User)
		}
	}

	return name
}

//
// Accounts
//

// AccountsPageSize bounds one page of the list.

const AccountsPageSize = 50

// The page comes from the address; a missing or malformed one is the first.

func AccountsQuery(request *http.Request) accounts.AccountQuery {
	page, err := strconv.ParseUint(request.URL.Query().Get("page"), 10, 32)

	if err != nil || page == 0 {
		page = 1
	}

	return accounts.NewAccountQuery(
		accounts.NewAccountFilter("", "", nil, nil),
		domquery.SortOrderAsc,
		accounts.AccountSortName,
		AccountsPageSize,
		uint(page),
	)
}

// Members who may edit the content are offered the page that adds an
// account.

func AccountsContentProps(
	request *http.Request,
	found app.MemberWorkspace,
	notices []string,
) pages.AccountsContentProps {
	props := pages.AccountsContentProps{Notices: notices}

	if found.Role.Allows(workspaces.ActionEditContent) {
		props.NewURL = AccountsPath(request, found.Workspace.ID) + "/new"
	}

	return props
}

// A new account's form starts with no member and no currency, and posts to the
// list, which it leads back to.

func NewAccountFormProps(
	request *http.Request,
	workspaceID workspaces.ID,
	members []app.WorkspaceMember,
	currencyList []*currencies.Currency,
) pages.AccountFormProps {
	ctx := request.Context()

	return pages.AccountFormProps{
		Action:     AccountsPath(request, workspaceID),
		Users:      AccountUserOptions(ctx, members, nil),
		Currencies: AccountCurrencyOptions(ctx, currencyList),
		Submit:     officei18n.T(ctx, "accounts.create_submit"),
		CancelURL:  AccountsPath(request, workspaceID),
	}
}

// The account keeps its currency, so its form shows the currency instead of
// offering one.

func EditAccountContentProps(
	request *http.Request,
	account *accounts.Account,
	members []app.WorkspaceMember,
) pages.EditAccountContentProps {
	ctx := request.Context()
	url := AccountPath(request, account.WorkspaceID, account.ID)
	user := ""

	if account.UserID != nil {
		user = uuid.UUID(*account.UserID).String()
	}

	return pages.EditAccountContentProps{
		Form: pages.AccountFormProps{
			Action:        url,
			Name:          string(account.Name),
			User:          user,
			Users:         AccountUserOptions(ctx, members, account.UserID),
			Currency:      string(account.CurrencyCode),
			CurrencyFixed: true,
			Submit:        officei18n.T(ctx, "accounts.save_submit"),
			CancelURL:     AccountsPath(request, account.WorkspaceID),
		},
		DeleteURL: url + "/delete",
		Name:      string(account.Name),
	}
}

// Each account names its member and its currency's code; one whose User has
// left the workspace names a former member, and one without a User says so.

func AccountItems(
	request *http.Request,
	found []*accounts.Account,
	members []app.WorkspaceMember,
	canEdit bool,
) []pages.AccountItem {
	ctx := request.Context()
	items := make([]pages.AccountItem, 0, len(found))

	for _, account := range found {
		item := pages.AccountItem{
			ID:       uuid.UUID(account.ID).String(),
			Name:     string(account.Name),
			Currency: string(account.CurrencyCode),
		}

		if account.UserID == nil {
			item.Member = officei18n.T(ctx, "accounts.no_member")
		} else if item.Member = memberName(members, *account.UserID); item.Member == "" {
			item.Member = officei18n.T(ctx, "accounts.former_member")
		}

		if canEdit {
			item.Href = AccountPath(request, account.WorkspaceID, account.ID)
		}

		items = append(items, item)
	}

	return items
}

// The pages are numbered from one; a list that fits on one page has no
// pagination.

func AccountsPagination(
	request *http.Request,
	workspaceID workspaces.ID,
	query accounts.AccountQuery,
	total int,
) pages.PaginationProps {
	var props pages.PaginationProps

	pageCount := (total + int(query.Limit) - 1) / int(query.Limit)
	page := int(query.Page)

	if pageCount > 1 {
		base := AccountsPath(request, workspaceID)
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
