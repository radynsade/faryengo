# Go file layout

This guide defines which declarations belong in which Go files and packages.
Use [code style](style.md) for organization within a file and
[tests](tests.md) for test conventions.
[Architecture](../architecture.md) and [project layout](../architecture/layout.md)
take precedence over these rules.

## Domain files

- Put each aggregate and its value types in `<entity>.go` in the domain
  package: `language.go`, `user.go`, `role.go`.
- Put a value type that several aggregates share in its own file, named after
  the concept: `password.go` (`Password`, `PasswordHash`, `PasswordHasher`),
  `text.go` (`Translation`, `Text`).
- Put the repository contract in `<entity>_repository.go` in the domain
  package. That file holds the repository sentinels, the typed write-failure
  interfaces, the repository interface and, when needed, the
  `<Entity>Filter`, `<Entity>Sort` and `<Entity>Query` types
  (`internal/security/role_repository.go:13-150`).
- Put the PostgreSQL implementation in `<domain>/pgxgoqu/<entity>_repository.go`
  as a `<Entity>Repository` struct.

The following tree illustrates these rules with an imaginary catalog domain:

```text
internal/catalog/
├── price.go
├── product.go
├── product_repository.go
└── pgxgoqu/
    └── product_repository.go
```

## Interface placement

- Declare an interface in the consumer's package and keep it small: one or two
  methods.
- Exception, following [architecture.md](../architecture.md): a domain package
  declares the contracts that infrastructure implements. A repository
  interface (`<Entity>Repository`) or a domain service port
  (`PasswordHasher`, `internal/security/password.go:79-82`) lives in the
  domain package and lists every operation the domain exposes, however many
  that is (`internal/languages/language_repository.go:33-41`).
- PostgreSQL repositories accept `pgxdb.DB` from `internal/infra/pgxdb/`
  rather than a concrete pool, so callers can pass a pool or a transaction.
  Keep any other adapter-specific contracts within the adapter package, and
  move one to `internal/infra/<implementation_name>/` only when several
  domains' adapters need it.

- The application layer's transaction contract, `Transactor`, lives in
  `internal/app/`, its consumer. Its PostgreSQL implementation lives in
  `internal/infra/pgxdb/` and must not import `internal/app/`; wiring in
  `main()` checks conformance at compile time.

See [interface return types](style.md#interfaces) for constructor results and
typed repository write failures.

## Supporting files and wiring

The repository root contains module and build files such as `go.mod` and
`Makefile`. Keep test files beside the packages they exercise; see
[test placement](tests.md#test-placement).

- Don't use a dependency injection framework. Wire dependencies explicitly in
  `main()`.
