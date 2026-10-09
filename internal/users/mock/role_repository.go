package mock

import (
	"context"

	"github.com/radynsade/faryengo/internal/users"
)

//
// Role write failures
//

type errRoleWriteFailed struct {
	role *users.Role
	err  error
}

func (e errRoleWriteFailed) Role() *users.Role {
	return e.role
}

func (e errRoleWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of users.ErrRoleCreateFailed

type ErrRoleCreateFailed struct {
	errRoleWriteFailed
}

func NewErrRoleCreateFailed(role *users.Role, err error) *ErrRoleCreateFailed {
	return &ErrRoleCreateFailed{
		errRoleWriteFailed: errRoleWriteFailed{role, err},
	}
}

func (e *ErrRoleCreateFailed) Error() string {
	return "failed to create a role"
}

// Implementation of users.ErrRoleUpdateFailed

type ErrRoleUpdateFailed struct {
	errRoleWriteFailed
}

func NewErrRoleUpdateFailed(role *users.Role, err error) *ErrRoleUpdateFailed {
	return &ErrRoleUpdateFailed{
		errRoleWriteFailed: errRoleWriteFailed{role, err},
	}
}

func (e *ErrRoleUpdateFailed) Error() string {
	return "failed to update a role"
}

// Implementation of users.ErrRoleDeleteFailed

type ErrRoleDeleteFailed struct {
	id  users.RoleID
	err error
}

func NewErrRoleDeleteFailed(id users.RoleID, err error) *ErrRoleDeleteFailed {
	return &ErrRoleDeleteFailed{id, err}
}

func (e *ErrRoleDeleteFailed) RoleID() users.RoleID {
	return e.id
}

func (e *ErrRoleDeleteFailed) Unwrap() error {
	return e.err
}

func (e *ErrRoleDeleteFailed) Error() string {
	return "failed to delete a role"
}

//
// Role repository
//

type RoleRepository struct {
	CreateFunc            func(ctx context.Context, role *users.Role) users.ErrRoleCreateFailed
	UpdateFunc            func(ctx context.Context, role *users.Role) users.ErrRoleUpdateFailed
	DeleteFunc            func(ctx context.Context, id users.RoleID) users.ErrRoleDeleteFailed
	FindByIDFunc          func(ctx context.Context, id users.RoleID) (*users.Role, error)
	FindByIDForUpdateFunc func(ctx context.Context, id users.RoleID) (*users.Role, error)
	FindFunc              func(ctx context.Context, query users.RoleQuery) ([]*users.Role, error)
	CountFunc             func(ctx context.Context, filter users.RoleFilter) (int, error)
}

var _ users.RoleRepository = (*RoleRepository)(nil)

// Create

func (r *RoleRepository) Create(
	ctx context.Context,
	role *users.Role,
) users.ErrRoleCreateFailed {
	var err users.ErrRoleCreateFailed

	if r.CreateFunc != nil {
		err = r.CreateFunc(ctx, role)
	} else {
		err = NewErrRoleCreateFailed(role, ErrNotConfigured)
	}

	return err
}

// Update

func (r *RoleRepository) Update(
	ctx context.Context,
	role *users.Role,
) users.ErrRoleUpdateFailed {
	var err users.ErrRoleUpdateFailed

	if r.UpdateFunc != nil {
		err = r.UpdateFunc(ctx, role)
	} else {
		err = NewErrRoleUpdateFailed(role, ErrNotConfigured)
	}

	return err
}

// Delete

func (r *RoleRepository) Delete(
	ctx context.Context,
	id users.RoleID,
) users.ErrRoleDeleteFailed {
	var err users.ErrRoleDeleteFailed

	if r.DeleteFunc != nil {
		err = r.DeleteFunc(ctx, id)
	} else {
		err = NewErrRoleDeleteFailed(id, ErrNotConfigured)
	}

	return err
}

// Find by an ID

func (r *RoleRepository) FindByID(
	ctx context.Context,
	id users.RoleID,
) (*users.Role, error) {
	var (
		role *users.Role
		err  = ErrNotConfigured
	)

	if r.FindByIDFunc != nil {
		role, err = r.FindByIDFunc(ctx, id)
	}

	return role, err
}

// Find by an ID for update

func (r *RoleRepository) FindByIDForUpdate(
	ctx context.Context,
	id users.RoleID,
) (*users.Role, error) {
	var (
		role *users.Role
		err  = ErrNotConfigured
	)

	if r.FindByIDForUpdateFunc != nil {
		role, err = r.FindByIDForUpdateFunc(ctx, id)
	}

	return role, err
}

// Find roles matching a query

func (r *RoleRepository) Find(
	ctx context.Context,
	query users.RoleQuery,
) ([]*users.Role, error) {
	var (
		result []*users.Role
		err    = ErrNotConfigured
	)

	if r.FindFunc != nil {
		result, err = r.FindFunc(ctx, query)
	}

	return result, err
}

// Count roles matching a filter

func (r *RoleRepository) Count(
	ctx context.Context,
	filter users.RoleFilter,
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
