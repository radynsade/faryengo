package languages

import (
	"context"
	"errors"
)

var (
	ErrLanguageNotFound              = errors.New("language not found")
	ErrLanguageAlreadyExists         = errors.New("language already exists")
	ErrFallbackLanguageAlreadyExists = errors.New("fallback language already exists")
	ErrFallbackLanguageAlreadyInUse  = errors.New("fallback language already in use")
	ErrLanguageInUse                 = errors.New("language is used by translations")
)

type ErrLanguageCreateFailed interface {
	error
	Language() *Language
	Unwrap() error
}

type ErrLanguageUpdateFailed interface {
	error
	Language() *Language
	Unwrap() error
}

type ErrLanguageDeleteFailed interface {
	error
	LanguageCode() Code
	Unwrap() error
}

type LanguageRepository interface {
	Create(ctx context.Context, language *Language) ErrLanguageCreateFailed
	Update(ctx context.Context, language *Language) ErrLanguageUpdateFailed
	Delete(ctx context.Context, code Code) ErrLanguageDeleteFailed
	FindByCode(ctx context.Context, code Code) (*Language, error)
	FindByCodeForUpdate(ctx context.Context, code Code) (*Language, error)
	FindFallback(ctx context.Context) (*Language, error)
	FindAll(ctx context.Context) ([]*Language, error)
}
