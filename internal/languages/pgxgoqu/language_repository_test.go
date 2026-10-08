package pgxgoqu

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/infra/pgxdb"
	"github.com/radynsade/faryengo/internal/languages"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestNewLanguageRepositoryRejectsNilDB(t *testing.T) {
	tests := []struct {
		name string
		pool pgxdb.DB
	}{
		{"nil interface", nil},
		{"typed nil pool", (*pgxpool.Pool)(nil)},
		{"typed nil connection", (*pgx.Conn)(nil)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, err := NewLanguageRepository(test.pool)

			if !errors.Is(err, pgxdb.ErrNilDB) || repository != nil {
				t.Fatalf("got %v, %v; want nil, %v", repository, err, pgxdb.ErrNilDB)
			}
		})
	}
}

// The pool never connects: every case must fail before any I/O.
func TestFindByCodeForUpdateRequiresTransaction(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")

	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	t.Cleanup(pool.Close)

	repository, err := NewLanguageRepository(pool)

	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	tests := []struct {
		name string
		code languages.Code
		want error
	}{
		{"invalid code", "EN", languages.ErrCodeInvalid},
		{"pool instead of a transaction", "en", pgxdb.ErrTransactionRequired},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			language, findErr := repository.FindByCodeForUpdate(context.Background(), test.code)

			if !errors.Is(findErr, test.want) || language != nil {
				t.Fatalf("got %v, %v; want nil, %v", language, findErr, test.want)
			}
		})
	}
}
