package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/radynsade/faryengo/pkg/domquery"
)

//
// Filter
//

const (
	MaxRoleFilterIDLikeLength   = 36
	MaxRoleFilterNameLikeLength = MaxRoleNameLength
)

var (
	ErrRoleFilterInvalid              = errors.New("invalid role filter")
	ErrRoleFilterIDLikeInvalidChars   = errors.New("invalid characters")
	ErrRoleFilterIDLikeTooLong        = fmt.Errorf("exceeds the limit of %d characters", MaxRoleFilterIDLikeLength)
	ErrRoleFilterNameLikeInvalidChars = errors.New("invalid characters")
	ErrRoleFilterNameLikeTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxRoleFilterNameLikeLength)
)

type RoleFilter struct {
	IDLike      string
	NameLike    string
	Permissions Permissions
	IsSuper     *bool
}

func (f RoleFilter) Validate() error {
	var err error

	if !utf8.ValidString(f.IDLike) || strings.ContainsRune(f.IDLike, '\x00') {
		err = ErrRoleFilterIDLikeInvalidChars
	}

	if !utf8.ValidString(f.NameLike) || strings.ContainsRune(f.NameLike, '\x00') {
		err = ErrRoleFilterNameLikeInvalidChars
	}

	if len(f.IDLike) > MaxRoleFilterIDLikeLength {
		err = ErrRoleFilterIDLikeTooLong
	}

	if len(f.NameLike) > MaxRoleFilterNameLikeLength {
		err = ErrRoleFilterNameLikeTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrRoleFilterInvalid, err)
	}

	return err
}

func (f RoleFilter) Count() int {
	count := 0

	for _, applied := range []bool{
		f.IDLike != "",
		f.NameLike != "",
		len(f.Permissions) > 0,
		f.IsSuper != nil,
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

type RoleSort string

const (
	RoleSortID      = RoleSort("id")
	RoleSortName    = RoleSort("name")
	RoleSortIsSuper = RoleSort("is_super")
)

func (s RoleSort) Validate() error {
	var err error

	if s != RoleSortID && s != RoleSortName && s != RoleSortIsSuper {
		err = ErrInvalidRoleQuery
	}

	return err
}

//
// Query
//

var ErrInvalidRoleQuery = errors.New("invalid role query")

type RoleQuery domquery.Query[RoleFilter, RoleSort]

// Page is 1-based, and both Limit and Page must be set.

func (q RoleQuery) Validate() error {
	err := q.Filter.Validate()

	if err == nil {
		err = q.SortBy.Validate()
	}

	if err == nil && (q.Limit == 0 || q.Page == 0) {
		err = ErrInvalidRoleQuery
	}

	return err
}

//
// Repository
//

var (
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleAlreadyExists = errors.New("role already exists")
	ErrRoleAlreadyInUse  = errors.New("role is assigned to users")
)

type ErrRoleCreateFailed interface {
	error
	Role() *Role
	Unwrap() error
}

type ErrRoleUpdateFailed interface {
	error
	Role() *Role
	Unwrap() error
}

type ErrRoleDeleteFailed interface {
	error
	RoleID() RoleID
	Unwrap() error
}

type RoleRepository interface {
	Create(ctx context.Context, role *Role) ErrRoleCreateFailed
	Update(ctx context.Context, role *Role) ErrRoleUpdateFailed
	Delete(ctx context.Context, id RoleID) ErrRoleDeleteFailed
	FindByID(ctx context.Context, id RoleID) (*Role, error)
	FindByIDForUpdate(ctx context.Context, id RoleID) (*Role, error)
	Find(ctx context.Context, query RoleQuery) ([]*Role, error)
	Count(ctx context.Context, filter RoleFilter) (int, error)
}
