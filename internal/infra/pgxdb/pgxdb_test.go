package pgxdb

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestIsNilAndIsTransaction(t *testing.T) {
	// The pool never connects.
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")

	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	t.Cleanup(pool.Close)

	tests := []struct {
		name            string
		db              DB
		wantNil, wantTx bool
	}{
		{"nil interface", nil, true, false},
		{"typed nil pool", (*pgxpool.Pool)(nil), true, false},
		{"typed nil connection", (*pgx.Conn)(nil), true, false},
		{"pool", pool, false, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsNil(test.db); got != test.wantNil {
				t.Errorf("IsNil: got %t, want %t", got, test.wantNil)
			}

			if got := IsTransaction(test.db); got != test.wantTx {
				t.Errorf("IsTransaction: got %t, want %t", got, test.wantTx)
			}
		})
	}
}
