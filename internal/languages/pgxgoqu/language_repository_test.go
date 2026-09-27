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
	queryContext context.Context
	query        string
	queryArgs    []any
	row          pgx.Row
}

func (f *fakeLanguageDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	f.execContext = ctx
	f.execQuery = query
	f.execArgs = args
	return pgconn.CommandTag{}, f.execErr
}

func (f *fakeLanguageDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	f.queryContext = ctx
	f.query = query
	f.queryArgs = args
	return f.row
}

type fakeLanguageRow struct {
	values []string
	err    error
}

func (r fakeLanguageRow) Scan(destinations ...any) error {
	err := r.err

	if err == nil {
		for index, value := range r.values {
			*destinations[index].(*string) = value
		}
	}

	return err
}

func testLanguage(t *testing.T) *languages.Language {
	t.Helper()
	language, err := languages.NewLanguage("en", "English", "English")

	if err != nil {
		t.Fatalf("NewLanguage() error = %v", err)
	}

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

func TestLanguageRepositorySave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tt := range []struct {
		name     string
		language *languages.Language
		execErr  error
		wantErr  error
		wantExec bool
	}{
		{name: "upsert", language: testLanguage(t), wantExec: true},
		{name: "nil language", wantErr: ErrNilLanguage},
		{name: "invalid language", language: &languages.Language{}, wantErr: languages.ErrInvalidLanguageCode},
		{name: "database error", language: testLanguage(t), execErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantExec: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeLanguageDB{execErr: tt.execErr}
			repository := &LanguageRepository{db: db}
			err := repository.Save(ctx, tt.language)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Save() error = %v, want %v", err, tt.wantErr)
			}

			if (db.execQuery != "") != tt.wantExec {
				t.Fatalf("Save() executed query = %v, want %v", db.execQuery != "", tt.wantExec)
			}

			if tt.wantExec {
				if db.execContext != ctx {
					t.Fatal("Save() did not forward the context")
				}

				if !strings.Contains(db.execQuery, `INSERT INTO "language"`) || !strings.Contains(db.execQuery, "ON CONFLICT") || !strings.Contains(db.execQuery, "$1") || strings.Contains(db.execQuery, "English") {
					t.Fatalf("Save() query = %q, want parameterized PostgreSQL upsert", db.execQuery)
				}

				if len(db.execArgs) != 5 {
					t.Fatalf("Save() args = %v, want five bound values", db.execArgs)
				}
			}
		})
	}
}

func TestLanguageRepositoryFindByCode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tt := range []struct {
		name      string
		code      languages.LanguageCode
		row       pgx.Row
		wantErr   error
		wantQuery bool
	}{
		{name: "found", code: "en", row: fakeLanguageRow{values: []string{"en", "English", "English"}}, wantQuery: true},
		{name: "not found", code: "lv", row: fakeLanguageRow{err: pgx.ErrNoRows}, wantErr: languages.ErrLanguageNotFound, wantQuery: true},
		{name: "invalid code", code: "EN", wantErr: languages.ErrInvalidLanguageCode},
		{name: "scan error", code: "en", row: fakeLanguageRow{err: context.DeadlineExceeded}, wantErr: context.DeadlineExceeded, wantQuery: true},
		{name: "invalid stored name", code: "en", row: fakeLanguageRow{values: []string{"en", "", "English"}}, wantErr: languages.ErrInvalidLanguageEnglishName, wantQuery: true},
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
				if language == nil || language.Code() != "en" || language.EnglishName() != "English" || language.NativeName() != "English" {
					t.Fatalf("FindByCode() language = %v, want English", language)
				}
			} else if language != nil {
				t.Fatalf("FindByCode() language = %v, want nil on error", language)
			}
		})
	}
}
