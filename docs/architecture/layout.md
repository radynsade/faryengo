# Project layout

The project follows a conventional Go layout. Directories are added when they
have a purpose; not every directory below needs to exist from the start.

| Directory | Purpose |
| --- | --- |
| `cmd/` | Application entry points. Each executable has its own directory (for example, `cmd/server/`) and wires its dependencies in `main`. |
| `internal/` | Application code that must not be imported by other repositories. Every directory directly under `internal/` is a domain scope, except `app/`, `config/`, and `infra/`. Keep domain logic and infrastructure implementations within their scopes; transport belongs outside domain directories. |
| `internal/app/` | Application services and the contracts they need from infrastructure that belong to no domain, such as the `Transactor` that runs several repository operations atomically. It must not import infrastructure packages. This is not a domain scope. |
| `internal/app/input/` | Application service input values and use-case error sentinels. Services construct domain values; inputs do not repeat domain validation. This is not a domain scope. |
| `internal/infra/<implementation_name>/` | Infrastructure shared by several domains' implementations and owned by none, such as `internal/infra/pgxdb/`, which defines the PostgreSQL handle that every pgx repository accepts. It must not import domain packages and holds no domain logic or transport. This is not a domain scope. |
| `internal/config/` | Application configuration struct and utilities to load `.env` with `godotenv` and read environment variables. This is not a domain scope. |
| `internal/<domain>/` | A domain-scoped directory. Put each aggregate in its own `.go` file named after the aggregate (for example, a `User` aggregate belongs in `user.go`). |
| `internal/<domain>/<implementation_name>/` | Infrastructure layer only: implementations of domain interfaces for Redis, PostgreSQL, hashing algorithms, and other infrastructure features. The team chooses a descriptive implementation name. For example, a `UserRepository` implementation using a pgx PostgreSQL connection pool and the goqu query builder belongs in `internal/users/pgxgoqu/`. Transport implementations must not live here. |
| `internal/<domain>/<implementation_name>/<nested_implementation_name>/` | Infrastructure layer only: implementations of an interface that an implementation package declares for its own dependencies, such as a Redis implementation of a token storage used by a JWT authenticator. The parent package owns the interface and its policy; the nested package may import its parent and domain packages but not sibling implementations. See [nested interfaces and implementations](../go/layout.md#nested-interfaces-and-implementations). |
| `pkg/` | Packages intended for import by other repositories. Add packages here only when they have a real external consumer. |
| `api/` | API transport handlers, routing, contracts, and schemas, such as OpenAPI or Protocol Buffers. Generate code from the source definitions rather than editing generated files. |
| `web/` | Web transport handlers, routing, page rendering, and frontend source and assets, including styles, scripts, and static files. |
| `web/admin/handlers/` | The admin's HTTP handlers: the `Handler` type, route registration, and the handler methods that authenticate, authorize, call application services, and choose responses. |
| `web/admin/utils/` | Admin transport code that is not a handler: cookies, request state, flash sessions, rendering and minification, request parsing and validation, field errors, and mapping domain values to template props. It must not import `web/admin/handlers/`. |
| `web/office/` | The office web application. It uses the admin's stack and mirrors its structure: `handlers/`, `utils/`, `i18n/`, `templates/`, and the Vite build in `assets/`. Its design is its own and does not reuse the admin's styles. |
| `middleware/` | Transport middleware shared by web and API endpoints. |
| `docs/` | Architecture, development, and operational documentation. |
| `db/migrations/` | Versioned PostgreSQL schema changes. Add migration files when the schema changes; a human runs migrations. |
| `deploy/` | Deployment configuration, manifests, and environment templates. |
