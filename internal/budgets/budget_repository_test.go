package budgets

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/domquery"
)

func TestBudgetFilterValidate(t *testing.T) {
	var (
		testCurrency       = currencies.Code("EUR")
		invalidWorkspaceID = workspaces.ID{}
		invalidCategoryID  = categories.ID{}
		invalidCurrency    = currencies.Code("eur")
	)

	tests := []struct {
		name   string
		filter BudgetFilter
		want   error
	}{
		{"empty", NewBudgetFilter("", nil, nil, nil), nil},
		{"every criterion", NewBudgetFilter("0192", &testWorkspaceID, &testCategoryID, &testCurrency), nil},
		{"ID at the limit", NewBudgetFilter(strings.Repeat("a", MaxBudgetFilterIDLikeLength), nil, nil, nil), nil},
		{"invalid ID characters", NewBudgetFilter("01\x00", nil, nil, nil), ErrBudgetFilterIDLikeInvalidChars},
		{"invalid UTF-8 ID", NewBudgetFilter("01\xff", nil, nil, nil), ErrBudgetFilterIDLikeInvalidChars},
		{"ID too long", NewBudgetFilter(strings.Repeat("a", MaxBudgetFilterIDLikeLength+1), nil, nil, nil), ErrBudgetFilterIDLikeTooLong},
		{"invalid workspace", NewBudgetFilter("", &invalidWorkspaceID, nil, nil), workspaces.ErrIDInvalid},
		{"invalid category", NewBudgetFilter("", nil, &invalidCategoryID, nil), categories.ErrIDInvalid},
		{"invalid currency", NewBudgetFilter("", nil, nil, &invalidCurrency), currencies.ErrCodeInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, ErrBudgetFilterInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrBudgetFilterInvalid)
			}
		})
	}
}

func TestBudgetFilterCount(t *testing.T) {
	testCurrency := currencies.Code("EUR")

	tests := []struct {
		name   string
		filter BudgetFilter
		want   int
	}{
		{"none", NewBudgetFilter("", nil, nil, nil), 0},
		{"category only", NewBudgetFilter("", nil, &testCategoryID, nil), 1},
		{"every criterion", NewBudgetFilter("0192", &testWorkspaceID, &testCategoryID, &testCurrency), 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Count(); got != tt.want {
				t.Fatalf("Count() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewBudgetSort(t *testing.T) {
	tests := []struct {
		value string
		want  error
	}{
		{"id", nil},
		{"amount", nil},
		{"", ErrBudgetQueryInvalid},
		{"currency", ErrBudgetQueryInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			sort, err := NewBudgetSort(tt.value)

			if sort != BudgetSort(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewBudgetSort(%q) = (%q, %v), want %v", tt.value, sort, err, tt.want)
			}
		})
	}
}

func TestBudgetQueryValidate(t *testing.T) {
	emptyFilter := NewBudgetFilter("", nil, nil, nil)

	tests := []struct {
		name  string
		query BudgetQuery
		want  error
	}{
		{"valid", NewBudgetQuery(emptyFilter, domquery.SortOrderAsc, BudgetSortAmount, 10, 1), nil},
		{"invalid filter", NewBudgetQuery(NewBudgetFilter("\x00", nil, nil, nil), domquery.SortOrderAsc, BudgetSortID, 10, 1), ErrBudgetFilterInvalid},
		{"missing sort", NewBudgetQuery(emptyFilter, domquery.SortOrderAsc, "", 10, 1), ErrBudgetQueryInvalid},
		{"zero limit", NewBudgetQuery(emptyFilter, domquery.SortOrderAsc, BudgetSortID, 0, 1), ErrBudgetQueryInvalid},
		{"zero page", NewBudgetQuery(emptyFilter, domquery.SortOrderAsc, BudgetSortID, 10, 0), ErrBudgetQueryInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}
