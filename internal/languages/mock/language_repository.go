package mock

import (
	"context"
	"errors"

	"github.com/radynsade/faryengo/internal/languages"
)

//
// Errors
//

var ErrNotConfigured = errors.New("mock operation is not configured")

type errLanguageWriteFailed struct {
	language *languages.Language
	err      error
}

func (e errLanguageWriteFailed) Language() *languages.Language {
	return e.language
}

func (e errLanguageWriteFailed) Unwrap() error {
	return e.err
}

// Implementation of languages.ErrLanguageCreateFailed

type ErrLanguageCreateFailed struct {
	errLanguageWriteFailed
}

func NewErrLanguageCreateFailed(language *languages.Language, err error) *ErrLanguageCreateFailed {
	return &ErrLanguageCreateFailed{
		errLanguageWriteFailed: errLanguageWriteFailed{language, err},
	}
}

func (e *ErrLanguageCreateFailed) Error() string {
	return "failed to create a language"
}

// Implementation of languages.ErrLanguageUpdateFailed

type ErrLanguageUpdateFailed struct {
	errLanguageWriteFailed
}

func NewErrLanguageUpdateFailed(language *languages.Language, err error) *ErrLanguageUpdateFailed {
	return &ErrLanguageUpdateFailed{
		errLanguageWriteFailed: errLanguageWriteFailed{language, err},
	}
}

func (e *ErrLanguageUpdateFailed) Error() string {
	return "failed to update a language"
}

// Implementation of languages.ErrLanguageDeleteFailed

type ErrLanguageDeleteFailed struct {
	code languages.Code
	err  error
}

func NewErrLanguageDeleteFailed(code languages.Code, err error) *ErrLanguageDeleteFailed {
	return &ErrLanguageDeleteFailed{code, err}
}

func (e *ErrLanguageDeleteFailed) LanguageCode() languages.Code {
	return e.code
}

func (e *ErrLanguageDeleteFailed) Unwrap() error {
	return e.err
}

func (e *ErrLanguageDeleteFailed) Error() string {
	return "failed to delete a language"
}

//
// Language repository
//

type LanguageRepository struct {
	CreateFunc              func(ctx context.Context, language *languages.Language) languages.ErrLanguageCreateFailed
	UpdateFunc              func(ctx context.Context, language *languages.Language) languages.ErrLanguageUpdateFailed
	DeleteFunc              func(ctx context.Context, code languages.Code) languages.ErrLanguageDeleteFailed
	FindByCodeFunc          func(ctx context.Context, code languages.Code) (*languages.Language, error)
	FindByCodeForUpdateFunc func(ctx context.Context, code languages.Code) (*languages.Language, error)
	FindFallbackFunc        func(ctx context.Context) (*languages.Language, error)
	FindAllFunc             func(ctx context.Context) ([]*languages.Language, error)
}

var _ languages.LanguageRepository = (*LanguageRepository)(nil)

// Create

func (r *LanguageRepository) Create(
	ctx context.Context,
	language *languages.Language,
) languages.ErrLanguageCreateFailed {
	var err languages.ErrLanguageCreateFailed

	if r.CreateFunc != nil {
		err = r.CreateFunc(ctx, language)
	} else {
		err = NewErrLanguageCreateFailed(language, ErrNotConfigured)
	}

	return err
}

// Update

func (r *LanguageRepository) Update(
	ctx context.Context,
	language *languages.Language,
) languages.ErrLanguageUpdateFailed {
	var err languages.ErrLanguageUpdateFailed

	if r.UpdateFunc != nil {
		err = r.UpdateFunc(ctx, language)
	} else {
		err = NewErrLanguageUpdateFailed(language, ErrNotConfigured)
	}

	return err
}

// Delete

func (r *LanguageRepository) Delete(
	ctx context.Context,
	code languages.Code,
) languages.ErrLanguageDeleteFailed {
	var err languages.ErrLanguageDeleteFailed

	if r.DeleteFunc != nil {
		err = r.DeleteFunc(ctx, code)
	} else {
		err = NewErrLanguageDeleteFailed(code, ErrNotConfigured)
	}

	return err
}

// Find by a code

func (r *LanguageRepository) FindByCode(
	ctx context.Context,
	code languages.Code,
) (*languages.Language, error) {
	var (
		language *languages.Language
		err      = ErrNotConfigured
	)

	if r.FindByCodeFunc != nil {
		language, err = r.FindByCodeFunc(ctx, code)
	}

	return language, err
}

// Find by a code for update

func (r *LanguageRepository) FindByCodeForUpdate(
	ctx context.Context,
	code languages.Code,
) (*languages.Language, error) {
	var (
		language *languages.Language
		err      = ErrNotConfigured
	)

	if r.FindByCodeForUpdateFunc != nil {
		language, err = r.FindByCodeForUpdateFunc(ctx, code)
	}

	return language, err
}

// Find the fallback language

func (r *LanguageRepository) FindFallback(ctx context.Context) (*languages.Language, error) {
	var (
		language *languages.Language
		err      = ErrNotConfigured
	)

	if r.FindFallbackFunc != nil {
		language, err = r.FindFallbackFunc(ctx)
	}

	return language, err
}

// Find all languages

func (r *LanguageRepository) FindAll(ctx context.Context) ([]*languages.Language, error) {
	var (
		result []*languages.Language
		err    = ErrNotConfigured
	)

	if r.FindAllFunc != nil {
		result, err = r.FindAllFunc(ctx)
	}

	return result, err
}
