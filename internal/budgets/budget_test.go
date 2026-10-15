package budgets

import (
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/period"
	"github.com/radynsade/faryengo/internal/workspaces"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

var (
	testBudgetID    = ID(uuid.MustParse("01920000-0000-7000-8000-000000000007"))
	testWorkspaceID = workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
	testCategoryID  = categories.ID(uuid.MustParse("01920000-0000-7000-8000-000000000004"))
	testCategoryID2 = categories.ID(uuid.MustParse("01920000-0000-7000-8000-000000000006"))
	testCategoryIDs = CategoryIDs{testCategoryID, testCategoryID2}
)

func TestNewID(t *testing.T) {
	tests := []struct {
		name  string
		value uuid.UUID
		want  error
	}{
		{"valid", uuid.UUID(testBudgetID), nil},
		{"nil", uuid.Nil, ErrIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewID(tt.value)

			if id != ID(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewID(%s) = (%v, %v), want %v", tt.value, id, err, tt.want)
			}
		})
	}
}

func TestCategoryIDsValidate(t *testing.T) {
	tests := []struct {
		name        string
		categoryIDs CategoryIDs
		want        error
	}{
		{"one category", CategoryIDs{testCategoryID}, nil},
		{"several categories", testCategoryIDs, nil},
		{"nil", nil, ErrCategoryIDsEmpty},
		{"empty", CategoryIDs{}, ErrCategoryIDsEmpty},
		{"invalid category ID", CategoryIDs{testCategoryID, categories.ID{}}, categories.ErrIDInvalid},
		{"duplicate", CategoryIDs{testCategoryID, testCategoryID2, testCategoryID}, ErrCategoryIDsDuplicate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			categoryIDs := NewCategoryIDs(tt.categoryIDs)

			if !slices.Equal(categoryIDs, tt.categoryIDs) {
				t.Fatalf("NewCategoryIDs() = %v, want %v", categoryIDs, tt.categoryIDs)
			}

			err := categoryIDs.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrCategoryIDsInvalid) {
				t.Fatalf("Validate() = %v, want it to wrap %v", err, ErrCategoryIDsInvalid)
			}
		})
	}
}

// The constructor keeps the value as given, even when it is invalid.

func TestNewAmount(t *testing.T) {
	tests := []struct {
		name  string
		value int64
		want  error
	}{
		{"zero", 0, nil},
		{"positive", 12_50, nil},
		{"maximum", math.MaxInt64, nil},
		{"negative", -1, ErrAmountNegative},
		{"minimum", math.MinInt64, ErrAmountNegative},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount, err := NewAmount(tt.value)

			if amount != Amount(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewAmount(%d) = (%d, %v), want %v", tt.value, amount, err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrAmountInvalid) {
				t.Fatalf("NewAmount(%d) = %v, want it to wrap %v", tt.value, err, ErrAmountInvalid)
			}
		})
	}
}

func TestBudgetValidate(t *testing.T) {
	var (
		monthStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		monthEnd   = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
		anyTime    = period.Period{}
		monthly    = period.NewPeriod(&monthStart, &monthEnd, period.NewRepeat(period.RepeatMonthly, 0, nil))
		unbounded  = period.NewPeriod(&monthStart, nil, period.NewRepeat(period.RepeatMonthly, 0, nil))
	)

	tests := []struct {
		name        string
		id          ID
		workspaceID workspaces.ID
		categoryIDs CategoryIDs
		currency    currencies.Code
		period      period.Period
		amount      Amount
		want        []error
	}{
		{"valid", testBudgetID, testWorkspaceID, testCategoryIDs, "EUR", anyTime, 100_00, nil},
		{"valid monthly", testBudgetID, testWorkspaceID, testCategoryIDs, "EUR", monthly, 100_00, nil},
		{"repeating without an end", testBudgetID, testWorkspaceID, testCategoryIDs, "EUR", unbounded, 100_00, []error{period.ErrPeriodRepeatUnbounded}},
		{"valid zero amount", testBudgetID, testWorkspaceID, testCategoryIDs, "EUR", anyTime, 0, nil},
		{"invalid ID", ID{}, testWorkspaceID, testCategoryIDs, "EUR", anyTime, 100_00, []error{ErrIDInvalid}},
		{"invalid workspace ID", testBudgetID, workspaces.ID{}, testCategoryIDs, "EUR", anyTime, 100_00, []error{workspaces.ErrIDInvalid}},
		{"no categories", testBudgetID, testWorkspaceID, nil, "EUR", anyTime, 100_00, []error{ErrCategoryIDsEmpty}},
		{"missing currency", testBudgetID, testWorkspaceID, testCategoryIDs, "", anyTime, 100_00, []error{currencies.ErrCodeInvalid}},
		{"invalid currency", testBudgetID, testWorkspaceID, testCategoryIDs, "eur", anyTime, 100_00, []error{currencies.ErrCodeInvalid}},
		{"negative amount", testBudgetID, testWorkspaceID, testCategoryIDs, "EUR", anyTime, -1, []error{ErrAmountNegative}},
		{
			"every field invalid",
			ID{}, workspaces.ID{}, CategoryIDs{testCategoryID, testCategoryID}, "", unbounded, -1,
			[]error{ErrIDInvalid, workspaces.ErrIDInvalid, ErrCategoryIDsDuplicate, currencies.ErrCodeInvalid, period.ErrPeriodInvalid, ErrAmountNegative},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget := NewBudget(tt.id, tt.workspaceID, tt.categoryIDs, tt.currency, tt.period, tt.amount)

			if budget.ID != tt.id ||
				budget.WorkspaceID != tt.workspaceID ||
				!slices.Equal(budget.Categories, tt.categoryIDs) ||
				budget.Currency != tt.currency ||
				budget.Period != tt.period ||
				budget.Amount != tt.amount {
				t.Fatalf("NewBudget() = %+v, want the given ID, workspace, categories, currency, period, and amount", budget)
			}

			err := budget.Validate()

			if (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if err != nil && !errors.Is(err, ErrBudgetInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrBudgetInvalid)
			}

			for _, want := range tt.want {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want it to include %v", err, want)
				}
			}
		})
	}

	if err := (*Budget)(nil).Validate(); !errors.Is(err, ErrBudgetNil) || !errors.Is(err, ErrBudgetInvalid) {
		t.Fatalf("Validate() on nil = %v, want %v wrapped in %v", err, ErrBudgetNil, ErrBudgetInvalid)
	}
}
