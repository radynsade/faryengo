package pgxgoqu

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

//
// Errors
//

var ErrNilPool = errors.New("nil PostgreSQL pool")

//
// Database contracts
//

type securityDB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type roleDB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

//
// Helpers
//

func bindUUIDArgs(args []any, ids ...[16]byte) error {
	return bindUUIDArgsAt(args, 0, ids...)
}

func bindUUIDArgsAt(args []any, start int, ids ...[16]byte) error {
	if start < 0 || len(args)-start < len(ids) {
		return fmt.Errorf("bind UUID arguments: got %d, need %d", len(args), len(ids))
	}

	for index, id := range ids {
		args[start+index] = pgtype.UUID{Bytes: id, Valid: true}
	}

	return nil
}
