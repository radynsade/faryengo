package input

import (
	"errors"

	"github.com/radynsade/faryengo/internal/security"
)

var (
	ErrInvalidCreateUserInput = errors.New("invalid create user input")
	ErrInvalidUpdateUserInput = errors.New("invalid update user input")
	// ErrInvalidUserID preserves compatibility; identity validity belongs to security.
	ErrInvalidUserID = security.ErrInvalidUserID
)

type CreateUserInput struct {
	RoleID    security.RoleID
	Email     string
	Phone     string
	Password  string
	FirstName string
	LastName  string
}

type UpdateUserInput struct {
	ID        security.UserID
	RoleID    security.RoleID
	Email     string
	Phone     string
	Password  *string // nil keeps the existing password hash
	FirstName string
	LastName  string
}
