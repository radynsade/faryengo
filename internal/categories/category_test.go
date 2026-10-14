package categories

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/domquery"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

var (
	testCategoryID  = CategoryID(uuid.MustParse("01920000-0000-7000-8000-000000000004"))
	testWorkspaceID = workspaces.WorkspaceID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
)

func iconPointer(icon Icon) *Icon {
	return &icon
}

func TestNewCategoryID(t *testing.T) {
	tests := []struct {
		name  string
		value uuid.UUID
		want  error
	}{
		{"valid", uuid.UUID(testCategoryID), nil},
		{"nil", uuid.Nil, ErrCategoryIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewCategoryID(tt.value)

			if id != CategoryID(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewCategoryID(%s) = (%v, %v), want %v", tt.value, id, err, tt.want)
			}
		})
	}
}

// The constructor keeps the value as given, even when it is invalid.

func TestNewCategoryName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{"valid", "Groceries", nil},
		{"only whitespace", " \t ", ErrCategoryNameEmpty},
		{"too long", strings.Repeat("a", MaxCategoryNameLength+1), ErrCategoryNameTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, err := NewCategoryName(tt.value)

			if name != CategoryName(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewCategoryName(%q) = (%q, %v), want %v", tt.value, name, err, tt.want)
			}
		})
	}
}

func TestCategoryNameValidate(t *testing.T) {
	tests := []struct {
		name         string
		categoryName CategoryName
		want         error
	}{
		{"valid", "Groceries", nil},
		{"single character", "a", nil},
		{"limit in ASCII characters", CategoryName(strings.Repeat("a", MaxCategoryNameLength)), nil},
		{"limit in multibyte characters", CategoryName(strings.Repeat("ā", MaxCategoryNameLength)), nil},
		{"empty", "", ErrCategoryNameEmpty},
		{"blank", " \t\n", ErrCategoryNameEmpty},
		{"invalid UTF-8", "Food\xff", ErrCategoryNameInvalidChars},
		{"too long", CategoryName(strings.Repeat("a", MaxCategoryNameLength+1)), ErrCategoryNameTooLong},
		{"too long in multibyte characters", CategoryName(strings.Repeat("ā", MaxCategoryNameLength+1)), ErrCategoryNameTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.categoryName.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, ErrCategoryNameInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrCategoryNameInvalid)
			}
		})
	}
}

func TestNewColor(t *testing.T) {
	tests := []struct {
		value string
		want  error
	}{
		{"green", nil},
		{"blue", nil},
		{"orange", nil},
		{"red", nil},
		{"purple", nil},
		{"brown", nil},
		{"grey", nil},
		{"yellow", nil},
		{"", ErrColorInvalid},
		{"Green", ErrColorInvalid},
		{"gray", ErrColorInvalid},
		{"#00ff00", ErrColorInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			color, err := NewColor(tt.value)

			if color != Color(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewColor(%q) = (%q, %v), want %v", tt.value, color, err, tt.want)
			}
		})
	}
}

func TestNewIcon(t *testing.T) {
	tests := []struct {
		value string
		want  error
	}{
		{"player-play", nil},
		{"device-gamepad-2", nil},
		{"device-gamepad2", ErrIconInvalid},
		{"receipt-2", nil},
		{"ball-american-football", nil},
		{"shirt", nil},
		{"", ErrIconInvalid},
		{"Shirt", ErrIconInvalid},
		{"shopping_cart", ErrIconInvalid},
		{"ti-shirt", ErrIconInvalid},
		{"rocket", ErrIconInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			icon, err := NewIcon(tt.value)

			if icon != Icon(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewIcon(%q) = (%q, %v), want %v", tt.value, icon, err, tt.want)
			}
		})
	}
}

func TestCategoryValidate(t *testing.T) {
	tests := []struct {
		name     string
		category *Category
		wantErrs []error
	}{
		{"valid with an icon", NewCategory(testCategoryID, testWorkspaceID, "Groceries", iconPointer(IconShoppingCart), ColorGreen), nil},
		{"valid without an icon", NewCategory(testCategoryID, testWorkspaceID, "Groceries", nil, ColorGreen), nil},
		{"nil", nil, []error{ErrCategoryNil}},
		{"nil ID", NewCategory(CategoryID{}, testWorkspaceID, "Groceries", nil, ColorGreen), []error{ErrCategoryIDInvalid}},
		{"nil workspace", NewCategory(testCategoryID, workspaces.WorkspaceID{}, "Groceries", nil, ColorGreen), []error{workspaces.ErrWorkspaceIDInvalid}},
		{"blank name", NewCategory(testCategoryID, testWorkspaceID, " ", nil, ColorGreen), []error{ErrCategoryNameEmpty}},
		{"unknown icon", NewCategory(testCategoryID, testWorkspaceID, "Groceries", iconPointer("rocket"), ColorGreen), []error{ErrIconInvalid}},
		{"empty icon", NewCategory(testCategoryID, testWorkspaceID, "Groceries", iconPointer(""), ColorGreen), []error{ErrIconInvalid}},
		{"missing color", NewCategory(testCategoryID, testWorkspaceID, "Groceries", nil, ""), []error{ErrColorInvalid}},
		{
			"every field invalid",
			NewCategory(CategoryID{}, workspaces.WorkspaceID{}, "", iconPointer("rocket"), "pink"),
			[]error{ErrCategoryIDInvalid, workspaces.ErrWorkspaceIDInvalid, ErrCategoryNameEmpty, ErrIconInvalid, ErrColorInvalid},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.category.Validate()

			if (err == nil) != (len(tt.wantErrs) == 0) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErrs)
			}

			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("Validate() = %v, want %v", err, want)
				}
			}

			if err != nil && !errors.Is(err, ErrCategoryInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrCategoryInvalid)
			}
		})
	}
}

func TestCategoryFilterValidate(t *testing.T) {
	invalidWorkspaceID := workspaces.WorkspaceID{}

	tests := []struct {
		name   string
		filter CategoryFilter
		want   error
	}{
		{"empty", NewCategoryFilter("", "", nil), nil},
		{"every criterion", NewCategoryFilter("0192", "Food", &testWorkspaceID), nil},
		{"ID at the limit", NewCategoryFilter(strings.Repeat("a", MaxCategoryFilterIDLikeLength), "", nil), nil},
		{"invalid ID characters", NewCategoryFilter("01\x00", "", nil), ErrCategoryFilterIDLikeInvalidChars},
		{"invalid name characters", NewCategoryFilter("", "Food\xff", nil), ErrCategoryFilterNameLikeInvalidChars},
		{"ID too long", NewCategoryFilter(strings.Repeat("a", MaxCategoryFilterIDLikeLength+1), "", nil), ErrCategoryFilterIDLikeTooLong},
		{"name too long", NewCategoryFilter("", strings.Repeat("a", MaxCategoryFilterNameLikeLength+1), nil), ErrCategoryFilterNameLikeTooLong},
		{"invalid workspace", NewCategoryFilter("", "", &invalidWorkspaceID), workspaces.ErrWorkspaceIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()

			if !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if tt.want != nil && !errors.Is(err, ErrCategoryFilterInvalid) {
				t.Fatalf("Validate() = %v, want it wrapped in %v", err, ErrCategoryFilterInvalid)
			}
		})
	}
}

func TestCategoryFilterCount(t *testing.T) {
	tests := []struct {
		name   string
		filter CategoryFilter
		want   int
	}{
		{"none", NewCategoryFilter("", "", nil), 0},
		{"name only", NewCategoryFilter("", "Food", nil), 1},
		{"every criterion", NewCategoryFilter("0192", "Food", &testWorkspaceID), 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Count(); got != tt.want {
				t.Fatalf("Count() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewCategorySort(t *testing.T) {
	tests := []struct {
		value string
		want  error
	}{
		{"id", nil},
		{"name", nil},
		{"", ErrCategoryQueryInvalid},
		{"color", ErrCategoryQueryInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			sort, err := NewCategorySort(tt.value)

			if sort != CategorySort(tt.value) || !errors.Is(err, tt.want) || (err == nil) != (tt.want == nil) {
				t.Fatalf("NewCategorySort(%q) = (%q, %v), want %v", tt.value, sort, err, tt.want)
			}
		})
	}
}

func TestCategoryQueryValidate(t *testing.T) {
	emptyFilter := NewCategoryFilter("", "", nil)

	tests := []struct {
		name  string
		query CategoryQuery
		want  error
	}{
		{"valid", NewCategoryQuery(emptyFilter, domquery.SortOrderAsc, CategorySortName, 10, 1), nil},
		{"invalid filter", NewCategoryQuery(NewCategoryFilter("\x00", "", nil), domquery.SortOrderAsc, CategorySortID, 10, 1), ErrCategoryFilterInvalid},
		{"missing sort", NewCategoryQuery(emptyFilter, domquery.SortOrderAsc, "", 10, 1), ErrCategoryQueryInvalid},
		{"zero limit", NewCategoryQuery(emptyFilter, domquery.SortOrderAsc, CategorySortID, 0, 1), ErrCategoryQueryInvalid},
		{"zero page", NewCategoryQuery(emptyFilter, domquery.SortOrderAsc, CategorySortID, 10, 0), ErrCategoryQueryInvalid},
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

func TestCatalogs(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		check  func(string) error
		want   int
	}{
		{"colors", stringsOf(Colors()), func(value string) error { return Color(value).Validate() }, 8},
		{"icons", stringsOf(Icons()), func(value string) error { return Icon(value).Validate() }, 72},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := make(map[string]bool, len(tt.values))

			for _, value := range tt.values {
				if err := tt.check(value); err != nil || seen[value] {
					t.Fatalf("%q: valid = %v, repeated = %t", value, err, seen[value])
				}

				seen[value] = true
			}

			if len(tt.values) != tt.want {
				t.Fatalf("catalog has %d values, want %d", len(tt.values), tt.want)
			}
		})
	}

	Colors()[0] = "pink"

	if Colors()[0] != ColorGreen {
		t.Fatal("Colors() exposes the catalog for changes")
	}
}

func stringsOf[T ~string](values []T) []string {
	result := make([]string, 0, len(values))

	for _, value := range values {
		result = append(result, string(value))
	}

	return result
}
