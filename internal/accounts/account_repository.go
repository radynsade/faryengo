package accounts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/domquery"
)

//
// Filter
//

const (
	MaxAccountFilterIDLikeLength   = 36
	MaxAccountFilterNameLikeLength = MaxNameLength
)

var (
	ErrAccountFilterInvalid              = errors.New("invalid account filter")
	ErrAccountFilterIDLikeInvalidChars   = errors.New("invalid characters")
	ErrAccountFilterIDLikeTooLong        = fmt.Errorf("exceeds the limit of %d characters", MaxAccountFilterIDLikeLength)
	ErrAccountFilterNameLikeInvalidChars = errors.New("invalid characters")
	ErrAccountFilterNameLikeTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxAccountFilterNameLikeLength)
)

// WorkspaceID limits the result to the Accounts of that Workspace, and UserID
// to the Accounts of that user.

type AccountFilter struct {
	IDLike      string
	NameLike    string
	WorkspaceID *workspaces.ID
	UserID      *users.UserID
}

func NewAccountFilter(
	idLike string,
	nameLike string,
	workspaceID *workspaces.ID,
	userID *users.UserID,
) AccountFilter {
	return AccountFilter{
		IDLike:      idLike,
		NameLike:    nameLike,
		WorkspaceID: workspaceID,
		UserID:      userID,
	}
}

func (f AccountFilter) Validate() error {
	var err error

	if !utf8.ValidString(f.IDLike) || strings.ContainsRune(f.IDLike, '\x00') {
		err = ErrAccountFilterIDLikeInvalidChars
	} else if !utf8.ValidString(f.NameLike) || strings.ContainsRune(f.NameLike, '\x00') {
		err = ErrAccountFilterNameLikeInvalidChars
	} else if utf8.RuneCountInString(f.IDLike) > MaxAccountFilterIDLikeLength {
		err = ErrAccountFilterIDLikeTooLong
	} else if utf8.RuneCountInString(f.NameLike) > MaxAccountFilterNameLikeLength {
		err = ErrAccountFilterNameLikeTooLong
	} else if f.WorkspaceID != nil {
		err = f.WorkspaceID.Validate()
	}

	if err == nil && f.UserID != nil {
		err = f.UserID.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrAccountFilterInvalid, err)
	}

	return err
}

func (f AccountFilter) Count() int {
	count := 0

	for _, applied := range []bool{
		f.IDLike != "",
		f.NameLike != "",
		f.WorkspaceID != nil,
		f.UserID != nil,
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

type AccountSort string

const (
	AccountSortID   = AccountSort("id")
	AccountSortName = AccountSort("name")
)

func NewAccountSort(value string) (AccountSort, error) {
	var (
		err  error
		sort AccountSort
	)

	sort = AccountSort(value)
	err = sort.Validate()
	return sort, err
}

func (s AccountSort) Validate() error {
	var err error

	switch s {
	case AccountSortID, AccountSortName:
	default:
		err = ErrAccountQueryInvalid
	}

	return err
}

//
// Query
//

var ErrAccountQueryInvalid = errors.New("invalid account query")

type AccountQuery domquery.Query[AccountFilter, AccountSort]

func NewAccountQuery(
	filter AccountFilter,
	sortOrder domquery.SortOrder,
	sortBy AccountSort,
	limit uint,
	page uint,
) AccountQuery {
	return AccountQuery{
		Filter:    filter,
		SortOrder: sortOrder,
		SortBy:    sortBy,
		Limit:     limit,
		Page:      page,
	}
}

// Page is 1-based, and both Limit and Page must be set.

func (q AccountQuery) Validate() error {
	err := q.Filter.Validate()

	if err == nil {
		err = q.SortBy.Validate()
	}

	if err == nil && (q.Limit == 0 || q.Page == 0) {
		err = ErrAccountQueryInvalid
	}

	return err
}

//
// Repository
//

var (
	ErrAccountNotFound      = errors.New("account not found")
	ErrAccountAlreadyExists = errors.New("account already exists")
)

type ErrAccountCreateFailed interface {
	error
	Account() *Account
	Unwrap() error
}

type ErrAccountUpdateFailed interface {
	error
	Account() *Account
	Unwrap() error
}

type ErrAccountDeleteFailed interface {
	error
	AccountID() ID
	Unwrap() error
}

// An Account stays in the Workspace it was created in, so updating one
// changes its user and name but never its Workspace; an Account submitted with
// a different Workspace is reported as ErrAccountNotFound. Writing an Account
// with a missing Workspace is reported as workspaces.ErrWorkspaceNotFound, and
// with a missing user as users.ErrUserNotFound.

type AccountRepository interface {
	Create(ctx context.Context, account *Account) ErrAccountCreateFailed
	Update(ctx context.Context, account *Account) ErrAccountUpdateFailed
	Delete(ctx context.Context, id ID) ErrAccountDeleteFailed
	FindByID(ctx context.Context, id ID) (*Account, error)
	FindByIDForUpdate(ctx context.Context, id ID) (*Account, error)
	Find(ctx context.Context, query AccountQuery) ([]*Account, error)
	Count(ctx context.Context, filter AccountFilter) (int, error)
}
