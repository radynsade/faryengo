package pgxgoqu

import (
	"context"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/languages"
)

var (
	ErrNilPool     = errors.New("nil PostgreSQL pool")
	ErrNilLanguage = errors.New("nil language")
)

type languageDB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type LanguageRepository struct {
	db languageDB
}

var _ languages.LanguageRepository = (*LanguageRepository)(nil)

func NewLanguageRepository(pool *pgxpool.Pool) (*LanguageRepository, error) {
	var repository *LanguageRepository
	var err error

	if pool == nil {
		err = ErrNilPool
	} else {
		repository = &LanguageRepository{db: pool}
	}

	return repository, err
}

// Create inserts a language, rejecting duplicate codes and a second fallback.
func (r *LanguageRepository) Create(ctx context.Context, language *languages.Language) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if language == nil {
		err = ErrNilLanguage
	} else if validationErr := validateLanguage(language); validationErr != nil {
		err = validationErr
	} else {
		code := string(language.Code())
		query, args, buildErr := goqu.Dialect("postgres").
			Insert("language").
			Rows(goqu.Record{
				"code":         code,
				"english_name": string(language.EnglishName()),
				"native_name":  string(language.NativeName()),
				"is_fallback":  language.IsFallback(),
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build create language %s query: %w", code, buildErr)
		} else {
			_, execErr := r.db.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("create language %s: %w", code, mapLanguageError(execErr))
			}
		}
	}

	return err
}

func (r *LanguageRepository) Update(ctx context.Context, language *languages.Language) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if language == nil {
		err = ErrNilLanguage
	} else if validationErr := validateLanguage(language); validationErr != nil {
		err = validationErr
	} else {
		code := string(language.Code())
		query, args, buildErr := goqu.Dialect("postgres").
			Update("language").
			Set(goqu.Record{
				"english_name": string(language.EnglishName()),
				"native_name":  string(language.NativeName()),
				"is_fallback":  language.IsFallback(),
			}).
			Where(goqu.Ex{"code": code}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build update language %s query: %w", code, buildErr)
		} else {
			tag, execErr := r.db.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("update language %s: %w", code, mapLanguageError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = fmt.Errorf("update language %s: %w", code, languages.ErrLanguageNotFound)
			}
		}
	}

	return err
}

func (r *LanguageRepository) Delete(ctx context.Context, code languages.LanguageCode) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if validationErr := code.Validate(); validationErr != nil {
		err = fmt.Errorf("delete language: %w", validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("language").
			Where(goqu.Ex{"code": string(code)}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build delete language %s query: %w", code, buildErr)
		} else {
			tag, execErr := r.db.Exec(ctx, query, args...)

			if execErr != nil {
				err = fmt.Errorf("delete language %s: %w", code, mapLanguageError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = fmt.Errorf("delete language %s: %w", code, languages.ErrLanguageNotFound)
			}
		}
	}

	return err
}

func (r *LanguageRepository) FindByCode(ctx context.Context, code languages.LanguageCode) (*languages.Language, error) {
	var language *languages.Language
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if validationErr := code.Validate(); validationErr != nil {
		err = fmt.Errorf("find language by code: %w", validationErr)
	} else {
		language, err = r.find(ctx, goqu.Ex{"code": string(code)})

		if err != nil {
			err = fmt.Errorf("find language %s: %w", code, err)
		}
	}

	return language, err
}

// FindFallback returns ErrLanguageNotFound when no fallback is configured.
func (r *LanguageRepository) FindFallback(ctx context.Context) (*languages.Language, error) {
	var language *languages.Language
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else {
		language, err = r.find(ctx, goqu.Ex{"is_fallback": true})

		if err != nil {
			err = fmt.Errorf("find fallback language: %w", err)
		}
	}

	return language, err
}

func (r *LanguageRepository) find(ctx context.Context, filter goqu.Ex) (*languages.Language, error) {
	var language *languages.Language
	var err error
	query, args, buildErr := goqu.Dialect("postgres").
		From("language").
		Select("code", "english_name", "native_name", "is_fallback").
		Where(filter).
		Prepared(true).
		ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("build language lookup query: %w", buildErr)
	} else {
		var storedCode string
		var englishName string
		var nativeName string
		var isFallback bool
		scanErr := r.db.QueryRow(ctx, query, args...).Scan(&storedCode, &englishName, &nativeName, &isFallback)

		if errors.Is(scanErr, pgx.ErrNoRows) {
			err = languages.ErrLanguageNotFound
		} else if scanErr != nil {
			err = fmt.Errorf("scan language: %w", scanErr)
		} else {
			language, err = languages.NewLanguage(
				languages.LanguageCode(storedCode),
				languages.LanguageEnglishName(englishName),
				languages.LanguageNativeName(nativeName),
				isFallback,
			)

			if err != nil {
				err = fmt.Errorf("decode language: %w", err)
			}
		}
	}

	return language, err
}

func mapLanguageError(err error) error {
	var postgresErr *pgconn.PgError
	result := err

	if errors.As(err, &postgresErr) {
		switch {
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "language_is_fallback_true":
			result = errors.Join(languages.ErrFallbackLanguageAlreadyExists, err)
		case postgresErr.Code == "23505" && postgresErr.ConstraintName == "language_pkey":
			result = errors.Join(languages.ErrLanguageAlreadyExists, err)
		case postgresErr.Code == "23514" && postgresErr.ConstraintName == "language_fallback_in_use":
			result = errors.Join(languages.ErrFallbackLanguageAlreadyInUse, err)
		}
	}

	return result
}

func validateLanguage(language *languages.Language) error {
	err := language.Code().Validate()

	if err != nil {
		err = fmt.Errorf("validate language code: %w", err)
	} else if validationErr := language.EnglishName().Validate(); validationErr != nil {
		err = fmt.Errorf("validate language English name: %w", validationErr)
	} else if validationErr := language.NativeName().Validate(); validationErr != nil {
		err = fmt.Errorf("validate language native name: %w", validationErr)
	}

	return err
}
