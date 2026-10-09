package input

import (
	"errors"
	"time"

	"github.com/radynsade/faryengo/internal/users"
)

//
// Create user input
//

var ErrCreateUserInputInvalid = errors.New("invalid create user input")

type CreateUserInput struct {
	RoleID    users.RoleID
	Email     string
	Phone     string
	Password  string
	FirstName string
	LastName  string
}

//
// Update user input
//

// A nil Password keeps the current password. UpdatedAt is the moment of the
// User's latest change that the caller saw; an update based on an older one is
// rejected as a conflict.

var ErrUpdateUserInputInvalid = errors.New("invalid update user input")

type UpdateUserInput struct {
	ID        users.UserID
	RoleID    users.RoleID
	Email     string
	Phone     string
	Password  *string
	FirstName string
	LastName  string
	UpdatedAt time.Time
}
