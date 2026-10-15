package mock

import (
	"context"
	"errors"

	"github.com/radynsade/faryengo/internal/budgets"
)

//
// Errors
//

var ErrNotConfigured = errors.New("mock operation is not configured")

//
// Budget write failures
//

type errBudgetWriteFailed struct {
	budget *budgets.Budget
	err    error
}

func (e errBudgetWriteFailed) Budget() *budgets.Budget {
	return e.budget
}

func (e errBudgetWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of budgets.ErrBudgetCreateFailed

type ErrBudgetCreateFailed struct {
	errBudgetWriteFailed
}

func NewErrBudgetCreateFailed(budget *budgets.Budget, err error) *ErrBudgetCreateFailed {
	return &ErrBudgetCreateFailed{
		errBudgetWriteFailed: errBudgetWriteFailed{budget, err},
	}
}

func (e *ErrBudgetCreateFailed) Error() string {
	return "failed to create a budget"
}

// Implementation of budgets.ErrBudgetUpdateFailed

type ErrBudgetUpdateFailed struct {
	errBudgetWriteFailed
}

func NewErrBudgetUpdateFailed(budget *budgets.Budget, err error) *ErrBudgetUpdateFailed {
	return &ErrBudgetUpdateFailed{
		errBudgetWriteFailed: errBudgetWriteFailed{budget, err},
	}
}

func (e *ErrBudgetUpdateFailed) Error() string {
	return "failed to update a budget"
}

// Implementation of budgets.ErrBudgetDeleteFailed

type ErrBudgetDeleteFailed struct {
	id  budgets.ID
	err error
}

func NewErrBudgetDeleteFailed(id budgets.ID, err error) *ErrBudgetDeleteFailed {
	return &ErrBudgetDeleteFailed{id, err}
}

func (e *ErrBudgetDeleteFailed) BudgetID() budgets.ID {
	return e.id
}

func (e *ErrBudgetDeleteFailed) Unwrap() error {
	return e.err
}

func (e *ErrBudgetDeleteFailed) Error() string {
	return "failed to delete a budget"
}

//
// Budget repository
//

type BudgetRepository struct {
	CreateFunc            func(ctx context.Context, budget *budgets.Budget) budgets.ErrBudgetCreateFailed
	UpdateFunc            func(ctx context.Context, budget *budgets.Budget) budgets.ErrBudgetUpdateFailed
	DeleteFunc            func(ctx context.Context, id budgets.ID) budgets.ErrBudgetDeleteFailed
	FindByIDFunc          func(ctx context.Context, id budgets.ID) (*budgets.Budget, error)
	FindByIDForUpdateFunc func(ctx context.Context, id budgets.ID) (*budgets.Budget, error)
	FindFunc              func(ctx context.Context, query budgets.BudgetQuery) ([]*budgets.Budget, error)
	CountFunc             func(ctx context.Context, filter budgets.BudgetFilter) (int, error)
}

var _ budgets.BudgetRepository = (*BudgetRepository)(nil)

// Create

func (r *BudgetRepository) Create(
	ctx context.Context,
	budget *budgets.Budget,
) budgets.ErrBudgetCreateFailed {
	var err budgets.ErrBudgetCreateFailed

	if r.CreateFunc != nil {
		err = r.CreateFunc(ctx, budget)
	} else {
		err = NewErrBudgetCreateFailed(budget, ErrNotConfigured)
	}

	return err
}

// Update

func (r *BudgetRepository) Update(
	ctx context.Context,
	budget *budgets.Budget,
) budgets.ErrBudgetUpdateFailed {
	var err budgets.ErrBudgetUpdateFailed

	if r.UpdateFunc != nil {
		err = r.UpdateFunc(ctx, budget)
	} else {
		err = NewErrBudgetUpdateFailed(budget, ErrNotConfigured)
	}

	return err
}

// Delete

func (r *BudgetRepository) Delete(
	ctx context.Context,
	id budgets.ID,
) budgets.ErrBudgetDeleteFailed {
	var err budgets.ErrBudgetDeleteFailed

	if r.DeleteFunc != nil {
		err = r.DeleteFunc(ctx, id)
	} else {
		err = NewErrBudgetDeleteFailed(id, ErrNotConfigured)
	}

	return err
}

// Find by an ID

func (r *BudgetRepository) FindByID(
	ctx context.Context,
	id budgets.ID,
) (*budgets.Budget, error) {
	var (
		budget *budgets.Budget
		err    = ErrNotConfigured
	)

	if r.FindByIDFunc != nil {
		budget, err = r.FindByIDFunc(ctx, id)
	}

	return budget, err
}

// Find by an ID for update

func (r *BudgetRepository) FindByIDForUpdate(
	ctx context.Context,
	id budgets.ID,
) (*budgets.Budget, error) {
	var (
		budget *budgets.Budget
		err    = ErrNotConfigured
	)

	if r.FindByIDForUpdateFunc != nil {
		budget, err = r.FindByIDForUpdateFunc(ctx, id)
	}

	return budget, err
}

// Find budgets matching a query

func (r *BudgetRepository) Find(
	ctx context.Context,
	query budgets.BudgetQuery,
) ([]*budgets.Budget, error) {
	var (
		result []*budgets.Budget
		err    = ErrNotConfigured
	)

	if r.FindFunc != nil {
		result, err = r.FindFunc(ctx, query)
	}

	return result, err
}

// Count budgets matching a filter

func (r *BudgetRepository) Count(
	ctx context.Context,
	filter budgets.BudgetFilter,
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
