# Architecture

## Architectural approach

We use Domain-Driven Design (DDD). Each domain scope under `internal/`
contains its aggregates and domain interfaces. The `internal/app/`
package coordinates use cases. The `internal/config/` package contains the
configuration struct and utilities to read configuration from environment
variables, loading `.env` with `godotenv` when present. Neither package is
a domain scope. Implementations of domain interfaces live in descriptive
subdirectories within their domain scope.

## Technological stack

The application is written in Go. The services and libraries below are the
intended stack; add their dependencies when the corresponding integration is
implemented. Keep technology-specific code in the implementation package for
its domain, and let the application layer coordinate use cases.

### Data and messaging

| Technology | Role and usage |
| --- | --- |
| [PostgreSQL](https://www.postgresql.org/docs/current/tutorial-transactions.html) | System of record for durable application data. Store aggregates and other persistent state here, and use transactions when a use case must update multiple records atomically. Database schema changes belong in migrations. |
| [pgx](https://github.com/jackc/pgx) | PostgreSQL driver for Go. Use `pgxpool` for a shared, concurrency-safe connection pool, pass request contexts to database calls, and close the pool on shutdown. Repository implementations own queries and row mapping. |
| [goqu](https://github.com/doug-martin/goqu) | SQL query builder for repository implementations. Use the PostgreSQL dialect and `Prepared(true)` to produce parameterized SQL and arguments for pgx; goqu does not replace PostgreSQL, pgx, or domain models. Keep query construction out of aggregates and HTTP handlers. |
| [Dragonfly](https://www.dragonflydb.io/docs) | Redis-compatible in-memory key-value store for cache entries and other data that can be rebuilt. Define key names and expiration rules in the owning implementation. PostgreSQL remains authoritative when a cached value is missing or stale. |
| [NATS](https://docs.nats.io/learn/core-nats/) | Broker for communication between processes through subjects. Use Core NATS for transient publish/subscribe messages; it does not retain messages for disconnected consumers. A workflow that needs stored messages, replay, or acknowledgments must explicitly use [JetStream](https://docs.nats.io/reference/2.12/jetstream). |

### Web interface

| Technology | Role and usage |
| --- | --- |
| [templ](https://templ.guide/core-concepts/components/) | Go-based HTML components for server-rendered pages and reusable fragments. Render both full pages and the fragments returned by interactive endpoints from the same component system. |
| [Datastar](https://data-star.dev/guide/reactive_signals) | Declarative browser reactivity through signals and server-driven updates through Server-Sent Events (SSE). Use it where local UI state or streamed changes are needed. Keep authoritative business state on the server and validate values received from the browser. |

A typical request enters a Go HTTP handler, runs an application use case, and
uses a domain interface whose implementation accesses PostgreSQL through pgx
and, when useful, goqu. The handler renders pages or fragments with templ;
Datastar can manage reactive UI state or receive SSE updates. Dragonfly can
accelerate reads of rebuildable data, and NATS can carry events to other
processes.

## Structure

The project follows a conventional Go layout. Directories are added when they
have a purpose; not every directory below needs to exist from the start.

| Directory | Purpose |
| --- | --- |
| `cmd/` | Application entry points. Each executable has its own directory (for example, `cmd/server/`) and wires its dependencies in `main`. |
| `internal/` | Application code that must not be imported by other repositories. Every directory directly under `internal/` is a domain scope, except `app/` and `config/`. Keep domain logic, services, storage, and transport implementations within their scopes. |
| `internal/app/input/` | Application service input values and their validation. This is not a domain scope. |
| `internal/config/` | Application configuration struct and utilities to load `.env` with `godotenv` and read environment variables. This is not a domain scope. |
| `internal/<domain>/` | A domain-scoped directory. Put each aggregate in its own `.go` file named after the aggregate (for example, a `User` aggregate belongs in `user.go`). |
| `internal/<domain>/<implementation_name>/` | Place each implementation of a domain interface in a directory under its domain scope. The team chooses a descriptive implementation name. For example, a `UserRepository` implementation using a pgx PostgreSQL connection pool and the goqu query builder belongs in `internal/security/pgxgoqu/`. |
| `pkg/` | Packages intended for import by other repositories. Add packages here only when they have a real external consumer. |
| `api/` | API contracts and schemas, such as OpenAPI or Protocol Buffers. Generate code from the source definitions rather than editing generated files. |
| `web/` | Web assets and frontend source, including styles, scripts, and static files. |
| `docs/` | Architecture, development, and operational documentation. |
| `db/migrations/` | Versioned PostgreSQL schema changes. Add migration files when the schema changes; a human runs migrations. |
| `deploy/` | Deployment configuration, manifests, and environment templates. |

The admin assets package embeds its Vite build and manifest into the Go binary. The Makefile builds frontend assets and generates templ code before Go checks and compilation. Asset serving uses the embedded filesystem, so deployed binaries do not need the build directory on disk.

Keep tests beside the Go packages they exercise. The root contains module and
build files such as `go.mod` and `Makefile`.

## Authentication state

Authentication uses Ed25519 JWT access and refresh tokens, PostgreSQL credential
versions, and revocable Redis/Dragonfly session state. Password changes and
account-wide logout invalidate the durable version. Session checks and current
role permissions are required for every access-token authentication; storage
outages deny access. See [authentication.md](authentication.md) for transport,
configuration, rotation policy, and operational requirements.
