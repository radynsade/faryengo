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

func NewUserID(value uuid.UUID) (UserID, error) {
	var (
		err error
		id  UserID
	)

	id = UserID(value)
	err = id.Validate()
	return id, err
}

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

func NewEmail(value string) (Email, error) {
	var (
		err   error
		email Email
	)

	email = Email(value)
	err = email.Validate()
	return email, err
}

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

func NewPhone(value string) (Phone, error) {
	var (
		err   error
		phone Phone
	)

	phone = Phone(value)
	err = phone.Validate()
	return phone, err
}

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

func NewFirstName(value string) (FirstName, error) {
	var (
		err  error
		name FirstName
	)

	name = FirstName(value)
	err = name.Validate()
	return name, err
}

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

func NewLastName(value string) (LastName, error) {
	var (
		err  error
		name LastName
	)

	name = LastName(value)
	err = name.Validate()
	return name, err
}

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
		err = errors.Join(
			u.ID.Validate(),
			u.RoleID.Validate(),
			u.Email.Validate(),
			u.Phone.Validate(),
			u.PasswordHash.Validate(),
			u.FirstName.Validate(),
			u.LastName.Validate(),
		)
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrUserInvalid, err)
	}

	return err
}
