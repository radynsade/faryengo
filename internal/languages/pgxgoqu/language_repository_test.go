package pgxgoqu

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/languages"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func testLanguage(t *testing.T) *languages.Language {
	t.Helper()

	return languages.NewLanguage("en", "English", "English", false)
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

func TestLanguageRepositoryMissingDatabase(t *testing.T) {
	for _, tt := range []struct {
		name       string
		repository *LanguageRepository
	}{
		{name: "nil receiver"},
		{name: "zero repository", repository: &LanguageRepository{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			language := testLanguage(t)

			for _, operation := range []struct {
				name string
				run  func() error
			}{
				{name: "create", run: func() error { return tt.repository.Create(t.Context(), language) }},
				{name: "update", run: func() error { return tt.repository.Update(t.Context(), language) }},
				{name: "delete", run: func() error { return tt.repository.Delete(t.Context(), "en") }},
				{name: "find by code", run: func() error {
					_, err := tt.repository.FindByCode(t.Context(), "en")

					return err
				}},
				{name: "find by code for update", run: func() error {
					_, err := tt.repository.FindByCodeForUpdate(t.Context(), "en")

					return err
				}},
				{name: "find fallback", run: func() error {
					_, err := tt.repository.FindFallback(t.Context())

					return err
				}},
				{name: "find all", run: func() error {
					_, err := tt.repository.FindAll(t.Context())

					return err
				}},
			} {
				t.Run(operation.name, func(t *testing.T) {
					if err := operation.run(); !errors.Is(err, ErrNilPool) {
						t.Fatalf("error = %v, want ErrNilPool", err)
					}
				})
			}
		})
	}
}

type fakeLanguageDB struct {
	languageDB
	queryContext context.Context
	query        string
	args         []any
	row          pgx.Row
}

func (f *fakeLanguageDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	f.queryContext = ctx
	f.query = query
	f.args = args

	return f.row
}

type fakeLanguageTx struct {
	pgx.Tx
	db *fakeLanguageDB
}

func (f *fakeLanguageTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return f.db.QueryRow(ctx, query, args...)
}

type fakeLanguageRow struct {
	language *languages.Language
	err      error
}

func (r fakeLanguageRow) Scan(destinations ...any) error {
	if r.err == nil {
		*destinations[0].(*string) = string(r.language.Code)
		*destinations[1].(*string) = string(r.language.EnglishName)
		*destinations[2].(*string) = string(r.language.NativeName)
		*destinations[3].(*bool) = r.language.IsFallback
	}

	return r.err
}

func TestLanguageRepositoryWithTx(t *testing.T) {
	db := &fakeLanguageDB{}
	base := &LanguageRepository{pool: db}
	tx := &fakeLanguageTx{db: db}

	for _, tt := range []struct {
		name       string
		repository *LanguageRepository
		tx         pgx.Tx
		wantErr    error
	}{
		{name: "bound transaction", repository: base, tx: tx},
		{name: "nil transaction", repository: base, wantErr: ErrNilTransaction},
		{name: "nil receiver", tx: tx, wantErr: ErrNilPool},
		{name: "zero repository", repository: &LanguageRepository{}, tx: tx, wantErr: ErrNilPool},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository, err := tt.repository.WithTx(tt.tx)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("WithTx() error = %v, want %v", err, tt.wantErr)
			}

			if tt.wantErr == nil {
				if repository == nil || repository == base || repository.pool != tx || base.pool != db {
					t.Fatal("WithTx() must bind a new repository without changing the original")
				}
			} else if repository != nil {
				t.Fatal("WithTx() returned a repository on error")
			}
		})
	}
}

func TestLanguageRepositoryFindByCodeForUpdate(t *testing.T) {
	for _, tt := range []struct {
		name             string
		code             languages.Code
		stored           *languages.Language
		rowErr           error
		nontransactional bool
		wantErr          error
		wantQuery        bool
	}{
		{name: "locked language", code: "en", stored: testLanguage(t), wantQuery: true},
		{name: "missing language", code: "en", rowErr: pgx.ErrNoRows, wantErr: languages.ErrLanguageNotFound, wantQuery: true},
		{name: "database error", code: "en", rowErr: context.Canceled, wantErr: context.Canceled, wantQuery: true},
		{name: "closed transaction", code: "en", rowErr: pgx.ErrTxClosed, wantErr: pgx.ErrTxClosed, wantQuery: true},
		{name: "invalid stored language", code: "en", stored: languages.NewLanguage("en", "", "English", false), wantErr: languages.ErrInvalidEnglishName, wantQuery: true},
		{name: "invalid code", code: "EN", wantErr: languages.ErrInvalidCode},
		{name: "empty code", wantErr: languages.ErrInvalidCode},
		{name: "transaction required", code: "en", nontransactional: true, wantErr: ErrTransactionRequired},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeLanguageDB{row: fakeLanguageRow{language: tt.stored, err: tt.rowErr}}
			repository := &LanguageRepository{pool: &fakeLanguageTx{db: db}}

			if tt.nontransactional {
				repository.pool = db
			}

			language, err := repository.FindByCodeForUpdate(t.Context(), tt.code)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FindByCodeForUpdate() error = %v, want %v", err, tt.wantErr)
			}

			if tt.wantErr == nil {
				if language == nil || *language != *tt.stored {
					t.Fatalf("FindByCodeForUpdate() = %v, want %v", language, tt.stored)
				}
			} else if language != nil {
				t.Fatal("FindByCodeForUpdate() returned a language on error")
			}

			if tt.wantQuery {
				if strings.TrimSpace(db.query) != `SELECT "code", "english_name", "native_name", "is_fallback" FROM "language" WHERE ("code" = $1) FOR UPDATE` ||
					!reflect.DeepEqual(db.args, []any{string(tt.code)}) || db.queryContext != t.Context() {
					t.Fatalf("lookup = %q, %v, context %v", db.query, db.args, db.queryContext)
				}
			} else if db.query != "" {
				t.Fatal("invalid lookup executed a database query")
			}
		})
	}
}

func TestLanguageRepositoryUnlockedLookups(t *testing.T) {
	for _, tt := range []struct {
		name string
		find func(*LanguageRepository, context.Context) (*languages.Language, error)
	}{
		{name: "by code", find: func(r *LanguageRepository, ctx context.Context) (*languages.Language, error) {
			return r.FindByCode(ctx, "en")
		}},
		{name: "fallback", find: (*LanguageRepository).FindFallback},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeLanguageDB{row: fakeLanguageRow{language: testLanguage(t)}}
			repository := &LanguageRepository{pool: &fakeLanguageTx{db: db}}
			_, err := tt.find(repository, t.Context())

			if err != nil || strings.Contains(db.query, "FOR UPDATE") {
				t.Fatalf("unlocked lookup = %q, %v", db.query, err)
			}
		})
	}
}
