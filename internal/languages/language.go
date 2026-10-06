package languages

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Alpha-2 language code

type LanguageCode string

var (
	ErrInvalidLanguageCode           = errors.New("invalid language code")
	ErrInvalidLanguageCodeCharacters = errors.New("invalid characters")
	ErrWrongLanguageCodeLength       = errors.New("is not 2 characters long")
	languageCodePattern              = regexp.MustCompile(`^[a-z]{2}$`)
)

func (c LanguageCode) Validate() error {
	var err error

	if !utf8.ValidString(string(c)) || !languageCodePattern.MatchString(string(c)) {
		err = fmt.Errorf("%w: %w", ErrInvalidLanguageCode, ErrInvalidLanguageCodeCharacters)
	} else if utf8.RuneCountInString(string(c)) > 2 {
		err = fmt.Errorf("%w: %w", ErrInvalidLanguageCode, ErrWrongLanguageCodeLength)
	}

	return err
}

// Language english name

type LanguageEnglishName string

const MaximumLanguageEnglishNameLength = 100

var (
	ErrInvalidLanguageEnglishName           = errors.New("invalid English name")
	ErrInvalidLanguageEnglishNameCharacters = errors.New("invalid characters")
	ErrTooLongLanguageEnglishName           = fmt.Errorf("exceeds the limit of %d characters", MaximumLanguageEnglishNameLength)
)

func (n LanguageEnglishName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" || !utf8.ValidString(string(n)) {
		err = fmt.Errorf("%w: %w", ErrInvalidLanguageEnglishName, ErrInvalidLanguageEnglishNameCharacters)
	} else if utf8.RuneCountInString(string(n)) > MaximumLanguageEnglishNameLength {
		err = fmt.Errorf("%w: %w", ErrInvalidLanguageEnglishName, ErrTooLongLanguageEnglishName)
	}

	return err
}

// Language native name

type LanguageNativeName string

const MaximumLanguageNativeNameLength = 100

var (
	ErrInvalidLanguageNativeName           = errors.New("invalid English name")
	ErrInvalidLanguageNativeNameCharacters = errors.New("invalid characters")
	ErrTooLongLanguageNativeName           = fmt.Errorf("exceeds the limit of %d characters", MaximumLanguageNativeNameLength)
)

func (n LanguageNativeName) Validate() error {
	var err error

	if strings.TrimSpace(string(n)) == "" || !utf8.ValidString(string(n)) {
		err = fmt.Errorf("%w: %w", ErrInvalidLanguageNativeName, ErrInvalidLanguageNativeNameCharacters)
	} else if utf8.RuneCountInString(string(n)) > MaximumLanguageEnglishNameLength {
		err = fmt.Errorf("%w: %w", ErrInvalidLanguageNativeName, ErrTooLongLanguageNativeName)
	}

	return err
}

// Language

type Language struct {
	code        LanguageCode
	englishName LanguageEnglishName
	nativeName  LanguageNativeName
	isFallback  bool
}

var ErrInvalidLanguage = errors.New("invalid language")

func NewLanguage(
	code LanguageCode,
	englishName LanguageEnglishName,
	nativeName LanguageNativeName,
	isFallback bool,
) (*Language, error) {
	return &Language{
		code:        code,
		englishName: englishName,
		nativeName:  nativeName,
		isFallback:  isFallback,
	}, nil
}

func (l *Language) Code() LanguageCode {
	return l.code
}

func (l *Language) EnglishName() LanguageEnglishName {
	return l.englishName
}

func (l *Language) NativeName() LanguageNativeName {
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

	if err := l.code.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidLanguage, err)
	}

	if err := l.englishName.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidLanguage, err)
	}

	if err := l.nativeName.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidLanguage, err)
	}

	return err
}

// Language repository

type LanguageRepository interface {
	Create(ctx context.Context, language *Language) error
	Update(ctx context.Context, language *Language) error
	Delete(ctx context.Context, code LanguageCode) error
	FindByCode(ctx context.Context, code LanguageCode) (*Language, error)
	FindFallback(ctx context.Context) (*Language, error)
	FindAll(ctx context.Context) ([]*Language, error)
}
