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

//
// Add member by email input
//

var ErrAddMemberByEmailInputInvalid = errors.New("invalid add member by email input")

type AddMemberByEmailInput struct {
	WorkspaceID workspaces.WorkspaceID
	Email       string
}
