package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/users"
)

//
// Language service
//

// Every write runs in a transaction, so it is atomic on its own and joins a
// transaction already carried by the caller's context.
//
// Every method acts for the caller's security.Identity in the context.
// Managing Languages needs PermissionManageLanguage. Any identity may read
// them, because translated names are shown throughout the application. A
// caller without an identity is reported as ErrUnauthenticated and one without
// the permission as users.ErrPermissionDenied.

var (
	ErrTransactorNil         = errors.New("transactor is nil")
	ErrLanguageRepositoryNil = errors.New("language repository is nil")
)

type LanguageService struct {
	transactor Transactor
	repository languages.LanguageRepository
}

func NewLanguageService(
	transactor Transactor,
	repository languages.LanguageRepository,
) (*LanguageService, error) {
	var (
		service *LanguageService
		err     error
	)

	if transactor == nil {
		err = ErrTransactorNil
	} else if repository == nil {
		err = ErrLanguageRepositoryNil
	} else {
		service = &LanguageService{transactor: transactor, repository: repository}
	}

	return service, err
}

// Create

func (s *LanguageService) Create(
	ctx context.Context,
	request input.CreateLanguageInput,
) (*languages.Language, error) {
	var (
		language *languages.Language
		err      error
	)

	if err = s.check(); err == nil {
		_, err = authorizeSystem(ctx, users.PermissionManageLanguage)

		if err != nil {
			err = fmt.Errorf("create language: %w", err)
		}
	}

	if err == nil {
		language, err = newLanguage(request.Code, request.EnglishName, request.NativeName, request.IsFallback)

		if err != nil {
			err = fmt.Errorf("create language: %w: %w", input.ErrCreateLanguageInputInvalid, err)
		} else {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var err error

				if createErr := s.repository.Create(ctx, language); createErr != nil {
					err = createErr
				}

				return err
			})

			if err != nil {
				err = fmt.Errorf("create language %s: %w", language.Code, err)
			}
		}
	}

	if err != nil {
		language = nil
	}

	return language, err
}

// Find by a code

func (s *LanguageService) FindByCode(ctx context.Context, code string) (*languages.Language, error) {
	var (
		language     *languages.Language
		languageCode languages.Code
		err          error
	)

	if s == nil || s.repository == nil {
		err = ErrLanguageRepositoryNil
	} else if languageCode, err = languages.NewCode(code); err != nil {
		err = fmt.Errorf("find language: %w", err)
	} else {
		_, err = currentIdentity(ctx)

		if err == nil {
			language, err = s.repository.FindByCode(ctx, languageCode)
		}

		if err != nil {
			err = fmt.Errorf("find language %s: %w", code, err)
		}
	}

	if err != nil {
		language = nil
	}

	return language, err
}

// List all languages ordered by a code

func (s *LanguageService) List(ctx context.Context) ([]*languages.Language, error) {
	var (
		result []*languages.Language
		err    error
	)

	if s == nil || s.repository == nil {
		err = ErrLanguageRepositoryNil
	} else {
		_, err = currentIdentity(ctx)

		if err == nil {
			result, err = s.repository.FindAll(ctx)
		}

		if err != nil {
			err = fmt.Errorf("list languages: %w", err)
			result = nil
		}
	}

	return result, err
}

// The update replaces every field of the Language identified by the code. The
// Language is locked first, so a missing one is reported as ErrLanguageNotFound
// and concurrent updates apply one after another.

func (s *LanguageService) Update(
	ctx context.Context,
	request input.UpdateLanguageInput,
) (*languages.Language, error) {
	var (
		language *languages.Language
		err      error
	)

	if err = s.check(); err == nil {
		_, err = authorizeSystem(ctx, users.PermissionManageLanguage)

		if err != nil {
			err = fmt.Errorf("update language %s: %w", request.Code, err)
		}
	}

	if err == nil {
		language, err = newLanguage(request.Code, request.EnglishName, request.NativeName, request.IsFallback)

		if err != nil {
			err = fmt.Errorf("update language: %w: %w", input.ErrUpdateLanguageInputInvalid, err)
		} else {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				_, err := s.repository.FindByCodeForUpdate(ctx, language.Code)

				if err == nil {
					if updateErr := s.repository.Update(ctx, language); updateErr != nil {
						err = updateErr
					}
				}

				return err
			})

			if err != nil {
				err = fmt.Errorf("update language %s: %w", language.Code, err)
			}
		}
	}

	if err != nil {
		language = nil
	}

	return language, err
}

// Delete

func (s *LanguageService) Delete(ctx context.Context, code string) error {
	var languageCode languages.Code

	err := s.check()

	if err == nil {
		languageCode, err = languages.NewCode(code)

		if err != nil {
			err = fmt.Errorf("delete language: %w", err)
		}
	}

	if err == nil {
		_, err = authorizeSystem(ctx, users.PermissionManageLanguage)

		if err == nil {
			err = s.transactor.InTransaction(ctx, func(ctx context.Context) error {
				var err error

				if deleteErr := s.repository.Delete(ctx, languageCode); deleteErr != nil {
					err = deleteErr
				}

				return err
			})
		}

		if err != nil {
			err = fmt.Errorf("delete language %s: %w", code, err)
		}
	}

	return err
}

//
// Helpers
//

func (s *LanguageService) check() error {
	var err error

	if s == nil || s.repository == nil {
		err = ErrLanguageRepositoryNil
	} else if s.transactor == nil {
		err = ErrTransactorNil
	}

	return err
}

// Every field is validated, rather than stopping at the first failure, so a
// caller can report all invalid fields at once.

func newLanguage(
	code string,
	englishName string,
	nativeName string,
	isFallback bool,
) (*languages.Language, error) {
	var language *languages.Language

	languageCode, codeErr := languages.NewCode(code)
	languageEnglishName, englishNameErr := languages.NewEnglishName(englishName)
	languageNativeName, nativeNameErr := languages.NewNativeName(nativeName)

	err := errors.Join(codeErr, englishNameErr, nativeNameErr)

	if err == nil {
		language = languages.NewLanguage(languageCode, languageEnglishName, languageNativeName, isFallback)
	}

	return language, err
}
