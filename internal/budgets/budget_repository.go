package budgets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/domquery"
)

//
// Filter
//

const MaxBudgetFilterIDLikeLength = 36

var (
	ErrBudgetFilterInvalid            = errors.New("invalid budget filter")
	ErrBudgetFilterIDLikeInvalidChars = errors.New("invalid characters")
	ErrBudgetFilterIDLikeTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxBudgetFilterIDLikeLength)
)

// WorkspaceID limits the result to the Budgets of that Workspace, CategoryID
// to the Budgets that cover that Category, and Currency to the Budgets in that
// Currency.

type BudgetFilter struct {
	IDLike      string
	WorkspaceID *workspaces.ID
	CategoryID  *categories.ID
	Currency    *currencies.Code
}

func NewBudgetFilter(
	idLike string,
	workspaceID *workspaces.ID,
	categoryID *categories.ID,
	currency *currencies.Code,
) BudgetFilter {
	return BudgetFilter{
		IDLike:      idLike,
		WorkspaceID: workspaceID,
		CategoryID:  categoryID,
		Currency:    currency,
	}
}

func (f BudgetFilter) Validate() error {
	var err error

	if !utf8.ValidString(f.IDLike) || strings.ContainsRune(f.IDLike, '\x00') {
		err = ErrBudgetFilterIDLikeInvalidChars
	} else if utf8.RuneCountInString(f.IDLike) > MaxBudgetFilterIDLikeLength {
		err = ErrBudgetFilterIDLikeTooLong
	} else if f.WorkspaceID != nil {
		err = f.WorkspaceID.Validate()
	}

	if err == nil && f.CategoryID != nil {
		err = f.CategoryID.Validate()
	}

	if err == nil && f.Currency != nil {
		err = f.Currency.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrBudgetFilterInvalid, err)
	}

	return err
}

func (f BudgetFilter) Count() int {
	count := 0

	for _, applied := range []bool{
		f.IDLike != "",
		f.WorkspaceID != nil,
		f.CategoryID != nil,
		f.Currency != nil,
	} {
		if applied {
			count++
		}
	}

	return count
}

//
// Sort
//

type BudgetSort string

const (
	BudgetSortID     = BudgetSort("id")
	BudgetSortAmount = BudgetSort("amount")
)

func NewBudgetSort(value string) (BudgetSort, error) {
	var (
		err  error
		sort BudgetSort
	)

	sort = BudgetSort(value)
	err = sort.Validate()
	return sort, err
}

func (s BudgetSort) Validate() error {
	var err error

	switch s {
	case BudgetSortID, BudgetSortAmount:
	default:
		err = ErrBudgetQueryInvalid
	}

	return err
}

//
// Query
//

var ErrBudgetQueryInvalid = errors.New("invalid budget query")

type BudgetQuery domquery.Query[BudgetFilter, BudgetSort]

func NewBudgetQuery(
	filter BudgetFilter,
	sortOrder domquery.SortOrder,
	sortBy BudgetSort,
	limit uint,
	page uint,
) BudgetQuery {
	return BudgetQuery{
		Filter:    filter,
		SortOrder: sortOrder,
		SortBy:    sortBy,
		Limit:     limit,
		Page:      page,
	}
}

// Page is 1-based, and both Limit and Page must be set.

func (q BudgetQuery) Validate() error {
	err := q.Filter.Validate()

	if err == nil {
		err = q.SortBy.Validate()
	}

	if err == nil && (q.Limit == 0 || q.Page == 0) {
		err = ErrBudgetQueryInvalid
	}

	return err
}

//
// Repository
//

var (
	ErrBudgetNotFound      = errors.New("budget not found")
	ErrBudgetAlreadyExists = errors.New("budget already exists")
)

type ErrBudgetCreateFailed interface {
	error
	Budget() *Budget
	Unwrap() error
}

type ErrBudgetUpdateFailed interface {
	error
	Budget() *Budget
	Unwrap() error
}

type ErrBudgetDeleteFailed interface {
	error
	BudgetID() ID
	Unwrap() error
}

// A Budget stays in the Workspace it was created in, so updating one changes
// its Categories, Currency, and Amount but never its Workspace; a Budget
// submitted with a different Workspace is reported as ErrBudgetNotFound.
// Writing a Budget with a missing Workspace is reported as
// workspaces.ErrWorkspaceNotFound, with a missing Currency as
// currencies.ErrCurrencyNotFound, and with a Category that is missing or
// belongs to another Workspace as categories.ErrCategoryNotFound. A found
// Budget lists its Categories in the order they were written.

type BudgetRepository interface {
	Create(ctx context.Context, budget *Budget) ErrBudgetCreateFailed
	Update(ctx context.Context, budget *Budget) ErrBudgetUpdateFailed
	Delete(ctx context.Context, id ID) ErrBudgetDeleteFailed
	FindByID(ctx context.Context, id ID) (*Budget, error)
	FindByIDForUpdate(ctx context.Context, id ID) (*Budget, error)
	Find(ctx context.Context, query BudgetQuery) ([]*Budget, error)
	Count(ctx context.Context, filter BudgetFilter) (int, error)
}
