package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/pkg/domquery"
)

//
// Filter
//

var (
	ErrRoleFilterInvalid              = errors.New("invalid role filter")
	ErrRoleFilterIDLikeInvalidChars   = errors.New("invalid characters")
	ErrRoleFilterNameLikeInvalidChars = errors.New("invalid characters")
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

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrRoleFilterInvalid, err)
	}

	if !utf8.ValidString(f.IDLike) ||
		!utf8.ValidString(f.NameLike) ||
		strings.ContainsRune(f.IDLike+f.NameLike, '\x00') ||
		len(f.IDLike) > 100 ||
		len(f.NameLike) > 500 {
		err = ErrInvalidRoleQuery
	} else if validationErr := f.Permissions.Validate(); validationErr != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidRoleQuery, validationErr)
	}

	return err
}

//
// Query
//

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

var ErrInvalidRoleQuery = errors.New("invalid role query")

func (f RoleFilter) Validate() error {
	var err error

	if !utf8.ValidString(f.IDLike) ||
		!utf8.ValidString(f.NameLike) ||
		strings.ContainsRune(f.IDLike+f.NameLike, '\x00') ||
		len(f.IDLike) > 100 ||
		len(f.NameLike) > 500 {
		err = ErrInvalidRoleQuery
	} else if validationErr := f.Permissions.Validate(); validationErr != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidRoleQuery, validationErr)
	}

	return err
}

func (f RoleFilter) Count() int {
	count := 0

	for _, applied := range []bool{f.IDLike != "", f.NameLike != "", len(f.Permissions) > 0, f.IsSuper != nil} {
		if applied {
			count++
		}
	}

	return count
}

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

const DefaultRolePageSize = 25

type RoleQuery struct {
	domquery.Query[RoleFilter, RoleSort]
	Language languages.Code
}

func NewRoleQuery(filter RoleFilter, sort RoleSort, order domquery.SortOrder, limit, page int, language languages.Code) RoleQuery {
	return RoleQuery{Query: domquery.NewQuery(filter, sort, order, limit, page), Language: language}
}

func (q RoleQuery) Validate() error {
	var err error
	err = q.Filters().Validate()

	if err == nil {
		if q.Page() < 1 || q.Page() > 1_000_000 || q.Limit() < 1 || q.Limit() > 100 {
			err = ErrInvalidRoleQuery
		} else {
			err = q.SortBy().Validate()
		}
	}

	if err == nil {
		if validationErr := q.Language.Validate(); validationErr != nil {
			err = fmt.Errorf("%w: %w", ErrInvalidRoleQuery, validationErr)
		}
	}

	return err
}

type RoleRepository interface {
	Create(ctx context.Context, role *Role) ErrRoleCreateFailed
	Update(ctx context.Context, role *Role) ErrRoleUpdateFailed
	Delete(ctx context.Context, id RoleID) ErrRoleDeleteFailed
	FindByID(ctx context.Context, id RoleID) (*Role, error)
	Find(ctx context.Context, query RoleQuery) ([]*Role, error)
	Count(ctx context.Context, filter RoleFilter) (int, error)
}
