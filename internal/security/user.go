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

var (
	ErrInvalidEmail        = errors.New("invalid email")
	ErrInvalidPhone        = errors.New("invalid phone")
	ErrInvalidPasswordHash = errors.New("invalid password hash")
	ErrInvalidFirstName    = errors.New("invalid first name")
	ErrInvalidLastName     = errors.New("invalid last name")
	ErrUserNotFound        = errors.New("user not found")
)

var phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)

type Email string

func NewEmail(value string) (Email, error) {
	email := Email(value)

	if err := email.Validate(); err != nil {
		return Email(""), err
	}

	return email, nil
}

func (e Email) Validate() error {
	address, err := mail.ParseAddress(string(e))

	if err != nil || address.Address != string(e) {
		return ErrInvalidEmail
	}

	return nil
}

type Phone string

func NewPhone(value string) (Phone, error) {
	phone := Phone(value)

	if err := phone.Validate(); err != nil {
		return Phone(""), err
	}

	return phone, nil
}

func (p Phone) Validate() error {
	if !phonePattern.MatchString(string(p)) {
		return ErrInvalidPhone
	}

	return nil
}

type PasswordHash string

func NewPasswordHash(value string) (PasswordHash, error) {
	hash := PasswordHash(value)

	if err := hash.Validate(); err != nil {
		return PasswordHash(""), err
	}

	return hash, nil
}

func (p PasswordHash) Validate() error {
	if strings.TrimSpace(string(p)) == "" {
		return ErrInvalidPasswordHash
	}

	return nil
}

type FirstName string

func NewFirstName(value string) (FirstName, error) {
	name := FirstName(value)

	if err := name.Validate(); err != nil {
		return FirstName(""), err
	}

	return name, nil
}

func (n FirstName) Validate() error {
	if strings.TrimSpace(string(n)) == "" || utf8.RuneCountInString(string(n)) > 100 {
		return ErrInvalidFirstName
	}

	return nil
}

type LastName string

func NewLastName(value string) (LastName, error) {
	name := LastName(value)

	if err := name.Validate(); err != nil {
		return LastName(""), err
	}

	return name, nil
}

func (n LastName) Validate() error {
	if strings.TrimSpace(string(n)) == "" || utf8.RuneCountInString(string(n)) > 100 {
		return ErrInvalidLastName
	}

	return nil
}

type UserID uuid.UUID

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
	if err := roleID.Validate(); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if err := email.Validate(); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if err := phone.Validate(); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if err := passwordHash.Validate(); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if err := firstName.Validate(); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if err := lastName.Validate(); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

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

func (u *User) SetRoleID(roleID RoleID) error {
	if err := roleID.Validate(); err != nil {
		return fmt.Errorf("set user role ID: %w", err)
	}

	u.roleID = roleID
	return nil
}

func (u *User) Email() Email {
	return u.email
}

func (u *User) SetEmail(email Email) error {
	if err := email.Validate(); err != nil {
		return fmt.Errorf("set user email: %w", err)
	}

	u.email = email
	return nil
}

func (u *User) Phone() Phone {
	return u.phone
}

func (u *User) SetPhone(phone Phone) error {
	if err := phone.Validate(); err != nil {
		return fmt.Errorf("set user phone: %w", err)
	}

	u.phone = phone
	return nil
}

func (u *User) PasswordHash() PasswordHash {
	return u.passwordHash
}

func (u *User) SetPasswordHash(passwordHash PasswordHash) error {
	if err := passwordHash.Validate(); err != nil {
		return fmt.Errorf("set user password hash: %w", err)
	}

	u.passwordHash = passwordHash
	return nil
}

func (u *User) FirstName() FirstName {
	return u.firstName
}

func (u *User) SetFirstName(firstName FirstName) error {
	if err := firstName.Validate(); err != nil {
		return fmt.Errorf("set user first name: %w", err)
	}

	u.firstName = firstName
	return nil
}

func (u *User) LastName() LastName {
	return u.lastName
}

func (u *User) SetLastName(lastName LastName) error {
	if err := lastName.Validate(); err != nil {
		return fmt.Errorf("set user last name: %w", err)
	}

	u.lastName = lastName
	return nil
}

// UserRepository stores and retrieves users by ID.
type UserRepository interface {
	Save(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id UserID) (*User, error)
}
