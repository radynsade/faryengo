# Technology stack

This guide describes the infrastructure services, libraries, and tools used
by the project. Go dependencies are defined in the module manifest; frontend
dependencies are defined in the npm manifest and lockfile. Keep versions there
rather than duplicating the full version list in this guide.

## Infrastructure services

| Service | Role in the project |
| --- | --- |
| PostgreSQL 18 or later | Durable storage for users, credentials, roles, permissions, languages, and translations. Transactions, constraints, and triggers protect consistency. Version 18 is required for database-generated UUIDv7 values. |
| Redis-compatible storage | Shared device sessions, revocation state, sign-in throttling, and flash messages. The application uses the Redis protocol and can target Redis or the architecture's intended Dragonfly service. |

PostgreSQL and Redis-compatible storage are required by the server. Their
connections are configured through `DATABASE_URL` and `REDIS_URL`.
Redis holds authentication state as well as temporary notifications; it must
be treated as security infrastructure rather than only a rebuildable cache.
Session lifetime, revocation, and outage behavior are described in
[authentication](../authentication.md), and notification storage conventions in
[flashmsg](../pkg/flashmsg.md#usage-conventions).

## Go libraries

| Library | Role in the project |
| --- | --- |
| Go standard library | HTTP serving and routing, structured logging, JSON, configuration parsing, cryptographic primitives, and embedded files. |
| pgx | PostgreSQL driver, connection pooling, transactions, and row access for repositories and migration tooling. |
| goqu | PostgreSQL query construction and bound arguments in repository implementations. |
| go-redis | Redis client shared by authentication, throttling, and flash storage; atomic state changes use Lua scripts. |
| templ | Server-rendered HTML components for full pages and interactive fragments; its generator produces Go code from template sources. |
| go-playground/validator | Structural validation of transport requests before they become application inputs. Domain validation remains separate. |
| go-i18n | Admin interface message catalogs, localized messages, fallback translations, and pluralization. |
| golang.org/x/text | Language identifiers used by interface localization. |
| tdewolff/minify | Minimizes rendered admin HTML, both documents and partial responses, without changing attributes or behavior. |
| google/uuid | Identity parsing and UUIDv7 generation. |
| golang.org/x/crypto | Argon2id password hashing and verification. |
| golang-jwt/jwt | JWT access and refresh token signing and verification with Ed25519. JWT support is separate from the admin's opaque browser sessions. |
| godotenv | Optional `.env` loading; existing environment variables take precedence. |
| golang.org/x/sync | Coordinated goroutines, cancellation, and bounded concurrent work. |

Infrastructure libraries belong in adapters, while rendering and request
validation belong in transport code. Application use cases coordinate those
components through domain contracts, as described in
[technical requirements](tech-requirements.md#architecture-and-dependencies).

## Admin frontend

| Technology | Role in the project |
| --- | --- |
| TypeScript | Browser interaction code and build-time type checking. |
| Sass, through sass-embedded | SCSS styles compiled to CSS. |
| HTMX | Requests and navigation that replace server-rendered HTML fragments. |
| HTMX head-support extension | Updates head elements during fragment navigation. |
| Idiomorph | Morphs returned markup into the existing page and preserves reusable elements during HTMX updates. |
| Tabler Icons webfont | Admin interface icons supplied through CSS and font assets. |

HTML is rendered by Go and templ. TypeScript handles browser interactions;
HTMX connects those interactions to server-rendered responses. Interface
translation catalogs are distinct from translated domain values.

## Build and development tools

| Tool | Role in the project |
| --- | --- |
| Go 1.26 toolchain | Compiles the server, CLI, and migration command; provides formatting, vet, and test tools. |
| Make | Coordinates asset building, template generation, Go checks, tests, and binary builds. |
| Node.js and npm | Install and run frontend build tools; npm's lockfile controls resolved frontend dependencies. |
| Vite | Frontend development server and production asset bundler; produces content-hashed files and a build manifest. |
| TypeScript compiler | Checks frontend types before Vite builds the assets. |
| templ generator | Generates Go rendering code before Go checks and compilation. |
| Prettier | Formats frontend source and checks its formatting during the lint stage. |
| golangci-lint | Runs Go lint checks. |
| Migration command | Project-maintained tool for versioned SQL migrations and migration status; migration execution is a manual operator action. |

The build produces frontend assets first, generates template code, and then
runs Go checks and compilation. The admin's compiled assets, manifest, static
files, and interface catalogs are embedded in the Go binary. Node.js and the
frontend build tools are needed to build assets, not to serve the deployed Go
application.

`make check` runs formatting, vet, frontend formatting checks, Go lint, and the
race-enabled tests after building assets and generating templates. Test commands
and conventions are documented in [Go tests](../go/tests.md); database changes
are documented in [migrations](../migrations.md).

## Test libraries

| Library or tool | Role in the project |
| --- | --- |
| Go testing tools | Package tests, HTTP handler tests, benchmarks, and race detection. |
| goleak | Detects goroutines left running after a package's tests. |
| miniredis | In-memory Redis test server for session, throttling, and flash-storage tests. |
| Real Redis and PostgreSQL | Optional integration tests for behavior requiring actual services; kept separate from ordinary test fixtures. |

## Planned infrastructure

NATS is part of the intended architecture for inter-process messaging. There
is currently no NATS client dependency, configuration, or integration in the
project, so it is not a runtime requirement of the existing server.
