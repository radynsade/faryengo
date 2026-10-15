package budgets

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// ID
//

var ErrIDInvalid = errors.New("invalid ID")

type ID uuid.UUID

func NewID(value uuid.UUID) (ID, error) {
	var (
		err error
		id  ID
	)

	id = ID(value)
	err = id.Validate()
	return id, err
}

func (id ID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrIDInvalid
	}

	return err
}

//
// Category IDs
//

var (
	ErrCategoryIDsInvalid   = errors.New("invalid categories")
	ErrCategoryIDsEmpty     = errors.New("are empty")
	ErrCategoryIDsDuplicate = errors.New("contain a duplicate")
)

type CategoryIDs []categories.ID

func NewCategoryIDs(ids []categories.ID) CategoryIDs {
	return CategoryIDs(ids)
}

func (c CategoryIDs) Validate() error {
	var err error

	seen := make(map[categories.ID]struct{}, len(c))

	if len(c) == 0 {
		err = ErrCategoryIDsEmpty
	}

	for _, id := range c {
		if err != nil {
			break
		}

		err = id.Validate()

		if err == nil {
			if _, isSeen := seen[id]; isSeen {
				err = ErrCategoryIDsDuplicate
			}

			seen[id] = struct{}{}
		}
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrCategoryIDsInvalid, err)
	}

	return err
}

//
// Amount
//

// An Amount is an exact quantity in the minor units of the Budget's Currency,
// such as cents.

var (
	ErrAmountInvalid  = errors.New("invalid amount")
	ErrAmountNegative = errors.New("is negative")
)

type Amount int64

func NewAmount(value int64) (Amount, error) {
	var (
		err    error
		amount Amount
	)

	amount = Amount(value)
	err = amount.Validate()
	return amount, err
}

func (a Amount) Validate() error {
	var err error

	if a < 0 {
		err = fmt.Errorf("%w: %w", ErrAmountInvalid, ErrAmountNegative)
	}

	return err
}

//
// Budget
//

var (
	ErrBudgetInvalid = errors.New("invalid budget")
	ErrBudgetNil     = errors.New("is nil")
)

// A Budget limits the Amount spent across Categories of its Workspace, in one
// Currency referenced by its code.

type Budget struct {
	ID          ID
	WorkspaceID workspaces.ID
	Categories  CategoryIDs
	Currency    currencies.Code
	Amount      Amount
}

func NewBudget(
	id ID,
	workspaceID workspaces.ID,
	categoryIDs CategoryIDs,
	currency currencies.Code,
	amount Amount,
) *Budget {
	return &Budget{
		ID:          id,
		WorkspaceID: workspaceID,
		Categories:  categoryIDs,
		Currency:    currency,
		Amount:      amount,
	}
}

func (b *Budget) Validate() error {
	var err error

	if b == nil {
		err = ErrBudgetNil
	} else {
		err = errors.Join(
			b.ID.Validate(),
			b.WorkspaceID.Validate(),
			b.Categories.Validate(),
			b.Currency.Validate(),
			b.Amount.Validate(),
		)
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrBudgetInvalid, err)
	}

	return err
}
