package security

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

//
// User ID
//

var ErrInvalidUserID = errors.New("invalid user ID")

type UserID uuid.UUID

func (id UserID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrInvalidUserID
	}

	return err
}

//
// Email
//

var ErrInvalidEmail = errors.New("invalid email")

type Email string

func (e Email) Validate() error {
	address, err := mail.ParseAddress(string(e))

	if err != nil || address.Address != string(e) {
		return ErrInvalidEmail
	}

	return nil
}

//
// Phone
//

var (
	ErrInvalidPhone = errors.New("invalid phone")
	phonePattern    = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)
)

type Phone string

func (p Phone) Validate() error {
	if !phonePattern.MatchString(string(p)) {
		return ErrInvalidPhone
	}

	return nil
}

//
// First name
//

const MaximumFirstNameLength = 100

var (
	ErrInvalidFirstName           = errors.New("invalid first name")
	ErrEmptyFirstName             = errors.New("is empty")
	ErrTooLongFirstName           = fmt.Errorf("exceeds the limit of %d characters", MaximumFirstNameLength)
	ErrInvalidFirstNameCharacters = errors.New("invalid characters")
)

type FirstName string

func (n FirstName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrEmptyFirstName
	} else if !utf8.ValidString(string(n)) {
		err = ErrInvalidFirstNameCharacters
	} else if utf8.RuneCountInString(string(n)) > MaximumFirstNameLength {
		err = ErrTooLongFirstName
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidFirstName, err)
	}

	return err
}

//
// Last name
//

const MaximumLastNameLength = 100

var (
	ErrInvalidLastName           = errors.New("invalid last name")
	ErrEmptyLastName             = errors.New("is empty")
	ErrTooLongLastName           = fmt.Errorf("exceeds the limit of %d characters", MaximumLastNameLength)
	ErrInvalidLastNameCharacters = errors.New("invalid characters")
)

type LastName string

func (n LastName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrEmptyLastName
	} else if !utf8.ValidString(string(n)) {
		err = ErrInvalidLastNameCharacters
	} else if utf8.RuneCountInString(string(n)) > MaximumLastNameLength {
		err = ErrTooLongLastName
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidLastName, err)
	}

	return err
}

//
// User
//

var (
	ErrInvalidUser = errors.New("invalid user")
	ErrNilUser     = errors.New("is nil")
)

type User struct {
	ID                UserID
	RoleID            RoleID
	Email             Email
	EmailChangedAt    time.Time
	Phone             Phone
	PhoneChangedAt    time.Time
	PasswordHash      PasswordHash
	PasswordChangedAt time.Time
	FirstName         FirstName
	LastName          LastName
	UpdatedAt         time.Time
	CreatedAt         time.Time
}

func NewUser(
	id UserID,
	roleID RoleID,
	email Email,
	emailChangedAt time.Time,
	phone Phone,
	phoneChangedAt time.Time,
	passwordHash PasswordHash,
	passwordChangedAt time.Time,
	firstName FirstName,
	lastName LastName,
	updatedAt time.Time,
	createdAt time.Time,
) *User {
	return &User{
		ID:                id,
		RoleID:            roleID,
		Email:             email,
		EmailChangedAt:    emailChangedAt,
		Phone:             phone,
		PhoneChangedAt:    phoneChangedAt,
		PasswordHash:      passwordHash,
		PasswordChangedAt: passwordChangedAt,
		FirstName:         firstName,
		LastName:          lastName,
		UpdatedAt:         updatedAt,
		CreatedAt:         createdAt,
	}
}

func (u *User) Validate() error {
	var err error

	if u == nil {
		err = ErrNilUser
	} else {
		err = u.ID.Validate()
	}

	if err == nil {
		err = u.RoleID.Validate()
	}

	if err == nil {
		err = u.Email.Validate()
	}

	if err == nil {
		err = u.Phone.Validate()
	}

	if err == nil {
		err = u.PasswordHash.Validate()
	}

	if err == nil {
		err = u.FirstName.Validate()
	}

	if err == nil {
		err = u.LastName.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidUser, err)
	}

	return err
}

//
// User repository
//

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrUserConflict      = errors.New("user changed since it was loaded")
)

type ErrUserCreateFailed interface {
	error
	User() *User
	Unwrap() error
}

type ErrUserUpdateFailed interface {
	error
	User() *User
	Unwrap() error
}

type ErrUserDeleteFailed interface {
	error
	UserID() UserID
	Unwrap() error
}

type UserRepository interface {
	Create(ctx context.Context, user *User) ErrUserCreateFailed
	Update(ctx context.Context, user *User) ErrUserUpdateFailed
	Delete(ctx context.Context, id UserID) ErrUserDeleteFailed
	FindByID(ctx context.Context, id UserID) (*User, error)
	FindByEmail(ctx context.Context, email Email) (*User, error)
}
