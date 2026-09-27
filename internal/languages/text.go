package languages

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidTranslationContent   = errors.New("invalid translation content")
	ErrDuplicateTranslation        = errors.New("duplicate translation language code")
	ErrTranslationLanguageMismatch = errors.New("translation language code does not match text key")
)

type Translation struct {
	languageCode LanguageCode
	content      string
}

func NewTranslation(languageCode LanguageCode, content string) (Translation, error) {
	translation := Translation{languageCode: languageCode, content: content}
	err := translation.Validate()

	if err != nil {
		translation = Translation{}
		err = fmt.Errorf("create translation: %w", err)
	}

	return translation, err
}

func (t Translation) Validate() error {
	err := t.languageCode.Validate()

	if err != nil {
		err = fmt.Errorf("validate translation language code: %w", err)
	} else if strings.TrimSpace(t.content) == "" || !utf8.ValidString(t.content) {
		err = ErrInvalidTranslationContent
	}

	return err
}

func (t Translation) LanguageCode() LanguageCode {
	return t.languageCode
}

func (t Translation) Content() string {
	return t.content
}

type Text map[LanguageCode]Translation

func NewText(translations []Translation) (Text, error) {
	text := make(Text, len(translations))
	var err error

	for _, translation := range translations {
		err = translation.Validate()

		if err != nil {
			err = fmt.Errorf("validate text translation: %w", err)
			break
		}

		code := translation.LanguageCode()

		if _, exists := text[code]; exists {
			err = ErrDuplicateTranslation
			break
		}

		text[code] = translation
	}

	if err != nil {
		text = nil
		err = fmt.Errorf("create text: %w", err)
	}

	return text, err
}

func (t Text) Validate() error {
	var err error

	for _, code := range slices.Sorted(maps.Keys(t)) {
		err = code.Validate()

		if err != nil {
			err = fmt.Errorf("validate text key: %w", err)
			break
		}

		translation := t[code]

		if translation.LanguageCode() != code {
			err = ErrTranslationLanguageMismatch
			break
		}

		err = translation.Validate()

		if err != nil {
			err = fmt.Errorf("validate text translation: %w", err)
			break
		}
	}

	return err
}

func (t Text) Translation(languageCode LanguageCode) (Translation, bool) {
	translation, found := t[languageCode]
	return translation, found
}

func (t Text) Translations() []Translation {
	translations := make([]Translation, 0, len(t))

	for _, code := range slices.Sorted(maps.Keys(t)) {
		translations = append(translations, t[code])
	}

	return translations
}
