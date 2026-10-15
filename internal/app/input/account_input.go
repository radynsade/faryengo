package input

import (
	"errors"

	"github.com/radynsade/faryengo/internal/accounts"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// Create account input
//

// Name is trimmed before it is validated, and a nil UserID means the Account
// belongs to no User.

var ErrCreateAccountInputInvalid = errors.New("invalid create account input")

type CreateAccountInput struct {
	WorkspaceID workspaces.ID
	UserID      *users.UserID
	Name        string
}

//
// Update account input
//

// Name is trimmed before it is validated, and a nil UserID removes the
// Account's User.

var ErrUpdateAccountInputInvalid = errors.New("invalid update account input")

type UpdateAccountInput struct {
	ID     accounts.ID
	UserID *users.UserID
	Name   string
}
