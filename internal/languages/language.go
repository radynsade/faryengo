package languages

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidLanguageCode        = errors.New("invalid language code")
	ErrInvalidLanguageEnglishName = errors.New("invalid language English name")
	ErrInvalidLanguageNativeName  = errors.New("invalid language native name")
)

var languageCodePattern = regexp.MustCompile(`^[a-z]{2}$`)

type LanguageCode string

func NewLanguageCode(value string) (LanguageCode, error) {
	code := LanguageCode(value)

	if err := code.Validate(); err != nil {
		return LanguageCode(""), err
	}

	return code, nil
}

func (c LanguageCode) Validate() error {
	if !languageCodePattern.MatchString(string(c)) {
		return ErrInvalidLanguageCode
	}

	return nil
}

type LanguageEnglishName string

func NewLanguageEnglishName(value string) (LanguageEnglishName, error) {
	name := LanguageEnglishName(value)

	if err := name.Validate(); err != nil {
		return LanguageEnglishName(""), err
	}

	return name, nil
}

func (n LanguageEnglishName) Validate() error {
	if strings.TrimSpace(string(n)) == "" || !utf8.ValidString(string(n)) || utf8.RuneCountInString(string(n)) > 100 {
		return ErrInvalidLanguageEnglishName
	}

	return nil
}

type LanguageNativeName string

func NewLanguageNativeName(value string) (LanguageNativeName, error) {
	name := LanguageNativeName(value)

	if err := name.Validate(); err != nil {
		return LanguageNativeName(""), err
	}

	return name, nil
}

func (n LanguageNativeName) Validate() error {
	if strings.TrimSpace(string(n)) == "" || !utf8.ValidString(string(n)) || utf8.RuneCountInString(string(n)) > 100 {
		return ErrInvalidLanguageNativeName
	}

	return nil
}

type Language struct {
	code        LanguageCode
	englishName LanguageEnglishName
	nativeName  LanguageNativeName
}

func NewLanguage(code LanguageCode, englishName LanguageEnglishName, nativeName LanguageNativeName) (*Language, error) {
	if err := code.Validate(); err != nil {
		return nil, fmt.Errorf("create language: %w", err)
	}

	if err := englishName.Validate(); err != nil {
		return nil, fmt.Errorf("create language: %w", err)
	}

	if err := nativeName.Validate(); err != nil {
		return nil, fmt.Errorf("create language: %w", err)
	}

	return &Language{code: code, englishName: englishName, nativeName: nativeName}, nil
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
