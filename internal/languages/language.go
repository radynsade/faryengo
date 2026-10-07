package languages

import (
	"context"
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
	ErrInvalidCode           = errors.New("invalid language code")
	ErrInvalidCodeCharacters = errors.New("invalid characters")
	ErrWrongCodeLength       = errors.New("is not 2 characters long")
	codePattern              = regexp.MustCompile(`^[a-z]{2}$`)
)

func (c Code) Validate() error {
	var err error

	if !utf8.ValidString(string(c)) || !codePattern.MatchString(string(c)) {
		err = ErrInvalidCodeCharacters
	} else if utf8.RuneCountInString(string(c)) != 2 {
		err = ErrWrongCodeLength
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidCode, err)
	}

	return err
}

//
// English name
//

type EnglishName string

const MaximumEnglishNameLength = 100

var (
	ErrInvalidEnglishName           = errors.New("invalid English name")
	ErrInvalidEnglishNameCharacters = errors.New("invalid characters")
	ErrEmptyEnglishName             = errors.New("is empty")
	ErrTooLongEnglishName           = fmt.Errorf("exceeds the limit of %d characters", MaximumEnglishNameLength)
)

func (n EnglishName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrEmptyEnglishName
	} else if !utf8.ValidString(string(n)) {
		err = ErrInvalidEnglishNameCharacters
	} else if utf8.RuneCountInString(string(n)) > MaximumEnglishNameLength {
		err = ErrTooLongEnglishName
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidEnglishName, err)
	}

	return err
}

//
// Native name
//

type NativeName string

const MaximumNativeNameLength = 100

var (
	ErrInvalidNativeName           = errors.New("invalid English name")
	ErrInvalidNativeNameCharacters = errors.New("invalid characters")
	ErrEmptyNativeName             = errors.New("is empty")
	ErrTooLongNativeName           = fmt.Errorf("exceeds the limit of %d characters", MaximumNativeNameLength)
)

func (n NativeName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" {
		err = ErrEmptyNativeName
	} else if !utf8.ValidString(string(n)) {
		err = ErrInvalidNativeNameCharacters
	} else if utf8.RuneCountInString(string(n)) > MaximumEnglishNameLength {
		err = ErrTooLongNativeName
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidNativeName, err)
	}

	return err
}

//
// Language
//

type Language struct {
	code        Code
	englishName EnglishName
	nativeName  NativeName
	isFallback  bool
}

var ErrInvalidLanguage = errors.New("invalid language")

func NewLanguage(
	code Code,
	englishName EnglishName,
	nativeName NativeName,
	isFallback bool,
) (*Language, error) {
	return &Language{
		code:        code,
		englishName: englishName,
		nativeName:  nativeName,
		isFallback:  isFallback,
	}, nil
}

func (l *Language) Code() Code {
	return l.code
}

func (l *Language) EnglishName() EnglishName {
	return l.englishName
}

func (l *Language) NativeName() NativeName {
	return l.nativeName
}

func (l *Language) IsFallback() bool {
	return l.isFallback
}

func (l *Language) SetIsFallback(isFallback bool) {
	l.isFallback = isFallback
}

func (l *Language) Validate() error {
	var err error
	err = l.code.Validate()

	if err == nil {
		err = l.englishName.Validate()
	}

	if err == nil {
		err = l.nativeName.Validate()
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidLanguage, err)
	}

	return err
}

//
// Language repository
//

var (
	ErrLanguageNotFound              = errors.New("language not found")
	ErrLanguageAlreadyExists         = errors.New("language already exists")
	ErrFallbackLanguageAlreadyExists = errors.New("fallback language already exists")
	ErrFallbackLanguageAlreadyInUse  = errors.New("fallback language already in use")
)

type ErrLanguageCreateFailed interface {
	error
	Language() *Language
	Unwrap() error
}

type ErrLanguageUpdateFailed interface {
	error
	Language() *Language
	Unwrap() error
}

type ErrLanguageDeleteFailed interface {
	error
	LanguageCode() Code
	Unwrap() error
}

type LanguageRepository interface {
	Create(ctx context.Context, language *Language) ErrLanguageCreateFailed
	Update(ctx context.Context, language *Language) ErrLanguageUpdateFailed
	Delete(ctx context.Context, code Code) ErrLanguageDeleteFailed
	FindByCode(ctx context.Context, code Code) (*Language, error)
	FindFallback(ctx context.Context) (*Language, error)
	FindAll(ctx context.Context) ([]*Language, error)
}
