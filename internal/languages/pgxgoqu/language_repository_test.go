package pgxgoqu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/radynsade/faryengo/internal/languages"
)

type fakeLanguageDB struct {
	execContext  context.Context
	execQuery    string
	execArgs     []any
	execErr      error
	execTag      pgconn.CommandTag
	queryContext context.Context
	query        string
	queryArgs    []any
	row          pgx.Row
}

func (f *fakeLanguageDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	f.execContext = ctx
	f.execQuery = query
	f.execArgs = args
	return f.execTag, f.execErr
}

func (f *fakeLanguageDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	f.queryContext = ctx
	f.query = query
	f.queryArgs = args
	return f.row
}

type fakeLanguageRow struct {
	values []any
	err    error
}

func (r fakeLanguageRow) Scan(destinations ...any) error {
	err := r.err

	if err == nil {
		for index, value := range r.values {
			switch destination := destinations[index].(type) {
			case *string:
				*destination = value.(string)
			case *bool:
				*destination = value.(bool)
			}
		}
	}

	return err
}

func testLanguage(t *testing.T) *languages.Language {
	t.Helper()
	language, err := languages.NewLanguage("en", "English", "English", false)

	if err != nil {
		t.Fatalf("NewLanguage() error = %v", err)
	}

	return language
}

func testFallbackLanguage(t *testing.T) *languages.Language {
	t.Helper()
	language := testLanguage(t)
	language.SetIsFallback(true)
	return language
}

func TestNewLanguageRepository(t *testing.T) {
	for _, tt := range []struct {
		name    string
		wantErr error
	}{
		{name: "nil pool", wantErr: ErrNilPool},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository, err := NewLanguageRepository(nil)

			if repository != nil || !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewLanguageRepository(nil) = (%v, %v), want nil and %v", repository, err, tt.wantErr)
			}
		})
	}
}

func TestLanguageRepositoryCreate(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name     string
		language *languages.Language
		tag      pgconn.CommandTag
		execErr  error
		wantErr  error
		wantExec bool
	}{
		{name: "created", language: testLanguage(t), tag: pgconn.NewCommandTag("INSERT 0 1"), wantExec: true},
		{name: "duplicate", language: testLanguage(t), execErr: &pgconn.PgError{Code: "23505", ConstraintName: "language_pkey"}, wantErr: languages.ErrLanguageAlreadyExists, wantExec: true},
		{name: "fallback created", language: testFallbackLanguage(t), tag: pgconn.NewCommandTag("INSERT 0 1"), wantExec: true},
		{name: "fallback conflict", language: testFallbackLanguage(t), execErr: &pgconn.PgError{Code: "23505", ConstraintName: "language_is_fallback_true"}, wantErr: languages.ErrFallbackLanguageAlreadyExists, wantExec: true},
		{name: "nil language", wantErr: ErrNilLanguage},
		{name: "invalid language", language: &languages.Language{}, wantErr: languages.ErrInvalidLanguageCode},
		{name: "database error", language: testLanguage(t), execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantExec: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeLanguageDB{execTag: tt.tag, execErr: tt.execErr}
			repository := &LanguageRepository{db: db}
			err := repository.Create(ctx, tt.language)
			if !errors.Is(err, tt.wantErr) || (db.execQuery != "") != tt.wantExec {
				t.Fatalf("Create() error = %v, query = %q; want error %v and exec %v", err, db.execQuery, tt.wantErr, tt.wantExec)
			}

			if tt.wantExec {
				if db.execContext != ctx || !strings.Contains(db.execQuery, `INSERT INTO "language"`) || strings.Contains(db.execQuery, "ON CONFLICT") || strings.Contains(db.execQuery, "English") {
					t.Fatalf("Create() query = %q, context = %v", db.execQuery, db.execContext)
				}

				if len(db.execArgs) != 4 || db.execArgs[0] != "en" || db.execArgs[1] != "English" || db.execArgs[2] != tt.language.IsFallback() || db.execArgs[3] != "English" {
					t.Fatalf("Create() args = %v, want bound language fields", db.execArgs)
				}
			}
		})
	}
}

func TestLanguageRepositoryUpdate(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name     string
		language *languages.Language
		tag      pgconn.CommandTag
		execErr  error
		wantErr  error
		wantExec bool
	}{
		{name: "updated", language: testLanguage(t), tag: pgconn.NewCommandTag("UPDATE 1"), wantExec: true},
		{name: "fallback updated", language: testFallbackLanguage(t), tag: pgconn.NewCommandTag("UPDATE 1"), wantExec: true},
		{name: "second fallback", language: testFallbackLanguage(t), execErr: &pgconn.PgError{Code: "23505", ConstraintName: "language_is_fallback_true"}, wantErr: languages.ErrFallbackLanguageAlreadyExists, wantExec: true},
		{name: "fallback in use", language: testLanguage(t), execErr: &pgconn.PgError{Code: "23514", ConstraintName: "language_fallback_in_use"}, wantErr: languages.ErrFallbackLanguageAlreadyInUse, wantExec: true},
		{name: "not found", language: testLanguage(t), tag: pgconn.NewCommandTag("UPDATE 0"), wantErr: languages.ErrLanguageNotFound, wantExec: true},
		{name: "nil language", wantErr: ErrNilLanguage},
		{name: "invalid language", language: &languages.Language{}, wantErr: languages.ErrInvalidLanguageCode},
		{name: "database error", language: testLanguage(t), execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantExec: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeLanguageDB{execTag: tt.tag, execErr: tt.execErr}
			repository := &LanguageRepository{db: db}
			err := repository.Update(ctx, tt.language)
			if !errors.Is(err, tt.wantErr) || (db.execQuery != "") != tt.wantExec {
				t.Fatalf("Update() error = %v, query = %q; want error %v and exec %v", err, db.execQuery, tt.wantErr, tt.wantExec)
			}

			if tt.wantExec {
				if db.execContext != ctx || !strings.Contains(db.execQuery, `UPDATE "language"`) || !strings.Contains(db.execQuery, `WHERE ("code" = $4)`) || strings.Contains(db.execQuery, "English") {
					t.Fatalf("Update() query = %q, context = %v", db.execQuery, db.execContext)
				}

				if len(db.execArgs) != 4 || db.execArgs[0] != "English" || db.execArgs[1] != tt.language.IsFallback() || db.execArgs[2] != "English" || db.execArgs[3] != "en" {
					t.Fatalf("Update() args = %v, want bound language fields and code", db.execArgs)
				}
			}
		})
	}
}

func TestLanguageRepositoryDelete(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name     string
		code     languages.LanguageCode
		tag      pgconn.CommandTag
		execErr  error
		wantErr  error
		wantExec bool
	}{
		{name: "deleted", code: "lv", tag: pgconn.NewCommandTag("DELETE 1"), wantExec: true},
		{name: "fallback in use", code: "lv", execErr: &pgconn.PgError{Code: "23514", ConstraintName: "language_fallback_in_use"}, wantErr: languages.ErrFallbackLanguageAlreadyInUse, wantExec: true},
		{name: "not found", code: "lv", tag: pgconn.NewCommandTag("DELETE 0"), wantErr: languages.ErrLanguageNotFound, wantExec: true},
		{name: "invalid code", code: "LV", wantErr: languages.ErrInvalidLanguageCode},
		{name: "database error", code: "lv", execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantExec: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeLanguageDB{execTag: tt.tag, execErr: tt.execErr}
			repository := &LanguageRepository{db: db}
			err := repository.Delete(ctx, tt.code)
			if !errors.Is(err, tt.wantErr) || (db.execQuery != "") != tt.wantExec {
				t.Fatalf("Delete() error = %v, query = %q; want %v and exec %v", err, db.execQuery, tt.wantErr, tt.wantExec)
			}

			if tt.wantExec && (db.execContext != ctx || !strings.Contains(db.execQuery, `DELETE FROM "language"`) || !strings.Contains(db.execQuery, `$1`) || len(db.execArgs) != 1 || db.execArgs[0] != "lv") {
				t.Fatalf("Delete() query = %q, args = %v, context = %v", db.execQuery, db.execArgs, db.execContext)
			}
		})
	}

	var nilRepository *LanguageRepository
	if err := nilRepository.Delete(ctx, "lv"); !errors.Is(err, ErrNilPool) {
		t.Fatalf("Delete(nil receiver) error = %v, want ErrNilPool", err)
	}
}

func TestLanguageRepositoryFindByCode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tt := range []struct {
		name         string
		code         languages.LanguageCode
		row          pgx.Row
		wantErr      error
		wantQuery    bool
		wantFallback bool
	}{
		{name: "found", code: "en", row: fakeLanguageRow{values: []any{"en", "English", "English", false}}, wantQuery: true},
		{name: "found fallback", code: "en", row: fakeLanguageRow{values: []any{"en", "English", "English", true}}, wantQuery: true, wantFallback: true},
		{name: "not found", code: "lv", row: fakeLanguageRow{err: pgx.ErrNoRows}, wantErr: languages.ErrLanguageNotFound, wantQuery: true},
		{name: "invalid code", code: "EN", wantErr: languages.ErrInvalidLanguageCode},
		{name: "scan error", code: "en", row: fakeLanguageRow{err: context.DeadlineExceeded}, wantErr: context.DeadlineExceeded, wantQuery: true},
		{name: "invalid stored name", code: "en", row: fakeLanguageRow{values: []any{"en", "", "English", false}}, wantErr: languages.ErrInvalidLanguageEnglishName, wantQuery: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeLanguageDB{row: tt.row}
			repository := &LanguageRepository{db: db}
			language, err := repository.FindByCode(ctx, tt.code)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FindByCode() error = %v, want %v", err, tt.wantErr)
			}

			if (db.query != "") != tt.wantQuery {
				t.Fatalf("FindByCode() queried = %v, want %v", db.query != "", tt.wantQuery)
			}

			if tt.wantQuery {
				if db.queryContext != ctx {
					t.Fatal("FindByCode() did not forward the context")
				}

				if !strings.Contains(db.query, `FROM "language"`) || !strings.Contains(db.query, "$1") || len(db.queryArgs) != 1 || db.queryArgs[0] != string(tt.code) {
					t.Fatalf("FindByCode() query = %q, args = %v, want parameterized lookup", db.query, db.queryArgs)
				}
			}

			if tt.wantErr == nil {
				if language == nil || language.Code() != "en" || language.EnglishName() != "English" || language.NativeName() != "English" || language.IsFallback() != tt.wantFallback {
					t.Fatalf("FindByCode() language = %v, want English", language)
				}
			} else if language != nil {
				t.Fatalf("FindByCode() language = %v, want nil on error", language)
			}
		})
	}
}

func TestLanguageRepositoryFindFallback(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name    string
		row     pgx.Row
		wantErr error
	}{
		{name: "found", row: fakeLanguageRow{values: []any{"en", "English", "English", true}}},
		{name: "not configured", row: fakeLanguageRow{err: pgx.ErrNoRows}, wantErr: languages.ErrLanguageNotFound},
		{name: "scan error", row: fakeLanguageRow{err: context.DeadlineExceeded}, wantErr: context.DeadlineExceeded},
		{name: "invalid stored language", row: fakeLanguageRow{values: []any{"en", "", "English", true}}, wantErr: languages.ErrInvalidLanguageEnglishName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeLanguageDB{row: tt.row}
			repository := &LanguageRepository{db: db}
			language, err := repository.FindFallback(ctx)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FindFallback() error = %v, want %v", err, tt.wantErr)
			}

			if db.queryContext != ctx || !strings.Contains(db.query, `WHERE ("is_fallback" IS TRUE)`) {
				t.Fatalf("FindFallback() query = %q, context = %v", db.query, db.queryContext)
			}

			if tt.wantErr == nil {
				if language == nil || language.Code() != "en" || !language.IsFallback() {
					t.Fatalf("FindFallback() language = %v, want English fallback", language)
				}
			} else if language != nil {
				t.Fatalf("FindFallback() language = %v, want nil on error", language)
			}
		})
	}

	var repository *LanguageRepository
	if language, err := repository.FindFallback(ctx); language != nil || !errors.Is(err, ErrNilPool) {
		t.Fatalf("FindFallback(nil receiver) = (%v, %v), want nil and ErrNilPool", language, err)
	}
}
