# Go

Go 1.26.

## Documentation guides

The guides describe the project's functional areas and conventions. The
architecture guides take precedence over conflicting guidance in this file and
in the Go guides.

| Guide | Description |
| --- | --- |
| [docs/how-to-doc.md](docs/how-to-doc.md) | General documentation writing conventions. |
| [docs/architecture/layout.md](docs/architecture/layout.md) | Project organization, directory responsibilities, and dependency boundaries. |
| [docs/architecture/tech-requirements.md](docs/architecture/tech-requirements.md) | Shared architectural requirements for identity, validation, persistence, security, builds, and verification. |
| [docs/architecture/tech-stack.md](docs/architecture/tech-stack.md) | Infrastructure services, libraries, and development tools used in the project. |
| [docs/go/layout.md](docs/go/layout.md) | Go file and package organization, declaration ownership, and dependency wiring. |
| [docs/go/style.md](docs/go/style.md) | Go code organization within a file, errors, control flow, naming, formatting, interfaces, and concurrency. |
| [docs/go/tests.md](docs/go/tests.md) | Test placement, table-driven tests, goroutine leak detection, and test commands. |
| [docs/domain/how-to-doc.md](docs/domain/how-to-doc.md) | Domain model documentation structure, schemas, field descriptions, and invariants. |
| [docs/domain/security.md](docs/domain/security.md) | Users, roles, permissions, and security domain rules. |
| [docs/domain/languages.md](docs/domain/languages.md) | Language catalog, fallback languages, and translation rules. |
| [docs/domain/budget.md](docs/domain/budget.md) | Budget concepts, relationships, calculations, and invariants. |
| [docs/admin/design.md](docs/admin/design.md) | The admin panel's visual system, reusable components, styling conventions, layouts, accessibility, and rendering behavior. |
| [docs/admin/i18n.md](docs/admin/i18n.md) | Admin interface translation catalogs, locale handling, language switching, pluralization, and localized messages. |
| [docs/admin/tech-requirements.md](docs/admin/tech-requirements.md) | Admin navigation, DOM updates, response minimization, and translatable interface text. |
| [docs/bin/cli.md](docs/bin/cli.md) | CLI commands, arguments, and options for managing languages, roles, and users. |
| [docs/bin/migrate.md](docs/bin/migrate.md) | Migration commands, database configuration, and the rule that only humans run migrations. |
| [docs/bin/server.md](docs/bin/server.md) | Server startup, environment settings, admin access, and shutdown. |
| [docs/pkg/how-to-doc.md](docs/pkg/how-to-doc.md) | Package documentation writing guide. |
| [docs/pkg/domquery.md](docs/pkg/domquery.md) | Shared filtering, sorting, and pagination query types. |
| [docs/pkg/flashmsg.md](docs/pkg/flashmsg.md) | Flash message API, session naming, persistence, lifecycle, and usage conventions. |
| [docs/pkg/staticast.md](docs/pkg/staticast.md) | Static asset loading, URL resolution, and HTTP delivery. |
| [docs/pkg/viteast.md](docs/pkg/viteast.md) | Vite manifest loading, compiled asset URL resolution, and HTTP delivery. |

## Commands

- Everything: `make check`  (gofmt, go vet, golangci-lint, go test -race)
- Test:       `go test -race -count=1 -timeout 60s ./...`
- One test:   `go test -run TestName ./internal/pkg -v`
- Bench:      `go test -bench=. -benchmem ./internal/pkg`

`-race` is always on and `-count=1` disables caching. Do not remove either to
make the suite faster.

## Code style

Follow [file layout](docs/go/layout.md), [code style](docs/go/style.md), and
[test conventions](docs/go/tests.md) for all Go code.

## Landmines

- internal/scheduler is leader-elected. Changing tick timing needs an ops review.
- internal/proto is generated. Edit the .proto and run `make proto`.
- cmd/migrate: write migrations, never run them. A human runs migrations.
