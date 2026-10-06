package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
)

var ErrNilLanguageRepository = errors.New("nil language repository")

type LanguageService struct {
	repository languages.LanguageRepository
}

func (s *LanguageService) List(ctx context.Context) ([]*languages.Language, error) {
	var result []*languages.Language
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilLanguageRepository
	} else {
		result, err = s.repository.FindAll(ctx)

		if err != nil {
			err = fmt.Errorf("list languages: %w", err)
		}
	}

	return result, err
}

func NewLanguageService(repository languages.LanguageRepository) (*LanguageService, error) {
	var service *LanguageService
	var err error

	if repository == nil {
		err = ErrNilLanguageRepository
	} else {
		service = &LanguageService{repository: repository}
	}

	return service, err
}

func (s *LanguageService) Create(ctx context.Context, request input.CreateLanguageInput) (*languages.Language, error) {
	var language *languages.Language
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilLanguageRepository
	} else {
		code, codeErr := languages.NewLanguageCode(request.Code)
		english, englishErr := languages.NewLanguageEnglishName(request.EnglishName)
		native, nativeErr := languages.NewLanguageNativeName(request.NativeName)
		err = errors.Join(fieldError("code", codeErr), fieldError("english name", englishErr), fieldError("native name", nativeErr))

		if err == nil {
			language, err = languages.NewLanguage(code, english, native, request.IsFallback)
		}

		if err != nil {
			err = fmt.Errorf("create language: %w: %w", input.ErrInvalidCreateLanguageInput, err)
		} else if language.IsFallback() {
			fallback, findErr := s.repository.FindFallback(ctx)

			if findErr != nil && !errors.Is(findErr, languages.ErrLanguageNotFound) {
				err = fmt.Errorf("find fallback language: %w", findErr)
			} else if fallback != nil {
				err = fmt.Errorf("create fallback language %s: %w", request.Code, languages.ErrFallbackLanguageAlreadyExists)
			}
		}

		if err == nil {
			if createErr := s.repository.Create(ctx, language); createErr != nil {
				err = fmt.Errorf("create language %s: %w", request.Code, createErr)
			}
		}

		if err != nil {
			language = nil
		}
	}

	return language, err
}

func (s *LanguageService) Delete(ctx context.Context, code string) error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrNilLanguageRepository
	} else {
		languageCode, validationErr := languages.NewLanguageCode(code)
		if validationErr != nil {
			err = fmt.Errorf("delete language: %w", validationErr)
		} else if deleteErr := s.repository.Delete(ctx, languageCode); deleteErr != nil {
			err = fmt.Errorf("delete language %s: %w", code, deleteErr)
		}
	}

	return err
}

func fieldError(field string, err error) error {
	if err != nil {
		err = fmt.Errorf("%s: %w", field, err)
	}

	return err
}
