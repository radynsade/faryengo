package input

import (
	"errors"

	"github.com/radynsade/faryengo/internal/budgets"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Create budget input
//

// CategoryIDs lists the Categories the Budget covers, CurrencyCode names the
// Currency it is set in, and Amount is counted in that Currency's minor units.

var ErrCreateBudgetInputInvalid = errors.New("invalid create budget input")

type CreateBudgetInput struct {
	WorkspaceID  workspaces.ID
	CategoryIDs  []categories.ID
	CurrencyCode string
	Amount       int64
}

//
// Update budget input
//

// The Budget's Categories are replaced by CategoryIDs, and Amount is counted in
// the minor units of the Currency that CurrencyCode names. The Budget keeps its
// Workspace.

var ErrUpdateBudgetInputInvalid = errors.New("invalid update budget input")

type UpdateBudgetInput struct {
	ID           budgets.ID
	CategoryIDs  []categories.ID
	CurrencyCode string
	Amount       int64
}
