# Go code style

This guide defines the project's conventions for organizing and writing code
within a Go file. Use [file layout](layout.md) for declaration ownership and
[tests](tests.md) for test conventions. Use [usages](usages.md) for how to use
the project's existing abstractions and utilities.
The architecture guides, [project layout](../architecture/layout.md) and
[technical requirements](../architecture/tech-requirements.md), take precedence
where guidance conflicts.

## Organization within a file

### Sections and declaration order

Separate top-level sections with three-line comment banners: a line containing
only `//`, the concept name as a comment, and another line containing only `//`.
Use sentence case for the concept name.

Give each domain type its own section. Put value types before the aggregate
that uses them. Adapter files use `Errors`, `Repository`, and `Helpers` sections
in that order. Domain repository files that define query types use `Filter`,
`Sort`, `Query`, and `Repository` sections in that order.

| Section | Declaration order |
| --- | --- |
| Value type | Limits, error declarations and related patterns, type, validation methods. |
| Enum-like value type | Type, explicitly typed constants, validation methods. |
| Aggregate | Error declarations, struct, constructor, validation methods. |
| Domain repository | Sentinels, typed create/update/delete failure contracts, repository interface. |
| Adapter errors | Shared failure behavior, operation-specific failure implementations. |
| Adapter repository | Struct, compile-time contract check, constructor, methods in contract order. |
| Adapter helpers | Private lookup and write helpers, column definitions, query builders, row mapping, error mapping. |

A domain file with a value type and the aggregate that uses it:

```go
//
// Title
//

const MaxTitleLengthChars = 120

var (
	ErrTitleInvalid = errors.New("invalid title")
	ErrTitleBlank   = errors.New("is blank")
)

type Title string

func (t Title) Validate() error {
	// ...
}

//
// Article
//

var ErrArticleNil = errors.New("article is nil")

type Article struct {
	ID    ArticleID
	Title Title
}

func NewArticle(id ArticleID, title Title) *Article {
	// ...
}

func (a *Article) Validate() error {
	// ...
}
```

A local adapter interface, when needed, precedes the struct that uses it.
Interfaces shared by several domains' adapters, such as `pgxdb.DB`, live in
`internal/infra/` instead.

### Sub-headers

Give each repository method a one-line operation sub-header in sentence case,
without a period, followed by a blank line. Use articles where they make the
operation name read naturally. Identify the domain failure contract above each
adapter failure implementation.

A sub-header may be a full-sentence explanation when the operation relies on
behavior that the code cannot express clearly.

An adapter file with failure-contract and operation sub-headers:

```go
//
// Errors
//

type errArticleWriteFailed struct {
	article *articles.Article
	err     error
}

// Implementation of articles.ErrArticleCreateFailed

type errArticleCreateFailed struct {
	errArticleWriteFailed
}

//
// Repository
//

type ArticleRepository struct {
	pool pgxdb.DB
}

var _ articles.ArticleRepository = (*ArticleRepository)(nil)

// Create

func (r *ArticleRepository) Create(
	ctx context.Context,
	article *articles.Article,
) articles.ErrArticleCreateFailed {
	// ...
}

// Find by an ID

func (r *ArticleRepository) FindByID(
	ctx context.Context,
	id articles.ArticleID,
) (*articles.Article, error) {
	// ...
}

// A row lock lasts only until its transaction ends, so locking requires a
// handle that can provide a transaction; any other handle is rejected before
// I/O.

func (r *ArticleRepository) FindByIDForUpdate(
	ctx context.Context,
	id articles.ArticleID,
) (*articles.Article, error) {
	// ...
}

//
// Helpers
//

func scanArticle(row pgx.Row) (*articles.Article, error) {
	// ...
}
```

## Naming

Use domain terms consistently across types, fields, parameters, and methods.
Name parameters after their domain concept or role rather than abbreviating
them. Use short, consistent receivers: the type's initial, `id` for identities,
and `n` for name values.

| Name or pattern | Convention |
| --- | --- |
| `ctx` | Request or operation context. |
| `pool` | Repository database connection field. |
| `filter`, `query` | Selection criteria and query options. |
| `dataset` | Query-builder state, distinct from the generated SQL. |
| `query`, `args` | Generated SQL and its bound arguments. |
| `storedID` | Identity converted for storage; qualify additional identities by field. |
| `err` | The operation's result error. |
| `<step>Err` | An error belonging to a particular step. |
| `index` | A slice index; avoid `i`. |
| `Is<Adjective>` | Boolean state fields. |
| `New<Entity>` | Aggregate constructors. |
| `Max<Subject><Unit>`, `Min<Subject><Unit>` | Named limits; distinguish character lengths from byte sizes. |

Derive related limits from one authoritative constant rather than repeating
numbers. Keep enum values explicit and consistently typed.

## Formatting

- Use standard Go formatting.
- Group imports into standard library, third-party dependencies, and project
  packages, with blank lines between groups.
- Separate multiline statements and nearby control-flow statements with blank
  lines at the same indentation level.
- Group related variable declarations. Put declarations of the same type on
  one line within the group and a blank line after the group.
- Break signatures with a context and additional parameters across lines, one
  parameter per line with a trailing comma. A context-only signature stays on
  one line.
- Break constructors and calls with many fields or arguments across lines.
- When declaring a result immediately before assigning it, keep the two lines
  together.
- Separate an error assignment from a following loop exit with a blank line.

## Comments

Use names, section banners, and sub-headers to carry the code's structure;
do not add doc comments to exported identifiers. Explain behavior the code
cannot show, rather than restating what a line does. Put such explanations in
the sub-header position as full sentences ending with a period.

## Domain values and aggregates

Use named domain types for domain primitives, including collection values.
Aggregate fields use those types; timestamps and boolean flags retain their
ordinary representations where appropriate.

Domain values expose explicit validation. Scalar and collection values use
value receivers; aggregates use pointer receivers. Constructors take fields
in declaration order and only assign them. Validation, identity generation,
and defaults are separate responsibilities governed by the architecture.

Validate without changing the value. For text, check blankness before invalid
content and length. Match the measurement to the declared unit: character
limits count characters, and byte limits count bytes. Check aggregate fields
in declaration order, starting with the receiver's presence. Stop at the first
failure and report it consistently. Apply collection checks in stable order.

Reuse a constituent value's validation before adding constraints for the
containing value. Keep allowed values and business limits in their owning
domain rather than scattering them through callers.

## Errors

### Error contracts

- Name sentinels `Err<Subject><Problem>`.
- Give each validated type a top-level invalid-value error. Declare it before
  its more specific errors and order those errors by their validation checks.
- Keep leaf errors distinct for each subject, even when their messages match.
- Use short, lower-case messages. Validation details should read naturally
  after the containing subject's error; build limit messages from named limits.
- Preserve both the containing validation error and its specific cause.
- Preserve underlying causes when adding context or mapping infrastructure
  failures to domain errors.

### Handling and context

Compare errors by identity or type rather than message text or direct equality.
Handle an error once: either return it with context or log it where it is
handled. Do not log and return the same error.

Every public operation's returned error identifies the operation. Context is a
lower-case verb phrase naming the failed step and the known entity identity or
key. Private helpers may return a cause unchanged when their caller supplies
the operation context. Do not repeat context at every internal step.

Use the phrase `failed to <verb> a <entity>` for typed write-failure messages,
not additional context. Read-operation validation failures receive operation
context; write-operation validation failures are carried by the typed failure
that already identifies the operation.

### Typed write failures

Repository create and update failures expose the submitted aggregate. Delete
failures expose its identity. Each failure preserves its underlying cause.
Share common failure behavior privately and keep operation-specific
implementations unexported. Use failure constructors consistently, and ensure
a successful operation returns a genuinely empty error interface.

Map storage failures consistently. When a storage failure identifies a domain
constraint, preserve both meanings without discarding the original cause.

## Control flow

Prefer explicit control flow with result variables declared at the beginning
and one return at the end. Avoid early and naked returns, including in short
validators and enum checks. Do not panic outside entry points and package
initialization.

Use a conditional chain when guards yield different errors. When several steps
share one result error, perform each step only while no error has occurred and
add final context once. Keep step-specific errors scoped to their step.

Stop loops on the first error. Clear partial returned values on failure so
callers cannot mistake incomplete results for successful ones.

## Interfaces

Interface ownership follows [file layout](layout.md#interface-placement).
Keep consumer interfaces narrow. Domain contracts may contain all operations
required by the domain.

Return concrete types from functions and constructors. Repository write
methods are the exception: they return their domain's typed failure contract
so callers can inspect the operation's input without depending on the adapter.
Verify adapter conformance to its domain contract at compile time.

## Repository methods and helpers

Keep repository method names and parameter names consistent with the domain
contract. Separate public operations from private query, row-mapping, write,
and error-mapping helpers. Keep shared column definitions and row mapping in
the same order, and reuse selection criteria for both listing and counting.

Check receiver and connection presence, argument presence, domain validity,
and operation preconditions before I/O, in that order. Validate reconstituted
domain values before returning them.

Keep query construction parameterized and separate from domain behavior.

Respect database-owned timestamps and established concurrency checks. A
concurrent change must not be silently overwritten where the aggregate's
contract requires conflict detection.

Use named, validated filter, sort, and query values. Keep optional criteria
explicit, apply the same filters to counts and results, and use a stable
tie-breaker for ordered results.

## Concurrency and resource lifetime

Every goroutine has a guaranteed exit path. Blocking operations must be able
to finish or be cancelled. Prefer coordinated cancellation and error handling
over hand-built goroutine bookkeeping.

Keep context as the first parameter of I/O operations and pass it through.
Register cleanup when acquiring a resource. Make ordering deterministic when
traversing unordered collections for output, validation, or other ordered work.

Prefer current standard-library facilities and structured logging. Dependency
wiring belongs in entry points as described in [file layout](layout.md).
