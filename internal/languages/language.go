package languages

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

//
// Alpha-2 code
//

type Code string

var (
	ErrCodeInvalid      = errors.New("invalid language code")
	ErrCodeInvalidChars = errors.New("invalid characters")
	ErrCodeWrongLength  = errors.New("is not 2 characters long")
	codePattern         = regexp.MustCompile(`^[a-z]{2}$`)
)

func (c Code) Validate() error {
	var err error

	if !utf8.ValidString(string(c)) || !codePattern.MatchString(string(c)) {
		err = ErrCodeInvalidChars
	} else if utf8.RuneCountInString(string(c)) != 2 {
		err = ErrCodeWrongLength
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrCodeInvalid, err)
	}

	return err
}

//
// English name
//

type EnglishName string

const MaxEnglishNameLength = 100

var (
	ErrEnglishNameInvalid      = errors.New("invalid English name")
	ErrEnglishNameInvalidChars = errors.New("invalid characters")
	ErrEnglishNameEmpty        = errors.New("is empty")
	ErrEnglishNameTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxEnglishNameLength)
)

func (n EnglishName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrEnglishNameEmpty
	} else if !utf8.ValidString(string(n)) {
		err = ErrEnglishNameInvalidChars
	} else if utf8.RuneCountInString(string(n)) > MaxEnglishNameLength {
		err = ErrEnglishNameTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrEnglishNameInvalid, err)
	}

	return err
}

//
// Native name
//

type NativeName string

const MaxNativeNameLength = 100

var (
	ErrNativeNameInvalid      = errors.New("invalid English name")
	ErrNativeNameInvalidChars = errors.New("invalid characters")
	ErrNativeNameEmpty        = errors.New("is empty")
	ErrNativeNameTooLong      = fmt.Errorf("exceeds the limit of %d characters", MaxNativeNameLength)
)

func (n NativeName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrNativeNameEmpty
	} else if !utf8.ValidString(string(n)) {
		err = ErrNativeNameInvalidChars
	} else if utf8.RuneCountInString(string(n)) > MaxEnglishNameLength {
		err = ErrNativeNameTooLong
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrNativeNameInvalid, err)
	}

	return err
}

//
// Language
//

type Language struct {
	Code        Code
	EnglishName EnglishName
	NativeName  NativeName
	IsFallback  bool
}

var (
	ErrLanguageInvalid = errors.New("invalid language")
	ErrLanguageNil     = errors.New("is nil")
)

func NewLanguage(
	code Code,
	englishName EnglishName,
	nativeName NativeName,
	isFallback bool,
) *Language {
	return &Language{
		Code:        code,
		EnglishName: englishName,
		NativeName:  nativeName,
		IsFallback:  isFallback,
	}
}

func (l *Language) Validate() error {
	var err error

	if l == nil {
		err = ErrLanguageNil
	}

	if err == nil {
		err = l.Code.Validate()
	}

	if err == nil {
		err = l.EnglishName.Validate()
	}

	if err == nil {
		err = l.NativeName.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrLanguageInvalid, err)
	}

	return err
}
