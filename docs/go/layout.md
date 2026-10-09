# Go file layout

This guide defines which declarations belong in which Go files and packages.
Use [code style](style.md) for organization within a file and
[tests](tests.md) for test conventions.
[Technical requirements](../architecture/tech-requirements.md) and
[project layout](../architecture/layout.md) take precedence over these rules.

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
  (`internal/users/role_repository.go:13-150`).
- Put the PostgreSQL implementation in `<domain>/pgxgoqu/<entity>_repository.go`
  as a `<Entity>Repository` struct.
- Put a domain service in `<service>.go`, named after the service, as a
  concrete struct with a `New<Service>` constructor that accepts the domain
  contracts it needs (`internal/security/identity_resolver.go`). A domain
  service holds domain rules that span several aggregates and depends on no
  infrastructure or authentication mechanism; callers use it directly rather
  than through each mechanism that produces its input.

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
- Exception, following [technical requirements](../architecture/tech-requirements.md#architecture-and-dependencies): a domain package
  declares the contracts that infrastructure implements. A repository
  interface (`<Entity>Repository`) or a domain service port
  (`PasswordHasher`, `internal/users/password.go:79-82`) lives in the
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

## Nested interfaces and implementations

An implementation of a domain interface may itself depend on a capability that
has several possible implementations, such as storage. The implementation
package then declares that capability as its own interface, and the
implementations of that interface nest below it.

- Declare the nested interface in the implementation package that consumes
  it, together with the sentinels its implementations must return. The
  interface describes only what that consumer needs.
- Put each implementation of the nested interface in
  `<implementation>/<nested_implementation_name>/`, named after its technology,
  and check its conformance at compile time.
- Keep policy in the consumer. The nested implementation stores, loads, and
  performs atomic comparisons; the consumer decides what a result means, such
  as revoking access after a failed comparison.
- A nested implementation may import its parent package and domain packages.
  It must not import sibling implementations or their nested packages.
- When several sibling implementations need the same contract, declare it once
  in the domain package that owns the underlying state rather than in each
  sibling.
- Test the consumer from an external test package (`package <name>_test`) with
  a real nested implementation connected to a test server, because the nested
  package imports its parent.
- Wire the nested implementation into its consumer in `main()`, like any other
  dependency.

The following tree illustrates these rules with an imaginary notifications
domain. `notifications.Sender` has an email implementation, which declares an
`email.Transport` interface with an SMTP implementation:

```text
internal/notifications/
├── sender.go               Sender interface
├── push/
│   └── sender.go           Sender implementation
└── email/
    ├── sender.go           Sender implementation, Transport interface
    └── smtp/
        └── transport.go    Transport implementation
```

`email/smtp` may import `email` and `notifications`, but not `push`.

## Supporting files and wiring

The repository root contains module and build files such as `go.mod` and
`Makefile`. Keep test files beside the packages they exercise; see
[test placement](tests.md#test-placement).

- Don't use a dependency injection framework. Wire dependencies explicitly in
  `main()`.
