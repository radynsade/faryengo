package pgxgoqu

import (
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/languages"
	languagepg "github.com/radynsade/faryengo/internal/languages/pgxgoqu"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/pkg/domquery"
)

// All schema and data are connection-local temporary objects. Migrations are
// never executed, and existing tables are never read or modified.
func TestRolePostgres(t *testing.T) {
	url := os.Getenv("FARYEN_ROLE_TEST_DATABASE_URL")

	if url == "" {
		t.Skip("set FARYEN_ROLE_TEST_DATABASE_URL to a disposable PostgreSQL instance")
	}

	config, err := pgxpool.ParseConfig(url)

	if err != nil {
		t.Fatal(err)
	}

	config.MaxConns = 1
	config.ConnConfig.RuntimeParams["search_path"] = "pg_catalog,pg_temp"
	config.MaxConnLifetime, config.MaxConnIdleTime = 0, 0
	pool, err := pgxpool.NewWithConfig(t.Context(), config)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)
	_, err = pool.Exec(t.Context(), `
CREATE TEMP TABLE "language" (code text PRIMARY KEY, english_name text, native_name text, is_fallback bool NOT NULL);
CREATE TEMP TABLE "text" (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY);
CREATE TYPE pg_temp.permission AS ENUM ('view_user', 'manage_user', 'view_role', 'manage_role');
CREATE TEMP TABLE "translation" (text_id bigint REFERENCES "text" ON DELETE CASCADE, language_code text REFERENCES "language", content text NOT NULL, PRIMARY KEY (text_id, language_code));
CREATE TEMP TABLE "role" (id uuid PRIMARY KEY, name_id bigint UNIQUE REFERENCES "text", permissions pg_temp.permission[] NOT NULL, is_super bool NOT NULL);
CREATE TEMP TABLE "user" (id uuid PRIMARY KEY, role_id uuid REFERENCES "role" ON DELETE RESTRICT);
CREATE FUNCTION pg_temp.delete_role_name() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN DELETE FROM pg_temp."text" WHERE id = OLD.name_id; RETURN NULL; END; $$;
CREATE TRIGGER delete_role_name AFTER DELETE ON "role" FOR EACH ROW EXECUTE FUNCTION pg_temp.delete_role_name();
INSERT INTO "language" VALUES ('en', 'English', 'English', true), ('lv', 'Latvian', 'Latviešu', false);`)

	if err != nil {
		t.Fatal(err)
	}

	repository, err := NewRoleRepository(pool)

	if err != nil {
		t.Fatal(err)
	}

	for _, data := range []struct {
		id          byte
		names       map[languages.Code]string
		permissions []security.Permission
		super       bool
	}{
		{id: 1, names: map[languages.Code]string{"en": "Editor", "lv": "Redaktors"}, permissions: []security.Permission{security.PermissionViewUser, security.PermissionViewRole}},
		{id: 2, names: map[languages.Code]string{"en": `100%_review\x`}, permissions: []security.Permission{security.PermissionViewRole}},
		{id: 3, names: map[languages.Code]string{"en": "Administrator"}, super: true},
		{id: 4, names: map[languages.Code]string{"lv": "Tikai"}},
	} {
		name := make(languages.Text)

		for code, content := range data.names {
			name[code] = languages.Translation(content)
		}

		role := security.NewRole(security.RoleID{data.id}, name, data.permissions, data.super)

		if createErr := repository.Create(t.Context(), role); createErr != nil {
			t.Fatal(createErr)
		}
	}

	no, yes := false, true

	for _, tt := range []struct {
		name    string
		filters security.RoleFilter
		count   int
	}{
		{name: "all", count: 4},
		{name: "UUID substring", filters: security.RoleFilter{IDLike: "01000000"}, count: 1},
		{name: "translated case insensitive name", filters: security.RoleFilter{NameLike: "REDAKT"}, count: 1},
		{name: "literal wildcards and slash", filters: security.RoleFilter{NameLike: `%_review\`}, count: 1},
		{name: "all permissions and super", filters: security.RoleFilter{Permissions: []security.Permission{security.PermissionViewUser, security.PermissionViewRole}}, count: 2},
		{name: "duplicate selection", filters: security.RoleFilter{Permissions: []security.Permission{security.PermissionViewRole, security.PermissionViewRole}}, count: 3},
		{name: "super", filters: security.RoleFilter{IsSuper: &yes}, count: 1},
		{name: "non-super", filters: security.RoleFilter{IsSuper: &no}, count: 3},
		{name: "combined", filters: security.RoleFilter{NameLike: "edit", IsSuper: &no, Permissions: []security.Permission{security.PermissionViewRole}}, count: 1},
		{name: "none", filters: security.RoleFilter{NameLike: "missing"}, count: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			count, countErr := repository.Count(t.Context(), tt.filters)

			if countErr != nil || count != tt.count {
				t.Fatalf("Count() = %d, %v, want %d", count, countErr, tt.count)
			}

			roles, findErr := repository.Find(t.Context(), security.NewRoleQuery(tt.filters, security.RoleSortID, domquery.SortOrderAsc, 25, 1, "en"))

			if findErr != nil || len(roles) != tt.count {
				t.Fatalf("Find() = %d roles, %v", len(roles), findErr)
			}
		})
	}

	for _, tt := range []struct {
		name       string
		sort       security.RoleSort
		language   languages.Code
		descending bool
		first      security.RoleID
	}{
		{name: "UUID ascending", sort: security.RoleSortID, language: "en", first: security.RoleID{1}},
		{name: "UUID descending", sort: security.RoleSortID, language: "en", descending: true, first: security.RoleID{4}},
		{name: "name English", sort: security.RoleSortName, language: "en", first: security.RoleID{2}},
		{name: "name Latvian", sort: security.RoleSortName, language: "lv", descending: true, first: security.RoleID{4}},
		{name: "super descending", sort: security.RoleSortIsSuper, language: "en", descending: true, first: security.RoleID{3}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			roles, findErr := repository.Find(t.Context(), security.NewRoleQuery(security.RoleFilter{}, tt.sort, domquery.SortOrder(!tt.descending), 1, 1, tt.language))

			if findErr != nil || len(roles) != 1 || roles[0].ID != tt.first {
				t.Fatalf("sort result = %v, %v", roles, findErr)
			}
		})
	}

	page, err := repository.Find(t.Context(), security.NewRoleQuery(security.RoleFilter{}, security.RoleSortID, domquery.SortOrderAsc, 2, 2, "en"))

	if err != nil || len(page) != 2 || page[0].ID != (security.RoleID{3}) || page[1].ID != (security.RoleID{4}) {
		t.Fatalf("pagination = %v, %v", page, err)
	}

	role, err := repository.FindByID(t.Context(), security.RoleID{1})

	if err != nil || len(role.Name) != 2 {
		t.Fatalf("stored translations = %v, %v", role, err)
	}

	role.Name = languages.Text{"zz": "Unknown"}

	if err := repository.Update(t.Context(), role); !errors.Is(err, languages.ErrLanguageNotFound) {
		t.Fatalf("unknown language = %v", err)
	}

	role, err = repository.FindByID(t.Context(), security.RoleID{1})

	if err != nil || string(role.Name["en"]) != "Editor" || len(role.Name) != 2 {
		t.Fatal("failed update did not roll back translations")
	}

	role.Permissions = []security.Permission{}
	if err := repository.Update(t.Context(), role); err != nil {
		t.Fatal(err)
	}

	_, err = pool.Exec(t.Context(), `INSERT INTO "user" VALUES ('00000000-0000-0000-0000-000000000001', $1)`, [16]byte(role.ID))

	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(t.Context(), role.ID); !errors.Is(err, security.ErrRoleAlreadyInUse) {
		t.Fatalf("assigned delete = %v", err)
	}
	if _, err := repository.FindByID(t.Context(), role.ID); err != nil {
		t.Fatal("assigned role was deleted")
	}
	if err := repository.Delete(t.Context(), security.RoleID{2}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FindByID(t.Context(), security.RoleID{2}); !errors.Is(err, security.ErrRoleNotFound) {
		t.Fatalf("deleted role = %v", err)
	}

	languageRepository, err := languagepg.NewLanguageRepository(pool)

	if err != nil {
		t.Fatal(err)
	}

	catalog, err := languageRepository.FindAll(t.Context())

	if err != nil || len(catalog) != 2 || catalog[0].Code != "en" || !catalog[0].IsFallback || catalog[1].Code != "lv" {
		t.Fatalf("language catalog = %v, %v", catalog, err)
	}
}
