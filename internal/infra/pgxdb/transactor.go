package pgxdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

//
// Errors
//

var ErrNilTransactionFunc = errors.New("nil transaction function")

//
// Transactor
//

const rollbackTimeout = 5 * time.Second

type transactionKey struct{}

// The owner is the handle the transaction was opened on, so a repository built
// on another handle never runs inside it.

type transaction struct {
	owner DB
	tx    pgx.Tx
}

type Transactor struct {
	pool DB
}

func NewTransactor(pool DB) (*Transactor, error) {
	var (
		transactor *Transactor
		err        error
	)

	if IsNil(pool) {
		err = ErrNilDB
	} else {
		transactor = &Transactor{pool: pool}
	}

	return transactor, err
}

// Run a function in a transaction

func (t *Transactor) InTransaction(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	var err error

	if t == nil {
		err = ErrNilDB
	} else {
		err = InTransaction(ctx, t.pool, fn)
	}

	return err
}

// InTransaction runs fn in a transaction opened on db, or in a savepoint when
// ctx already carries a transaction opened on db. A failed fn keeps its error
// and gains any rollback failure. A panicking fn is rolled back and keeps
// panicking with its original value; that rollback's failure is discarded.

func InTransaction(
	ctx context.Context,
	db DB,
	fn func(ctx context.Context) error,
) error {
	var err error

	if IsNil(db) {
		err = ErrNilDB
	} else if fn == nil {
		err = ErrNilTransactionFunc
	} else if tx, beginErr := FromContext(ctx, db).Begin(ctx); beginErr != nil {
		err = fmt.Errorf("begin a transaction: %w", beginErr)
	} else {
		err = runInTransaction(ctx, transaction{owner: db, tx: tx}, fn)
	}

	return err
}

// FromContext returns the transaction that ctx carries for db, or db itself
// when there is none. Repositories resolve their handle through it on every
// operation so that they join the caller's transaction.

func FromContext(ctx context.Context, db DB) DB {
	result := db

	if current, ok := ctx.Value(transactionKey{}).(transaction); ok && current.owner == db {
		result = current.tx
	}

	return result
}

//
// Helpers
//

// The deferred rollback runs whenever fn does not return normally, which
// covers both panics and runtime.Goexit.

func runInTransaction(
	ctx context.Context,
	current transaction,
	fn func(ctx context.Context) error,
) error {
	var (
		err        error
		isFinished bool
	)

	defer func() {
		if !isFinished {
			_ = rollback(ctx, current.tx)
		}
	}()

	err = fn(context.WithValue(ctx, transactionKey{}, current))
	isFinished = true

	if err != nil {
		if rollbackErr := rollback(ctx, current.tx); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("roll back a transaction: %w", rollbackErr))
		}
	} else if commitErr := current.tx.Commit(ctx); commitErr != nil {
		err = fmt.Errorf("commit a transaction: %w", commitErr)
	}

	return err
}

// Rollback ignores the caller's cancellation, which is often the reason for
// rolling back, but still has its own deadline.

func rollback(ctx context.Context, tx pgx.Tx) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()

	err := tx.Rollback(rollbackCtx)

	if errors.Is(err, pgx.ErrTxClosed) {
		err = nil
	}

	return err
}
