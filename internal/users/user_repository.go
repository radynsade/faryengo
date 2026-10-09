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
	MaxUserFilterIDLikeLength    = 36
	MaxUserFilterEmailLikeLength = 254
	MaxUserFilterNameLikeLength  = MaxFirstNameLength + MaxLastNameLength + 1
)

var (
	ErrUserFilterInvalid               = errors.New("invalid user filter")
	ErrUserFilterIDLikeInvalidChars    = errors.New("invalid characters")
	ErrUserFilterIDLikeTooLong         = fmt.Errorf("exceeds the limit of %d characters", MaxUserFilterIDLikeLength)
	ErrUserFilterEmailLikeInvalidChars = errors.New("invalid characters")
	ErrUserFilterEmailLikeTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxUserFilterEmailLikeLength)
	ErrUserFilterNameLikeInvalidChars  = errors.New("invalid characters")
	ErrUserFilterNameLikeTooLong       = fmt.Errorf("exceeds the limit of %d characters", MaxUserFilterNameLikeLength)
)

// NameLike matches the first name, the last name, or both joined by a space.

type UserFilter struct {
	IDLike    string
	EmailLike string
	NameLike  string
	RoleID    *RoleID
}

func (f UserFilter) Validate() error {
	var err error

	if !utf8.ValidString(f.IDLike) || strings.ContainsRune(f.IDLike, '\x00') {
		err = ErrUserFilterIDLikeInvalidChars
	} else if !utf8.ValidString(f.EmailLike) || strings.ContainsRune(f.EmailLike, '\x00') {
		err = ErrUserFilterEmailLikeInvalidChars
	} else if !utf8.ValidString(f.NameLike) || strings.ContainsRune(f.NameLike, '\x00') {
		err = ErrUserFilterNameLikeInvalidChars
	} else if utf8.RuneCountInString(f.IDLike) > MaxUserFilterIDLikeLength {
		err = ErrUserFilterIDLikeTooLong
	} else if utf8.RuneCountInString(f.EmailLike) > MaxUserFilterEmailLikeLength {
		err = ErrUserFilterEmailLikeTooLong
	} else if utf8.RuneCountInString(f.NameLike) > MaxUserFilterNameLikeLength {
		err = ErrUserFilterNameLikeTooLong
	} else if f.RoleID != nil {
		err = f.RoleID.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrUserFilterInvalid, err)
	}

	return err
}

func (f UserFilter) Count() int {
	count := 0

	for _, applied := range []bool{
		f.IDLike != "",
		f.EmailLike != "",
		f.NameLike != "",
		f.RoleID != nil,
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

type UserSort string

const (
	UserSortID        = UserSort("id")
	UserSortEmail     = UserSort("email")
	UserSortFirstName = UserSort("first_name")
	UserSortLastName  = UserSort("last_name")
	UserSortCreatedAt = UserSort("created_at")
)

func (s UserSort) Validate() error {
	var err error

	switch s {
	case UserSortID, UserSortEmail, UserSortFirstName, UserSortLastName, UserSortCreatedAt:
	default:
		err = ErrInvalidUserQuery
	}

	return err
}

//
// Query
//

var ErrInvalidUserQuery = errors.New("invalid user query")

type UserQuery domquery.Query[UserFilter, UserSort]

// Page is 1-based, and both Limit and Page must be set.

func (q UserQuery) Validate() error {
	err := q.Filter.Validate()

	if err == nil {
		err = q.SortBy.Validate()
	}

	if err == nil && (q.Limit == 0 || q.Page == 0) {
		err = ErrInvalidUserQuery
	}

	return err
}

//
// Repository
//

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrUserConflict      = errors.New("user changed since it was loaded")
)

type ErrUserCreateFailed interface {
	error
	User() *User
	Unwrap() error
}

type ErrUserUpdateFailed interface {
	error
	User() *User
	Unwrap() error
}

type ErrUserDeleteFailed interface {
	error
	UserID() UserID
	Unwrap() error
}

type UserRepository interface {
	Create(ctx context.Context, user *User) ErrUserCreateFailed
	Update(ctx context.Context, user *User) ErrUserUpdateFailed
	Delete(ctx context.Context, id UserID) ErrUserDeleteFailed
	FindByID(ctx context.Context, id UserID) (*User, error)
	FindByEmail(ctx context.Context, email Email) (*User, error)
	Find(ctx context.Context, query UserQuery) ([]*User, error)
	Count(ctx context.Context, filter UserFilter) (int, error)
}
