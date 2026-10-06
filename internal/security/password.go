package security

import (
	"strings"
	"unicode/utf8"
)

// Password is a bounded plaintext credential, distinct from PasswordHash.
// Its zero value is invalid. Constructors do not trim or normalize secrets.
type Password struct {
	value string
}

func NewPassword(value string) (Password, error) {
	var password Password
	var err error

	if value == "" || len(value) > MaxPasswordBytes {
		err = ErrInvalidPassword
	} else {
		password = Password{value: value}
	}

	return password, err
}

// NewRegistrationPassword applies the additional policy for new accounts.
// Existing password replacements and sign-in retain their historical policy.
func NewRegistrationPassword(value string) (Password, error) {
	password, err := NewPassword(value)

	if err == nil && (strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) < 8) {
		password, err = Password{}, ErrInvalidPassword
	}

	return password, err
}

func (p Password) Value() string {
	return p.value
}
