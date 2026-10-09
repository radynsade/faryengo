package app

import "context"

//
// Transaction
//

// InTransaction commits when fn returns nil and rolls back when fn returns an
// error or panics. Repositories take part only through the ctx passed to fn. A
// nested call runs in a savepoint: its failure rolls back only its own work,
// and the outermost call alone commits.

type Transactor interface {
	InTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
