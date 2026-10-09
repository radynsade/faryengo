# Go usages

This guide defines how to use the project's existing abstractions and
utilities. Use [code style](style.md) for how code looks and
[file layout](layout.md) for where declarations live.

## Database handles

PostgreSQL repository connection fields use `pgxdb.DB`, which accepts a pool,
a connection, or a transaction. Introduce any other adapter interface only when
the adapter needs an abstraction; do not require one for every repository.

Resolve the database handle with `pgxdb.FromContext(ctx, r.pool)` for every
statement, never by calling `r.pool` directly, so the operation joins the
caller's transaction. Multi-step writes run their statements inside
`pgxdb.InTransaction` rather than calling `Begin`, `Commit`, or `Rollback` by
hand. Operations needing a transaction, such as row locks, check the resolved
handle and reject one that cannot provide a transaction.

## Transactions

Application code makes several repository operations atomic with
`app.Transactor`:

```go
err := s.transactor.InTransaction(ctx, func(ctx context.Context) error {
	role, err := s.roles.FindByIDForUpdate(ctx, id)

	if err == nil {
		err = s.roles.Update(ctx, role)
	}

	return err
})
```

- The transaction commits when the function returns nil and rolls back when it
  returns an error or panics. A panic keeps propagating after the rollback.
- When rollback fails too, the returned error joins the function's error with
  the rollback failure, so both remain inspectable with `errors.Is`.
- Inside the function, pass only the function's `ctx` to repositories. The
  transaction travels in that context; an operation given an outer context runs
  outside the transaction.
- A nested `InTransaction` call runs in a savepoint. Its failure rolls back
  only the savepoint and returns the error to the enclosing function, which
  decides whether to recover or return it. Only the outermost call commits.
- A transaction belongs to the database handle it was opened on. Repositories
  built on another handle, including an explicit `pgx.Tx`, keep using their
  own handle.
- Do not start goroutines that use the transaction's context: a transaction
  runs on one connection and is not safe for concurrent use.
