package mock

import (
	"context"

	"github.com/radynsade/faryengo/internal/users"
)

//
// User write failures
//

type errUserWriteFailed struct {
	user *users.User
	err  error
}

func (e errUserWriteFailed) User() *users.User {
	return e.user
}

func (e errUserWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of users.ErrUserCreateFailed

type ErrUserCreateFailed struct {
	errUserWriteFailed
}

func NewErrUserCreateFailed(user *users.User, err error) *ErrUserCreateFailed {
	return &ErrUserCreateFailed{
		errUserWriteFailed: errUserWriteFailed{user, err},
	}
}

func (e *ErrUserCreateFailed) Error() string {
	return "failed to create a user"
}

// Implementation of users.ErrUserUpdateFailed

type ErrUserUpdateFailed struct {
	errUserWriteFailed
}

func NewErrUserUpdateFailed(user *users.User, err error) *ErrUserUpdateFailed {
	return &ErrUserUpdateFailed{
		errUserWriteFailed: errUserWriteFailed{user, err},
	}
}

func (e *ErrUserUpdateFailed) Error() string {
	return "failed to update a user"
}

// Implementation of users.ErrUserDeleteFailed

type ErrUserDeleteFailed struct {
	id  users.UserID
	err error
}

func NewErrUserDeleteFailed(id users.UserID, err error) *ErrUserDeleteFailed {
	return &ErrUserDeleteFailed{id, err}
}

func (e *ErrUserDeleteFailed) UserID() users.UserID {
	return e.id
}

func (e *ErrUserDeleteFailed) Unwrap() error {
	return e.err
}

func (e *ErrUserDeleteFailed) Error() string {
	return "failed to delete a user"
}

//
// User repository
//

type UserRepository struct {
	CreateFunc      func(ctx context.Context, user *users.User) users.ErrUserCreateFailed
	UpdateFunc      func(ctx context.Context, user *users.User) users.ErrUserUpdateFailed
	DeleteFunc      func(ctx context.Context, id users.UserID) users.ErrUserDeleteFailed
	FindByIDFunc    func(ctx context.Context, id users.UserID) (*users.User, error)
	FindByEmailFunc func(ctx context.Context, email users.Email) (*users.User, error)
	FindFunc        func(ctx context.Context, query users.UserQuery) ([]*users.User, error)
	CountFunc       func(ctx context.Context, filter users.UserFilter) (int, error)
}

var _ users.UserRepository = (*UserRepository)(nil)

// Create

func (r *UserRepository) Create(
	ctx context.Context,
	user *users.User,
) users.ErrUserCreateFailed {
	var err users.ErrUserCreateFailed

	if r.CreateFunc != nil {
		err = r.CreateFunc(ctx, user)
	} else {
		err = NewErrUserCreateFailed(user, ErrNotConfigured)
	}

	return err
}

// Update

func (r *UserRepository) Update(
	ctx context.Context,
	user *users.User,
) users.ErrUserUpdateFailed {
	var err users.ErrUserUpdateFailed

	if r.UpdateFunc != nil {
		err = r.UpdateFunc(ctx, user)
	} else {
		err = NewErrUserUpdateFailed(user, ErrNotConfigured)
	}

	return err
}

// Delete

func (r *UserRepository) Delete(
	ctx context.Context,
	id users.UserID,
) users.ErrUserDeleteFailed {
	var err users.ErrUserDeleteFailed

	if r.DeleteFunc != nil {
		err = r.DeleteFunc(ctx, id)
	} else {
		err = NewErrUserDeleteFailed(id, ErrNotConfigured)
	}

	return err
}

// Find by an ID

func (r *UserRepository) FindByID(
	ctx context.Context,
	id users.UserID,
) (*users.User, error) {
	var (
		user *users.User
		err  = ErrNotConfigured
	)

	if r.FindByIDFunc != nil {
		user, err = r.FindByIDFunc(ctx, id)
	}

	return user, err
}

// Find by an email

func (r *UserRepository) FindByEmail(
	ctx context.Context,
	email users.Email,
) (*users.User, error) {
	var (
		user *users.User
		err  = ErrNotConfigured
	)

	if r.FindByEmailFunc != nil {
		user, err = r.FindByEmailFunc(ctx, email)
	}

	return user, err
}

// Find users matching a query

func (r *UserRepository) Find(
	ctx context.Context,
	query users.UserQuery,
) ([]*users.User, error) {
	var (
		result []*users.User
		err    = ErrNotConfigured
	)

	if r.FindFunc != nil {
		result, err = r.FindFunc(ctx, query)
	}

	return result, err
}

// Count users matching a filter

func (r *UserRepository) Count(
	ctx context.Context,
	filter users.UserFilter,
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
