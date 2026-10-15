package pgxgoqu

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/budgets"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/domquery"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

var (
	testBudgetID    = budgets.ID(uuid.MustParse("01920000-0000-7000-8000-000000000007"))
	testWorkspaceID = workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
	testCategoryID  = categories.ID(uuid.MustParse("01920000-0000-7000-8000-000000000004"))
	testCategoryID2 = categories.ID(uuid.MustParse("01920000-0000-7000-8000-000000000006"))
)

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

func TestNewBudgetRepositoryRejectsNilDB(t *testing.T) {
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
			repository, err := NewBudgetRepository(test.pool)

			if !errors.Is(err, pgxdb.ErrNilDB) || repository != nil {
				t.Fatalf("got (%v, %v), want (nil, %v)", repository, err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestBudgetMethodsRejectMissingPool(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"create", func() error { return (&BudgetRepository{}).Create(ctx, &budgets.Budget{}) }},
		{"update", func() error { return (&BudgetRepository{}).Update(ctx, &budgets.Budget{}) }},
		{"delete", func() error { return (*BudgetRepository)(nil).Delete(ctx, testBudgetID) }},
		{"find by ID", func() error { _, err := (*BudgetRepository)(nil).FindByID(ctx, testBudgetID); return err }},
		{"find by ID for update", func() error { _, err := (&BudgetRepository{}).FindByIDForUpdate(ctx, testBudgetID); return err }},
		{"find", func() error { _, err := (&BudgetRepository{}).Find(ctx, budgets.BudgetQuery{}); return err }},
		{"count", func() error { _, err := (&BudgetRepository{}).Count(ctx, budgets.BudgetFilter{}); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, pgxdb.ErrNilDB) {
				t.Fatalf("got %v, want %v", err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestBudgetMethodsValidateBeforeIO(t *testing.T) {
	ctx := context.Background()
	repository := &BudgetRepository{pool: newLazyPool(t)}
	categoryIDs := budgets.CategoryIDs{testCategoryID}
	invalidCategoryID := categories.ID{}

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"create nil", func() error { return repository.Create(ctx, nil) }, budgets.ErrBudgetNil},
		{"create invalid", func() error { return repository.Create(ctx, &budgets.Budget{}) }, budgets.ErrBudgetInvalid},
		{"create without a workspace", func() error {
			return repository.Create(ctx, budgets.NewBudget(testBudgetID, workspaces.ID{}, categoryIDs, "EUR", 100))
		}, workspaces.ErrIDInvalid},
		{"create without categories", func() error {
			return repository.Create(ctx, budgets.NewBudget(testBudgetID, testWorkspaceID, nil, "EUR", 100))
		}, budgets.ErrCategoryIDsEmpty},
		{"create without a currency", func() error {
			return repository.Create(ctx, budgets.NewBudget(testBudgetID, testWorkspaceID, categoryIDs, "", 100))
		}, currencies.ErrCodeInvalid},
		{"update nil", func() error { return repository.Update(ctx, nil) }, budgets.ErrBudgetNil},
		{"update with duplicate categories", func() error {
			return repository.Update(ctx, budgets.NewBudget(
				testBudgetID, testWorkspaceID, budgets.CategoryIDs{testCategoryID, testCategoryID}, "EUR", 100,
			))
		}, budgets.ErrCategoryIDsDuplicate},
		{"update with a negative amount", func() error {
			return repository.Update(ctx, budgets.NewBudget(testBudgetID, testWorkspaceID, categoryIDs, "EUR", -1))
		}, budgets.ErrAmountNegative},
		{"delete invalid ID", func() error { return repository.Delete(ctx, budgets.ID{}) }, budgets.ErrIDInvalid},
		{"find by invalid ID", func() error {
			_, err := repository.FindByID(ctx, budgets.ID{})
			return err
		}, budgets.ErrIDInvalid},
		{"find by invalid ID for update", func() error {
			_, err := repository.FindByIDForUpdate(ctx, budgets.ID{})
			return err
		}, budgets.ErrIDInvalid},
		{"find by ID for update outside a transaction", func() error {
			_, err := repository.FindByIDForUpdate(ctx, testBudgetID)
			return err
		}, pgxdb.ErrTransactionRequired},
		{"find without sort", func() error {
			_, err := repository.Find(ctx, budgets.BudgetQuery{Limit: 10, Page: 1})
			return err
		}, budgets.ErrBudgetQueryInvalid},
		{"find with an invalid category", func() error {
			_, err := repository.Find(ctx, budgets.BudgetQuery{
				Filter: budgets.BudgetFilter{CategoryID: &invalidCategoryID},
				SortBy: budgets.BudgetSortID,
				Limit:  10,
				Page:   1,
			})
			return err
		}, categories.ErrIDInvalid},
		{"count with invalid filter", func() error {
			_, err := repository.Count(ctx, budgets.BudgetFilter{IDLike: strings.Repeat("a", budgets.MaxBudgetFilterIDLikeLength+1)})
			return err
		}, budgets.ErrBudgetFilterInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestBudgetWriteFailuresExposeInput(t *testing.T) {
	ctx := context.Background()
	budget := &budgets.Budget{}
	id := budgets.ID{}
	repository := &BudgetRepository{}

	if err := repository.Create(ctx, budget); err == nil || err.Budget() != budget {
		t.Fatalf("Create() = %v, want a failure exposing the budget", err)
	}

	if err := repository.Update(ctx, budget); err == nil || err.Budget() != budget {
		t.Fatalf("Update() = %v, want a failure exposing the budget", err)
	}

	if err := repository.Delete(ctx, id); err == nil || err.BudgetID() != id {
		t.Fatalf("Delete() = %v, want a failure exposing the ID", err)
	}
}

func TestBudgetFindQuery(t *testing.T) {
	currency := currencies.Code("EUR")

	tests := []struct {
		name     string
		query    budgets.BudgetQuery
		wantSQL  []string
		wantArgs []any
	}{
		{
			name:  "categories in their written order",
			query: budgets.BudgetQuery{SortBy: budgets.BudgetSortID, Limit: 1, Page: 1},
			wantSQL: []string{
				`SELECT "id", "workspace_id", ARRAY(SELECT "budget_category".category_id FROM "budget_category" ` +
					`WHERE "budget_category".budget_id = "budget".id ORDER BY "budget_category".position), ` +
					`"currency_code", "amount" FROM "budget"`,
			},
			wantArgs: []any{int64(1)},
		},
		{
			name:     "sort by ID descending without a tie-breaker",
			query:    budgets.BudgetQuery{SortBy: budgets.BudgetSortID, SortOrder: domquery.SortOrderDesc, Limit: 10, Page: 3},
			wantSQL:  []string{`ORDER BY "id" DESC LIMIT $1 OFFSET $2`},
			wantArgs: []any{int64(10), int64(20)},
		},
		{
			name:     "sort by amount with the ID tie-breaker",
			query:    budgets.BudgetQuery{SortBy: budgets.BudgetSortAmount, SortOrder: domquery.SortOrderAsc, Limit: 5, Page: 1},
			wantSQL:  []string{`ORDER BY "amount" ASC, "id" ASC LIMIT $1`},
			wantArgs: []any{int64(5)},
		},
		{
			name: "every filter",
			query: budgets.BudgetQuery{
				Filter: budgets.BudgetFilter{
					IDLike:      "01_",
					WorkspaceID: &testWorkspaceID,
					CategoryID:  &testCategoryID,
					Currency:    &currency,
				},
				SortBy: budgets.BudgetSortID,
				Limit:  1,
				Page:   1,
			},
			wantSQL: []string{
				`"budget".id::text ILIKE $1`,
				`"workspace_id" = $2`,
				`EXISTS (SELECT 1 FROM "budget_category" WHERE "budget_category".budget_id = "budget".id ` +
					`AND "budget_category".category_id = $3)`,
				`"currency_code" = $4`,
			},
			wantArgs: []any{
				`%01\_%`,
				"01920000-0000-7000-8000-000000000003",
				"01920000-0000-7000-8000-000000000004",
				"EUR",
				int64(1),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql, args, err := budgetFilterDataset(test.query.Filter).
				Select(budgetColumns()...).
				Order(budgetOrder(test.query)...).
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

			if !slices.Equal(args, test.wantArgs) {
				t.Fatalf("got args %#v, want %#v", args, test.wantArgs)
			}
		})
	}
}

func TestBudgetCategoriesInsert(t *testing.T) {
	budget := budgets.NewBudget(
		testBudgetID,
		testWorkspaceID,
		budgets.CategoryIDs{testCategoryID2, testCategoryID},
		"EUR",
		100,
	)

	sql, args, err := budgetCategoriesInsert(budget).ToSQL()

	if err != nil {
		t.Fatalf("build insert: %v", err)
	}

	wantSQL := `INSERT INTO "budget_category" ("budget_id", "category_id", "position", "workspace_id") ` +
		`VALUES ($1, $2, $3, $4), ($5, $6, $7, $8)`
	wantArgs := []any{
		"01920000-0000-7000-8000-000000000007", "01920000-0000-7000-8000-000000000006", int64(0), "01920000-0000-7000-8000-000000000003",
		"01920000-0000-7000-8000-000000000007", "01920000-0000-7000-8000-000000000004", int64(1), "01920000-0000-7000-8000-000000000003",
	}

	if sql != wantSQL {
		t.Fatalf("got %q, want %q", sql, wantSQL)
	}

	if !slices.Equal(args, wantArgs) {
		t.Fatalf("got args %#v, want %#v", args, wantArgs)
	}
}

func TestDecodeCategoryIDs(t *testing.T) {
	stored := func(ids ...categories.ID) []pgtype.UUID {
		result := make([]pgtype.UUID, 0, len(ids))

		for _, id := range ids {
			result = append(result, pgtype.UUID{Bytes: id, Valid: true})
		}

		return result
	}

	tests := []struct {
		name   string
		stored []pgtype.UUID
		want   budgets.CategoryIDs
		err    error
	}{
		{"in stored order", stored(testCategoryID2, testCategoryID), budgets.CategoryIDs{testCategoryID2, testCategoryID}, nil},
		{"none", nil, nil, budgets.ErrCategoryIDsEmpty},
		{"nil UUID", stored(testCategoryID, categories.ID{}), nil, categories.ErrIDInvalid},
		{"duplicate", stored(testCategoryID, testCategoryID), nil, budgets.ErrCategoryIDsDuplicate},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeCategoryIDs(test.stored)

			if !errors.Is(err, test.err) || (err == nil) != (test.err == nil) {
				t.Fatalf("got error %v, want %v", err, test.err)
			}

			if !slices.Equal(got, test.want) || (got == nil) != (test.want == nil) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestMapBudgetErrors(t *testing.T) {
	cause := errors.New("connection reset")
	rolledBack := errors.Join(
		&pgconn.PgError{Code: "23503", ConstraintName: "budget_category_category_fkey"},
		errors.New("roll back a transaction: conn closed"),
	)

	tests := []struct {
		name    string
		err     error
		want    error
		wantRaw bool
	}{
		{"budget ID taken", &pgconn.PgError{Code: "23505", ConstraintName: "budget_pkey"}, budgets.ErrBudgetAlreadyExists, true},
		{"category repeated", &pgconn.PgError{Code: "23505", ConstraintName: "budget_category_pkey"}, budgets.ErrCategoryIDsDuplicate, true},
		{"workspace missing", &pgconn.PgError{Code: "23503", ConstraintName: "budget_workspace_id_fkey"}, workspaces.ErrWorkspaceNotFound, true},
		{"currency missing", &pgconn.PgError{Code: "23503", ConstraintName: "budget_currency_code_fkey"}, currencies.ErrCurrencyNotFound, true},
		{"category missing or foreign", &pgconn.PgError{Code: "23503", ConstraintName: "budget_category_category_fkey"}, categories.ErrCategoryNotFound, true},
		{"category missing with a failed rollback", rolledBack, categories.ErrCategoryNotFound, true},
		{"budget missing on update", budgets.ErrBudgetNotFound, budgets.ErrBudgetNotFound, false},
		{"unrelated", cause, cause, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := mapBudgetError(test.err)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if _, ok := errors.AsType[*pgconn.PgError](err); ok != test.wantRaw {
				t.Fatalf("PostgreSQL cause preserved: got %t, want %t", ok, test.wantRaw)
			}
		})
	}
}
