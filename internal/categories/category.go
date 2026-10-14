package categories

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Category ID
//

var ErrCategoryIDInvalid = errors.New("invalid category ID")

type CategoryID uuid.UUID

func NewCategoryID(value uuid.UUID) (CategoryID, error) {
	var (
		err error
		id  CategoryID
	)

	id = CategoryID(value)
	err = id.Validate()
	return id, err
}

func (id CategoryID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrCategoryIDInvalid
	}

	return err
}

//
// Category name
//

const MaxCategoryNameLength = 100

var (
	ErrCategoryNameInvalid      = errors.New("invalid category name")
	ErrCategoryNameEmpty        = errors.New("is empty")
	ErrCategoryNameInvalidChars = errors.New("invalid characters")
	ErrCategoryNameTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxCategoryNameLength)
)

type CategoryName string

func NewCategoryName(value string) (CategoryName, error) {
	var (
		err  error
		name CategoryName
	)

	name = CategoryName(value)
	err = name.Validate()
	return name, err
}

func (n CategoryName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrCategoryNameEmpty
	} else if !utf8.ValidString(string(n)) {
		err = ErrCategoryNameInvalidChars
	} else if utf8.RuneCountInString(string(n)) > MaxCategoryNameLength {
		err = ErrCategoryNameTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrCategoryNameInvalid, err)
	}

	return err
}

//
// Category
//

var (
	ErrCategoryInvalid = errors.New("invalid category")
	ErrCategoryNil     = errors.New("is nil")
)

// A Category without an icon has a nil Icon.

type Category struct {
	ID          CategoryID
	WorkspaceID workspaces.WorkspaceID
	Name        CategoryName
	Icon        *Icon
	Color       Color
}

func NewCategory(
	id CategoryID,
	workspaceID workspaces.WorkspaceID,
	name CategoryName,
	icon *Icon,
	color Color,
) *Category {
	return &Category{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        name,
		Icon:        icon,
		Color:       color,
	}
}

func (c *Category) Validate() error {
	var (
		err     error
		iconErr error
	)

	if c == nil {
		err = ErrCategoryNil
	} else {
		if c.Icon != nil {
			iconErr = c.Icon.Validate()
		}

		err = errors.Join(
			c.ID.Validate(),
			c.WorkspaceID.Validate(),
			c.Name.Validate(),
			iconErr,
			c.Color.Validate(),
		)
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrCategoryInvalid, err)
	}

	return err
}
