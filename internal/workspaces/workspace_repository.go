package workspaces

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/pkg/domquery"
)

//
// Filter
//

const (
	MaxWorkspaceFilterIDLikeLength   = 36
	MaxWorkspaceFilterNameLikeLength = MaxNameLength
)

var (
	ErrWorkspaceFilterInvalid              = errors.New("invalid workspace filter")
	ErrWorkspaceFilterIDLikeInvalidChars   = errors.New("invalid characters")
	ErrWorkspaceFilterIDLikeTooLong        = fmt.Errorf("exceeds the limit of %d characters", MaxWorkspaceFilterIDLikeLength)
	ErrWorkspaceFilterNameLikeInvalidChars = errors.New("invalid characters")
	ErrWorkspaceFilterNameLikeTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxWorkspaceFilterNameLikeLength)
)

// MemberID limits the result to the Workspaces that User belongs to.

type WorkspaceFilter struct {
	IDLike   string
	NameLike string
	MemberID *users.UserID
}

func NewWorkspaceFilter(
	idLike string,
	nameLike string,
	memberID *users.UserID,
) WorkspaceFilter {
	return WorkspaceFilter{
		IDLike:   idLike,
		NameLike: nameLike,
		MemberID: memberID,
	}
}

func (f WorkspaceFilter) Validate() error {
	var err error

	if !utf8.ValidString(f.IDLike) || strings.ContainsRune(f.IDLike, '\x00') {
		err = ErrWorkspaceFilterIDLikeInvalidChars
	} else if !utf8.ValidString(f.NameLike) || strings.ContainsRune(f.NameLike, '\x00') {
		err = ErrWorkspaceFilterNameLikeInvalidChars
	} else if utf8.RuneCountInString(f.IDLike) > MaxWorkspaceFilterIDLikeLength {
		err = ErrWorkspaceFilterIDLikeTooLong
	} else if utf8.RuneCountInString(f.NameLike) > MaxWorkspaceFilterNameLikeLength {
		err = ErrWorkspaceFilterNameLikeTooLong
	} else if f.MemberID != nil {
		err = f.MemberID.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrWorkspaceFilterInvalid, err)
	}

	return err
}

func (f WorkspaceFilter) Count() int {
	count := 0

	for _, applied := range []bool{
		f.IDLike != "",
		f.NameLike != "",
		f.MemberID != nil,
	} {
		if applied {
			count++
		}
	}

	return count
}

//
// Sort
//

type WorkspaceSort string

const (
	WorkspaceSortID   = WorkspaceSort("id")
	WorkspaceSortName = WorkspaceSort("name")
)

func NewWorkspaceSort(value string) (WorkspaceSort, error) {
	var (
		err  error
		sort WorkspaceSort
	)

	sort = WorkspaceSort(value)
	err = sort.Validate()
	return sort, err
}

func (s WorkspaceSort) Validate() error {
	var err error

	switch s {
	case WorkspaceSortID, WorkspaceSortName:
	default:
		err = ErrWorkspaceQueryInvalid
	}

	return err
}

//
// Query
//

var ErrWorkspaceQueryInvalid = errors.New("invalid workspace query")

type WorkspaceQuery domquery.Query[WorkspaceFilter, WorkspaceSort]

func NewWorkspaceQuery(
	filter WorkspaceFilter,
	sortOrder domquery.SortOrder,
	sortBy WorkspaceSort,
	limit uint,
	page uint,
) WorkspaceQuery {
	return WorkspaceQuery{
		Filter:    filter,
		SortOrder: sortOrder,
		SortBy:    sortBy,
		Limit:     limit,
		Page:      page,
	}
}

// Page is 1-based, and both Limit and Page must be set.

func (q WorkspaceQuery) Validate() error {
	err := q.Filter.Validate()

	if err == nil {
		err = q.SortBy.Validate()
	}

	if err == nil && (q.Limit == 0 || q.Page == 0) {
		err = ErrWorkspaceQueryInvalid
	}

	return err
}

//
// Repository
//

var (
	ErrWorkspaceNotFound       = errors.New("workspace not found")
	ErrWorkspaceAlreadyExists  = errors.New("workspace already exists")
	ErrMembershipNotFound      = errors.New("user is not a member of the workspace")
	ErrMembershipAlreadyExists = errors.New("user is already a member of the workspace")
)

type ErrWorkspaceCreateFailed interface {
	error
	Workspace() *Workspace
	Unwrap() error
}

type ErrWorkspaceUpdateFailed interface {
	error
	Workspace() *Workspace
	Unwrap() error
}

type ErrWorkspaceDeleteFailed interface {
	error
	WorkspaceID() ID
	Unwrap() error
}

type ErrMembershipAddFailed interface {
	error
	Membership() Membership
	Unwrap() error
}

type ErrMembershipUpdateFailed interface {
	error
	Membership() Membership
	Unwrap() error
}

type ErrMembershipRemoveFailed interface {
	error
	Membership() Membership
	Unwrap() error
}

// Memberships belong to the Workspace aggregate, so this repository writes
// them too. Deleting a Workspace removes its memberships. Updating a
// membership changes only its Role. A User who is not a member is reported
// as ErrMembershipNotFound.

type WorkspaceRepository interface {
	Create(ctx context.Context, workspace *Workspace) ErrWorkspaceCreateFailed
	Update(ctx context.Context, workspace *Workspace) ErrWorkspaceUpdateFailed
	Delete(ctx context.Context, id ID) ErrWorkspaceDeleteFailed
	FindByID(ctx context.Context, id ID) (*Workspace, error)
	FindByIDForUpdate(ctx context.Context, id ID) (*Workspace, error)
	Find(ctx context.Context, query WorkspaceQuery) ([]*Workspace, error)
	Count(ctx context.Context, filter WorkspaceFilter) (int, error)
	AddMember(ctx context.Context, membership Membership) ErrMembershipAddFailed
	UpdateMember(ctx context.Context, membership Membership) ErrMembershipUpdateFailed
	RemoveMember(ctx context.Context, membership Membership) ErrMembershipRemoveFailed
	FindMember(ctx context.Context, id ID, userID users.UserID) (Membership, error)
	FindMembers(ctx context.Context, id ID) (Memberships, error)
}
