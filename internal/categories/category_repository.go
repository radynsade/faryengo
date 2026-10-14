package categories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/pkg/domquery"
)

//
// Filter
//

const (
	MaxCategoryFilterIDLikeLength   = 36
	MaxCategoryFilterNameLikeLength = MaxCategoryNameLength
)

var (
	ErrCategoryFilterInvalid              = errors.New("invalid category filter")
	ErrCategoryFilterIDLikeInvalidChars   = errors.New("invalid characters")
	ErrCategoryFilterIDLikeTooLong        = fmt.Errorf("exceeds the limit of %d characters", MaxCategoryFilterIDLikeLength)
	ErrCategoryFilterNameLikeInvalidChars = errors.New("invalid characters")
	ErrCategoryFilterNameLikeTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxCategoryFilterNameLikeLength)
)

// WorkspaceID limits the result to the Categories of that Workspace.

type CategoryFilter struct {
	IDLike      string
	NameLike    string
	WorkspaceID *workspaces.WorkspaceID
}

func NewCategoryFilter(
	idLike string,
	nameLike string,
	workspaceID *workspaces.WorkspaceID,
) CategoryFilter {
	return CategoryFilter{
		IDLike:      idLike,
		NameLike:    nameLike,
		WorkspaceID: workspaceID,
	}
}

func (f CategoryFilter) Validate() error {
	var err error

	if !utf8.ValidString(f.IDLike) || strings.ContainsRune(f.IDLike, '\x00') {
		err = ErrCategoryFilterIDLikeInvalidChars
	} else if !utf8.ValidString(f.NameLike) || strings.ContainsRune(f.NameLike, '\x00') {
		err = ErrCategoryFilterNameLikeInvalidChars
	} else if utf8.RuneCountInString(f.IDLike) > MaxCategoryFilterIDLikeLength {
		err = ErrCategoryFilterIDLikeTooLong
	} else if utf8.RuneCountInString(f.NameLike) > MaxCategoryFilterNameLikeLength {
		err = ErrCategoryFilterNameLikeTooLong
	} else if f.WorkspaceID != nil {
		err = f.WorkspaceID.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrCategoryFilterInvalid, err)
	}

	return err
}

func (f CategoryFilter) Count() int {
	count := 0

	for _, applied := range []bool{
		f.IDLike != "",
		f.NameLike != "",
		f.WorkspaceID != nil,
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

type CategorySort string

const (
	CategorySortID   = CategorySort("id")
	CategorySortName = CategorySort("name")
)

func NewCategorySort(value string) (CategorySort, error) {
	var (
		err  error
		sort CategorySort
	)

	sort = CategorySort(value)
	err = sort.Validate()
	return sort, err
}

func (s CategorySort) Validate() error {
	var err error

	switch s {
	case CategorySortID, CategorySortName:
	default:
		err = ErrCategoryQueryInvalid
	}

	return err
}

//
// Query
//

var ErrCategoryQueryInvalid = errors.New("invalid category query")

type CategoryQuery domquery.Query[CategoryFilter, CategorySort]

func NewCategoryQuery(
	filter CategoryFilter,
	sortOrder domquery.SortOrder,
	sortBy CategorySort,
	limit uint,
	page uint,
) CategoryQuery {
	return CategoryQuery{
		Filter:    filter,
		SortOrder: sortOrder,
		SortBy:    sortBy,
		Limit:     limit,
		Page:      page,
	}
}

// Page is 1-based, and both Limit and Page must be set.

func (q CategoryQuery) Validate() error {
	err := q.Filter.Validate()

	if err == nil {
		err = q.SortBy.Validate()
	}

	if err == nil && (q.Limit == 0 || q.Page == 0) {
		err = ErrCategoryQueryInvalid
	}

	return err
}

//
// Repository
//

var (
	ErrCategoryNotFound      = errors.New("category not found")
	ErrCategoryAlreadyExists = errors.New("category already exists")
)

type ErrCategoryCreateFailed interface {
	error
	Category() *Category
	Unwrap() error
}

type ErrCategoryUpdateFailed interface {
	error
	Category() *Category
	Unwrap() error
}

type ErrCategoryDeleteFailed interface {
	error
	CategoryID() CategoryID
	Unwrap() error
}

// A Category stays in the Workspace it was created in, so updating one
// changes its name, icon, and color but never its Workspace; a Category
// submitted with a different Workspace is reported as ErrCategoryNotFound.
// Creating a Category in a missing Workspace is reported as
// workspaces.ErrWorkspaceNotFound.

type CategoryRepository interface {
	Create(ctx context.Context, category *Category) ErrCategoryCreateFailed
	Update(ctx context.Context, category *Category) ErrCategoryUpdateFailed
	Delete(ctx context.Context, id CategoryID) ErrCategoryDeleteFailed
	FindByID(ctx context.Context, id CategoryID) (*Category, error)
	FindByIDForUpdate(ctx context.Context, id CategoryID) (*Category, error)
	Find(ctx context.Context, query CategoryQuery) ([]*Category, error)
	Count(ctx context.Context, filter CategoryFilter) (int, error)
}
