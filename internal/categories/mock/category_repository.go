package mock

import (
	"context"
	"errors"

	"github.com/radynsade/faryengo/internal/categories"
)

//
// Errors
//

var ErrNotConfigured = errors.New("mock operation is not configured")

//
// Category write failures
//

type errCategoryWriteFailed struct {
	category *categories.Category
	err      error
}

func (e errCategoryWriteFailed) Category() *categories.Category {
	return e.category
}

func (e errCategoryWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of categories.ErrCategoryCreateFailed

type ErrCategoryCreateFailed struct {
	errCategoryWriteFailed
}

func NewErrCategoryCreateFailed(category *categories.Category, err error) *ErrCategoryCreateFailed {
	return &ErrCategoryCreateFailed{
		errCategoryWriteFailed: errCategoryWriteFailed{category, err},
	}
}

func (e *ErrCategoryCreateFailed) Error() string {
	return "failed to create a category"
}

// Implementation of categories.ErrCategoryUpdateFailed

type ErrCategoryUpdateFailed struct {
	errCategoryWriteFailed
}

func NewErrCategoryUpdateFailed(category *categories.Category, err error) *ErrCategoryUpdateFailed {
	return &ErrCategoryUpdateFailed{
		errCategoryWriteFailed: errCategoryWriteFailed{category, err},
	}
}

func (e *ErrCategoryUpdateFailed) Error() string {
	return "failed to update a category"
}

// Implementation of categories.ErrCategoryDeleteFailed

type ErrCategoryDeleteFailed struct {
	id  categories.ID
	err error
}

func NewErrCategoryDeleteFailed(id categories.ID, err error) *ErrCategoryDeleteFailed {
	return &ErrCategoryDeleteFailed{id, err}
}

func (e *ErrCategoryDeleteFailed) CategoryID() categories.ID {
	return e.id
}

func (e *ErrCategoryDeleteFailed) Unwrap() error {
	return e.err
}

func (e *ErrCategoryDeleteFailed) Error() string {
	return "failed to delete a category"
}

//
// Category repository
//

type CategoryRepository struct {
	CreateFunc            func(ctx context.Context, category *categories.Category) categories.ErrCategoryCreateFailed
	UpdateFunc            func(ctx context.Context, category *categories.Category) categories.ErrCategoryUpdateFailed
	DeleteFunc            func(ctx context.Context, id categories.ID) categories.ErrCategoryDeleteFailed
	FindByIDFunc          func(ctx context.Context, id categories.ID) (*categories.Category, error)
	FindByIDForUpdateFunc func(ctx context.Context, id categories.ID) (*categories.Category, error)
	FindFunc              func(ctx context.Context, query categories.CategoryQuery) ([]*categories.Category, error)
	CountFunc             func(ctx context.Context, filter categories.CategoryFilter) (int, error)
}

var _ categories.CategoryRepository = (*CategoryRepository)(nil)

// Create

func (r *CategoryRepository) Create(
	ctx context.Context,
	category *categories.Category,
) categories.ErrCategoryCreateFailed {
	var err categories.ErrCategoryCreateFailed

	if r.CreateFunc != nil {
		err = r.CreateFunc(ctx, category)
	} else {
		err = NewErrCategoryCreateFailed(category, ErrNotConfigured)
	}

	return err
}

// Update

func (r *CategoryRepository) Update(
	ctx context.Context,
	category *categories.Category,
) categories.ErrCategoryUpdateFailed {
	var err categories.ErrCategoryUpdateFailed

	if r.UpdateFunc != nil {
		err = r.UpdateFunc(ctx, category)
	} else {
		err = NewErrCategoryUpdateFailed(category, ErrNotConfigured)
	}

	return err
}

// Delete

func (r *CategoryRepository) Delete(
	ctx context.Context,
	id categories.ID,
) categories.ErrCategoryDeleteFailed {
	var err categories.ErrCategoryDeleteFailed

	if r.DeleteFunc != nil {
		err = r.DeleteFunc(ctx, id)
	} else {
		err = NewErrCategoryDeleteFailed(id, ErrNotConfigured)
	}

	return err
}

// Find by an ID

func (r *CategoryRepository) FindByID(
	ctx context.Context,
	id categories.ID,
) (*categories.Category, error) {
	var (
		category *categories.Category
		err      = ErrNotConfigured
	)

	if r.FindByIDFunc != nil {
		category, err = r.FindByIDFunc(ctx, id)
	}

	return category, err
}

// Find by an ID for update

func (r *CategoryRepository) FindByIDForUpdate(
	ctx context.Context,
	id categories.ID,
) (*categories.Category, error) {
	var (
		category *categories.Category
		err      = ErrNotConfigured
	)

	if r.FindByIDForUpdateFunc != nil {
		category, err = r.FindByIDForUpdateFunc(ctx, id)
	}

	return category, err
}

// Find categories matching a query

func (r *CategoryRepository) Find(
	ctx context.Context,
	query categories.CategoryQuery,
) ([]*categories.Category, error) {
	var (
		result []*categories.Category
		err    = ErrNotConfigured
	)

	if r.FindFunc != nil {
		result, err = r.FindFunc(ctx, query)
	}

	return result, err
}

// Count categories matching a filter

func (r *CategoryRepository) Count(
	ctx context.Context,
	filter categories.CategoryFilter,
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
