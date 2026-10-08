# Go

Go 1.26.

## Documentation guides

The guides describe the project's functional areas and conventions. The
architecture guide takes precedence over conflicting guidance in this file and
in the Go code style guide.

| Guide | Description |
| --- | --- |
| [docs/architecture.md](docs/architecture.md) | Project organization, component responsibilities, dependency boundaries, and architectural conventions. |
| [docs/go-code-style.md](docs/go-code-style.md) | How Go code is written: file layout, errors, control flow, concurrency, interfaces, repositories, naming, formatting, comments, and tests. |
| [docs/domain/security.md](docs/domain/security.md) | Users, roles, permissions, and security domain rules. |
| [docs/domain/languages.md](docs/domain/languages.md) | Language catalog, fallback languages, and translation rules. |
| [docs/domain/budget.md](docs/domain/budget.md) | Budget concepts, relationships, calculations, and invariants. |
| [docs/authentication.md](docs/authentication.md) | Identity verification, access control, session lifecycles, and security policies. |
| [docs/admin-design.md](docs/admin-design.md) | The admin panel's visual system, reusable components, styling conventions, layouts, accessibility, and rendering behavior. |
| [docs/admin-i18n.md](docs/admin-i18n.md) | Admin interface translation catalogs, locale handling, language switching, pluralization, and localized messages. |
| [docs/assets.md](docs/assets.md) | Application asset building, packaging, resolution, and delivery. |
| [docs/flash-messages.md](docs/flash-messages.md) | Temporary user notifications and their lifecycle, persistence, and presentation. |
| [docs/migrations.md](docs/migrations.md) | Database schema evolution, migration workflows, and rollback procedures. |
| [docs/cli.md](docs/cli.md) | Command-line interface conventions, argument handling, and output behavior. |

## Commands

- Everything: `make check`  (gofmt, go vet, golangci-lint, go test -race)
- Test:       `go test -race -count=1 -timeout 60s ./...`
- One test:   `go test -run TestName ./internal/pkg -v`
- Bench:      `go test -bench=. -benchmem ./internal/pkg`

`-race` is always on and `-count=1` disables caching. Do not remove either to
make the suite faster.

## Code style

Follow [docs/go-code-style.md](docs/go-code-style.md) for all Go code.

## Landmines

- internal/scheduler is leader-elected. Changing tick timing needs an ops review.
- internal/proto is generated. Edit the .proto and run `make proto`.
- cmd/migrate: write migrations, never run them. A human runs migrations.
