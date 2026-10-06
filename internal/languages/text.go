package languages

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// Translation content

type TranslationContent string

var (
	ErrInvalidTranslationContent           = errors.New("invalid translation content")
	ErrInvalidTranslationContentCharacters = errors.New("invalid characters")
)

func (tc TranslationContent) Validate() error {
	var err error

	if strings.TrimSpace(string(tc)) == "" || !utf8.ValidString(string(tc)) {
		err = fmt.Errorf("%w: %w", ErrInvalidTranslationContent, ErrInvalidTranslationContentCharacters)
	}

	return err
}

// Text

type Text map[LanguageCode]TranslationContent

var ErrInvalidText = errors.New("invalid text")

func (t Text) Validate() error {
	var err error

	for _, code := range slices.Sorted(maps.Keys(t)) {
		err = code.Validate()

		if err != nil {
			err = fmt.Errorf("%w: %w", ErrInvalidText, err)
			break
		}

		err = t[code].Validate()

		if err != nil {
			err = fmt.Errorf("%w: %w", ErrInvalidText, err)
			break
		}
	}

	return err
}
