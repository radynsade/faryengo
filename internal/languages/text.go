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
	ErrEmptyTranslationContent             = errors.New("is empty")
)

func (tc TranslationContent) Validate() error {
	var err error

	if strings.TrimSpace(string(tc)) == "" {
		err = ErrEmptyTranslationContent
	} else if !utf8.ValidString(string(tc)) {
		err = ErrInvalidTranslationContentCharacters
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidTranslationContent, err)
	}

	return err
}

// Text

type Text map[Code]TranslationContent

var (
	ErrInvalidText           = errors.New("invalid text")
	ErrNilText               = errors.New("is nil")
	ErrTextHasNoTranslations = errors.New("there is no any translation")
)

func (t Text) Validate() error {
	var err error

	if t == nil {
		err = ErrNilText
	}

	if err == nil {
		if len(t) == 0 {
			err = ErrTextHasNoTranslations
		} else {
			for _, code := range slices.Sorted(maps.Keys(t)) {
				err = code.Validate()

				if err != nil {
					break
				}

				err = t[code].Validate()

				if err != nil {
					break
				}
			}
		}
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidText, err)
	}

	return err
}
