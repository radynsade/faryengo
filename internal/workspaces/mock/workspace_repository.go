package mock

import (
	"context"
	"errors"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Errors
//

var ErrNotConfigured = errors.New("mock operation is not configured")

//
// Workspace write failures
//

type errWorkspaceWriteFailed struct {
	workspace *workspaces.Workspace
	err       error
}

func (e errWorkspaceWriteFailed) Workspace() *workspaces.Workspace {
	return e.workspace
}

func (e errWorkspaceWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of workspaces.ErrWorkspaceCreateFailed

type ErrWorkspaceCreateFailed struct {
	errWorkspaceWriteFailed
}

func NewErrWorkspaceCreateFailed(workspace *workspaces.Workspace, err error) *ErrWorkspaceCreateFailed {
	return &ErrWorkspaceCreateFailed{
		errWorkspaceWriteFailed: errWorkspaceWriteFailed{workspace, err},
	}
}

func (e *ErrWorkspaceCreateFailed) Error() string {
	return "failed to create a workspace"
}

// Implementation of workspaces.ErrWorkspaceUpdateFailed

type ErrWorkspaceUpdateFailed struct {
	errWorkspaceWriteFailed
}

func NewErrWorkspaceUpdateFailed(workspace *workspaces.Workspace, err error) *ErrWorkspaceUpdateFailed {
	return &ErrWorkspaceUpdateFailed{
		errWorkspaceWriteFailed: errWorkspaceWriteFailed{workspace, err},
	}
}

func (e *ErrWorkspaceUpdateFailed) Error() string {
	return "failed to update a workspace"
}

// Implementation of workspaces.ErrWorkspaceDeleteFailed

type ErrWorkspaceDeleteFailed struct {
	id  workspaces.WorkspaceID
	err error
}

func NewErrWorkspaceDeleteFailed(id workspaces.WorkspaceID, err error) *ErrWorkspaceDeleteFailed {
	return &ErrWorkspaceDeleteFailed{id, err}
}

func (e *ErrWorkspaceDeleteFailed) WorkspaceID() workspaces.WorkspaceID {
	return e.id
}

func (e *ErrWorkspaceDeleteFailed) Unwrap() error {
	return e.err
}

func (e *ErrWorkspaceDeleteFailed) Error() string {
	return "failed to delete a workspace"
}

//
// Membership write failures
//

type errMembershipWriteFailed struct {
	membership workspaces.Membership
	err        error
}

func (e errMembershipWriteFailed) Membership() workspaces.Membership {
	return e.membership
}

func (e errMembershipWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of workspaces.ErrMembershipAddFailed

type ErrMembershipAddFailed struct {
	errMembershipWriteFailed
}

func NewErrMembershipAddFailed(membership workspaces.Membership, err error) *ErrMembershipAddFailed {
	return &ErrMembershipAddFailed{
		errMembershipWriteFailed: errMembershipWriteFailed{membership, err},
	}
}

func (e *ErrMembershipAddFailed) Error() string {
	return "failed to add a workspace member"
}

// Implementation of workspaces.ErrMembershipRemoveFailed

type ErrMembershipRemoveFailed struct {
	errMembershipWriteFailed
}

func NewErrMembershipRemoveFailed(membership workspaces.Membership, err error) *ErrMembershipRemoveFailed {
	return &ErrMembershipRemoveFailed{
		errMembershipWriteFailed: errMembershipWriteFailed{membership, err},
	}
}

func (e *ErrMembershipRemoveFailed) Error() string {
	return "failed to remove a workspace member"
}

//
// Workspace repository
//

type WorkspaceRepository struct {
	CreateFunc            func(ctx context.Context, workspace *workspaces.Workspace) workspaces.ErrWorkspaceCreateFailed
	UpdateFunc            func(ctx context.Context, workspace *workspaces.Workspace) workspaces.ErrWorkspaceUpdateFailed
	DeleteFunc            func(ctx context.Context, id workspaces.WorkspaceID) workspaces.ErrWorkspaceDeleteFailed
	FindByIDFunc          func(ctx context.Context, id workspaces.WorkspaceID) (*workspaces.Workspace, error)
	FindByIDForUpdateFunc func(ctx context.Context, id workspaces.WorkspaceID) (*workspaces.Workspace, error)
	FindFunc              func(ctx context.Context, query workspaces.WorkspaceQuery) ([]*workspaces.Workspace, error)
	CountFunc             func(ctx context.Context, filter workspaces.WorkspaceFilter) (int, error)
	AddMemberFunc         func(ctx context.Context, membership workspaces.Membership) workspaces.ErrMembershipAddFailed
	RemoveMemberFunc      func(ctx context.Context, membership workspaces.Membership) workspaces.ErrMembershipRemoveFailed
	HasMemberFunc         func(ctx context.Context, membership workspaces.Membership) (bool, error)
	FindMemberIDsFunc     func(ctx context.Context, id workspaces.WorkspaceID) ([]users.UserID, error)
}

var _ workspaces.WorkspaceRepository = (*WorkspaceRepository)(nil)

// Create

func (r *WorkspaceRepository) Create(
	ctx context.Context,
	workspace *workspaces.Workspace,
) workspaces.ErrWorkspaceCreateFailed {
	var err workspaces.ErrWorkspaceCreateFailed

	if r.CreateFunc != nil {
		err = r.CreateFunc(ctx, workspace)
	} else {
		err = NewErrWorkspaceCreateFailed(workspace, ErrNotConfigured)
	}

	return err
}

// Update

func (r *WorkspaceRepository) Update(
	ctx context.Context,
	workspace *workspaces.Workspace,
) workspaces.ErrWorkspaceUpdateFailed {
	var err workspaces.ErrWorkspaceUpdateFailed

	if r.UpdateFunc != nil {
		err = r.UpdateFunc(ctx, workspace)
	} else {
		err = NewErrWorkspaceUpdateFailed(workspace, ErrNotConfigured)
	}

	return err
}

// Delete

func (r *WorkspaceRepository) Delete(
	ctx context.Context,
	id workspaces.WorkspaceID,
) workspaces.ErrWorkspaceDeleteFailed {
	var err workspaces.ErrWorkspaceDeleteFailed

	if r.DeleteFunc != nil {
		err = r.DeleteFunc(ctx, id)
	} else {
		err = NewErrWorkspaceDeleteFailed(id, ErrNotConfigured)
	}

	return err
}

// Find by an ID

func (r *WorkspaceRepository) FindByID(
	ctx context.Context,
	id workspaces.WorkspaceID,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       = ErrNotConfigured
	)

	if r.FindByIDFunc != nil {
		workspace, err = r.FindByIDFunc(ctx, id)
	}

	return workspace, err
}

// Find by an ID for update

func (r *WorkspaceRepository) FindByIDForUpdate(
	ctx context.Context,
	id workspaces.WorkspaceID,
) (*workspaces.Workspace, error) {
	var (
		workspace *workspaces.Workspace
		err       = ErrNotConfigured
	)

	if r.FindByIDForUpdateFunc != nil {
		workspace, err = r.FindByIDForUpdateFunc(ctx, id)
	}

	return workspace, err
}

// Find workspaces matching a query

func (r *WorkspaceRepository) Find(
	ctx context.Context,
	query workspaces.WorkspaceQuery,
) ([]*workspaces.Workspace, error) {
	var (
		result []*workspaces.Workspace
		err    = ErrNotConfigured
	)

	if r.FindFunc != nil {
		result, err = r.FindFunc(ctx, query)
	}

	return result, err
}

// Count workspaces matching a filter

func (r *WorkspaceRepository) Count(
	ctx context.Context,
	filter workspaces.WorkspaceFilter,
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

// Add a member

func (r *WorkspaceRepository) AddMember(
	ctx context.Context,
	membership workspaces.Membership,
) workspaces.ErrMembershipAddFailed {
	var err workspaces.ErrMembershipAddFailed

	if r.AddMemberFunc != nil {
		err = r.AddMemberFunc(ctx, membership)
	} else {
		err = NewErrMembershipAddFailed(membership, ErrNotConfigured)
	}

	return err
}

// Remove a member

func (r *WorkspaceRepository) RemoveMember(
	ctx context.Context,
	membership workspaces.Membership,
) workspaces.ErrMembershipRemoveFailed {
	var err workspaces.ErrMembershipRemoveFailed

	if r.RemoveMemberFunc != nil {
		err = r.RemoveMemberFunc(ctx, membership)
	} else {
		err = NewErrMembershipRemoveFailed(membership, ErrNotConfigured)
	}

	return err
}

// Report whether a user is a member of a workspace

func (r *WorkspaceRepository) HasMember(
	ctx context.Context,
	membership workspaces.Membership,
) (bool, error) {
	var (
		isMember bool
		err      = ErrNotConfigured
	)

	if r.HasMemberFunc != nil {
		isMember, err = r.HasMemberFunc(ctx, membership)
	}

	return isMember, err
}

// Find the IDs of a workspace's members

func (r *WorkspaceRepository) FindMemberIDs(
	ctx context.Context,
	id workspaces.WorkspaceID,
) ([]users.UserID, error) {
	var (
		result []users.UserID
		err    = ErrNotConfigured
	)

	if r.FindMemberIDsFunc != nil {
		result, err = r.FindMemberIDsFunc(ctx, id)
	}

	return result, err
}
