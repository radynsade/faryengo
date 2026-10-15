package pgxgoqu

import (
	"context"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/infra/pgxdb"
)

//
// Errors
//

type errCurrencyWriteFailed struct {
	currency *currencies.Currency
	err      error
}

func (e errCurrencyWriteFailed) Currency() *currencies.Currency {
	return e.currency
}

func (e errCurrencyWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of currencies.ErrCurrencyCreateFailed

type errCurrencyCreateFailed struct {
	errCurrencyWriteFailed
}

func newErrCurrencyCreateFailed(currency *currencies.Currency, err error) *errCurrencyCreateFailed {
	return &errCurrencyCreateFailed{
		errCurrencyWriteFailed: errCurrencyWriteFailed{currency, err},
	}
}

func (e *errCurrencyCreateFailed) Error() string {
	return "failed to create a currency"
}

// Implementation of currencies.ErrCurrencyUpdateFailed

type errCurrencyUpdateFailed struct {
	errCurrencyWriteFailed
}

func newErrCurrencyUpdateFailed(currency *currencies.Currency, err error) *errCurrencyUpdateFailed {
	return &errCurrencyUpdateFailed{
		errCurrencyWriteFailed: errCurrencyWriteFailed{currency, err},
	}
}

func (e *errCurrencyUpdateFailed) Error() string {
	return "failed to update a currency"
}

// Implementation of currencies.ErrCurrencyDeleteFailed

type errCurrencyDeleteFailed struct {
	code currencies.Code
	err  error
}

func newErrCurrencyDeleteFailed(code currencies.Code, err error) *errCurrencyDeleteFailed {
	return &errCurrencyDeleteFailed{code, err}
}

func (e *errCurrencyDeleteFailed) CurrencyCode() currencies.Code {
	return e.code
}

func (e *errCurrencyDeleteFailed) Unwrap() error {
	return e.err
}

func (e *errCurrencyDeleteFailed) Error() string {
	return "failed to delete a currency"
}

//
// Repository
//

type CurrencyRepository struct {
	pool pgxdb.DB
}

var _ currencies.CurrencyRepository = (*CurrencyRepository)(nil)

func NewCurrencyRepository(pool pgxdb.DB) (*CurrencyRepository, error) {
	var (
		repository *CurrencyRepository
		err        error
	)

	if pgxdb.IsNil(pool) {
		err = pgxdb.ErrNilDB
	} else {
		repository = &CurrencyRepository{pool: pool}
	}

	return repository, err
}

// The primary key reports an existing currency.

func (r *CurrencyRepository) Create(
	ctx context.Context,
	currency *currencies.Currency,
) currencies.ErrCurrencyCreateFailed {
	var err currencies.ErrCurrencyCreateFailed

	if r == nil || r.pool == nil {
		err = newErrCurrencyCreateFailed(currency, pgxdb.ErrNilDB)
	} else if currency == nil {
		err = newErrCurrencyCreateFailed(currency, currencies.ErrCurrencyNil)
	} else if validationErr := currency.Validate(); validationErr != nil {
		err = newErrCurrencyCreateFailed(currency, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("currency").
			Rows(goqu.Record{
				"code":   string(currency.Code),
				"symbol": string(currency.Symbol),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrCurrencyCreateFailed(currency, buildErr)
		} else {
			_, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrCurrencyCreateFailed(currency, mapCurrencyError(execErr))
			}
		}
	}

	return err
}

// Update

func (r *CurrencyRepository) Update(
	ctx context.Context,
	currency *currencies.Currency,
) currencies.ErrCurrencyUpdateFailed {
	var err currencies.ErrCurrencyUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrCurrencyUpdateFailed(currency, pgxdb.ErrNilDB)
	} else if currency == nil {
		err = newErrCurrencyUpdateFailed(currency, currencies.ErrCurrencyNil)
	} else if validationErr := currency.Validate(); validationErr != nil {
		err = newErrCurrencyUpdateFailed(currency, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Update("currency").
			Set(goqu.Record{"symbol": string(currency.Symbol)}).
			Where(goqu.Ex{"code": string(currency.Code)}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrCurrencyUpdateFailed(currency, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrCurrencyUpdateFailed(currency, mapCurrencyError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrCurrencyUpdateFailed(currency, currencies.ErrCurrencyNotFound)
			}
		}
	}

	return err
}

// An account that holds money in the currency, or a budget set in it, reports it
// as in use.

func (r *CurrencyRepository) Delete(
	ctx context.Context,
	code currencies.Code,
) currencies.ErrCurrencyDeleteFailed {
	var err currencies.ErrCurrencyDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrCurrencyDeleteFailed(code, pgxdb.ErrNilDB)
	} else if validationErr := code.Validate(); validationErr != nil {
		err = newErrCurrencyDeleteFailed(code, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("currency").
			Where(goqu.Ex{"code": string(code)}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrCurrencyDeleteFailed(code, buildErr)
		} else {
			tag, execErr := pgxdb.FromContext(ctx, r.pool).Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrCurrencyDeleteFailed(code, mapCurrencyError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrCurrencyDeleteFailed(code, currencies.ErrCurrencyNotFound)
			}
		}
	}

	return err
}

// Find by a code

func (r *CurrencyRepository) FindByCode(
	ctx context.Context,
	code currencies.Code,
) (*currencies.Currency, error) {
	var (
		currency *currencies.Currency
		err      error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := code.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a currency by code: %w", validationErr)
	} else {
		currency, err = r.find(ctx, goqu.Ex{"code": string(code)}, false)

		if err != nil {
			err = fmt.Errorf("failed to find a currency %s: %w", code, err)
		}
	}

	return currency, err
}

// A row lock lasts only until its transaction ends, so locking requires a
// handle that can provide a transaction; a pool or a bare connection is
// rejected before I/O.

func (r *CurrencyRepository) FindByCodeForUpdate(
	ctx context.Context,
	code currencies.Code,
) (*currencies.Currency, error) {
	var (
		currency *currencies.Currency
		err      error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else if validationErr := code.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a currency by code for update: %w", validationErr)
	} else if !pgxdb.IsTransaction(pgxdb.FromContext(ctx, r.pool)) {
		err = pgxdb.ErrTransactionRequired
	} else {
		currency, err = r.find(ctx, goqu.Ex{"code": string(code)}, true)

		if err != nil {
			err = fmt.Errorf("failed to find a currency %s for update: %w", code, err)
		}
	}

	return currency, err
}

// Find all currencies ordered by a code

func (r *CurrencyRepository) FindAll(ctx context.Context) ([]*currencies.Currency, error) {
	var (
		result []*currencies.Currency
		err    error
	)

	if r == nil || r.pool == nil {
		err = pgxdb.ErrNilDB
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			From("currency").
			Select(currencyColumns()...).
			Order(goqu.C("code").Asc()).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the list currencies query: %w", buildErr)
		} else {
			rows, queryErr := pgxdb.FromContext(ctx, r.pool).Query(ctx, query, args...)

			if queryErr != nil {
				err = fmt.Errorf("failed to list currencies: %w", queryErr)
			} else {
				defer rows.Close()

				result = make([]*currencies.Currency, 0)

				for rows.Next() {
					var currency *currencies.Currency
					currency, err = scanCurrency(rows)

					if err != nil {
						break
					}

					result = append(result, currency)
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("failed to read currencies: %w", err)
					result = nil
				}
			}
		}
	}

	return result, err
}

//
// Helpers
//

func (r *CurrencyRepository) find(
	ctx context.Context,
	filter goqu.Ex,
	forUpdate bool,
) (*currencies.Currency, error) {
	var (
		currency *currencies.Currency
		err      error
	)

	dataset := goqu.Dialect("postgres").
		From("currency").
		Select(currencyColumns()...).
		Where(filter).
		Prepared(true)

	if forUpdate {
		dataset = dataset.ForUpdate(exp.Wait)
	}

	query, args, buildErr := dataset.ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the currency lookup query: %w", buildErr)
	} else {
		currency, err = scanCurrency(pgxdb.FromContext(ctx, r.pool).QueryRow(ctx, query, args...))
	}

	return currency, err
}

func currencyColumns() []any {
	return []any{
		"code",
		"symbol",
	}
}

func scanCurrency(row pgx.Row) (*currencies.Currency, error) {
	var (
		currency     *currencies.Currency
		err          error
		code, symbol string
	)

	scanErr := row.Scan(&code, &symbol)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = currencies.ErrCurrencyNotFound
	} else if scanErr != nil {
		err = fmt.Errorf("scan currency: %w", scanErr)
	} else {
		var (
			currencyCode   currencies.Code
			currencySymbol currencies.Symbol
		)

		currencyCode, err = currencies.NewCode(code)

		if err == nil {
			currencySymbol, err = currencies.NewSymbol(symbol)
		}

		if err == nil {
			currency = currencies.NewCurrency(currencyCode, currencySymbol)
		} else {
			err = fmt.Errorf("failed to decode a currency: %w: %w", currencies.ErrCurrencyInvalid, err)
		}
	}

	return currency, err
}

func mapCurrencyError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "currency_pkey":
			result = errors.Join(currencies.ErrCurrencyAlreadyExists, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "account_currency_code_fkey":
			result = errors.Join(currencies.ErrCurrencyInUse, err)
		case postgresErr.Code == "23503" && postgresErr.ConstraintName == "budget_currency_code_fkey":
			result = errors.Join(currencies.ErrCurrencyInUse, err)
		}
	}

	return result
}
