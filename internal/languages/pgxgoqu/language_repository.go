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

func (r *LanguageRepository) Save(ctx context.Context, language *languages.Language) error {
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if language == nil {
		err = ErrNilLanguage
	} else {
		err = validateLanguage(language)

		if err == nil {
			code := string(language.Code())
			englishName := string(language.EnglishName())
			nativeName := string(language.NativeName())
			query, args, buildErr := goqu.Dialect("postgres").
				Insert("language").
				Rows(goqu.Record{
					"code":         code,
					"english_name": englishName,
					"native_name":  nativeName,
				}).
				OnConflict(goqu.DoUpdate("code", goqu.Record{
					"english_name": englishName,
					"native_name":  nativeName,
				})).
				Prepared(true).
				ToSQL()

			if buildErr != nil {
				err = fmt.Errorf("build save language %s query: %w", code, buildErr)
			} else {
				_, execErr := r.db.Exec(ctx, query, args...)

				if execErr != nil {
					err = fmt.Errorf("save language %s: %w", code, execErr)
				}
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
		query, args, buildErr := goqu.Dialect("postgres").
			From("language").
			Select("code", "english_name", "native_name").
			Where(goqu.Ex{"code": string(code)}).
			Prepared(true).
			ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build find language %s query: %w", code, buildErr)
		} else {
			var storedCode string
			var englishName string
			var nativeName string
			scanErr := r.db.QueryRow(ctx, query, args...).Scan(&storedCode, &englishName, &nativeName)

			if errors.Is(scanErr, pgx.ErrNoRows) {
				err = fmt.Errorf("find language %s: %w", code, languages.ErrLanguageNotFound)
			} else if scanErr != nil {
				err = fmt.Errorf("find language %s: %w", code, scanErr)
			} else {
				language, err = languages.NewLanguage(
					languages.LanguageCode(storedCode),
					languages.LanguageEnglishName(englishName),
					languages.LanguageNativeName(nativeName),
				)

				if err != nil {
					err = fmt.Errorf("decode language %s: %w", code, err)
				}
			}
		}
	}

	return language, err
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
