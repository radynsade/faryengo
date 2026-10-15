package accounts

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/domquery"
)

func TestAccountFilterValidate(t *testing.T) {
	var (
		invalidWorkspaceID = workspaces.ID{}
		invalidUserID      = users.UserID{}
	)

	tests := []struct {
		name   string
		filter AccountFilter
		want   error
	}{
		{"empty", NewAccountFilter("", "", nil, nil), nil},
		{"every criterion", NewAccountFilter("0192", "Cash", &testWorkspaceID, &testUserID), nil},
		{"ID at the limit", NewAccountFilter(strings.Repeat("a", MaxAccountFilterIDLikeLength), "", nil, nil), nil},
		{"invalid ID characters", NewAccountFilter("01\x00", "", nil, nil), ErrAccountFilterIDLikeInvalidChars},
		{"invalid name characters", NewAccountFilter("", "Cash\xff", nil, nil), ErrAccountFilterNameLikeInvalidChars},
		{"ID too long", NewAccountFilter(strings.Repeat("a", MaxAccountFilterIDLikeLength+1), "", nil, nil), ErrAccountFilterIDLikeTooLong},
		{"name too long", NewAccountFilter("", strings.Repeat("a", MaxAccountFilterNameLikeLength+1), nil, nil), ErrAccountFilterNameLikeTooLong},
		{"invalid workspace", NewAccountFilter("", "", &invalidWorkspaceID, nil), workspaces.ErrIDInvalid},
		{"invalid user", NewAccountFilter("", "", nil, &invalidUserID), users.ErrUserIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, ErrAccountFilterInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrAccountFilterInvalid)
			}
		})
	}
}

func TestAccountFilterCount(t *testing.T) {
	tests := []struct {
		name   string
		filter AccountFilter
		want   int
	}{
		{"none", NewAccountFilter("", "", nil, nil), 0},
		{"name only", NewAccountFilter("", "Cash", nil, nil), 1},
		{"every criterion", NewAccountFilter("0192", "Cash", &testWorkspaceID, &testUserID), 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Count(); got != tt.want {
				t.Fatalf("Count() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewAccountSort(t *testing.T) {
	tests := []struct {
		value string
		want  error
	}{
		{"id", nil},
		{"name", nil},
		{"", ErrAccountQueryInvalid},
		{"user", ErrAccountQueryInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			sort, err := NewAccountSort(tt.value)

			if sort != AccountSort(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewAccountSort(%q) = (%q, %v), want %v", tt.value, sort, err, tt.want)
			}
		})
	}
}

func TestAccountQueryValidate(t *testing.T) {
	emptyFilter := NewAccountFilter("", "", nil, nil)

	tests := []struct {
		name  string
		query AccountQuery
		want  error
	}{
		{"valid", NewAccountQuery(emptyFilter, domquery.SortOrderAsc, AccountSortName, 10, 1), nil},
		{"invalid filter", NewAccountQuery(NewAccountFilter("\x00", "", nil, nil), domquery.SortOrderAsc, AccountSortID, 10, 1), ErrAccountFilterInvalid},
		{"missing sort", NewAccountQuery(emptyFilter, domquery.SortOrderAsc, "", 10, 1), ErrAccountQueryInvalid},
		{"zero limit", NewAccountQuery(emptyFilter, domquery.SortOrderAsc, AccountSortID, 0, 1), ErrAccountQueryInvalid},
		{"zero page", NewAccountQuery(emptyFilter, domquery.SortOrderAsc, AccountSortID, 10, 0), ErrAccountQueryInvalid},
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
