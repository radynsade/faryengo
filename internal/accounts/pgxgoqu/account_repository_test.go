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

	"github.com/radynsade/faryengo/internal/accounts"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/users"
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

func TestNewAccountRepositoryRejectsNilDB(t *testing.T) {
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
			repository, err := NewAccountRepository(test.pool)

			if !errors.Is(err, pgxdb.ErrNilDB) || repository != nil {
				t.Fatalf("got (%v, %v), want (nil, %v)", repository, err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestAccountMethodsRejectMissingPool(t *testing.T) {
	ctx := context.Background()
	id := accounts.ID(uuid.Must(uuid.NewV7()))

	tests := []struct {
		name string
		call func() error
	}{
		{"create", func() error { return (&AccountRepository{}).Create(ctx, &accounts.Account{}) }},
		{"update", func() error { return (&AccountRepository{}).Update(ctx, &accounts.Account{}) }},
		{"delete", func() error { return (*AccountRepository)(nil).Delete(ctx, id) }},
		{"find by ID", func() error { _, err := (*AccountRepository)(nil).FindByID(ctx, id); return err }},
		{"find by ID for update", func() error { _, err := (&AccountRepository{}).FindByIDForUpdate(ctx, id); return err }},
		{"find", func() error { _, err := (&AccountRepository{}).Find(ctx, accounts.AccountQuery{}); return err }},
		{"count", func() error { _, err := (&AccountRepository{}).Count(ctx, accounts.AccountFilter{}); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, pgxdb.ErrNilDB) {
				t.Fatalf("got %v, want %v", err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestAccountMethodsValidateBeforeIO(t *testing.T) {
	ctx := context.Background()
	repository := &AccountRepository{pool: newLazyPool(t)}
	validID := accounts.ID(uuid.Must(uuid.NewV7()))
	workspaceID := workspaces.ID(uuid.Must(uuid.NewV7()))
	invalidUserID := users.UserID{}

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"create nil", func() error { return repository.Create(ctx, nil) }, accounts.ErrAccountNil},
		{"create invalid", func() error { return repository.Create(ctx, &accounts.Account{}) }, accounts.ErrAccountInvalid},
		{"create without a workspace", func() error {
			return repository.Create(ctx, accounts.NewAccount(validID, workspaces.ID{}, nil, "Cash"))
		}, workspaces.ErrIDInvalid},
		{"create with an invalid user", func() error {
			return repository.Create(ctx, accounts.NewAccount(validID, workspaceID, &invalidUserID, "Cash"))
		}, users.ErrUserIDInvalid},
		{"update nil", func() error { return repository.Update(ctx, nil) }, accounts.ErrAccountNil},
		{"update without a name", func() error {
			return repository.Update(ctx, accounts.NewAccount(validID, workspaceID, nil, ""))
		}, accounts.ErrNameInvalid},
		{"delete invalid ID", func() error { return repository.Delete(ctx, accounts.ID{}) }, accounts.ErrIDInvalid},
		{"find by invalid ID", func() error {
			_, err := repository.FindByID(ctx, accounts.ID{})
			return err
		}, accounts.ErrIDInvalid},
		{"find by invalid ID for update", func() error {
			_, err := repository.FindByIDForUpdate(ctx, accounts.ID{})
			return err
		}, accounts.ErrIDInvalid},
		{"find by ID for update outside a transaction", func() error {
			_, err := repository.FindByIDForUpdate(ctx, validID)
			return err
		}, pgxdb.ErrTransactionRequired},
		{"find without sort", func() error {
			_, err := repository.Find(ctx, accounts.AccountQuery{Limit: 10, Page: 1})
			return err
		}, accounts.ErrAccountQueryInvalid},
		{"find with an invalid user", func() error {
			_, err := repository.Find(ctx, accounts.AccountQuery{
				Filter: accounts.AccountFilter{UserID: &invalidUserID},
				SortBy: accounts.AccountSortID,
				Limit:  10,
				Page:   1,
			})
			return err
		}, users.ErrUserIDInvalid},
		{"count with invalid filter", func() error {
			_, err := repository.Count(ctx, accounts.AccountFilter{NameLike: strings.Repeat("a", accounts.MaxAccountFilterNameLikeLength+1)})
			return err
		}, accounts.ErrAccountFilterInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestAccountWriteFailuresExposeInput(t *testing.T) {
	ctx := context.Background()
	account := &accounts.Account{}
	id := accounts.ID{}
	repository := &AccountRepository{}

	if err := repository.Create(ctx, account); err == nil || err.Account() != account {
		t.Fatalf("Create() = %v, want a failure exposing the account", err)
	}

	if err := repository.Update(ctx, account); err == nil || err.Account() != account {
		t.Fatalf("Update() = %v, want a failure exposing the account", err)
	}

	if err := repository.Delete(ctx, id); err == nil || err.AccountID() != id {
		t.Fatalf("Delete() = %v, want a failure exposing the ID", err)
	}
}

func TestAccountFindQuery(t *testing.T) {
	workspaceID := workspaces.ID(uuid.MustParse("01920000-0000-7000-8000-000000000003"))
	userID := users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001"))

	tests := []struct {
		name     string
		query    accounts.AccountQuery
		wantSQL  []string
		wantArgs []any
	}{
		{
			name:     "sort by ID descending without a tie-breaker",
			query:    accounts.AccountQuery{SortBy: accounts.AccountSortID, SortOrder: domquery.SortOrderDesc, Limit: 10, Page: 3},
			wantSQL:  []string{`ORDER BY "id" DESC LIMIT $1 OFFSET $2`},
			wantArgs: []any{int64(10), int64(20)},
		},
		{
			name:     "sort by name with the ID tie-breaker",
			query:    accounts.AccountQuery{SortBy: accounts.AccountSortName, SortOrder: domquery.SortOrderAsc, Limit: 5, Page: 1},
			wantSQL:  []string{`ORDER BY lower("account".name) ASC, "id" ASC LIMIT $1`},
			wantArgs: []any{int64(5)},
		},
		{
			name: "every filter",
			query: accounts.AccountQuery{
				Filter: accounts.AccountFilter{
					IDLike:      "01_",
					NameLike:    "50%",
					WorkspaceID: &workspaceID,
					UserID:      &userID,
				},
				SortBy: accounts.AccountSortName,
				Limit:  1,
				Page:   1,
			},
			wantSQL: []string{
				`"account".id::text ILIKE $1`,
				`"account".name ILIKE $2`,
				`"workspace_id" = $3`,
				`"user_id" = $4`,
			},
			wantArgs: []any{
				`%01\_%`,
				`%50\%%`,
				"01920000-0000-7000-8000-000000000003",
				"01920000-0000-7000-8000-000000000001",
				int64(1),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql, args, err := accountFilterDataset(test.query.Filter).
				Select(accountColumns()...).
				Order(accountOrder(test.query)...).
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

func TestStoredUserID(t *testing.T) {
	userID := users.UserID(uuid.MustParse("01920000-0000-7000-8000-000000000001"))

	if got := storedUserID(nil); got != nil {
		t.Fatalf("storedUserID(nil) = %v, want nil", *got)
	}

	if got := storedUserID(&userID); got == nil || *got != "01920000-0000-7000-8000-000000000001" {
		t.Fatalf("storedUserID() = %v, want the user ID as text", got)
	}
}

func TestMapAccountErrors(t *testing.T) {
	cause := errors.New("connection reset")

	tests := []struct {
		name    string
		err     error
		want    error
		wantRaw bool
	}{
		{"account ID taken", &pgconn.PgError{Code: "23505", ConstraintName: "account_pkey"}, accounts.ErrAccountAlreadyExists, true},
		{"workspace missing", &pgconn.PgError{Code: "23503", ConstraintName: "account_workspace_id_fkey"}, workspaces.ErrWorkspaceNotFound, true},
		{"user missing", &pgconn.PgError{Code: "23503", ConstraintName: "account_user_id_fkey"}, users.ErrUserNotFound, true},
		{"unrelated", cause, cause, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := mapAccountError(test.err)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if _, ok := errors.AsType[*pgconn.PgError](err); ok != test.wantRaw {
				t.Fatalf("PostgreSQL cause preserved: got %t, want %t", ok, test.wantRaw)
			}
		})
	}
}
