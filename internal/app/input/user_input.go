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

//
// Delete user
//

var ErrDeleteOwnUser = errors.New("a user cannot delete their own account")

//
// Update profile input
//

// A User changes their own names and email. Changing the email requires the
// current password, because it replaces the credentials; the names alone do
// not. UpdatedAt is the moment of the User's latest change that the caller
// saw.

var ErrUpdateProfileInputInvalid = errors.New("invalid update profile input")

type UpdateProfileInput struct {
	ID              users.UserID
	FirstName       string
	LastName        string
	Email           string
	CurrentPassword string
	UpdatedAt       time.Time
}

//
// Change password input
//

var ErrChangePasswordInputInvalid = errors.New("invalid change password input")

type ChangePasswordInput struct {
	ID              users.UserID
	CurrentPassword string
	NewPassword     string
}

//
// Current password
//

var (
	ErrCurrentPasswordRequired  = errors.New("the current password is required")
	ErrCurrentPasswordIncorrect = errors.New("the current password is incorrect")
)
