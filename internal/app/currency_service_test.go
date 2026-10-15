package app

import (
	"context"
	"errors"
	"testing"

	"github.com/radynsade/faryengo/internal/app/input"
	appmock "github.com/radynsade/faryengo/internal/app/mock"
	"github.com/radynsade/faryengo/internal/currencies"
	currenciesmock "github.com/radynsade/faryengo/internal/currencies/mock"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
)

func TestNewCurrencyService(t *testing.T) {
	for _, tt := range []struct {
		name       string
		transactor Transactor
		repository currencies.CurrencyRepository
		wantErr    error
	}{
		{name: "created", transactor: &appmock.Transactor{}, repository: &currenciesmock.CurrencyRepository{}},
		{name: "nil transactor", repository: &currenciesmock.CurrencyRepository{}, wantErr: ErrTransactorNil},
		{name: "nil repository", transactor: &appmock.Transactor{}, wantErr: ErrCurrencyRepositoryNil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewCurrencyService(tt.transactor, tt.repository)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) || (service == nil) != (tt.wantErr != nil) {
				t.Fatalf("NewCurrencyService() = (%v, %v), want error %v", service, err, tt.wantErr)
			}
		})
	}
}

func TestCurrencyServiceNilReceiver(t *testing.T) {
	ctx := systemContext(t)

	var service *CurrencyService

	currency, createErr := service.Create(ctx, input.CreateCurrencyInput{})
	found, findErr := service.FindByCode(ctx, "EUR")
	list, listErr := service.List(ctx)
	updated, updateErr := service.Update(ctx, input.UpdateCurrencyInput{})
	deleteErr := service.Delete(ctx, "EUR")

	for _, err := range []error{createErr, findErr, listErr, updateErr, deleteErr} {
		if !errors.Is(err, ErrCurrencyRepositoryNil) {
			t.Fatalf("nil receiver error = %v, want ErrCurrencyRepositoryNil", err)
		}
	}

	if currency != nil || found != nil || list != nil || updated != nil {
		t.Fatalf("nil receiver returned values: %v, %v, %v, %v", currency, found, list, updated)
	}
}

func TestCurrencyServiceCreate(t *testing.T) {
	valid := input.CreateCurrencyInput{Code: "EUR", Symbol: "€"}

	for _, tt := range []struct {
		name        string
		request     input.CreateCurrencyInput
		beginErr    error
		createErr   error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{name: "created", request: valid, wantTxCalls: 1, wantWrites: 1},
		{
			name:     "invalid code",
			request:  input.CreateCurrencyInput{Code: "eur", Symbol: "€"},
			wantErrs: []error{input.ErrCreateCurrencyInputInvalid, currencies.ErrCodeInvalid},
		},
		{
			name:     "long symbol",
			request:  input.CreateCurrencyInput{Code: "VES", Symbol: "Bs.F.S"},
			wantErrs: []error{input.ErrCreateCurrencyInputInvalid, currencies.ErrSymbolTooLong},
		},
		{
			name:    "all fields invalid",
			request: input.CreateCurrencyInput{},
			wantErrs: []error{
				input.ErrCreateCurrencyInputInvalid,
				currencies.ErrCodeInvalid,
				currencies.ErrSymbolEmpty,
			},
		},
		{
			name:        "transaction not started",
			request:     valid,
			beginErr:    context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
		},
		{
			name:        "duplicate",
			request:     valid,
			createErr:   currencies.ErrCurrencyAlreadyExists,
			wantErrs:    []error{currencies.ErrCurrencyAlreadyExists},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0

			var stored *currencies.Currency

			repository := &currenciesmock.CurrencyRepository{
				CreateFunc: func(
					ctx context.Context,
					currency *currencies.Currency,
				) currencies.ErrCurrencyCreateFailed {
					var err currencies.ErrCurrencyCreateFailed

					writes++
					stored = currency

					if !appmock.InTransaction(ctx) {
						t.Error("Create() ran outside a transaction")
					}

					if tt.createErr != nil {
						err = currenciesmock.NewErrCurrencyCreateFailed(currency, tt.createErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{BeginErr: tt.beginErr}
			service, err := NewCurrencyService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			currency, err := service.Create(userContext(t, testUserID, users.PermissionManageCurrency), tt.request)

			if transactor.Calls != tt.wantTxCalls || writes != tt.wantWrites {
				t.Fatalf("Create() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if currency != nil {
					t.Fatalf("Create() currency = %v, want nil on error", currency)
				}
			} else if currency == nil || currency != stored ||
				currency.Code != currencies.Code(tt.request.Code) ||
				currency.Symbol != currencies.Symbol(tt.request.Symbol) {
				t.Fatalf("Create() currency = %+v, stored = %+v, want %+v", currency, stored, tt.request)
			}
		})
	}
}

func TestCurrencyServiceFindByCode(t *testing.T) {
	euro := currencies.NewCurrency("EUR", "€")

	for _, tt := range []struct {
		name      string
		code      string
		findErr   error
		wantErrs  []error
		wantCalls int
	}{
		{name: "found", code: "EUR", wantCalls: 1},
		{name: "invalid code", code: "eur", wantErrs: []error{currencies.ErrCodeInvalid}},
		{
			name:      "not found",
			code:      "EUR",
			findErr:   currencies.ErrCurrencyNotFound,
			wantErrs:  []error{currencies.ErrCurrencyNotFound},
			wantCalls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := userContext(t, testUserID)
			calls := 0

			repository := &currenciesmock.CurrencyRepository{
				FindByCodeFunc: func(
					callCtx context.Context,
					code currencies.Code,
				) (*currencies.Currency, error) {
					var currency *currencies.Currency

					calls++

					if callCtx != ctx || code != currencies.Code(tt.code) {
						t.Errorf("FindByCode() called with (%v, %q)", callCtx, code)
					}

					if tt.findErr == nil {
						currency = euro
					}

					return currency, tt.findErr
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewCurrencyService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			currency, err := service.FindByCode(ctx, tt.code)

			if calls != tt.wantCalls || transactor.Calls != 0 {
				t.Fatalf("FindByCode() calls = (repository %d, transaction %d), want (%d, 0)", calls, transactor.Calls, tt.wantCalls)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && currency != nil {
				t.Fatalf("FindByCode() currency = %v, want nil on error", currency)
			} else if len(tt.wantErrs) == 0 && currency != euro {
				t.Fatalf("FindByCode() currency = %v, want %v", currency, euro)
			}
		})
	}
}

func TestCurrencyServiceList(t *testing.T) {
	all := []*currencies.Currency{
		currencies.NewCurrency("EUR", "€"),
		currencies.NewCurrency("USD", "$"),
	}

	for _, tt := range []struct {
		name     string
		findErr  error
		wantErrs []error
	}{
		{name: "listed"},
		{name: "repository error", findErr: context.DeadlineExceeded, wantErrs: []error{context.DeadlineExceeded}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := userContext(t, testUserID)

			repository := &currenciesmock.CurrencyRepository{
				FindAllFunc: func(callCtx context.Context) ([]*currencies.Currency, error) {
					if callCtx != ctx {
						t.Errorf("FindAll() context = %v, want %v", callCtx, ctx)
					}

					return all, tt.findErr
				},
			}

			service, err := NewCurrencyService(&appmock.Transactor{}, repository)

			if err != nil {
				t.Fatal(err)
			}

			result, err := service.List(ctx)

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 && result != nil {
				t.Fatalf("List() = %v, want nil on error", result)
			} else if len(tt.wantErrs) == 0 && len(result) != len(all) {
				t.Fatalf("List() = %v, want %v", result, all)
			}
		})
	}
}

func TestCurrencyServiceUpdate(t *testing.T) {
	valid := input.UpdateCurrencyInput{Code: "BRL", Symbol: "R$"}

	for _, tt := range []struct {
		name        string
		request     input.UpdateCurrencyInput
		lockErr     error
		updateErr   error
		wantErrs    []error
		wantTxCalls int
		wantLocks   int
		wantWrites  int
	}{
		{name: "updated", request: valid, wantTxCalls: 1, wantLocks: 1, wantWrites: 1},
		{
			name:     "invalid fields",
			request:  input.UpdateCurrencyInput{Code: "BR", Symbol: " "},
			wantErrs: []error{input.ErrUpdateCurrencyInputInvalid, currencies.ErrCodeInvalid, currencies.ErrSymbolEmpty},
		},
		{
			name:        "not found",
			request:     valid,
			lockErr:     currencies.ErrCurrencyNotFound,
			wantErrs:    []error{currencies.ErrCurrencyNotFound},
			wantTxCalls: 1,
			wantLocks:   1,
		},
		{
			name:        "write failed",
			request:     valid,
			updateErr:   context.DeadlineExceeded,
			wantErrs:    []error{context.DeadlineExceeded},
			wantTxCalls: 1,
			wantLocks:   1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			locks, writes := 0, 0

			var stored *currencies.Currency

			repository := &currenciesmock.CurrencyRepository{
				FindByCodeForUpdateFunc: func(
					ctx context.Context,
					code currencies.Code,
				) (*currencies.Currency, error) {
					var currency *currencies.Currency

					locks++

					if !appmock.InTransaction(ctx) || code != currencies.Code(tt.request.Code) {
						t.Errorf("FindByCodeForUpdate() called with (%v, %q), want a transaction", ctx, code)
					}

					if tt.lockErr == nil {
						currency = currencies.NewCurrency(code, "$")
					}

					return currency, tt.lockErr
				},
				UpdateFunc: func(
					ctx context.Context,
					currency *currencies.Currency,
				) currencies.ErrCurrencyUpdateFailed {
					var err currencies.ErrCurrencyUpdateFailed

					writes++
					stored = currency

					if !appmock.InTransaction(ctx) {
						t.Error("Update() ran outside a transaction")
					}

					if tt.updateErr != nil {
						err = currenciesmock.NewErrCurrencyUpdateFailed(currency, tt.updateErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewCurrencyService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			currency, err := service.Update(userContext(t, testUserID, users.PermissionManageCurrency), tt.request)

			if transactor.Calls != tt.wantTxCalls || locks != tt.wantLocks || writes != tt.wantWrites {
				t.Fatalf(
					"Update() calls = (transaction %d, lock %d, write %d), want (%d, %d, %d)",
					transactor.Calls, locks, writes, tt.wantTxCalls, tt.wantLocks, tt.wantWrites,
				)
			}

			assertErrors(t, err, tt.wantErrs)

			if len(tt.wantErrs) > 0 {
				if currency != nil {
					t.Fatalf("Update() currency = %v, want nil on error", currency)
				}
			} else if currency == nil || currency != stored ||
				currency.Code != currencies.Code(tt.request.Code) ||
				currency.Symbol != currencies.Symbol(tt.request.Symbol) {
				t.Fatalf("Update() currency = %+v, stored = %+v, want %+v", currency, stored, tt.request)
			}
		})
	}
}

func TestCurrencyServiceDelete(t *testing.T) {
	for _, tt := range []struct {
		name        string
		code        string
		deleteErr   error
		wantErrs    []error
		wantTxCalls int
		wantWrites  int
	}{
		{name: "deleted", code: "EUR", wantTxCalls: 1, wantWrites: 1},
		{name: "invalid code", code: "eur", wantErrs: []error{currencies.ErrCodeInvalid}},
		{name: "missing code", wantErrs: []error{currencies.ErrCodeInvalid}},
		{
			name:        "not found",
			code:        "EUR",
			deleteErr:   currencies.ErrCurrencyNotFound,
			wantErrs:    []error{currencies.ErrCurrencyNotFound},
			wantTxCalls: 1,
			wantWrites:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0

			repository := &currenciesmock.CurrencyRepository{
				DeleteFunc: func(
					ctx context.Context,
					code currencies.Code,
				) currencies.ErrCurrencyDeleteFailed {
					var err currencies.ErrCurrencyDeleteFailed

					writes++

					if !appmock.InTransaction(ctx) || code != currencies.Code(tt.code) {
						t.Errorf("Delete() called with (%v, %q), want a transaction", ctx, code)
					}

					if tt.deleteErr != nil {
						err = currenciesmock.NewErrCurrencyDeleteFailed(code, tt.deleteErr)
					}

					return err
				},
			}

			transactor := &appmock.Transactor{}
			service, err := NewCurrencyService(transactor, repository)

			if err != nil {
				t.Fatal(err)
			}

			err = service.Delete(userContext(t, testUserID, users.PermissionManageCurrency), tt.code)

			if transactor.Calls != tt.wantTxCalls || writes != tt.wantWrites {
				t.Fatalf("Delete() calls = (transaction %d, write %d), want (%d, %d)", transactor.Calls, writes, tt.wantTxCalls, tt.wantWrites)
			}

			assertErrors(t, err, tt.wantErrs)
		})
	}
}

// Managing currencies needs its own permission; any identity reads them. Every
// refusal comes before any repository call.

func TestCurrencyServiceAccess(t *testing.T) {
	operations := []struct {
		name  string
		write bool
		call  func(ctx context.Context, service *CurrencyService) error
	}{
		{"create", true, func(ctx context.Context, service *CurrencyService) error {
			_, err := service.Create(ctx, input.CreateCurrencyInput{Code: "EUR", Symbol: "€"})
			return err
		}},
		{"find", false, func(ctx context.Context, service *CurrencyService) error {
			_, err := service.FindByCode(ctx, "EUR")
			return err
		}},
		{"list", false, func(ctx context.Context, service *CurrencyService) error {
			_, err := service.List(ctx)
			return err
		}},
		{"update", true, func(ctx context.Context, service *CurrencyService) error {
			_, err := service.Update(ctx, input.UpdateCurrencyInput{Code: "EUR", Symbol: "€"})
			return err
		}},
		{"delete", true, func(ctx context.Context, service *CurrencyService) error {
			return service.Delete(ctx, "EUR")
		}},
	}

	for _, tt := range []struct {
		name     string
		ctx      context.Context
		wantErrs func(write bool) []error
	}{
		{"missing identity", t.Context(), func(bool) []error { return []error{ErrUnauthenticated, security.ErrIdentityMissing} }},
		{"without the permission", userContext(t, testUserID, users.PermissionManageLanguage), func(write bool) []error {
			var wantErrs []error

			if write {
				wantErrs = []error{users.ErrPermissionDenied}
			}

			return wantErrs
		}},
	} {
		for _, operation := range operations {
			t.Run(tt.name+" "+operation.name, func(t *testing.T) {
				transactor := &appmock.Transactor{}
				calls := 0
				repository := &currenciesmock.CurrencyRepository{
					FindByCodeFunc: func(context.Context, currencies.Code) (*currencies.Currency, error) {
						calls++
						return currencies.NewCurrency("EUR", "€"), nil
					},
					FindAllFunc: func(context.Context) ([]*currencies.Currency, error) {
						calls++
						return nil, nil
					},
				}

				service, err := NewCurrencyService(transactor, repository)

				if err != nil {
					t.Fatal(err)
				}

				err = operation.call(tt.ctx, service)
				wantErrs := tt.wantErrs(operation.write)

				if len(wantErrs) == 0 {
					if calls != 1 || err != nil {
						t.Fatalf("%s = %v after %d repository calls, want it allowed", operation.name, err, calls)
					}
				} else if calls != 0 || transactor.Calls != 0 {
					t.Fatalf("%s called the repository %d and the transactor %d times, want none", operation.name, calls, transactor.Calls)
				}

				assertErrors(t, err, wantErrs)
			})
		}
	}
}
