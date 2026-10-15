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
// ID
//

var ErrIDInvalid = errors.New("invalid ID")

type ID uuid.UUID

func NewID(value uuid.UUID) (ID, error) {
	var (
		err error
		id  ID
	)

	id = ID(value)
	err = id.Validate()
	return id, err
}

func (id ID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrIDInvalid
	}

	return err
}

//
// Name
//

const MaxNameLength = 100

var (
	ErrNameInvalid      = errors.New("invalid name")
	ErrNameEmpty        = errors.New("is empty")
	ErrNameInvalidChars = errors.New("invalid characters")
	ErrNameTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxNameLength)
)

type Name string

func NewName(value string) (Name, error) {
	var (
		err  error
		name Name
	)

	name = Name(value)
	err = name.Validate()
	return name, err
}

func (n Name) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrNameEmpty
	} else if !utf8.ValidString(string(n)) {
		err = ErrNameInvalidChars
	} else if utf8.RuneCountInString(string(n)) > MaxNameLength {
		err = ErrNameTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrNameInvalid, err)
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
	ID          ID
	WorkspaceID workspaces.ID
	Name        Name
	Icon        *Icon
	Color       Color
}

func NewCategory(
	id ID,
	workspaceID workspaces.ID,
	name Name,
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
