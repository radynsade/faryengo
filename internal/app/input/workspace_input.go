package input

import (
	"errors"

	"github.com/radynsade/faryengo/internal/users"
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
	ID   workspaces.ID
	Name string
}

//
// Add member by email input
//

// An empty Role adds a viewer.

var ErrAddMemberByEmailInputInvalid = errors.New("invalid add member by email input")

type AddMemberByEmailInput struct {
	WorkspaceID workspaces.ID
	Email       string
	Role        string
}

//
// Change member role input
//

var ErrChangeMemberRoleInputInvalid = errors.New("invalid change member role input")

type ChangeMemberRoleInput struct {
	WorkspaceID workspaces.ID
	UserID      users.UserID
	Role        string
}
