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
	MinPasswordLength = 6
	MaxPasswordBytes  = 4096
)

var (
	ErrPasswordInvalid      = errors.New("invalid password")
	ErrPasswordEmpty        = errors.New("is empty")
	ErrPasswordTooShort     = fmt.Errorf("less than %d characters", MinPasswordLength)
	ErrPasswordTooLong      = fmt.Errorf("exceeds the limit of %d bytes", MaxPasswordBytes)
	ErrPasswordInvalidChars = errors.New("invalid characters")
)

type Password string

func (p Password) Validate() error {
	var err error

	if strings.TrimSpace(string(p)) == "" {
		err = ErrPasswordEmpty
	} else if !utf8.ValidString(string(p)) {
		err = ErrPasswordInvalidChars
	} else if len(p) > MaxPasswordBytes {
		err = ErrPasswordTooLong
	} else if utf8.RuneCountInString(string(p)) < MinPasswordLength {
		err = ErrPasswordTooShort
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrPasswordInvalid, err)
	}

	return err
}

//
// Password hash
//

var (
	ErrPasswordHashInvalid = errors.New("invalid password hash")
	ErrPasswordHashEmpty   = errors.New("is empty")
)

type PasswordHash string

func (p PasswordHash) Validate() error {
	var err error

	if strings.TrimSpace(string(p)) == "" {
		err = ErrPasswordHashEmpty
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrPasswordHashInvalid, err)
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
