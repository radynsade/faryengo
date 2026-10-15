package input

import (
	"errors"

	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Create category input
//

// Name is trimmed before it is validated, and an empty Icon means the
// Category has no icon.

var ErrCreateCategoryInputInvalid = errors.New("invalid create category input")

type CreateCategoryInput struct {
	WorkspaceID workspaces.ID
	Name        string
	Icon        string
	Color       string
}

//
// Update category input
//

// Name is trimmed before it is validated, and an empty Icon removes the
// Category's icon.

var ErrUpdateCategoryInputInvalid = errors.New("invalid update category input")

type UpdateCategoryInput struct {
	ID    categories.ID
	Name  string
	Icon  string
	Color string
}
