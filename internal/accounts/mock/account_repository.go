package mock

import (
	"context"
	"errors"

	"github.com/radynsade/faryengo/internal/accounts"
)

//
// Errors
//

var ErrNotConfigured = errors.New("mock operation is not configured")

//
// Account write failures
//

type errAccountWriteFailed struct {
	account *accounts.Account
	err     error
}

func (e errAccountWriteFailed) Account() *accounts.Account {
	return e.account
}

func (e errAccountWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of accounts.ErrAccountCreateFailed

type ErrAccountCreateFailed struct {
	errAccountWriteFailed
}

func NewErrAccountCreateFailed(account *accounts.Account, err error) *ErrAccountCreateFailed {
	return &ErrAccountCreateFailed{
		errAccountWriteFailed: errAccountWriteFailed{account, err},
	}
}

func (e *ErrAccountCreateFailed) Error() string {
	return "failed to create an account"
}

// Implementation of accounts.ErrAccountUpdateFailed

type ErrAccountUpdateFailed struct {
	errAccountWriteFailed
}

func NewErrAccountUpdateFailed(account *accounts.Account, err error) *ErrAccountUpdateFailed {
	return &ErrAccountUpdateFailed{
		errAccountWriteFailed: errAccountWriteFailed{account, err},
	}
}

func (e *ErrAccountUpdateFailed) Error() string {
	return "failed to update an account"
}

// Implementation of accounts.ErrAccountDeleteFailed

type ErrAccountDeleteFailed struct {
	id  accounts.ID
	err error
}

func NewErrAccountDeleteFailed(id accounts.ID, err error) *ErrAccountDeleteFailed {
	return &ErrAccountDeleteFailed{id, err}
}

func (e *ErrAccountDeleteFailed) AccountID() accounts.ID {
	return e.id
}

func (e *ErrAccountDeleteFailed) Unwrap() error {
	return e.err
}

func (e *ErrAccountDeleteFailed) Error() string {
	return "failed to delete an account"
}

//
// Account repository
//

type AccountRepository struct {
	CreateFunc            func(ctx context.Context, account *accounts.Account) accounts.ErrAccountCreateFailed
	UpdateFunc            func(ctx context.Context, account *accounts.Account) accounts.ErrAccountUpdateFailed
	DeleteFunc            func(ctx context.Context, id accounts.ID) accounts.ErrAccountDeleteFailed
	FindByIDFunc          func(ctx context.Context, id accounts.ID) (*accounts.Account, error)
	FindByIDForUpdateFunc func(ctx context.Context, id accounts.ID) (*accounts.Account, error)
	FindFunc              func(ctx context.Context, query accounts.AccountQuery) ([]*accounts.Account, error)
	CountFunc             func(ctx context.Context, filter accounts.AccountFilter) (int, error)
}

var _ accounts.AccountRepository = (*AccountRepository)(nil)

// Create

func (r *AccountRepository) Create(
	ctx context.Context,
	account *accounts.Account,
) accounts.ErrAccountCreateFailed {
	var err accounts.ErrAccountCreateFailed

	if r.CreateFunc != nil {
		err = r.CreateFunc(ctx, account)
	} else {
		err = NewErrAccountCreateFailed(account, ErrNotConfigured)
	}

	return err
}

// Update

func (r *AccountRepository) Update(
	ctx context.Context,
	account *accounts.Account,
) accounts.ErrAccountUpdateFailed {
	var err accounts.ErrAccountUpdateFailed

	if r.UpdateFunc != nil {
		err = r.UpdateFunc(ctx, account)
	} else {
		err = NewErrAccountUpdateFailed(account, ErrNotConfigured)
	}

	return err
}

// Delete

func (r *AccountRepository) Delete(
	ctx context.Context,
	id accounts.ID,
) accounts.ErrAccountDeleteFailed {
	var err accounts.ErrAccountDeleteFailed

	if r.DeleteFunc != nil {
		err = r.DeleteFunc(ctx, id)
	} else {
		err = NewErrAccountDeleteFailed(id, ErrNotConfigured)
	}

	return err
}

// Find by an ID

func (r *AccountRepository) FindByID(
	ctx context.Context,
	id accounts.ID,
) (*accounts.Account, error) {
	var (
		account *accounts.Account
		err     = ErrNotConfigured
	)

	if r.FindByIDFunc != nil {
		account, err = r.FindByIDFunc(ctx, id)
	}

	return account, err
}

// Find by an ID for update

func (r *AccountRepository) FindByIDForUpdate(
	ctx context.Context,
	id accounts.ID,
) (*accounts.Account, error) {
	var (
		account *accounts.Account
		err     = ErrNotConfigured
	)

	if r.FindByIDForUpdateFunc != nil {
		account, err = r.FindByIDForUpdateFunc(ctx, id)
	}

	return account, err
}

// Find accounts matching a query

func (r *AccountRepository) Find(
	ctx context.Context,
	query accounts.AccountQuery,
) ([]*accounts.Account, error) {
	var (
		result []*accounts.Account
		err    = ErrNotConfigured
	)

	if r.FindFunc != nil {
		result, err = r.FindFunc(ctx, query)
	}

	return result, err
}

// Count accounts matching a filter

func (r *AccountRepository) Count(
	ctx context.Context,
	filter accounts.AccountFilter,
) (int, error) {
	var (
		total int
		err   = ErrNotConfigured
	)

	if r.CountFunc != nil {
		total, err = r.CountFunc(ctx, filter)
	}

	return total, err
}
