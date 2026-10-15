package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Currency service
//

// Every write runs in a transaction, so it is atomic on its own and joins a
// transaction already carried by the caller's context.
//
// Every method acts for the caller's security.Identity in the context.
// Managing Currencies needs PermissionManageCurrency. Any identity may read
// them, because currencies supply the monetary notation shown throughout the
// application. A caller without an identity is reported as ErrUnauthenticated
// and one without the permission as users.ErrPermissionDenied.

var ErrCurrencyRepositoryNil = errors.New("currency repository is nil")

type CurrencyService struct {
	transactor Transactor
	repository currencies.CurrencyRepository
}

func NewCurrencyService(
	transactor Transactor,
	repository currencies.CurrencyRepository,
) (*CurrencyService, error) {
	var (
		service *CurrencyService
		err     error
	)

	if transactor == nil {
		err = ErrTransactorNil
	} else if repository == nil {
		err = ErrCurrencyRepositoryNil
	} else {
		service = &CurrencyService{transactor: transactor, repository: repository}
	}

	return service, err
}

// Create

func (s *CurrencyService) Create(
	ctx context.Context,
	request input.CreateCurrencyInput,
) (*currencies.Currency, error) {
	var (
		currency *currencies.Currency
		err      error
	)

	if err = s.check(); err == nil {
		_, err = authorizeSystem(ctx, users.PermissionManageCurrency)

		if err != nil {
			err = fmt.Errorf("create currency: %w", err)
		}
	}

	if err == nil {
		currency, err = newCurrency(request.Code, request.Symbol)

		if err != nil {
			err = fmt.Errorf("create currency: %w: %w", input.ErrCreateCurrencyInputInvalid, err)
		} else {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var err error

				if createErr := s.repository.Create(ctx, currency); createErr != nil {
					err = createErr
				}

				return err
			})

			if err != nil {
				err = fmt.Errorf("create currency %s: %w", currency.Code, err)
			}
		}
	}

	if err != nil {
		currency = nil
	}

	return currency, err
}

// Find by a code

func (s *CurrencyService) FindByCode(ctx context.Context, code string) (*currencies.Currency, error) {
	var (
		currency     *currencies.Currency
		currencyCode currencies.Code
		err          error
	)

	if s == nil || s.repository == nil {
		err = ErrCurrencyRepositoryNil
	} else if currencyCode, err = currencies.NewCode(code); err != nil {
		err = fmt.Errorf("find currency: %w", err)
	} else {
		_, err = currentIdentity(ctx)

		if err == nil {
			currency, err = s.repository.FindByCode(ctx, currencyCode)
		}

		if err != nil {
			err = fmt.Errorf("find currency %s: %w", code, err)
		}
	}

	if err != nil {
		currency = nil
	}

	return currency, err
}

// List all currencies ordered by a code

func (s *CurrencyService) List(ctx context.Context) ([]*currencies.Currency, error) {
	var (
		result []*currencies.Currency
		err    error
	)

	if s == nil || s.repository == nil {
		err = ErrCurrencyRepositoryNil
	} else {
		_, err = currentIdentity(ctx)

		if err == nil {
			result, err = s.repository.FindAll(ctx)
		}

		if err != nil {
			err = fmt.Errorf("list currencies: %w", err)
			result = nil
		}
	}

	return result, err
}

// A Currency is identified by its code, so the update replaces only the symbol
// of the Currency with that code. The Currency is locked first, so a missing
// one is reported as ErrCurrencyNotFound and concurrent updates apply one
// after another.

func (s *CurrencyService) Update(
	ctx context.Context,
	request input.UpdateCurrencyInput,
) (*currencies.Currency, error) {
	var (
		currency *currencies.Currency
		err      error
	)

	if err = s.check(); err == nil {
		_, err = authorizeSystem(ctx, users.PermissionManageCurrency)

		if err != nil {
			err = fmt.Errorf("update currency %s: %w", request.Code, err)
		}
	}

	if err == nil {
		currency, err = newCurrency(request.Code, request.Symbol)

		if err != nil {
			err = fmt.Errorf("update currency: %w: %w", input.ErrUpdateCurrencyInputInvalid, err)
		} else {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				_, err := s.repository.FindByCodeForUpdate(ctx, currency.Code)

				if err == nil {
					if updateErr := s.repository.Update(ctx, currency); updateErr != nil {
						err = updateErr
					}
				}

				return err
			})

			if err != nil {
				err = fmt.Errorf("update currency %s: %w", currency.Code, err)
			}
		}
	}

	if err != nil {
		currency = nil
	}

	return currency, err
}

// Delete

func (s *CurrencyService) Delete(ctx context.Context, code string) error {
	var currencyCode currencies.Code

	err := s.check()

	if err == nil {
		currencyCode, err = currencies.NewCode(code)

		if err != nil {
			err = fmt.Errorf("delete currency: %w", err)
		}
	}

	if err == nil {
		_, err = authorizeSystem(ctx, users.PermissionManageCurrency)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var err error

				if deleteErr := s.repository.Delete(ctx, currencyCode); deleteErr != nil {
					err = deleteErr
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("delete currency %s: %w", code, err)
		}
	}

	return err
}

//
// Helpers
//

func (s *CurrencyService) check() error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrCurrencyRepositoryNil
	} else if s.transactor == nil {
		err = ErrTransactorNil
	}

	return err
}

// Every field is validated, rather than stopping at the first failure, so a
// caller can report all invalid fields at once.

func newCurrency(code string, symbol string) (*currencies.Currency, error) {
	var currency *currencies.Currency

	currencyCode, codeErr := currencies.NewCode(code)
	currencySymbol, symbolErr := currencies.NewSymbol(symbol)

	err := errors.Join(codeErr, symbolErr)

	if err == nil {
		currency = currencies.NewCurrency(currencyCode, currencySymbol)
	}

	return currency, err
}
