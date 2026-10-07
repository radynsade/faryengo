package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/pkg/domquery"
)

//
// Permission
//

var (
	ErrInvalidPermission = errors.New("invalid permission")
	ErrPermissionDenied  = errors.New("permission denied")
)

type Permission string

const (
	PermissionManageUser Permission = "manage_user"
	PermissionViewUser   Permission = "view_user"
	PermissionManageRole Permission = "manage_role"
	PermissionViewRole   Permission = "view_role"
)

func AllPermissions() []Permission {
	return []Permission{
		PermissionManageUser,
		PermissionViewUser,
		PermissionManageRole,
		PermissionViewRole,
	}
}

func (p Permission) Validate() error {
	switch p {
	case PermissionManageUser, PermissionViewUser, PermissionManageRole, PermissionViewRole:
		return nil
	default:
		return ErrInvalidPermission
	}
}

//
// Role ID
//

var ErrInvalidRoleID = errors.New("invalid role ID")

type RoleID uuid.UUID

func (id RoleID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrInvalidRoleID
	}

	return err
}

//
// Role
//

var ErrInvalidRole = errors.New("invalid role")

type Role struct {
	ID          RoleID
	Name        languages.Text
	Permissions []Permission
	IsSuper     bool
}

func NewRole(id RoleID, name languages.Text, permissions []Permission, isSuper bool) *Role {
	return &Role{
		ID:          id,
		Name:        name,
		Permissions: permissions,
		IsSuper:     isSuper,
	}
}

func (r *Role) Validate() error {
	var err error

	if r == nil {
		err = ErrInvalidRole
	} else {
		err = r.ID.Validate()
	}

	if err == nil {
		err = r.Name.Validate()
	}

	if err == nil {
		for _, permission := range r.Permissions {
			err = permission.Validate()

			if err != nil {
				break
			}
		}
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidRole, err)
	}

	return err
}

//
// Role repository
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

type RoleFilter struct {
	IDLike      string
	NameLike    string
	Permissions []Permission
	IsSuper     *bool
}

var ErrInvalidRoleQuery = errors.New("invalid role query")

func (f RoleFilter) Validate() error {
	var err error

	if !utf8.ValidString(f.IDLike) || !utf8.ValidString(f.NameLike) || strings.ContainsRune(f.IDLike+f.NameLike, '\x00') || len(f.IDLike) > 100 || len(f.NameLike) > 500 {
		err = ErrInvalidRoleQuery
	} else if validationErr := validatePermissions(f.Permissions); validationErr != nil {
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

func validatePermissions(permissions []Permission) error {
	var err error

	for _, permission := range permissions {
		err = permission.Validate()

		if err != nil {
			break
		}
	}

	return err
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
