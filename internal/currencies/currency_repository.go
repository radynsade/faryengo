package currencies

import (
	"context"
	"errors"
)

//
// Repository
//

var (
	ErrCurrencyNotFound      = errors.New("currency not found")
	ErrCurrencyAlreadyExists = errors.New("currency already exists")
)

type ErrCurrencyCreateFailed interface {
	error
	Currency() *Currency
	Unwrap() error
}

type ErrCurrencyUpdateFailed interface {
	error
	Currency() *Currency
	Unwrap() error
}

type ErrCurrencyDeleteFailed interface {
	error
	CurrencyCode() Code
	Unwrap() error
}

// A Currency is identified by its code, so updating one changes only its
// symbol.

type CurrencyRepository interface {
	Create(ctx context.Context, currency *Currency) ErrCurrencyCreateFailed
	Update(ctx context.Context, currency *Currency) ErrCurrencyUpdateFailed
	Delete(ctx context.Context, code Code) ErrCurrencyDeleteFailed
	FindByCode(ctx context.Context, code Code) (*Currency, error)
	FindByCodeForUpdate(ctx context.Context, code Code) (*Currency, error)
	FindAll(ctx context.Context) ([]*Currency, error)
}
