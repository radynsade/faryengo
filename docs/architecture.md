# Architecture

## Architectural approach

We use Domain-Driven Design (DDD). Each domain scope under `internal/`
contains its aggregates and domain interfaces. The `internal/app/`
package coordinates use cases. The `internal/config/` package contains the
configuration struct and utilities to read configuration from environment
variables, loading `.env` with `godotenv` when present. Neither package is
a domain scope. Subdirectories within a domain scope are reserved for
infrastructure implementations of domain interfaces, such as Redis,
PostgreSQL, hashing algorithms, and other infrastructure features. The
transport layer lives outside domain directories, under `web/`, `api/`, and
`middleware/`, and calls application use cases.

## Technological stack

The application is written in Go. The services and libraries below are the
intended stack; add their dependencies when the corresponding integration is
implemented. Keep infrastructure-specific code in the implementation package
for its domain and transport-specific code under `web/`, `api/`, or
`middleware/`. Let the application layer coordinate use cases.

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
| [HTMX](https://htmx.org/docs/) | HTML-driven interaction through requests and server-rendered HTML fragments. Use boosted links for navigation and swap the returned content into the page. Keep authoritative business state on the server and validate values received from the browser. |

A typical request enters a Go HTTP handler, runs an application use case, and
uses a domain interface whose implementation accesses PostgreSQL through pgx
and, when useful, goqu. The handler renders pages or fragments with templ;
HTMX can request and swap HTML fragments without reloading assets. Dragonfly can
accelerate reads of rebuildable data, and NATS can carry events to other
processes.

## Structure

The project follows a conventional Go layout. Directories are added when they
have a purpose; not every directory below needs to exist from the start.

| Directory | Purpose |
| --- | --- |
| `cmd/` | Application entry points. Each executable has its own directory (for example, `cmd/server/`) and wires its dependencies in `main`. |
| `internal/` | Application code that must not be imported by other repositories. Every directory directly under `internal/` is a domain scope, except `app/` and `config/`. Keep domain logic and infrastructure implementations within their scopes; transport belongs outside domain directories. |
| `internal/app/input/` | Application service input values and use-case error sentinels. Services construct domain values; inputs do not repeat domain validation. This is not a domain scope. |
| `internal/config/` | Application configuration struct and utilities to load `.env` with `godotenv` and read environment variables. This is not a domain scope. |
| `internal/<domain>/` | A domain-scoped directory. Put each aggregate in its own `.go` file named after the aggregate (for example, a `User` aggregate belongs in `user.go`). |
| `internal/<domain>/<implementation_name>/` | Infrastructure layer only: implementations of domain interfaces for Redis, PostgreSQL, hashing algorithms, and other infrastructure features. The team chooses a descriptive implementation name. For example, a `UserRepository` implementation using a pgx PostgreSQL connection pool and the goqu query builder belongs in `internal/security/pgxgoqu/`. Transport implementations must not live here. |
| `pkg/` | Packages intended for import by other repositories. Add packages here only when they have a real external consumer. |
| `api/` | API transport handlers, routing, contracts, and schemas, such as OpenAPI or Protocol Buffers. Generate code from the source definitions rather than editing generated files. |
| `web/` | Web transport handlers, routing, page rendering, and frontend source and assets, including styles, scripts, and static files. |
| `middleware/` | Transport middleware shared by web and API endpoints. |
| `docs/` | Architecture, development, and operational documentation. |
| `db/migrations/` | Versioned PostgreSQL schema changes. Add migration files when the schema changes; a human runs migrations. |
| `deploy/` | Deployment configuration, manifests, and environment templates. |

The admin assets package embeds its Vite build and manifest into the Go binary. The Makefile builds frontend assets and generates templ code before Go checks and compilation. Asset serving uses the embedded filesystem, so deployed binaries do not need the build directory on disk.

Keep tests beside the Go packages they exercise. The root contains module and
build files such as `go.mod` and `Makefile`.

Generate all new UUIDs as version 7 for time-ordered identifiers. Go code uses
`github.com/google/uuid.NewV7()` and handles its error; PostgreSQL uses the
built-in `uuidv7()` function, requiring PostgreSQL 18 or later. Existing UUIDs
remain valid regardless of their version.

## Authentication state

Authentication has a transport-independent identity service that resolves
`security.Principal`, with separate opaque-session and Ed25519 JWT access/refresh
mechanisms. The admin uses only server-side sessions and a random opaque cookie;
JWT remains available for API transports. PostgreSQL credential versions and
revocable Redis/Dragonfly device state guard both mechanisms. Password changes
and account-wide logout invalidate the durable version. Current permissions
are loaded on every authentication, and authorization accepts the common
principal. Storage outages deny access. See [authentication.md](authentication.md)
for transport, configuration, rotation policy, and operational requirements.

## Validation boundaries

HTTP handlers decode primitive request DTOs, validate their structure, and then
map them to application inputs. DTO tags use the shared
`middleware/requestvalidation` validator, initialized with
`WithRequiredStructEnabled`. It converts failures to sorted transport field
errors containing names, rules, and limits, without submitted values or library
error types. Admin forms attach localized errors to their controls in the current
response, without writing field errors to flash storage. Body limits, media types,
duplicate parameters, and form/catalog binding remain transport checks.
The admin mailbox rule delegates to `security.NewEmail` because the library's
built-in email format excludes some mailboxes accepted by this domain.
The HTTP UUID format rule also retains the existing parser's accepted encodings;
the CLI continues requiring canonical hyphenated UUID arguments.

Application services construct domain values before hashing, loading, or writing
state. They coordinate existence, authorization, fallback-language policy, and
conflicts. Repository errors also represent constraints enforced atomically by
the database, including unique emails, missing references, and deletion of
assigned roles. Input structs have no generic `Validate` method. CLI parsing
uses domain constructors to reject invalid arguments before opening a database
connection; services independently construct values again for all callers.
The sign-in account-lookup limit of 254 bytes remains a use-case constraint.

Security and Languages expose typed values and aggregate fields with explicit
`Validate` methods. Constructors retain supplied values; callers validate before
performing side effects. Domain packages never import the transport validator.
A cast or constructor alone is not a validity guarantee. Repositories validate
aggregates before writes and validate rehydrated values before returning them.
User and Role validation reject zero identities; existing UUID versions remain
unrestricted. Repository create, update, and delete failures expose their
operation data and unwrap the underlying cause.

`security.Password` is a plaintext value distinct from `PasswordHash`.
Its validation requires valid UTF-8, nonblank input, at least six characters,
and at most 4096 bytes. Password verification accepts bounded raw candidates so
existing credentials can still be checked independently of the creation policy.
Hash implementations retain algorithm-specific encoded-hash checks and resource
bounds. `AuthenticationSnapshot` combines a user and its durable version in one
repository read; it is stored server-side. `PrincipalRepository` projects current
user and role data without a password hash. Such a projection does not itself
authenticate a caller. Session intrinsic state and token issuance relationships
belong to Security; clock-dependent expiration, current authentication snapshot
versions, and revocation require orchestration.
Role filter and query validity remain domain checks for non-HTTP callers and
repositories, with DTO tags providing early HTTP feedback. Configuration,
asset-manifest, migration-file, and storage-format checks remain in their owning
parsers and adapters; they are not aggregate invariants.
