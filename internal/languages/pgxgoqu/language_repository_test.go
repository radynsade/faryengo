package pgxgoqu

import (
	"errors"
	"testing"

	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/languages"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func testLanguage(t *testing.T) *languages.Language {
	t.Helper()
	language, err := languages.NewLanguage("en", "English", "English", false)

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
