package pgxgoqu

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
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

func TestNewCurrencyRepositoryRejectsNilDB(t *testing.T) {
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
			repository, err := NewCurrencyRepository(test.pool)

			if !errors.Is(err, pgxdb.ErrNilDB) || repository != nil {
				t.Fatalf("got (%v, %v), want (nil, %v)", repository, err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestCurrencyMethodsRejectMissingPool(t *testing.T) {
	ctx := context.Background()
	code := currencies.Code("USD")

	tests := []struct {
		name string
		call func() error
	}{
		{"create", func() error { return (&CurrencyRepository{}).Create(ctx, &currencies.Currency{}) }},
		{"update", func() error { return (&CurrencyRepository{}).Update(ctx, &currencies.Currency{}) }},
		{"delete", func() error { return (*CurrencyRepository)(nil).Delete(ctx, code) }},
		{"find by code", func() error { _, err := (*CurrencyRepository)(nil).FindByCode(ctx, code); return err }},
		{"find by code for update", func() error { _, err := (&CurrencyRepository{}).FindByCodeForUpdate(ctx, code); return err }},
		{"find all", func() error { _, err := (&CurrencyRepository{}).FindAll(ctx); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, pgxdb.ErrNilDB) {
				t.Fatalf("got %v, want %v", err, pgxdb.ErrNilDB)
			}
		})
	}
}

func TestCurrencyMethodsValidateBeforeIO(t *testing.T) {
	ctx := context.Background()
	repository := &CurrencyRepository{pool: newLazyPool(t)}

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"create nil", func() error { return repository.Create(ctx, nil) }, currencies.ErrCurrencyNil},
		{"create invalid", func() error { return repository.Create(ctx, &currencies.Currency{}) }, currencies.ErrCurrencyInvalid},
		{"create with an invalid code", func() error {
			return repository.Create(ctx, currencies.NewCurrency("usd", "$"))
		}, currencies.ErrCodeInvalid},
		{"update nil", func() error { return repository.Update(ctx, nil) }, currencies.ErrCurrencyNil},
		{"update with an invalid symbol", func() error {
			return repository.Update(ctx, currencies.NewCurrency("USD", "U S"))
		}, currencies.ErrSymbolInvalid},
		{"delete invalid code", func() error { return repository.Delete(ctx, "US") }, currencies.ErrCodeInvalid},
		{"find by invalid code", func() error {
			_, err := repository.FindByCode(ctx, "US")
			return err
		}, currencies.ErrCodeInvalid},
		{"find by invalid code for update", func() error {
			_, err := repository.FindByCodeForUpdate(ctx, "US")
			return err
		}, currencies.ErrCodeInvalid},
		{"find by code for update outside a transaction", func() error {
			_, err := repository.FindByCodeForUpdate(ctx, "USD")
			return err
		}, pgxdb.ErrTransactionRequired},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestCurrencyWriteFailuresExposeInput(t *testing.T) {
	ctx := context.Background()
	currency := &currencies.Currency{}
	code := currencies.Code("USD")
	repository := &CurrencyRepository{}

	if err := repository.Create(ctx, currency); err == nil || err.Currency() != currency {
		t.Fatalf("Create() = %v, want a failure exposing the currency", err)
	}

	if err := repository.Update(ctx, currency); err == nil || err.Currency() != currency {
		t.Fatalf("Update() = %v, want a failure exposing the currency", err)
	}

	if err := repository.Delete(ctx, code); err == nil || err.CurrencyCode() != code {
		t.Fatalf("Delete() = %v, want a failure exposing the code", err)
	}
}

func TestMapCurrencyErrors(t *testing.T) {
	cause := errors.New("connection reset")

	tests := []struct {
		name    string
		err     error
		want    error
		wantRaw bool
	}{
		{"currency code taken", &pgconn.PgError{Code: "23505", ConstraintName: "currency_pkey"}, currencies.ErrCurrencyAlreadyExists, true},
		{"currency used by an account", &pgconn.PgError{Code: "23503", ConstraintName: "account_currency_code_fkey"}, currencies.ErrCurrencyInUse, true},
		{"unrelated", cause, cause, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := mapCurrencyError(test.err)

			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}

			if _, ok := errors.AsType[*pgconn.PgError](err); ok != test.wantRaw {
				t.Fatalf("PostgreSQL cause preserved: got %t, want %t", ok, test.wantRaw)
			}
		})
	}
}
