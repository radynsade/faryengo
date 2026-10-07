package pgxgoqu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/security"
)

func TestRoleCountQueries(t *testing.T) {
	no := false

	for _, tt := range []struct {
		name      string
		filters   security.RoleFilters
		fragments []string
		args      []any
	}{
		{name: "all", fragments: []string{`COUNT(*)`, `FROM "role"`}},
		{name: "literal UUID", filters: security.RoleFilters{IDLike: "AB%_\\"}, fragments: []string{`id::text ILIKE $1`}, args: []any{`%AB\%\_\\%`}},
		{name: "any translation", filters: security.RoleFilters{NameLike: "Reader"}, fragments: []string{`EXISTS (SELECT 1 FROM "translation"`, `text_id = "role".name_id`, `content ILIKE $1`}, args: []any{"%Reader%"}},
		{name: "all permissions or super", filters: security.RoleFilters{Permissions: []security.Permission{security.PermissionViewRole, security.PermissionManageRole}}, fragments: []string{`"is_super" IS TRUE`, `permissions @> ARRAY[$1::permission,$2::permission]::permission[]`, " OR "}, args: []any{"view_role", "manage_role"}},
		{name: "false super", filters: security.RoleFilters{IsSuper: &no}, fragments: []string{`"is_super" IS FALSE`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{row: fakeSecurityRow{values: []any{int64(7)}}}
			repository := &RoleRepository{db: db}
			total, err := repository.Count(t.Context(), tt.filters)

			if err != nil || total != 7 || db.queryContext != t.Context() || len(db.queryArgs) != len(tt.args) {
				t.Fatalf("Count() = %d, %v; query = %s, args = %v", total, err, db.query, db.queryArgs)
			}

			for _, fragment := range tt.fragments {
				if !strings.Contains(db.query, fragment) {
					t.Fatalf("query %s missing %s", db.query, fragment)
				}
			}

			for index, value := range tt.args {
				if db.queryArgs[index] != value {
					t.Fatalf("bound args = %v, want %v", db.queryArgs, tt.args)
				}
			}
		})
	}
}

func TestRoleQueryRejectsInvalidInput(t *testing.T) {
	for _, tt := range []struct {
		name  string
		query security.RoleQuery
	}{
		{name: "invalid permissions", query: security.RoleQuery{Filters: security.RoleFilters{Permissions: []security.Permission{"bad"}}, Page: 1, PageSize: 25, Sort: security.RoleSortID, Language: "en"}},
		{name: "unbounded page", query: security.RoleQuery{Page: 1, PageSize: 1000, Sort: security.RoleSortID, Language: "en"}},
		{name: "sort injection", query: security.RoleQuery{Page: 1, PageSize: 25, Sort: "id; DROP TABLE role", Language: "en"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeSecurityDB{}
			roles, err := (&RoleRepository{db: db}).Find(t.Context(), tt.query)

			if !errors.Is(err, security.ErrInvalidRoleQuery) || roles != nil || db.query != "" {
				t.Fatalf("invalid Find() = %v, %v, query = %s", roles, err, db.query)
			}
		})
	}

	var repository *RoleRepository

	if _, err := repository.Count(context.Background(), security.RoleFilters{}); !errors.Is(err, ErrNilPool) {
		t.Fatal(err)
	}
	if _, err := repository.Find(context.Background(), security.RoleQuery{}); !errors.Is(err, ErrNilPool) {
		t.Fatal(err)
	}
}

func TestRoleFindQueryFailure(t *testing.T) {
	for _, sort := range []security.RoleSort{security.RoleSortID, security.RoleSortName, security.RoleSortSuper} {
		t.Run(string(sort), func(t *testing.T) {
			db := &fakeSecurityDB{}
			options := security.RoleQuery{Filters: security.RoleFilters{NameLike: "O'Reilly"}, Sort: sort, Page: 2, PageSize: 25, Language: "lv", Descending: true}
			roles, err := (&RoleRepository{db: db}).Find(t.Context(), options)

			if !errors.Is(err, context.Canceled) || roles != nil || db.queryContext != t.Context() || strings.Contains(db.query, "O'Reilly") || !strings.Contains(db.query, "ORDER BY") || !strings.Contains(db.query, "OFFSET") {
				t.Fatalf("Find() = %v, %v; query = %s", roles, err, db.query)
			}

			if sort == security.RoleSortName && (!strings.Contains(db.query, "l.is_fallback DESC") || !strings.Contains(db.query, `"id" ASC`)) {
				t.Fatal("name sorting must use the fallback and a stable UUID tie breaker")
			}
		})
	}
}
