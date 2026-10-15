package input

import (
	"errors"
	"time"

	"github.com/radynsade/faryengo/internal/budgets"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Period input
//

// From and To bound the first window, and a nil bound leaves that side open.
// Repeat names how the window repeats, empty for not at all; Every is the step
// of a repeat of its own interval, and Until ends the repetition. Calendar
// steps follow the location of the times.

type PeriodInput struct {
	From   *time.Time
	To     *time.Time
	Repeat string
	Every  time.Duration
	Until  *time.Time
}

//
// Create budget input
//

// CategoryIDs lists the Categories the Budget covers, CurrencyCode names the
// Currency it is set in, Period says when it applies, and Amount is counted in
// that Currency's minor units.

var ErrCreateBudgetInputInvalid = errors.New("invalid create budget input")

type CreateBudgetInput struct {
	WorkspaceID  workspaces.ID
	CategoryIDs  []categories.ID
	CurrencyCode string
	Period       PeriodInput
	Amount       int64
}

//
// Update budget input
//

// The Budget's Categories are replaced by CategoryIDs, its Period by Period,
// and Amount is counted in the minor units of the Currency that CurrencyCode
// names. The Budget keeps its Workspace.

var ErrUpdateBudgetInputInvalid = errors.New("invalid update budget input")

type UpdateBudgetInput struct {
	ID           budgets.ID
	CategoryIDs  []categories.ID
	CurrencyCode string
	Period       PeriodInput
	Amount       int64
}
