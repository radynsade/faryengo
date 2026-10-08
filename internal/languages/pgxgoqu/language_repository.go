package pgxgoqu

import (
	"context"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/internal/languages"
)

//
// Errors
//

var (
	ErrNilPool             = errors.New("nil PostgreSQL pool")
	ErrTransactionRequired = errors.New("language locking requires a PostgreSQL transaction")
)

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

type errLanguageCreateFailed struct {
	errLanguageWriteFailed
}

func newErrLanguageCreateFailed(language *languages.Language, err error) *errLanguageCreateFailed {
	return &errLanguageCreateFailed{
		errLanguageWriteFailed: errLanguageWriteFailed{language, err},
	}
}

func (e *errLanguageCreateFailed) Error() string {
	return "failed to create a language"
}

// Implementation of languages.ErrLanguageUpdateFailed

type errLanguageUpdateFailed struct {
	errLanguageWriteFailed
}

func newErrLanguageUpdateFailed(language *languages.Language, err error) *errLanguageUpdateFailed {
	return &errLanguageUpdateFailed{
		errLanguageWriteFailed: errLanguageWriteFailed{language, err},
	}
}

func (e *errLanguageUpdateFailed) Error() string {
	return "failed to update a language"
}

// Implementation of languages.ErrLanguageDeleteFailed

type errLanguageDeleteFailed struct {
	code languages.Code
	err  error
}

func newErrLanguageDeleteFailed(code languages.Code, err error) *errLanguageDeleteFailed {
	return &errLanguageDeleteFailed{code, err}
}

func (e errLanguageDeleteFailed) LanguageCode() languages.Code {
	return e.code
}

func (e errLanguageDeleteFailed) Unwrap() error {
	return e.err
}

func (e *errLanguageDeleteFailed) Error() string {
	return "failed to delete a language"
}

//
// Repository
//

type LanguageRepository struct {
	pool *pgxpool.Pool
}

var _ languages.LanguageRepository = (*LanguageRepository)(nil)

func NewLanguageRepository(pool *pgxpool.Pool) (*LanguageRepository, error) {
	var (
		repository *LanguageRepository
		err        error
	)

	if pool == nil {
		err = ErrNilPool
	} else {
		repository = &LanguageRepository{pool: pool}
	}

	return repository, err
}

// Create

func (r *LanguageRepository) Create(
	ctx context.Context,
	language *languages.Language,
) languages.ErrLanguageCreateFailed {
	var err languages.ErrLanguageCreateFailed

	if r == nil || r.pool == nil {
		err = newErrLanguageCreateFailed(language, ErrNilPool)
	} else if language == nil {
		err = newErrLanguageCreateFailed(language, languages.ErrLanguageNil)
	} else if validationErr := language.Validate(); validationErr != nil {
		err = newErrLanguageCreateFailed(language, validationErr)
	} else {
		code := string(language.Code)

		query, args, buildErr := goqu.Dialect("postgres").
			Insert("language").
			Rows(goqu.Record{
				"code":         code,
				"english_name": string(language.EnglishName),
				"native_name":  string(language.NativeName),
				"is_fallback":  language.IsFallback,
			}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrLanguageCreateFailed(language, buildErr)
		} else {
			_, execErr := r.pool.Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrLanguageCreateFailed(language, mapLanguageError(execErr))
			}
		}
	}

	return err
}

// Update

func (r *LanguageRepository) Update(
	ctx context.Context,
	language *languages.Language,
) languages.ErrLanguageUpdateFailed {
	var err languages.ErrLanguageUpdateFailed

	if r == nil || r.pool == nil {
		err = newErrLanguageUpdateFailed(language, ErrNilPool)
	} else if language == nil {
		err = newErrLanguageUpdateFailed(language, languages.ErrLanguageNil)
	} else if validationErr := language.Validate(); validationErr != nil {
		err = newErrLanguageUpdateFailed(language, validationErr)
	} else {
		code := string(language.Code)

		query, args, buildErr := goqu.Dialect("postgres").
			Update("language").
			Set(goqu.Record{
				"english_name": string(language.EnglishName),
				"native_name":  string(language.NativeName),
				"is_fallback":  language.IsFallback,
			}).
			Where(goqu.Ex{"code": code}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrLanguageUpdateFailed(language, buildErr)
		} else {
			tag, execErr := r.pool.Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrLanguageUpdateFailed(language, mapLanguageError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrLanguageUpdateFailed(language, languages.ErrLanguageNotFound)
			}
		}
	}

	return err
}

// Delete

func (r *LanguageRepository) Delete(
	ctx context.Context,
	code languages.Code,
) languages.ErrLanguageDeleteFailed {
	var err languages.ErrLanguageDeleteFailed

	if r == nil || r.pool == nil {
		err = newErrLanguageDeleteFailed(code, ErrNilPool)
	} else if validationErr := code.Validate(); validationErr != nil {
		err = newErrLanguageDeleteFailed(code, validationErr)
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			Delete("language").
			Where(goqu.Ex{"code": string(code)}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = newErrLanguageDeleteFailed(code, buildErr)
		} else {
			tag, execErr := r.pool.Exec(ctx, query, args...)

			if execErr != nil {
				err = newErrLanguageDeleteFailed(code, mapLanguageError(execErr))
			} else if tag.RowsAffected() == 0 {
				err = newErrLanguageDeleteFailed(code, languages.ErrLanguageNotFound)
			}
		}
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
		err      error
	)

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else if validationErr := code.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a language by code: %w", validationErr)
	} else {
		language, err = r.find(ctx, goqu.Ex{"code": string(code)}, false)

		if err != nil {
			err = fmt.Errorf("failed to find a language %s: %w", code, err)
		}
	}

	return language, err
}

// Find by a code with a pessimistic row lock

func (r *LanguageRepository) FindByCodeForUpdate(
	ctx context.Context,
	code languages.Code,
) (*languages.Language, error) {
	var (
		language *languages.Language
		err      error
	)

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else if validationErr := code.Validate(); validationErr != nil {
		err = fmt.Errorf("failed to find a language by code for update: %w", validationErr)
	} else {
		err = ErrTransactionRequired
	}

	return language, err
}

// Find the fallback language

func (r *LanguageRepository) FindFallback(ctx context.Context) (*languages.Language, error) {
	var (
		language *languages.Language
		err      error
	)

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else {
		language, err = r.find(ctx, goqu.Ex{"is_fallback": true}, false)

		if err != nil {
			err = fmt.Errorf("failed to find the fallback language: %w", err)
		}
	}

	return language, err
}

// Find all languages ordered by a code

func (r *LanguageRepository) FindAll(ctx context.Context) ([]*languages.Language, error) {
	var (
		result []*languages.Language
		err    error
	)

	if r == nil || r.pool == nil {
		err = ErrNilPool
	} else {
		query, args, buildErr := goqu.Dialect("postgres").
			From("language").
			Select("code", "english_name", "native_name", "is_fallback").
			Order(goqu.C("code").Asc()).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("failed to build the list languages query: %w", buildErr)
		} else {
			rows, queryErr := r.pool.Query(ctx, query, args...)

			if queryErr != nil {
				err = fmt.Errorf("failed to list languages: %w", queryErr)
			} else {
				defer rows.Close()

				result = make([]*languages.Language, 0)

				for rows.Next() {
					var language *languages.Language
					language, err = scanLanguage(rows)

					if err != nil {
						break
					}

					result = append(result, language)
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("failed to read languages: %w", err)
					result = nil
				}
			}
		}
	}

	return result, err
}

//
// Helpers
//

func (r *LanguageRepository) find(
	ctx context.Context,
	filter goqu.Ex,
	forUpdate bool,
) (*languages.Language, error) {
	var (
		language *languages.Language
		err      error
	)

	dataset := goqu.Dialect("postgres").
		From("language").
		Select("code", "english_name", "native_name", "is_fallback").
		Where(filter).
		Prepared(true)

	if forUpdate {
		dataset = dataset.ForUpdate(exp.Wait)
	}

	query, args, buildErr := dataset.ToSQL()

	if buildErr != nil {
		err = fmt.Errorf("failed to build the language lookup query: %w", buildErr)
	} else {
		language, err = scanLanguage(r.pool.QueryRow(ctx, query, args...))
	}

	return language, err
}

func scanLanguage(row pgx.Row) (*languages.Language, error) {
	var (
		language              *languages.Language
		err                   error
		code, english, native string
		fallback              bool
	)

	scanErr := row.Scan(&code, &english, &native, &fallback)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = languages.ErrLanguageNotFound
	} else if scanErr != nil {
		err = fmt.Errorf("scan language: %w", scanErr)
	} else {
		language = languages.NewLanguage(
			languages.Code(code),
			languages.EnglishName(english),
			languages.NativeName(native),
			fallback,
		)

		err = language.Validate()

		if err != nil {
			err = fmt.Errorf("failed to decode a language: %w", err)
			language = nil
		}
	}

	return language, err
}

func mapLanguageError(err error) error {
	result := err

	if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
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
