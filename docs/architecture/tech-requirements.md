# General technical requirements

These requirements apply across the project. They define architectural and
technical constraints; domain rules belong in the domain model documents, and
implementation conventions belong in the Go guides.

## Architecture and dependencies

Use Domain-Driven Design (DDD). Keep domain concepts and invariants within
clearly defined domain scopes, and use aggregates as consistency boundaries.

| Responsibility | Requirement |
| --- | --- |
| Domain | Own aggregates, value objects, business rules, and contracts for required infrastructure. Keep transport and storage details outside domain logic. |
| Application | Coordinate use cases, domain validation, references, authorization requirements, and persistence through domain contracts. |
| Infrastructure | Implement domain contracts and own queries, storage formats, external clients, and infrastructure-specific errors. |
| Transport | Decode and constrain requests, invoke use cases, and present results. Keep HTTP, browser, and CLI concerns here. |
| Configuration and composition | Load settings and wire concrete dependencies explicitly. Keep configuration separate from domain scopes. |

Infrastructure implements contracts defined by the domain. Application and
domain logic must not depend on transport handlers or concrete storage
implementations. Select abstractions for production responsibilities; do not
introduce dependency wrappers solely for tests.

Follow [project layout](layout.md) for package boundaries and
[Go file layout](../go/layout.md) for declaration ownership.

## Identity and domain validity

- Generate all new UUIDs as version 7 for time-ordered identifiers.
- Preserve valid existing identities regardless of their UUID version.
- Treat identity generation, construction, validation, and persistence as
  separate responsibilities. Constructing a value does not prove it is valid.
- Keep authoritative business constraints in the owning domain, and perform
  validation before side effects. Validation must not modify domain state.

## Validation boundaries

Transport validates request structure, allowed parameters, content types, and
size limits before mapping requests to application inputs. Domain validation
remains independent of transport validation libraries.

Application use cases construct and validate domain values before hashing,
loading, or writing state. They coordinate existence checks, policy, and
conflicts. Repositories validate values before writes and validate restored
aggregates before returning them.

Infrastructure and configuration parsers own checks for their formats and
settings. Keep those checks separate from aggregate invariants. Detailed
validation responsibilities are defined in
[validation boundaries](#validation-boundaries).

## Persistence and consistency

- Use PostgreSQL as the system of record for durable domain state.
- Use parameterized queries and keep query construction inside infrastructure.
- Enforce persistent uniqueness and reference constraints atomically. Checks
  performed before a write do not replace database guarantees.
- Use transactions when a use case must persist several changes atomically.
  Application code runs such work through the application layer's
  transaction contract, never through a database handle; repositories join
  the transaction through the operation's context. A nested transaction is a
  savepoint: its failure undoes only its own work, and only the outermost
  transaction commits.
- Preserve established conflict detection for concurrent changes; stale writes
  must not silently overwrite newer state where that protection is required.
- Respect database-owned timestamps and other database-maintained values.
- Merge and consume shared session state atomically. Preserve expiration and
  revocation checks during every operation that depends on that state.

Redis-compatible storage holds security state as well as temporary data.
Authentication must not treat that state as a disposable cache or bypass its
checks when storage is unavailable. Retry an operation only when replay cannot
repeat a change, duplicate a result, or consume state twice.

Schema changes require versioned migrations. Preserve already applied migration
files and write new versions for later changes. Humans execute migrations;
application startup must not apply them. See [migrations](../migrations.md).

## Security and access

Authentication establishes identity; authorization evaluates the current
identity and its permissions. Use the same authorization model across
transports. Verify session validity, revocation, and current credential state
before accepting authenticated access. Changes to credentials and authority
must follow the lifecycle defined in [authentication](../authentication.md).

- Deny authenticated access when required security storage or checks are
  unavailable.
- Use HTTPS in production and follow the documented secure-cookie and
  cross-origin protection policies for browser requests.
- Keep passwords, bearer credentials, signing secrets, and other sensitive
  values out of logs, notifications, and error responses.
- Return errors appropriate to the transport without exposing infrastructure
  causes or sensitive account details.
- Bound request sizes, authentication attempts, and expensive concurrent work.

## Configuration and resource lifetime

Use environment-based configuration with optional local configuration files.
Existing environment values take precedence. Keep secrets outside source
control, validate settings before use, and reject missing required
configuration rather than inventing insecure defaults.

Give external operations deadlines and propagate cancellation. Every goroutine
must have an exit path. The component that owns a connection pool, client, or
other resource must close it during shutdown. Coordinate shutdown so requests
and background work can finish or be cancelled.

Changes to a leader-elected scheduler's tick timing require an operations
review.

## Web delivery and localization

Keep authoritative business state on the server and validate all input from
the browser. Render full documents and partial responses through the same
component system. Follow the
[admin technical requirements](../admin/tech-requirements.md) for navigation,
DOM updates, minimized responses, and progressive enhancement.

User-facing interface text comes from translation catalogs. Keep interface
localization distinct from translated domain data. Follow
[admin internationalization](../admin/i18n.md) for locale selection and message
behavior, and [flash-message conventions](../pkg/flashmsg.md#usage-conventions)
for temporary notifications and field errors.

## Builds and generated artifacts

Use the project's documented [technology stack](tech-stack.md). Dependency
manifests and lockfiles define dependency versions.

Build frontend assets and generate template code before Go checks and
compilation. Embed the web application's compiled assets, manifests, static
files, and interface catalogs so deployed binaries do not require frontend
build tools or a separate asset build directory.

Change generated artifacts through their source definitions and regenerate
outputs. Do not edit generated code directly.

## Verification

Follow [Go code style](../go/style.md) and [test conventions](../go/tests.md).
Run the checks appropriate to the change, retaining race detection and uncached
execution for Go tests.

Production code must not be changed to accommodate tests. Mock only complete
domain interfaces; never mock database pools, connections, clients, or other
infrastructure dependencies. Follow the test guide's current temporary
restriction on writing integration tests. When integration testing resumes,
use real implementations of domain interfaces connected to real services.
