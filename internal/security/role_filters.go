package security

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/radynsade/faryengo/internal/languages"
)

var ErrInvalidRoleQuery = errors.New("invalid role query")

// RoleFilters combines filters with AND. Permissions requires every selected
// permission; super roles satisfy any permission selection. NameLike searches
// every translation. IDLike and NameLike are literal, case-insensitive substrings.
type RoleFilters struct {
	IDLike      string
	NameLike    string
	Permissions []Permission
	IsSuper     *bool
}

func (f RoleFilters) Validate() error {
	var err error

	if !utf8.ValidString(f.IDLike) || !utf8.ValidString(f.NameLike) || strings.ContainsRune(f.IDLike+f.NameLike, '\x00') || len(f.IDLike) > 100 || len(f.NameLike) > 500 {
		err = ErrInvalidRoleQuery
	} else if permissionErr := validatePermissions(f.Permissions); permissionErr != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidRoleQuery, permissionErr)
	}

	return err
}

func (f RoleFilters) Count() int {
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
	RoleSortID          RoleSort = "uuid"
	RoleSortName        RoleSort = "name"
	RoleSortSuper       RoleSort = "super"
	DefaultRolePageSize          = 25
)

func NewRoleSort(value string) (RoleSort, error) {
	sort := RoleSort(value)
	err := sort.Validate()

	if err != nil {
		sort = ""
	}

	return sort, err
}

func (s RoleSort) Validate() error {
	var err error

	if s != RoleSortID && s != RoleSortName && s != RoleSortSuper {
		err = ErrInvalidRoleQuery
	}

	return err
}

type RoleQuery struct {
	Filters    RoleFilters
	Sort       RoleSort
	Descending bool
	Page       int
	PageSize   int
	Language   languages.LanguageCode
}

func (q RoleQuery) Validate() error {
	var err error

	if filterErr := q.Filters.Validate(); filterErr != nil {
		err = filterErr
	} else if q.Page < 1 || q.Page > 1_000_000 || q.PageSize < 1 || q.PageSize > 100 {
		err = ErrInvalidRoleQuery
	} else if sortErr := q.Sort.Validate(); sortErr != nil {
		err = sortErr
	} else if languageErr := q.Language.Validate(); languageErr != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidRoleQuery, languageErr)
	}

	return err
}

type RolePage struct {
	Roles    []*Role
	Total    int
	Page     int
	PageSize int
}

func (p RolePage) Pages() int {
	pages := 1

	if p.Total > 0 && p.PageSize > 0 {
		pages = (p.Total + p.PageSize - 1) / p.PageSize
	}

	return pages
}
