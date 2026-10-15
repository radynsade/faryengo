package accounts

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/internal/workspaces"
)

//
// ID
//

var ErrIDInvalid = errors.New("invalid ID")

type ID uuid.UUID

func NewID(value uuid.UUID) (ID, error) {
	var (
		err error
		id  ID
	)

	id = ID(value)
	err = id.Validate()
	return id, err
}

func (id ID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrIDInvalid
	}

	return err
}

//
// Name
//

const MaxNameLength = 100

var (
	ErrNameInvalid      = errors.New("invalid name")
	ErrNameEmpty        = errors.New("is empty")
	ErrNameInvalidChars = errors.New("invalid characters")
	ErrNameTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxNameLength)
)

type Name string

func NewName(value string) (Name, error) {
	var (
		err  error
		name Name
	)

	name = Name(value)
	err = name.Validate()
	return name, err
}

func (n Name) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrNameEmpty
	} else if !utf8.ValidString(string(n)) {
		err = ErrNameInvalidChars
	} else if utf8.RuneCountInString(string(n)) > MaxNameLength {
		err = ErrNameTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrNameInvalid, err)
	}

	return err
}

//
// Account
//

var (
	ErrAccountInvalid = errors.New("invalid account")
	ErrAccountNil     = errors.New("is nil")
)

// Every Account holds money in one Currency, referenced by its code.

type Account struct {
	ID           ID
	WorkspaceID  workspaces.ID
	UserID       *users.UserID
	CurrencyCode currencies.Code
	Name         Name
}

func NewAccount(
	id ID,
	workspaceID workspaces.ID,
	userID *users.UserID,
	currencyCode currencies.Code,
	name Name,
) *Account {
	return &Account{
		ID:           id,
		WorkspaceID:  workspaceID,
		UserID:       userID,
		CurrencyCode: currencyCode,
		Name:         name,
	}
}

func (a *Account) Validate() error {
	var (
		err       error
		userIDErr error
	)

	if a == nil {
		err = ErrAccountNil
	} else {
		if a.UserID != nil {
			userIDErr = a.UserID.Validate()
		}

		err = errors.Join(
			a.ID.Validate(),
			a.WorkspaceID.Validate(),
			userIDErr,
			a.CurrencyCode.Validate(),
			a.Name.Validate(),
		)
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrAccountInvalid, err)
	}

	return err
}
