package pgxdb

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//
// Errors
//

var (
	ErrNilDB               = errors.New("nil PostgreSQL database")
	ErrTransactionRequired = errors.New("row locking requires a PostgreSQL transaction")
)

//
// Database
//

// DB is satisfied by *pgxpool.Pool, *pgx.Conn, and pgx.Tx, so one repository
// works with any of them. Begin on a pgx.Tx opens a savepoint, so multi-step
// writes nest inside a caller's transaction.

type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// IsNil also detects typed nil pointers, which are not equal to nil once
// stored in an interface.

func IsNil(db DB) bool {
	var isNil bool

	switch typed := db.(type) {
	case nil:
		isNil = true
	case *pgxpool.Pool:
		isNil = typed == nil
	case *pgx.Conn:
		isNil = typed == nil
	}

	return isNil
}

// IsTransaction reports whether row locks taken through db outlive a single
// statement.

func IsTransaction(db DB) bool {
	_, ok := db.(pgx.Tx)

	return ok
}
