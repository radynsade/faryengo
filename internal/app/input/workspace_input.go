package input

import (
	"errors"

	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Create workspace input
//

// Name is trimmed before it is validated.

var ErrCreateWorkspaceInputInvalid = errors.New("invalid create workspace input")

type CreateWorkspaceInput struct {
	Name string
}

//
// Rename workspace input
//

// Name is trimmed before it is validated.

var ErrRenameWorkspaceInputInvalid = errors.New("invalid rename workspace input")

type RenameWorkspaceInput struct {
	ID   workspaces.WorkspaceID
	Name string
}
