package pgxgoqu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/domquery"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// The pool never connects: every case must fail before any I/O.
func newLazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")

	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func TestNewCategoryRepositoryRejectsNilDB(t *testing.T) {
	tests := []struct {
		name string
		pool pgxdb.DB
	}{
		{"nil", nil},
		{"typed nil pool", (*pgxpool.Pool)(nil)},
		{"typed nil connection", (*pgx.Conn)(nil)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, err := NewCategoryRepository(test.pool)

			if !errors.Is(err, pgxdb.ErrNilDB) || repository != nil {
				t.Fatalf("got (%v, %v), want (nil, %v)", repository, err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestCategoryMethodsRejectMissingPool(t *testing.T) {
	ctx := context.Background()
	id := categories.ID(uuid.Must(uuid.NewV7()))

	tests := []struct {
		name string
		call func() error
	}{
		{"create", func() error { return (&CategoryRepository{}).Create(ctx, &categories.Category{}) }},
		{"update", func() error { return (&CategoryRepository{}).Update(ctx, &categories.Category{}) }},
		{"delete", func() error { return (*CategoryRepository)(nil).Delete(ctx, id) }},
		{"find by ID", func() error { _, err := (*CategoryRepository)(nil).FindByID(ctx, id); return err }},
		{"find by ID for update", func() error { _, err := (&CategoryRepository{}).FindByIDForUpdate(ctx, id); return err }},
		{"find", func() error { _, err := (&CategoryRepository{}).Find(ctx, categories.CategoryQuery{}); return err }},
		{"count", func() error { _, err := (&CategoryRepository{}).Count(ctx, categories.CategoryFilter{}); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, pgxdb.ErrNilDB) {
				t.Fatalf("got %v, want %v", err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestCategoryMethodsValidateBeforeIO(t *testing.T) {
	ctx := context.Background()
	repository := &CategoryRepository{pool: newLazyPool(t)}
	validID := categories.ID(uuid.Must(uuid.NewV7()))
	workspaceID := workspaces.ID(uuid.Must(uuid.NewV7()))
	unknownIcon := categories.Icon("rocket")

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"create nil", func() error { return repository.Create(ctx, nil) }, categories.ErrCategoryNil},
		{"create invalid", func() error { return repository.Create(ctx, &categories.Category{}) }, categories.ErrCategoryInvalid},
		{"create with an unknown icon", func() error {
			return repository.Create(ctx, categories.NewCategory(validID, workspaceID, "Food", &unknownIcon, categories.ColorRed))
		}, categories.ErrIconInvalid},
		{"create without a workspace", func() error {
			return repository.Create(ctx, categories.NewCategory(validID, workspaces.ID{}, "Food", nil, categories.ColorRed))
		}, workspaces.ErrIDInvalid},
		{"update nil", func() error { return repository.Update(ctx, nil) }, categories.ErrCategoryNil},
		{"update without a color", func() error {
			return repository.Update(ctx, categories.NewCategory(validID, workspaceID, "Food", nil, ""))
		}, categories.ErrColorInvalid},
		{"delete invalid ID", func() error { return repository.Delete(ctx, categories.ID{}) }, categories.ErrIDInvalid},
		{"find by invalid ID", func() error {
			_, err := repository.FindByID(ctx, categories.ID{})
			return err
		}, categories.ErrIDInvalid},
		{"find by invalid ID for update", func() error {
			_, err := repository.FindByIDForUpdate(ctx, categories.ID{})
			return err
		}, categories.ErrIDInvalid},
		{"find by ID for update outside a transaction", func() error {
			_, err := repository.FindByIDForUpdate(ctx, validID)
			return err
		}, pgxdb.ErrTransactionRequired},
		{"find without sort", func() error {
			_, err := repository.Find(ctx, categories.CategoryQuery{Limit: 10, Page: 1})
			return err
		}, categories.ErrCategoryQueryInvalid},
		{"find with zero page", func() error {
			_, err := repository.Find(ctx, categories.CategoryQuery{SortBy: categories.CategorySortID, Limit: 10})
			return err
		}, categories.ErrCategoryQueryInvalid},
		{"find with invalid workspace", func() error {
			_, err := repository.Find(ctx, categories.CategoryQuery{
				Filter: categories.CategoryFilter{WorkspaceID: &workspaces.ID{}},
				SortBy: categories.CategorySortID,
				Limit:  10,
				Page:   1,
			})
			return err
		}, workspaces.ErrIDInvalid},
		{"count with invalid filter", func() error {
			_, err := repository.Count(ctx, categories.CategoryFilter{NameLike: strings.Repeat("a", categories.MaxCategoryFilterNameLikeLength+1)})
			return err
		}, categories.ErrCategoryFilterInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestCategoryWriteFailuresExposeInput(t *testing.T) {
	ctx := context.Background()
	category := &categories.Category{}
	id := categories.ID{}
	repository := &CategoryRepository{}

	if err := repository.Create(ctx, category); err == nil || err.Category() != category {
		t.Fatalf("Create() = %v, want a failure exposing the category", err)
	}

	if err := repository.Update(ctx, category); err == nil || err.Category() != category {
		t.Fatalf("Update() = %v, want a failure exposing the category", err)
	}

	if err := repository.Delete(ctx, id); err == nil || err.CategoryID() != id {
		t.Fatalf("Delete() = %v, want a failure exposing the ID", err)
	}
}

func TestCategoryFindQuery(t *testing.T) {
	workspaceID := workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))

	tests := []struct {
		name     string
		query    categories.CategoryQuery
		wantSQL  []string
		wantArgs []any
	}{
		{
			name:     "sort by ID descending without a tie-breaker",
			query:    categories.CategoryQuery{SortBy: categories.CategorySortID, SortOrder: domquery.SortOrderDesc, Limit: 10, Page: 3},
			wantSQL:  []string{`ORDER BY "id" DESC LIMIT $1 OFFSET $2`},
			wantArgs: []any{int64(10), int64(20)},
		},
		{
			name:     "sort by name with the ID tie-breaker",
			query:    categories.CategoryQuery{SortBy: categories.CategorySortName, SortOrder: domquery.SortOrderAsc, Limit: 5, Page: 1},
			wantSQL:  []string{`ORDER BY lower("category".name) ASC, "id" ASC LIMIT $1`},
			wantArgs: []any{int64(5)},
		},
		{
			name: "every filter",
			query: categories.CategoryQuery{
				Filter: categories.CategoryFilter{
					IDLike:      "01_",
					NameLike:    "50%",
					WorkspaceID: &workspaceID,
				},
				SortBy: categories.CategorySortName,
				Limit:  1,
				Page:   1,
			},
			wantSQL: []string{
				`"category".id::text ILIKE $1`,
				`"category".name ILIKE $2`,
				`"workspace_id" = $3`,
			},
			wantArgs: []any{`%01\_%`, `%50\%%`, "01920000-0000-7000-8000-000000000003", int64(1)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql, args, err := categoryFilterDataset(test.query.Filter).
				Select(categoryColumns()...).
				Order(categoryOrder(test.query)...).
				Limit(test.query.Limit).
				Offset((test.query.Page - 1) * test.query.Limit).
				Prepared(true).
				ToSQL()

			if err != nil {
				t.Fatalf("build query: %v", err)
			}

			for _, fragment := range test.wantSQL {
				if !strings.Contains(sql, fragment) {
					t.Errorf("query %q does not contain %q", sql, fragment)
				}
			}

			if len(args) != len(test.wantArgs) {
				t.Fatalf("got args %v, want %v", args, test.wantArgs)
			}

			for index, want := range test.wantArgs {
				if args[index] != want {
					t.Errorf("arg %d: got %#v, want %#v", index, args[index], want)
				}
			}
		})
	}
}

func TestStoredIcon(t *testing.T) {
	icon := categories.IconCar

	tests := []struct {
		name string
		icon *categories.Icon
		want *string
	}{
		{"absent", nil, nil},
		{"present", &icon, new("car")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := storedIcon(test.icon)

			if (got == nil) != (test.want == nil) || (got != nil && *got != *test.want) {
				t.Fatalf("storedIcon() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMapCategoryErrors(t *testing.T) {
	cause := errors.New("connection reset")

	tests := []struct {
		name    string
		err     error
		want    error
		wantRaw bool
	}{
		{"category ID taken", &pgconn.PgError{Code: "23505", ConstraintName: "category_pkey"}, categories.ErrCategoryAlreadyExists, true},
		{"workspace missing", &pgconn.PgError{Code: "23503", ConstraintName: "category_workspace_id_fkey"}, workspaces.ErrWorkspaceNotFound, true},
		{"category covered by a budget", &pgconn.PgError{Code: "23503", ConstraintName: "budget_category_category_fkey"}, categories.ErrCategoryInUse, true},
		{"unrelated", cause, cause, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := mapCategoryError(test.err)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if _, ok := errors.AsType[*pgconn.PgError](err); ok != test.wantRaw {
				t.Fatalf("PostgreSQL cause preserved: got %t, want %t", ok, test.wantRaw)
			}
		})
	}
}
