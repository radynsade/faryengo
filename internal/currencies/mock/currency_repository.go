package mock

import (
	"context"
	"errors"

	"github.com/radynsade/faryengo/internal/currencies"
)

//
// Errors
//

var ErrNotConfigured = errors.New("mock operation is not configured")

//
// Currency write failures
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

type ErrCurrencyCreateFailed struct {
	errCurrencyWriteFailed
}

func NewErrCurrencyCreateFailed(currency *currencies.Currency, err error) *ErrCurrencyCreateFailed {
	return &ErrCurrencyCreateFailed{
		errCurrencyWriteFailed: errCurrencyWriteFailed{currency, err},
	}
}

func (e *ErrCurrencyCreateFailed) Error() string {
	return "failed to create a currency"
}

// Implementation of currencies.ErrCurrencyUpdateFailed

type ErrCurrencyUpdateFailed struct {
	errCurrencyWriteFailed
}

func NewErrCurrencyUpdateFailed(currency *currencies.Currency, err error) *ErrCurrencyUpdateFailed {
	return &ErrCurrencyUpdateFailed{
		errCurrencyWriteFailed: errCurrencyWriteFailed{currency, err},
	}
}

func (e *ErrCurrencyUpdateFailed) Error() string {
	return "failed to update a currency"
}

// Implementation of currencies.ErrCurrencyDeleteFailed

type ErrCurrencyDeleteFailed struct {
	code currencies.Code
	err  error
}

func NewErrCurrencyDeleteFailed(code currencies.Code, err error) *ErrCurrencyDeleteFailed {
	return &ErrCurrencyDeleteFailed{code, err}
}

func (e *ErrCurrencyDeleteFailed) CurrencyCode() currencies.Code {
	return e.code
}

func (e *ErrCurrencyDeleteFailed) Unwrap() error {
	return e.err
}

func (e *ErrCurrencyDeleteFailed) Error() string {
	return "failed to delete a currency"
}

//
// Currency repository
//

type CurrencyRepository struct {
	CreateFunc              func(ctx context.Context, currency *currencies.Currency) currencies.ErrCurrencyCreateFailed
	UpdateFunc              func(ctx context.Context, currency *currencies.Currency) currencies.ErrCurrencyUpdateFailed
	DeleteFunc              func(ctx context.Context, code currencies.Code) currencies.ErrCurrencyDeleteFailed
	FindByCodeFunc          func(ctx context.Context, code currencies.Code) (*currencies.Currency, error)
	FindByCodeForUpdateFunc func(ctx context.Context, code currencies.Code) (*currencies.Currency, error)
	FindAllFunc             func(ctx context.Context) ([]*currencies.Currency, error)
}

var _ currencies.CurrencyRepository = (*CurrencyRepository)(nil)

// Create

func (r *CurrencyRepository) Create(
	ctx context.Context,
	currency *currencies.Currency,
) currencies.ErrCurrencyCreateFailed {
	var err currencies.ErrCurrencyCreateFailed

	if r.CreateFunc != nil {
		err = r.CreateFunc(ctx, currency)
	} else {
		err = NewErrCurrencyCreateFailed(currency, ErrNotConfigured)
	}

	return err
}

// Update

func (r *CurrencyRepository) Update(
	ctx context.Context,
	currency *currencies.Currency,
) currencies.ErrCurrencyUpdateFailed {
	var err currencies.ErrCurrencyUpdateFailed

	if r.UpdateFunc != nil {
		err = r.UpdateFunc(ctx, currency)
	} else {
		err = NewErrCurrencyUpdateFailed(currency, ErrNotConfigured)
	}

	return err
}

// Delete

func (r *CurrencyRepository) Delete(
	ctx context.Context,
	code currencies.Code,
) currencies.ErrCurrencyDeleteFailed {
	var err currencies.ErrCurrencyDeleteFailed

	if r.DeleteFunc != nil {
		err = r.DeleteFunc(ctx, code)
	} else {
		err = NewErrCurrencyDeleteFailed(code, ErrNotConfigured)
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
		err      = ErrNotConfigured
	)

	if r.FindByCodeFunc != nil {
		currency, err = r.FindByCodeFunc(ctx, code)
	}

	return currency, err
}

// Find by a code for update

func (r *CurrencyRepository) FindByCodeForUpdate(
	ctx context.Context,
	code currencies.Code,
) (*currencies.Currency, error) {
	var (
		currency *currencies.Currency
		err      = ErrNotConfigured
	)

	if r.FindByCodeForUpdateFunc != nil {
		currency, err = r.FindByCodeForUpdateFunc(ctx, code)
	}

	return currency, err
}

// Find all currencies

func (r *CurrencyRepository) FindAll(ctx context.Context) ([]*currencies.Currency, error) {
	var (
		result []*currencies.Currency
		err    = ErrNotConfigured
	)

	if r.FindAllFunc != nil {
		result, err = r.FindAllFunc(ctx)
	}

	return result, err
}
