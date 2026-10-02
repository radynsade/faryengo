package input

import (
	"errors"
	"fmt"

	"github.com/radynsade/faryengo/internal/languages"
)

var ErrInvalidCreateLanguageInput = errors.New("invalid create language input")

type CreateLanguageInput struct {
	Code        string
	EnglishName string
	NativeName  string
	IsFallback  bool
}

func (input CreateLanguageInput) Validate() error {
	var validationErrors []error

	if err := languages.LanguageCode(input.Code).Validate(); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("code: %w", err))
	}

	if err := languages.LanguageEnglishName(input.EnglishName).Validate(); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("english name: %w", err))
	}

	if err := languages.LanguageNativeName(input.NativeName).Validate(); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("native name: %w", err))
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidCreateLanguageInput, errors.Join(validationErrors...))
	}

	return nil
}
