package security

import (
	"errors"
	"strings"
	"testing"
)

func TestRoleQueryValidation(t *testing.T) {
	valid := RoleQuery{Page: 1, PageSize: DefaultRolePageSize, Sort: RoleSortID, Language: "en"}

	for _, tt := range []struct {
		name   string
		change func(*RoleQuery)
		want   error
	}{
		{name: "default", change: func(*RoleQuery) {}},
		{name: "name sort", change: func(q *RoleQuery) { q.Sort = RoleSortName }},
		{name: "super sort", change: func(q *RoleQuery) { q.Sort = RoleSortSuper }},
		{name: "bad sort", change: func(q *RoleQuery) { q.Sort = "name; DROP TABLE role" }, want: ErrInvalidRoleQuery},
		{name: "zero page", change: func(q *RoleQuery) { q.Page = 0 }, want: ErrInvalidRoleQuery},
		{name: "huge page", change: func(q *RoleQuery) { q.Page = 1_000_001 }, want: ErrInvalidRoleQuery},
		{name: "large page size", change: func(q *RoleQuery) { q.PageSize = 101 }, want: ErrInvalidRoleQuery},
		{name: "invalid language", change: func(q *RoleQuery) { q.Language = "EN" }, want: ErrInvalidRoleQuery},
		{name: "invalid permission", change: func(q *RoleQuery) { q.Filters.Permissions = []Permission{"root"} }, want: ErrInvalidPermission},
		{name: "long UUID", change: func(q *RoleQuery) { q.Filters.IDLike = strings.Repeat("a", 101) }, want: ErrInvalidRoleQuery},
		{name: "invalid UTF8", change: func(q *RoleQuery) { q.Filters.NameLike = string([]byte{255}) }, want: ErrInvalidRoleQuery},
		{name: "NUL", change: func(q *RoleQuery) { q.Filters.NameLike = "a\x00b" }, want: ErrInvalidRoleQuery},
	} {
		t.Run(tt.name, func(t *testing.T) {
			query := valid
			tt.change(&query)

			if err := query.Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRoleFilterCount(t *testing.T) {
	no := false
	filters := RoleFilters{IDLike: "a", NameLike: "Admin", Permissions: AllPermissions(), IsSuper: &no}

	if filters.Count() != 4 || (RoleFilters{}).Count() != 0 {
		t.Fatal("filter count must count each active field, including false super status")
	}
}

func TestRolePages(t *testing.T) {
	for _, tt := range []struct {
		name              string
		total, size, want int
	}{
		{name: "empty", size: 25, want: 1},
		{name: "uninitialized", want: 1},
		{name: "one", total: 25, size: 25, want: 1},
		{name: "two", total: 26, size: 25, want: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := (RolePage{Total: tt.total, PageSize: tt.size}).Pages(); got != tt.want {
				t.Fatalf("Pages() = %d, want %d", got, tt.want)
			}
		})
	}
}
