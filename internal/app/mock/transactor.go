package mock

import "context"

//
// Transactor
//

// Transactor stands in for app.Transactor without a database: it runs the
// function directly, with a context that InTransaction reports as
// transactional, and cannot undo work. It does not import internal/app, so
// tests inside that package can use it; app.Transactor conformance is checked
// where it is passed.

type transactionKey struct{}

type Transactor struct {
	BeginErr error
	Calls    int
}

func (t *Transactor) InTransaction(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	err := t.BeginErr

	t.Calls++

	if err == nil {
		err = fn(context.WithValue(ctx, transactionKey{}, t))
	}

	return err
}

// Report whether the context was created by a Transactor

func InTransaction(ctx context.Context) bool {
	_, ok := ctx.Value(transactionKey{}).(*Transactor)

	return ok
}
