package languages

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// Translation

type Translation string

var (
	ErrTranslationInvalid      = errors.New("invalid translation")
	ErrTranslationInvalidChars = errors.New("invalid characters")
	ErrTranslationEmpty        = errors.New("is empty")
)

func NewTranslation(value string) (Translation, error) {
	var (
		err         error
		translation Translation
	)

	translation = Translation(value)
	err = translation.Validate()
	return translation, err
}

func (tc Translation) Validate() error {
	var err error

	if strings.TrimSpace(string(tc)) == "" {
		err = ErrTranslationEmpty
	} else if !utf8.ValidString(string(tc)) {
		err = ErrTranslationInvalidChars
	}

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrTranslationInvalid, err)
	}

	return err
}

// Text

type Text map[Code]Translation

var (
	ErrTextInvalid             = errors.New("invalid text")
	ErrTextNil                 = errors.New("is nil")
	ErrTextWithoutTranslations = errors.New("there is no any translation")
)

func NewText(translations map[Code]Translation) Text {
	return Text(translations)
}

func (t Text) Validate() error {
	var err error

	if t == nil {
		err = ErrTextNil
	}

	if err == nil {
		if len(t) == 0 {
			err = ErrTextWithoutTranslations
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
		err = fmt.Errorf("%w: %w", ErrTextInvalid, err)
	}

	return err
}
