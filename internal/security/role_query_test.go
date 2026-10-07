package security_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/pkg/domquery"
)

func TestRoleQueryValidate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		filter      security.RoleFilter
		sort        security.RoleSort
		limit, page int
		language    languages.Code
		want        error
	}{
		{name: "ID", sort: security.RoleSortID, limit: 25, page: 1, language: "en"},
		{name: "name", sort: security.RoleSortName, limit: 25, page: 1, language: "lv"},
		{name: "super", sort: security.RoleSortIsSuper, limit: 100, page: 1, language: "en"},
		{name: "sort injection", sort: "id; DROP TABLE role", limit: 25, page: 1, language: "en", want: security.ErrInvalidRoleQuery},
		{name: "zero page", sort: security.RoleSortID, limit: 25, language: "en", want: security.ErrInvalidRoleQuery},
		{name: "oversized page", sort: security.RoleSortID, limit: 25, page: 1_000_001, language: "en", want: security.ErrInvalidRoleQuery},
		{name: "oversized limit", sort: security.RoleSortID, limit: 101, page: 1, language: "en", want: security.ErrInvalidRoleQuery},
		{name: "invalid language", sort: security.RoleSortID, limit: 25, page: 1, language: "EN", want: languages.ErrInvalidCode},
		{name: "invalid permission", filter: security.RoleFilter{Permissions: []security.Permission{"unknown"}}, want: security.ErrInvalidPermission},
		{name: "long ID", filter: security.RoleFilter{IDLike: strings.Repeat("a", 101)}, want: security.ErrInvalidRoleQuery},
		{name: "long name", filter: security.RoleFilter{NameLike: strings.Repeat("a", 501)}, want: security.ErrInvalidRoleQuery},
		{name: "NUL", filter: security.RoleFilter{NameLike: "a\x00b"}, want: security.ErrInvalidRoleQuery},
		{name: "invalid UTF-8", filter: security.RoleFilter{NameLike: "a\xff"}, want: security.ErrInvalidRoleQuery},
	} {
		t.Run(tt.name, func(t *testing.T) {
			query := security.NewRoleQuery(tt.filter, tt.sort, domquery.SortOrderDesc, tt.limit, tt.page, tt.language)
			err := query.Validate()

			if !errors.Is(err, tt.want) || (tt.want != nil && !errors.Is(err, security.ErrInvalidRoleQuery)) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}

			if query.SortBy() != tt.sort || !query.SortOrder().IsDesc() || query.Limit() != tt.limit || query.Page() != tt.page {
				t.Fatal("query changed")
			}
		})
	}
}

func TestRoleFilterCount(t *testing.T) {
	no := false

	for _, tt := range []struct {
		name   string
		filter security.RoleFilter
		want   int
	}{
		{name: "none"},
		{name: "non-super is a filter", filter: security.RoleFilter{IsSuper: &no}, want: 1},
		{name: "empty permissions", filter: security.RoleFilter{Permissions: []security.Permission{}}},
		{name: "all", filter: security.RoleFilter{IDLike: "a", NameLike: "b", IsSuper: &no, Permissions: security.AllPermissions()}, want: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Count(); got != tt.want {
				t.Fatalf("Count() = %d, want %d", got, tt.want)
			}
		})
	}
}
