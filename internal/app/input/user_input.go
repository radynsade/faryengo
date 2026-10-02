package input

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/security"
)

var (
	ErrInvalidCreateUserInput = errors.New("invalid create user input")
	ErrInvalidUpdateUserInput = errors.New("invalid update user input")
	ErrInvalidUserID          = errors.New("invalid user ID")
)

type CreateUserInput struct {
	RoleID    security.RoleID
	Email     string
	Phone     string
	Password  string
	FirstName string
	LastName  string
}

func (request CreateUserInput) Validate() error {
	validationErrors := validateUserFields(request.RoleID, request.Email, request.Phone, request.FirstName, request.LastName)

	if request.Password == "" || len(request.Password) > security.MaxPasswordBytes {
		validationErrors = append(validationErrors, fmt.Errorf("password: %w", security.ErrInvalidPassword))
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidCreateUserInput, errors.Join(validationErrors...))
	}

	return nil
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

func (request UpdateUserInput) Validate() error {
	validationErrors := validateUserFields(request.RoleID, request.Email, request.Phone, request.FirstName, request.LastName)

	if uuid.UUID(request.ID) == uuid.Nil {
		validationErrors = append(validationErrors, fmt.Errorf("ID: %w", ErrInvalidUserID))
	}

	if request.Password != nil && (*request.Password == "" || len(*request.Password) > security.MaxPasswordBytes) {
		validationErrors = append(validationErrors, fmt.Errorf("password: %w", security.ErrInvalidPassword))
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidUpdateUserInput, errors.Join(validationErrors...))
	}

	return nil
}

func validateUserFields(roleID security.RoleID, email, phone, firstName, lastName string) []error {
	var validationErrors []error

	if err := roleID.Validate(); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("role ID: %w", err))
	}

	if err := security.Email(email).Validate(); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("email: %w", err))
	}

	if err := security.Phone(phone).Validate(); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("phone: %w", err))
	}

	if err := security.FirstName(firstName).Validate(); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("first name: %w", err))
	}

	if err := security.LastName(lastName).Validate(); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("last name: %w", err))
	}

	return validationErrors
}
