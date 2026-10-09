package users

import (
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

var ErrUserIDInvalid = errors.New("invalid user ID")

type UserID uuid.UUID

func (id UserID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrUserIDInvalid
	}

	return err
}

//
// Email
//

var ErrEmailInvalid = errors.New("invalid email")

type Email string

func (e Email) Validate() error {
	address, err := mail.ParseAddress(string(e))

	if err != nil || address.Address != string(e) {
		return ErrEmailInvalid
	}

	return nil
}

//
// Phone
//

var (
	ErrPhoneInvalid = errors.New("invalid phone")
	phonePattern    = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)
)

type Phone string

func (p Phone) Validate() error {
	if !phonePattern.MatchString(string(p)) {
		return ErrPhoneInvalid
	}

	return nil
}

//
// First name
//

const MaxFirstNameLength = 100

var (
	ErrFirstNameInvalid      = errors.New("invalid first name")
	ErrFirstNameEmpty        = errors.New("is empty")
	ErrFirstNameTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxFirstNameLength)
	ErrFirstNameInvalidChars = errors.New("invalid characters")
)

type FirstName string

func (n FirstName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrFirstNameEmpty
	} else if !utf8.ValidString(string(n)) {
		err = ErrFirstNameInvalidChars
	} else if utf8.RuneCountInString(string(n)) > MaxFirstNameLength {
		err = ErrFirstNameTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrFirstNameInvalid, err)
	}

	return err
}

//
// Last name
//

const MaxLastNameLength = 100

var (
	ErrLastNameInvalid      = errors.New("invalid last name")
	ErrLastNameEmpty        = errors.New("is empty")
	ErrLastNameTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxLastNameLength)
	ErrLastNameInvalidChars = errors.New("invalid characters")
)

type LastName string

func (n LastName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrLastNameEmpty
	} else if !utf8.ValidString(string(n)) {
		err = ErrLastNameInvalidChars
	} else if utf8.RuneCountInString(string(n)) > MaxLastNameLength {
		err = ErrLastNameTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrLastNameInvalid, err)
	}

	return err
}

//
// User
//

var (
	ErrUserInvalid = errors.New("invalid user")
	ErrUserNil     = errors.New("is nil")
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
		err = ErrUserNil
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
		err = fmt.Errorf("%w: %w", ErrUserInvalid, err)
	}

	return err
}
