package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

//
// Password
//

const (
	MinimumPasswordLength = 6
	MaxPasswordBytes      = 4096
)

var (
	ErrInvalidPassword           = errors.New("invalid password")
	ErrEmptyPassword             = errors.New("is empty")
	ErrTooShortPassword          = fmt.Errorf("less than %d characters", MinimumPasswordLength)
	ErrTooLongPassword           = fmt.Errorf("exceeds the limit of %d bytes", MaxPasswordBytes)
	ErrInvalidPasswordCharacters = errors.New("invalid characters")
)

type Password string

func (p Password) Validate() error {
	var err error

	if strings.TrimSpace(string(p)) == "" {
		err = ErrEmptyPassword
	} else if !utf8.ValidString(string(p)) {
		err = ErrInvalidPasswordCharacters
	} else if len(p) > MaxPasswordBytes {
		err = ErrTooLongPassword
	} else if utf8.RuneCountInString(string(p)) < MinimumPasswordLength {
		err = ErrTooShortPassword
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidPassword, err)
	}

	return err
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
// Password hasher
//

type PasswordHasher interface {
	Hash(ctx context.Context, password string) (PasswordHash, error)
	Verify(ctx context.Context, password string, hash PasswordHash) (bool, error)
}
