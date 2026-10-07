package security

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
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
// Password hash
//

var (
	ErrInvalidPasswordHash = errors.New("invalid password hash")
	ErrEmptyPasswordHash   = errors.New("is empty")
)

type PasswordHash string

func (p PasswordHash) Validate() error {
	var err error

	if strings.TrimSpace(string(p)) == "" {
		err = ErrEmptyPasswordHash
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidPasswordHash, err)
	}

	return err
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
	}

	if utf8.RuneCountInString(string(n)) > MaximumFirstNameLength {
		err = ErrTooLongFirstName
	}

	if !utf8.ValidString(string(n)) {
		err = ErrInvalidFirstNameCharacters
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidFirstName, err)
	}

	return nil
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
	}

	if utf8.RuneCountInString(string(n)) > MaximumLastNameLength {
		err = ErrTooLongLastName
	}

	if !utf8.ValidString(string(n)) {
		err = ErrInvalidLastNameCharacters
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidLastName, err)
	}

	return nil
}

//
// User
//

type User struct {
	id           UserID
	roleID       RoleID
	email        Email
	phone        Phone
	passwordHash PasswordHash
	firstName    FirstName
	lastName     LastName
}

func NewUser(
	id UserID,
	roleID RoleID,
	email Email,
	phone Phone,
	passwordHash PasswordHash,
	firstName FirstName,
	lastName LastName,
) (*User, error) {
	return &User{
		id:           id,
		roleID:       roleID,
		email:        email,
		phone:        phone,
		passwordHash: passwordHash,
		firstName:    firstName,
		lastName:     lastName,
	}, nil
}

func (u *User) ID() UserID {
	return u.id
}

func (u *User) SetID(id UserID) {
	u.id = id
}

func (u *User) RoleID() RoleID {
	return u.roleID
}

func (u *User) SetRoleID(roleID RoleID) {
	u.roleID = roleID
}

func (u *User) Email() Email {
	return u.email
}

func (u *User) SetEmail(email Email) {
	u.email = email
}

func (u *User) Phone() Phone {
	return u.phone
}

func (u *User) SetPhone(phone Phone) {
	u.phone = phone
}

func (u *User) PasswordHash() PasswordHash {
	return u.passwordHash
}

func (u *User) SetPasswordHash(passwordHash PasswordHash) {
	u.passwordHash = passwordHash
}

func (u *User) FirstName() FirstName {
	return u.firstName
}

func (u *User) SetFirstName(firstName FirstName) {
	u.firstName = firstName
}

func (u *User) LastName() LastName {
	return u.lastName
}

func (u *User) SetLastName(lastName LastName) {
	u.lastName = lastName
}

//
// User repository
//

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
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
}
