# Go code style

This guide is the single source of Go coding conventions for the project. It
combines the project-wide rules that used to live in [AGENTS.md](../AGENTS.md)
with the conventions of the code that has already been refactored.
[architecture.md](architecture.md) takes precedence where the two conflict and
isn't repeated here. Known deviations in the code are listed in
[Inconsistencies / open questions](#inconsistencies--open-questions).

Reference files (the source for the codebase-specific rules):

- `internal/languages/language.go`, `text.go`, `language_repository.go`
- `internal/languages/pgxgoqu/language_repository.go`
- `internal/security/user.go`, `role.go`, `password.go`
- `internal/security/user_repository.go`, `role_repository.go`
- `internal/security/pgxgoqu/user_repository.go`, `role_repository.go`

All references below are `path:line`. Adapter paths are shortened to
`languages/pgxgoqu/…` and `security/pgxgoqu/…`, both under `internal/`.

---

## 1. File layout

### Files

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

### Section banners

- Separate top-level sections with a three-line banner:

  ```go
  //
  // Alpha-2 code
  //
  ```

  (`internal/languages/language.go:11-13`)
- In domain files, give each named type its own banner, named in sentence case
  after the concept: `// User ID`, `// First name`, `// Password hash`
  (`internal/security/user.go:15-17`, `user.go:70-72`, `password.go:50-52`).
- Put the aggregate's banner last, after all its value types
  (`internal/security/user.go:136`, `internal/languages/language.go:106`).
- In adapter files, use exactly these banners, in this order: `Errors`,
  `Repository`, `Helpers` (`languages/pgxgoqu/language_repository.go:18`, `:95`,
  `:378`).
- In a domain `_repository.go` that defines query types, use the banners
  `Filter`, `Sort`, `Query`, `Repository` in that order
  (`internal/security/role_repository.go:13`, `:80`, `:102`, `:114`).

### Sub-headers

- Inside the `Repository` section, put a one-line comment, followed by a blank
  line, above each method. It names the operation in sentence case with no
  period and uses articles:

  ```go
  // Find by a code

  func (r *LanguageRepository) FindByCode(
  ```

  (`languages/pgxgoqu/language_repository.go:247-249`). Other examples:
  `// Create`, `// Find by an ID`, `// Find the fallback language`,
  `// Count roles matching filters`.
- Inside the `Errors` section, put `// Implementation of <pkg>.<Interface>`
  above each typed failure implementation
  (`languages/pgxgoqu/language_repository.go:40`, `:56`, `:72`).
- A sub-header may become a full-sentence explanation when the method relies
  on behaviour that isn't obvious, such as database triggers or concurrency
  tokens. See [Comments](#10-comments).

### Order inside a section

- Value-type sections: constants, then the `var` block of errors (with any
  regexps), then the type, then `Validate`. This is the order in
  `internal/security/user.go:74-85` and `password.go:15-30`. The `languages`
  package puts the type first; see open question 3.
- Constants of an enum-like type come right after the type
  (`internal/security/role.go:23-30`, `role_repository.go:84-90`).
- Aggregate sections: error `var` block, struct, `New<Entity>`, `Validate`
  (`internal/security/role.go:120-163`).
- Domain repository section: sentinels, then `Err<Entity>CreateFailed`,
  `UpdateFailed`, `DeleteFailed`, then the repository interface
  (`internal/security/user_repository.go:12-42`).
- Adapter `Repository` section: the DB interface (if it's local to the file),
  then the struct, the `var _` check, `New<X>Repository`, then the methods in
  the interface's order (`languages/pgxgoqu/language_repository.go:99-376`).
- Adapter `Helpers` section: the private lookup or write helpers, then
  `<entity>Columns`, query builders, `scan<Entity>`, `map<Entity>Error`
  (`security/pgxgoqu/user_repository.go:314-425`).

---

## 2. Domain value types

- Wrap every domain primitive in a named type. Never use a bare `string` or
  `uuid.UUID` in an aggregate field.

  ```go
  type Email string
  type UserID uuid.UUID
  type RoleName languages.Text
  ```

  (`internal/security/user.go:39`, `:21`, `internal/security/role.go:95`)
- Give each type a value-receiver `Validate() error`, including slices and
  maps (`Permissions`, `Text`): `internal/security/role.go:43`,
  `internal/languages/text.go:48`.
- Declare limits as exported `Max<Subject><Unit>` / `Min<Subject><Unit>`
  constants. Use `Length` for a rune count and `Bytes` for a byte count:

  ```go
  const (
  	MinPasswordLength = 6
  	MaxPasswordBytes  = 4096
  )
  ```

  (`internal/security/password.go:15-18`). Derive one limit from another
  instead of repeating the number:
  `MaxRoleFilterNameLikeLength = MaxRoleNameLength`
  (`internal/security/role_repository.go:19`).
- Measure characters with `utf8.RuneCountInString` and bytes with `len`
  (`internal/security/password.go:37-39`).
- Check in this order: blank (`strings.TrimSpace(...) == ""`), then invalid
  UTF-8 or characters, then length:

  ```go
  if strings.TrimSpace(string(n)) == "" {
  	err = ErrFirstNameEmpty
  } else if !utf8.ValidString(string(n)) {
  	err = ErrFirstNameInvalidChars
  } else if utf8.RuneCountInString(string(n)) > MaxFirstNameLength {
  	err = ErrFirstNameTooLong
  }
  ```

  (`internal/security/user.go:88-94`)
- After the checks, wrap the leaf in the type's `Err<Subject>Invalid`. See
  [Errors](#4-errors).
- Treat a zero UUID as an invalid ID; any UUID version is accepted
  (`internal/security/user.go:26`).
- Put a regexp the type depends on in its `var` block, unexported, compiled
  with `regexp.MustCompile` (`internal/security/user.go:55-58`).
- For an enum, give the constants explicit types and validate with a `switch`
  over every value (`internal/security/role.go:25-39`).
- Validate a collection by validating its elements in a stable order: sort map
  keys or values with `slices.Sorted(maps.Keys(...))` and stop at the first
  failure (`internal/languages/text.go:59-71`).
- Validate a type built on another type by converting it and delegating, then
  adding its own checks: `languages.Text(r).Validate()`
  (`internal/security/role.go:99`).

---

## 3. Aggregates

- Make the aggregate an exported struct of exported fields typed with the
  domain value types. Use `time.Time` for timestamps and `bool` for flags
  named `Is<Adjective>` (`IsFallback`, `IsSuper`):
  `internal/security/user.go:145-158`, `internal/security/role.go:125-130`.
- Provide `New<Entity>(...) *<Entity>` that takes every field as a parameter
  in field order and only assigns them. It doesn't validate, generate IDs or
  set defaults (`internal/security/user.go:160-188`). Callers validate before
  side effects ([architecture.md](architecture.md#validation-boundaries)).
- Give `Validate` a pointer receiver. Check for a nil receiver first, then
  validate fields in declaration order, then wrap once:

  ```go
  func (r *Role) Validate() error {
  	var err error

  	if r == nil {
  		err = ErrRoleNil
  	} else {
  		err = r.ID.Validate()
  	}

  	if err == nil {
  		err = r.Name.Validate()
  	}
  	...
  	if err != nil {
  		err = fmt.Errorf("%w: %w", ErrRoleInvalid, err)
  	}

  	return err
  }
  ```

  (`internal/security/role.go:141-163`)
- Don't validate timestamps and booleans; they have no `Validate`
  (`internal/security/user.go:190-228`).

---

## 4. Errors

### General rules

- Wrap with context and `%w`: `fmt.Errorf("load user %s: %w", id, err)`.
  Never use `%v` for an error, because it breaks `errors.Is` for every caller.
- Compare with `errors.Is` / `errors.As` (or `errors.AsType`), never `==`.
- Handle an error once: add context and return it. Never log and return the
  same error.
- Never use naked returns. Never `panic` outside `main()` and package `init`.
- Add context at the boundary of a public operation: every error a public
  method returns names the operation. Private helpers and `scan<Entity>` may
  return an error unchanged when their caller adds the context. See
  [Context messages](#context-messages).

### Names and messages

- Name sentinels `Err<Subject><Problem>`: `ErrFirstNameEmpty`,
  `ErrRoleNameTooLong`, `ErrUserNotFound`, `ErrFallbackLanguageAlreadyInUse`.
- Give every type one top-level `Err<Subject>Invalid` with the message
  `"invalid <subject>"`, in lower case (`"invalid user ID"`, `"invalid English
  name"`), declared first in the block (`internal/security/user.go:77`).
- Write leaf messages as short predicates that read correctly after the
  subject's message:

  | Leaf | Message |
  | --- | --- |
  | `Err<X>Empty` | `"is empty"` |
  | `Err<X>Nil` | `"is nil"` |
  | `Err<X>InvalidChars` | `"invalid characters"` |
  | `Err<X>TooLong` | `fmt.Errorf("exceeds the limit of %d characters", Max…)` |
  | `Err<X>TooShort` | `fmt.Errorf("less than %d characters", Min…)` |

  (`internal/security/user.go:76-81`, `password.go:20-26`)
- Declare a separate leaf sentinel for each type, even when the message
  matches another type's. `ErrFirstNameEmpty` and `ErrLastNameEmpty` are
  different values (`internal/security/user.go:78`, `:111`).
- Build a limit message from its constant with `fmt.Errorf` at package level.
  Never hard-code the number (`internal/security/user.go:79`).
- Name repository sentinels `<entity> not found`, `<entity> already exists`,
  or describe the state: `"user changed since it was loaded"`,
  `"role is assigned to users"`
  (`internal/security/user_repository.go:12-16`, `role_repository.go:118-122`).

### Wrapping chain

- Wrap a leaf in the type's top-level error with `%w: %w`, so both stay
  matchable:

  ```go
  if err != nil {
  	err = fmt.Errorf("%w: %w", ErrEnglishNameInvalid, err)
  }
  ```

  (`internal/languages/language.go:66-68`)
- The aggregate wraps again in the same way, which gives messages like
  `invalid user: invalid first name: is empty`
  (`internal/security/user.go:223-225`).

### Typed write failures

- For each write method, declare an interface in the domain
  `_repository.go`. It embeds `error`, exposes the operation's input and
  `Unwrap() error`:

  ```go
  type ErrUserCreateFailed interface {
  	error
  	User() *User
  	Unwrap() error
  }
  ```

  (`internal/security/user_repository.go:18-22`)
- Create and Update expose the aggregate (`User() *User`). Delete exposes the
  identifier, through an accessor named after its type: `UserID() UserID`,
  `LanguageCode() Code` (`internal/security/user_repository.go:30-34`,
  `internal/languages/language_repository.go:27-31`).
- Have write methods return the interface type, not `error`:
  `Create(ctx context.Context, user *User) ErrUserCreateFailed`
  (`internal/security/user_repository.go:37`).

### Implementing typed failures in adapters

- Share the accessor and `Unwrap` in an unexported `err<Entity>WriteFailed`
  struct with value receivers (`security/pgxgoqu/user_repository.go:26-37`).
- Embed it in one unexported struct per operation, which adds only a
  pointer-receiver `Error()` returning `"failed to <verb> a <entity>"`:

  ```go
  type errUserCreateFailed struct {
  	errUserWriteFailed
  }

  func newErrUserCreateFailed(user *security.User, err error) *errUserCreateFailed {
  	return &errUserCreateFailed{
  		errUserWriteFailed: errUserWriteFailed{user, err},
  	}
  }

  func (e *errUserCreateFailed) Error() string {
  	return "failed to create a user"
  }
  ```

  (`security/pgxgoqu/user_repository.go:41-53`)
- The delete failure carries an ID instead of an aggregate, so declare its
  fields directly without the embedded struct
  (`security/pgxgoqu/user_repository.go:73-92`).
- Always build failures through `newErr<Entity><Op>Failed(...)`. Never use a
  struct literal at the call site.
- Hold the result in a variable of the domain interface type so that the
  nil-interface return stays nil:
  `var err security.ErrUserCreateFailed` (`security/pgxgoqu/user_repository.go:125`).

### Mapping PostgreSQL errors

- Put the mapping in one `map<Entity>Error(err error) error` per adapter.
  Extract `*pgconn.PgError` with `errors.AsType`, switch on `Code` together
  with `ConstraintName`, and attach the domain sentinel with `errors.Join` so
  that both the sentinel and the driver error stay matchable:

  ```go
  if postgresErr, ok := errors.AsType[*pgconn.PgError](err); ok {
  	switch {
  	case postgresErr.Code == "23503" && postgresErr.ConstraintName == "user_role_id_fkey":
  		result = errors.Join(security.ErrRoleNotFound, err)
  ```

  (`security/pgxgoqu/user_repository.go:415-418`)
- Return the input error unchanged when no case matches (`result := err`).
- Turn `pgx.ErrNoRows` into `Err<Entity>NotFound`
  (`languages/pgxgoqu/language_repository.go:423-424`).

### Context messages

- Write context as a lower-case verb phrase without `failed to`. Include the
  entity's ID or key when it's known:
  `fmt.Errorf("delete role %s: %w", storedID, ...)`,
  `fmt.Errorf("build find role %s query: %w", storedID, buildErr)`
  (`security/pgxgoqu/role_repository.go:196`, `:230`).
- Name the step that failed: `build … query`, `bind … query`, `begin …`,
  `read <entities>`, `check user after unsuccessful update`
  (`security/pgxgoqu/role_repository.go:191`, `:374`, `:348`,
  `user_repository.go:220`).
- Use `failed to <verb> a <entity>` only as the `Error()` text of a typed
  write failure (`security/pgxgoqu/user_repository.go:52`), never in a
  context message.
- Wrap a validation error in a read method too:
  `fmt.Errorf("find role by ID: %w", validationErr)`
  (`security/pgxgoqu/role_repository.go:218`). In a write method, pass it to
  the typed failure unwrapped, because the failure type already names the
  operation.
- Adapter sentinels describe the missing thing: `"nil PostgreSQL pool"`,
  `"nil user"` (`languages/pgxgoqu/language_repository.go:23`,
  `security/pgxgoqu/user_repository.go:24`).

---

## 5. Control flow

- Avoid early returns; prefer explicit control flow. This applies to every
  function, including a one-check `Validate` and a `switch` over enum values.
  The patterns below show how.
- Declare the result variables at the top, assign them along the way, and
  have a single `return` as the last statement:

  ```go
  func (c Code) Validate() error {
  	var err error

  	if ... {
  		err = ErrCodeInvalidChars
  	} else if ... {
  		err = ErrCodeWrongLength
  	}
  	...
  	return err
  }
  ```

  (`internal/languages/language.go:24-38`)
- When each guard yields a different error, use an `if / else if` chain and
  scope the step's error to its condition (`validationErr :=`):

  ```go
  if r == nil || r.pool == nil {
  	err = newErrLanguageCreateFailed(language, ErrNilPool)
  } else if language == nil {
  	err = newErrLanguageCreateFailed(language, languages.ErrLanguageNil)
  } else if validationErr := language.Validate(); validationErr != nil {
  	err = newErrLanguageCreateFailed(language, validationErr)
  } else {
  	...
  }
  ```

  (`languages/pgxgoqu/language_repository.go:134-163`)
- When the steps share one error and are wrapped once at the end, chain them
  with `if err == nil { ... }` and finish with a single
  `if err != nil { wrap }`:

  ```go
  if err == nil {
  	err = writeRoleTranslations(ctx, tx, id, nameID, role.Name)
  }

  if err == nil {
  	err = tx.Commit(ctx)
  }

  if err != nil {
  	err = fmt.Errorf("create role %s: %w", id, err)
  }
  ```

  (`security/pgxgoqu/role_repository.go:416-426`)
- Use `break` only to leave a loop on the first error. When a statement comes
  before `break` in the same block, put a blank line between them:

  ```go
  if err != nil {
  	err = fmt.Errorf("write name translation for role %s: %w", id, err)

  	break
  }
  ```

  (`security/pgxgoqu/role_repository.go:578-582`, `internal/languages/text.go:62-64`)
- After a failure in a function that also returns a value, set that value to
  nil so a partial result never leaks: `language = nil`, `result = nil`
  (`languages/pgxgoqu/language_repository.go:439`, `:369`).

---

## 6. Repositories

### Contract

- Declare the repository interface in the domain package as
  `<Entity>Repository` (see [architecture.md](architecture.md) and
  [Interfaces](#11-interfaces)). Use the
  method names `Create`, `Update`, `Delete`, `FindBy<Key>`,
  `FindBy<Key>ForUpdate`, `Find`, `Count`, `FindAll`
  (`internal/security/role_repository.go:142-150`,
  `internal/languages/language_repository.go:33-41`).
- Have reads return `(*<Entity>, error)` or `([]*<Entity>, error)`.

### Adapter construction

- Add a compile-time check right after the struct:
  `var _ languages.LanguageRepository = (*LanguageRepository)(nil)`
  (`languages/pgxgoqu/language_repository.go:109`).
- Store the connection in a field named `pool`, typed as a small unexported
  interface (`languageDB`) so that either a `*pgxpool.Pool` or a `pgx.Tx`
  can be used (`languages/pgxgoqu/language_repository.go:99-107`).
- Have `New<X>Repository(pool *pgxpool.Pool) (*<X>, error)` reject a nil pool
  with `ErrNilPool` (`languages/pgxgoqu/language_repository.go:111-124`).

### Guards before I/O

- Start every method with the same guard chain, in this order:
  1. nil receiver or pool: `if r == nil || r.pool == nil`
  2. nil aggregate argument
  3. `Validate()` on the argument (aggregate, ID, code, email, filter or query)
  4. any operation-specific precondition (zero `UpdatedAt`, transaction
     required)
  5. the I/O itself, in the final `else`

  (`security/pgxgoqu/user_repository.go:179-187`,
  `languages/pgxgoqu/language_repository.go:284-291`)
- Never send unvalidated input to the database, and validate every row that
  comes back. `scan<Entity>` calls `Validate()` on the rebuilt aggregate and
  returns `nil` when it fails (`security/pgxgoqu/user_repository.go:402-406`).

### goqu

- Build all SQL with `goqu.Dialect("postgres")`, end every chain with
  `.Prepared(true).ToSQL()`, and put one call on each line:

  ```go
  query, args, buildErr := goqu.Dialect("postgres").
  	Delete("language").
  	Where(goqu.Ex{"code": string(code)}).
  	Prepared(true).
  	ToSQL()
  ```

  (`languages/pgxgoqu/language_repository.go:225-229`)
- Convert domain values to primitives explicitly where they go into the query:
  `string(user.Email)`, `uuid.UUID(id).String()`.
- Use `goqu.Ex` for equality, `goqu.L` for SQL fragments with `?`
  placeholders, and `goqu.I` for identifiers
  (`security/pgxgoqu/user_repository.go:304`).
- Use the table name in the singular (`"user"`, `"role"`, `"language"`) and
  quote reserved names in literal SQL (`"user"`, `"text"`).
- Bind UUIDs in two steps. Pass `id.String()` to goqu, then replace the
  placeholder arguments with the raw bytes via `bindUUIDArgs` or
  `bindUUIDArgsAt` (defined outside the reference set):

  ```go
  } else if bindErr := bindUUIDArgs(args, [16]byte(rawID)); bindErr != nil {
  ```

  (`security/pgxgoqu/user_repository.go:254`)
- Scan UUID columns into `pgtype.UUID` and convert with
  `security.UserID(id.Bytes)` (`security/pgxgoqu/user_repository.go:363`, `:389`).
- Escape user input in `LIKE` patterns with `likeSubstring`
  (`security/pgxgoqu/role_repository.go:531-533`).

### Row helpers

- Return the column list from `<entity>Columns() []any`, in the same order as
  `scan<Entity>` reads it (`security/pgxgoqu/user_repository.go:343-358`).
- Give `scan<Entity>(row pgx.Row) (*<Entity>, error)` a `pgx.Row` parameter
  so it works for both `QueryRow` and `rows` in a loop
  (`languages/pgxgoqu/language_repository.go:354`, `:407`).
- Inside `scan<Entity>`, scan into local primitives, rebuild with
  `New<Entity>`, then `Validate`.
- Build the shared `WHERE` clause for `Find` and `Count` in one function,
  `<entity>FilterQuery(filter) *goqu.SelectDataset`
  (`security/pgxgoqu/role_repository.go:506-529`).

### Writes

- Detect a duplicate on create with `OnConflict(goqu.DoNothing())` plus
  `RowsAffected() == 0` (or `ErrNoRows` with `Returning`), and turn it into
  `Err<Entity>AlreadyExists` (`security/pgxgoqu/user_repository.go:148-163`).
- Treat `RowsAffected() == 0` on update or delete as
  `Err<Entity>NotFound` (`languages/pgxgoqu/language_repository.go:238-239`).
- Run every external call's error through `map<Entity>Error`.

### Transactions

- Put a multi-statement write in a private `<verb>Valid<Entity>` helper. Call
  it from the public method only after validation
  (`security/pgxgoqu/role_repository.go:137`, `:365`).
- Begin the transaction, register the rollback with `defer` straight away, make
  `Commit` the last step of the `if err == nil` chain, and wrap once:

  ```go
  tx, err := r.pool.Begin(ctx)

  if err != nil {
  	err = fmt.Errorf("begin create role %s: %w", id, err)
  } else {
  	defer func() { _ = tx.Rollback(ctx) }()
  ```

  (`security/pgxgoqu/role_repository.go:371-376`)
- Have `FindBy<Key>ForUpdate` add `ForUpdate(exp.Wait)` and refuse to run
  outside a transaction:
  `else if _, transactional := r.pool.(pgx.Tx); !transactional { err = ErrTransactionRequired }`
  (`languages/pgxgoqu/language_repository.go:288-289`, `:398-400`).

### Optimistic concurrency

- Use the loaded `UpdatedAt` as the concurrency token. Reject a zero
  `UpdatedAt` before I/O, put `updated_at` in the `WHERE`, and on
  `RowsAffected() == 0` reload by ID to tell `NotFound` (returned by the
  reload) from `Err<Entity>Conflict`
  (`security/pgxgoqu/user_repository.go:185-224`).
- Never write timestamps from Go. Database triggers own them
  (`security/pgxgoqu/user_repository.go:170-171`).

---

## 7. Filters, sorting and queries

- `<Entity>Filter` is a value struct of optional criteria. Strings are
  `<Field>Like`, a zero value means "not applied", and a tri-state flag is a
  `*bool` (`internal/security/role_repository.go:30-35`).
- Give the filter a `Validate()` with its own `Err<Entity>FilterInvalid` and
  leaf errors named per field (`ErrRoleFilterIDLikeTooLong`), with limits in
  `Max<Entity>Filter<Field>Length` constants
  (`internal/security/role_repository.go:17-28`).
- Give the filter `Count() int`, which returns how many criteria are applied
  and satisfies `domquery.Filter`. Count them over a `[]bool` literal:

  ```go
  for _, applied := range []bool{
  	f.IDLike != "",
  	f.NameLike != "",
  	len(f.Permissions) > 0,
  	f.IsSuper != nil,
  } {
  ```

  (`internal/security/role_repository.go:63-78`)
- `<Entity>Sort` is a `string` type whose values are the column names, with a
  `Validate()` that accepts only the declared constants
  (`internal/security/role_repository.go:84-100`).
- Define `<Entity>Query` as a named instance of the generic query:
  `type RoleQuery domquery.Query[RoleFilter, RoleSort]`
  (`internal/security/role_repository.go:108`). `domquery.Query` (in
  `pkg/domquery`) holds `Filter`, `SortOrder`, `SortBy`, `Limit` and `Page`.
- Order by the requested sort, then add `id ASC` as a tie-breaker unless the
  sort is already `id` (`security/pgxgoqu/role_repository.go:302-312`).
- Count on the same filter dataset with `goqu.COUNT("*")`
  (`security/pgxgoqu/role_repository.go:261-264`).

---

## 8. Naming

### Receivers

- Use the first letter of the type: `r` for repositories, `e` for error types,
  `f` for filters, `s` for sorts, `q` for queries, `l`/`u` for aggregates.
- Use `id` for ID types and `n` for `…Name` types:
  `func (id UserID) Validate()`, `func (n FirstName) Validate()`
  (`internal/security/user.go:23`, `:85`).

### Parameters

- Name parameters after their domain type in full: `language`, `user`,
  `role`, `code`, `id`, `email`. Use `ctx` for the context.
- Name a parameter that holds a predicate after its role: `filter goqu.Ex`,
  `predicate exp.Expression`, `forUpdate bool`
  (`languages/pgxgoqu/language_repository.go:382-386`).

### Locals

| Name | Use | Reference |
| --- | --- | --- |
| `err` | the single result error | everywhere |
| `query, args, buildErr` | output of `ToSQL()` | `languages/pgxgoqu/language_repository.go:143` |
| `dataset` | a goqu dataset before `ToSQL()` | `languages/pgxgoqu/language_repository.go:392` |
| `tag, execErr` | output of `Exec` | `languages/pgxgoqu/language_repository.go:199` |
| `rows, queryErr` | output of `Query` | `languages/pgxgoqu/language_repository.go:343` |
| `validationErr`, `bindErr`, `scanErr`, `findErr` | step errors scoped to an `if` | `security/pgxgoqu/user_repository.go:131`, `:154`, `:217` |
| `storedID` | a domain ID converted to `uuid.UUID` for the database | `security/pgxgoqu/role_repository.go:180` |
| `postgresErr` | the extracted `*pgconn.PgError` | `security/pgxgoqu/user_repository.go:415` |
| `result` | the value returned by a mapper | `security/pgxgoqu/user_repository.go:413` |
| `index` | slice index in loops (never `i`) | `security/pgxgoqu/role_repository.go:539` |

### Imports

- Use three groups, separated by blank lines: standard library, third-party,
  then `github.com/radynsade/faryengo/...`:

  ```go
  import (
  	"context"
  	...

  	"github.com/doug-martin/goqu/v9"
  	...

  	"github.com/radynsade/faryengo/internal/security"
  )
  ```

  (`security/pgxgoqu/user_repository.go:3-18`)
- Import the goqu PostgreSQL dialect with a blank import in the adapter
  package: `_ "github.com/doug-martin/goqu/v9/dialect/postgres"`
  (`languages/pgxgoqu/language_repository.go:9`).

---

## 9. Formatting

- Put a blank line between multiline operations (statements ending in `;`) at
  the same indentation level.
- Separate control-flow statements (`if`, `for`, `switch`, `select`, etc.) from
  nearby statements with blank lines when they are at the same indentation
  level:

  ```go
  tag, execErr := r.pool.Exec(ctx, query, args...)

  if execErr != nil {
  ```

  (`languages/pgxgoqu/language_repository.go:199-201`)
- Break a signature across lines, one parameter per line with a trailing
  comma, when the function takes `ctx` and at least one other parameter,
  even if the line would be short:

  ```go
  func (r *LanguageRepository) Delete(
  	ctx context.Context,
  	code languages.Code,
  ) languages.ErrLanguageDeleteFailed {
  ```

  (`languages/pgxgoqu/language_repository.go:214-217`). Keep signatures with
  only `ctx` on one line (`FindAll(ctx context.Context)`, `:324`).
- Break a constructor that takes many fields in the same way
  (`internal/security/user.go:160-173`).
- Put each argument on its own line when a call takes many arguments
  (`NewUser(...)`, `row.Scan(...)`):
  `security/pgxgoqu/user_repository.go:367-380`, `:387-400`.
- Group several declarations into one `var ( ... )` block with aligned names,
  and put a blank line after it (`languages/pgxgoqu/language_repository.go:253-256`).
- Declare variables of the same type on one line inside the block:
  `code, english, native string` (`languages/pgxgoqu/language_repository.go:417`).
- Declare a loop's result variable on the line before the assignment that
  uses it, with no blank line:

  ```go
  var language *languages.Language
  language, err = scanLanguage(rows)
  ```

  (`languages/pgxgoqu/language_repository.go:353-354`)
- Put a blank line between a multiline goqu chain and the `if` that checks its
  error (`languages/pgxgoqu/language_repository.go:152-154`).

---

## 10. Comments

- Don't write doc comments on exported identifiers. Names, banners and
  sub-headers carry the structure. None of the reference files has a doc
  comment.
- Write an explanatory comment only for behaviour the code can't show, such as
  database triggers, foreign keys and concurrency tokens. Write it in the
  sub-header position as full sentences ending with a period:

  ```go
  // Delete relies on PostgreSQL's foreign key to reject roles assigned to users.
  // The existing delete_role_name trigger removes the name and its translations
  // in the same statement, so a rejected deletion leaves them intact.

  func (r *RoleRepository) Delete(
  ```

  (`security/pgxgoqu/role_repository.go:165-169`, `user_repository.go:170-173`)
- Never comment on what a line does.

---

## 11. Interfaces

- Declare an interface in the consumer's package and keep it small: one or two
  methods. Return concrete types from functions and constructors:
  `NewLanguageRepository(...) (*LanguageRepository, error)`
  (`languages/pgxgoqu/language_repository.go:111`).
- Exception, following [architecture.md](architecture.md): a domain package
  declares the contracts that infrastructure implements. A repository
  interface (`<Entity>Repository`) or a domain service port
  (`PasswordHasher`, `internal/security/password.go:79-82`) lives in the
  domain package and lists every operation the domain exposes, however many
  that is (`internal/languages/language_repository.go:33-41`).
- Exception: repository write methods return their typed failure interface
  (`Err<Entity><Op>Failed`) instead of a concrete type, so callers get the
  operation's input without importing the adapter
  (`internal/security/user_repository.go:37-39`). Adapters implement them with
  unexported structs (see [Typed write failures](#typed-write-failures)).
- Inside an adapter, declare the narrow DB interface it needs next to the
  repository, not a shared driver interface
  (`languages/pgxgoqu/language_repository.go:99-103`).

---

## 12. Concurrency

- Give every goroutine a guaranteed exit path. If it can block on a send,
  buffer the channel or give it a context.
- Prefer `errgroup.WithContext` over a hand-written `WaitGroup` plus channels.
- Make `context.Context` the first parameter of anything doing I/O, and pass
  it through. Never accept it and drop it
  (`languages/pgxgoqu/language_repository.go:157`).
- Never range a map to produce output or to do ordered work. Sort the keys
  first: `for _, code := range slices.Sorted(maps.Keys(t))`
  (`internal/languages/text.go:59`, `security/pgxgoqu/role_repository.go:556`).

---

## 13. Standard library and wiring

- Use the current standard library: `os.ReadFile`, not `ioutil`; `any`, not
  `interface{}`; the `slices` and `maps` packages; `log/slog`, not logrus;
  `math/rand/v2`; `errors.AsType`
  (`security/pgxgoqu/user_repository.go:415`).
- Don't use a dependency injection framework. Wire dependencies explicitly in
  `main()`.

---

## 14. Tests

- Write table-driven tests with `t.Run` subtests.
- Call `goleak.VerifyTestMain` from `TestMain` in every package that has
  tests.

---

## Inconsistencies / open questions

Each item names the conflicting locations and recommends one option.
Locations use the same short paths as above. **Rec** marks the
recommendation.

### Former conflicts with AGENTS.md (resolved)

The rules that were in AGENTS.md now live in this guide, and these conflicts
are settled. Items 1–3 are code that doesn't follow the guide yet and still
needs a Go change. Item 4 changed the rule.

1. **Early returns. Resolved: the rule stands.** See
   [Control flow](#5-control-flow). The code that still breaks it is
   `Email.Validate`, `Phone.Validate` and `Permission.Validate`
   (`internal/security/user.go:41-49`, `:62-68`, `internal/security/role.go:32-39`).
   Rewrite them with a result variable.
2. **Context message format. Resolved: lower-case verb phrases, no
   `failed to`.** See [Context messages](#context-messages). The code that
   still breaks it is the languages adapter
   (`languages/pgxgoqu/language_repository.go:261`, `:266`, `:341`, `:346`,
   `:368`, `:405`, `:438`).
3. **Errors returned without context. Resolved: public methods always add
   context; private helpers and `scan<Entity>` may pass errors through.** See
   [General rules](#general-rules). The code that still breaks it is
   `UserRepository.FindByID` and `FindByEmail`
   (`security/pgxgoqu/user_repository.go:282`, `:302`) and
   `RoleRepository.Count` and `Find` (`security/pgxgoqu/role_repository.go:259`,
   `:288`). `scanUser` and `scanRole` are now allowed.
4. **Interface size and return types. Resolved: the rule now has explicit
   exceptions.** See [Interfaces](#11-interfaces). Domain repository and port
   interfaces live in the domain package and may be as large as the domain
   needs. Repository write methods return their typed failure interfaces. No
   code change is needed.

### File layout and comments

5. **Banner style.** `text.go` uses one-line `// Translation` / `// Text` for
   top-level sections (`internal/languages/text.go:12`, `:38`). Every other
   domain file uses the three-line banner (`internal/languages/language.go:11-13`).
   **Rec:** use the three-line banner.
6. **Repository file banner.** `internal/languages/language_repository.go` has
   no banner, `internal/security/user_repository.go:8-10` uses
   `// User repository`, and `internal/security/role_repository.go:114-116`
   uses `// Repository`. **Rec:** always use `// Repository`.
7. **Order inside a value-type section.** The languages files put the type
   first, then the constant and `var` block (`internal/languages/language.go:44-53`).
   Security puts the constant and `var` block first
   (`internal/security/user.go:74-83`, `:19-21`). **Rec:** constants, `var`,
   type, methods (security's order, used by 3 of 4 domain files).
8. **Order of leaf errors in a `var` block.** It's `Invalid, InvalidChars,
   Empty, TooLong` in `internal/languages/language.go:48-53`, and
   `Invalid, Empty, TooLong, InvalidChars` in `internal/security/user.go:76-81`.
   **Rec:** `Invalid` first, then the leaves in the order `Validate` checks
   them.
9. **Method order in the role adapter.** `Count` comes before `Find`
   (`security/pgxgoqu/role_repository.go:249`, `:278`), but the interface
   lists `Find` first (`internal/security/role_repository.go:148-149`).
   **Rec:** follow the interface order.
10. **Explanatory comments aren't doc comments.** The blank line between the
    explanation and `func` (`security/pgxgoqu/user_repository.go:170-173`,
    `role_repository.go:165-169`) means godoc and linters don't see it.
    **Rec:** keep this, since the codebase deliberately has no doc comments,
    but decide explicitly.

### Value types and validation

11. **Leaf errors on single-check types.** `UserID`, `RoleID`, `Email`,
    `Phone` and `Permission` return `ErrXInvalid` directly with no leaf
    (`internal/security/user.go:23-31`, `:41-49`, `:62-68`,
    `internal/security/role.go:32-39`, `:74-82`). `PasswordHash`, which also
    has one check, uses a leaf plus wrap (`internal/security/password.go:54-73`).
    **Rec:** always use a leaf plus wrap (for example `ErrUserIDNil = "is nil UUID"`)
    so messages read the same way.
12. **Missing top-level wrap.** `RoleName.Validate` returns `Text` errors and
    `ErrRoleNameTooLong` without wrapping them in `ErrRoleNameInvalid`
    (`internal/security/role.go:97-114`). `Permissions.Validate` passes
    element errors through (`role.go:43-55`). **Rec:** wrap both in their
    `Err…Invalid`.
13. **Nil-check shape in aggregate `Validate`.** `Language` uses
    `if l == nil {…}` and then `if err == nil { l.Code.Validate() }`
    (`internal/languages/language.go:139-145`). `User` and `Role` use
    `if … nil {…} else { err = u.ID.Validate() }`
    (`internal/security/user.go:193-197`). **Rec:** the `else` form, used by
    2 of 3 aggregates.
14. **Declaring and assigning `err` separately.** `RoleName.Validate` has
    `var err error` and then `err = …` on the next line
    (`internal/security/role.go:98-99`). **Rec:** keep `var err error` and a
    blank line, then use the step inside the `if` chain like the other types.
15. **`RoleFilter.Validate` overwrites errors.** It uses four independent
    `if`s, so the last failing check wins
    (`internal/security/role_repository.go:40-54`). **Rec:** use an
    `if / else if` chain, matching every other `Validate`.
16. **`RoleFilter` measures bytes but reports characters.** It uses
    `len(f.IDLike)` and `len(f.NameLike)` with `…Length` constants and
    "characters" messages (`internal/security/role_repository.go:48`, `:52`).
    `MaxRoleNameLength` is checked with `utf8.RuneCountInString`
    (`internal/security/role.go:103`). **Rec:** use `utf8.RuneCountInString`.
17. **Copy-paste bugs (not style).** `ErrNativeNameInvalid` says
    `"invalid English name"` (`internal/languages/language.go:82`).
    `NativeName.Validate` checks against `MaxEnglishNameLength`
    (`language.go:95`). The `ErrCodeWrongLength` branch can never run,
    because the regexp already requires two characters (`language.go:27-30`).
    **Rec:** fix them in a separate change.

### Errors

18. **Nil-argument sentinel.** The languages adapter uses the domain's
    `languages.ErrLanguageNil` (`languages/pgxgoqu/language_repository.go:137`).
    The security adapters define their own `ErrNilUser` and `ErrNilRole`
    (`security/pgxgoqu/user_repository.go:24`, `role_repository.go:28`),
    even though `security.ErrUserNil` and `ErrRoleNil` exist
    (`internal/security/user.go:142`, `role.go:122`). **Rec:** use the
    domain sentinel.
19. **`ErrNil<X>` vs `Err<X>Nil`.** The adapters use `ErrNilPool`,
    `ErrNilUser` and `ErrNilRole`, while the domain uses `ErrUserNil`.
    **Rec:** follow `Err<Subject><Problem>` (`ErrPoolNil`), or document
    adapter sentinels as an exception.
20. **`ErrInvalidRoleQuery`** (`internal/security/role_repository.go:106`)
    breaks the `Err<Subject>Invalid` pattern, and `RoleSort.Validate` returns
    it without a leaf (`:96`). **Rec:** rename it to `ErrRoleQueryInvalid` and
    add `ErrRoleSortInvalid` as the wrapped leaf.
21. **Entity ID in context.** `RoleRepository` always adds it
    (`security/pgxgoqu/role_repository.go:189-198`, `:230-239`, `:374`).
    `UserRepository` never does (`security/pgxgoqu/user_repository.go:153`,
    `:208`, `:253`, `:337`). Languages does sometimes: `FindByCode` adds it
    at `languages/pgxgoqu/language_repository.go:266` but not at `:261`.
    **Rec:** always include the ID, as in the [General rules](#general-rules) example.
22. **Wrapping build and bind errors.** The languages adapter passes
    `buildErr` to the typed failure unwrapped
    (`languages/pgxgoqu/language_repository.go:155`, `:197`, `:232`). The user
    adapter wraps build errors but not bind errors
    (`security/pgxgoqu/user_repository.go:153-155`). The role adapter wraps
    both (`role_repository.go:189-191`), but labels a bind error as
    `"build find role %s query"` (`:232`). **Rec:** wrap both, using `build …`
    and `bind …` respectively.
23. **Wrapping validation errors.** `RoleRepository.Delete` wraps them
    (`security/pgxgoqu/role_repository.go:178`). `Create` and `Update` don't
    (`:136`, `:157`). `FindByID` wraps them as `"find role by ID"` (`:218`),
    but the user equivalent doesn't (`user_repository.go:282`). **Rec:** pass
    validation errors to the typed failure unwrapped, because the failure
    type is the context, and wrap them in reads.
24. **Where `ErrNoRows` becomes not-found.** `scanLanguage` and `scanUser`
    convert it (`languages/pgxgoqu/language_repository.go:423`,
    `security/pgxgoqu/user_repository.go:382`). `scanRole` doesn't, and
    `FindByID` converts it instead (`security/pgxgoqu/role_repository.go:236`).
    **Rec:** convert it inside `scan<Entity>`.
25. **Wrapping inside `scan<Entity>`.** `scanLanguage` wraps both the scan
    error and the decode error (`languages/pgxgoqu/language_repository.go:426`,
    `:438`). `scanUser` wraps neither (`security/pgxgoqu/user_repository.go:385`,
    `:402`). `scanRole` wraps only structural problems (`role_repository.go:598`,
    `:604`). **Rec:** return driver errors unwrapped, and prefix decoding
    problems with `decode <entity>:`. The caller adds the operation.
26. **`errors.Join` vs `%w: %w` in mappers.** `mapLanguageError` and
    `mapUserError` use `errors.Join`
    (`languages/pgxgoqu/language_repository.go:452`,
    `security/pgxgoqu/user_repository.go:418`). `mapRoleError` uses
    `fmt.Errorf("%w: %w", …)` inside a one-case `switch`
    (`security/pgxgoqu/role_repository.go:642-645`). **Rec:** use
    `errors.Join`.
27. **Unmapped exec error.** `UserRepository.Delete` passes `execErr`
    without calling `mapUserError` (`security/pgxgoqu/user_repository.go:260`).
    The languages and role adapters map theirs. **Rec:** always map.
28. **`fmt.Errorf` with no verbs.** `fmt.Errorf("decode role: mismatched name translations")`
    (`security/pgxgoqu/role_repository.go:598`). **Rec:** use `errors.New`.
29. **Where `ErrNilPool` lives.** It's declared in the languages repository
    file (`languages/pgxgoqu/language_repository.go:22-25`), but security
    declares it in `pgxgoqu/db.go`, which is outside the reference set.
    **Rec:** put package-wide adapter sentinels and the DB interface in
    `db.go` in each adapter package.

### Control flow and declarations

30. **`var` grouping.** Grouped `var ( … )` blocks are used in
    `languages/pgxgoqu/language_repository.go:253-256`, `:414-419` and every
    `New<X>Repository`. Separate `var a` / `var b` lines are used in
    `security/pgxgoqu/user_repository.go:276-277`, `:361-365` and
    `role_repository.go:212-213`, `:253-254`, `:282-283`, `:589-592`.
    **Rec:** use grouped blocks.
31. **Where results are cleared on error.** `FindAll` sets `result = nil`
    inside the inner branch (`languages/pgxgoqu/language_repository.go:369`).
    `Find` does it in a separate block before `return`
    (`security/pgxgoqu/role_repository.go:354-356`). **Rec:** use the
    separate block before `return`, so no path is missed.

### Repositories

32. **Column lists.** The languages adapter repeats the column names inline
    (`languages/pgxgoqu/language_repository.go:335`, `:394`). The security
    adapters use `userColumns()` and `roleColumns()`. **Rec:** add
    `languageColumns()`.
33. **Insert style.** Languages uses `Rows(goqu.Record{…})`
    (`languages/pgxgoqu/language_repository.go:145`). User and role use
    `Cols(…).Vals(goqu.Vals{…})` (`security/pgxgoqu/user_repository.go:138-147`,
    `role_repository.go:392-398`). **Rec:** `Cols/Vals` (3 of 4 uses).
34. **Duplicate detection.** Languages relies on the `23505` mapping
    (`languages/pgxgoqu/language_repository.go:453`). User and role use
    `OnConflict(DoNothing())` plus a check for zero rows
    (`security/pgxgoqu/user_repository.go:148`, `:161`, `role_repository.go:399`,
    `:411`). `mapUserError` also maps `user_pkey`, so that case can't occur.
    **Rec:** map by constraint name. It's explicit about which uniqueness
    rule failed, and `DoNothing()` with no target hides email conflicts as
    "already exists".
35. **Shape of the lookup helper.** Languages uses
    `find(ctx, filter goqu.Ex, forUpdate bool)`
    (`languages/pgxgoqu/language_repository.go:382`). User uses
    `find(ctx, predicate exp.Expression, ids ...uuid.UUID)` and only ever
    reads `ids[0]` (`security/pgxgoqu/user_repository.go:314-330`). Role has
    no helper and builds the query inline in `FindByID` (`role_repository.go:222`).
    **Rec:** one `find(ctx, predicate, forUpdate)` per adapter, with UUID
    binding done by the caller or an explicit parameter instead of a
    variadic one.
36. **`FindByIDForUpdate` isn't implemented.** It's in `security.RoleRepository`
    (`internal/security/role_repository.go:147`), but
    `pgxgoqu.RoleRepository` has no such method, so the `var _` check at
    `security/pgxgoqu/role_repository.go:106` won't compile. **Rec:**
    implement it following `FindByCodeForUpdate`
    (`languages/pgxgoqu/language_repository.go:275-299`).
37. **`RoleQuery` doesn't match its use.** The domain type has the fields
    `Filter`, `SortBy`, `SortOrder`, `Limit` and `Page`, and no `Language`
    (`pkg/domquery/domquery.go`). The adapter calls the methods
    `options.Filters()`, `SortBy()`, `SortOrder()`, `Limit()` and `Page()`,
    and reads `options.Language` (`security/pgxgoqu/role_repository.go:290-316`).
    `RoleQuery.Validate` checks only `Filter`, not `SortBy`, `Limit` or
    `Page` (`internal/security/role_repository.go:110-112`). With `Page == 0`,
    `(Page-1)*Limit` wraps around. **Rec:** decide whether the query needs
    `Language`, use fields rather than methods, and validate sort and paging
    in `RoleQuery.Validate`.
38. **Optimistic concurrency.** Only `User` has `UpdatedAt` and the
    conflict check (`security/pgxgoqu/user_repository.go:170-229`). `Role`
    and `Language` updates are last-write-wins. **Rec:** decide whether every
    mutable aggregate gets `UpdatedAt`.
39. **Raw SQL alongside goqu.** The role adapter has literal statements:
    `INSERT INTO "text" DEFAULT VALUES RETURNING id` and
    `DELETE FROM "translation" WHERE text_id = $1`
    (`security/pgxgoqu/role_repository.go:380`, `:477`). Long `goqu.L`
    subqueries are also used (`:297`, `:500-501`, `:514`). **Rec:** build them
    with goqu, and allow `goqu.L` only for expressions goqu can't produce.
40. **Positional UUID binding.**
    `bindUUIDArgsAt(args, len(args)-3, [16]byte(roleID), [16]byte(id))`
    (`security/pgxgoqu/user_repository.go:209`) and `len(args)-1`
    (`role_repository.go:463`) depend on the order goqu emits arguments in.
    **Rec:** an open question for whoever owns `db.go`. Consider binding
    UUIDs through a `pgtype.UUID` value in the record instead.

### Naming

41. **Parameter names that differ from the interface.** `Count(ctx, filters)`
    (`security/pgxgoqu/role_repository.go:251`) vs `filter` in the interface
    (`internal/security/role_repository.go:149`). `Find(ctx, options)`
    (`role_repository.go:280`) vs `query` (`role_repository.go:148`).
    `roleFilterQuery(filters)` (`:506`). **Rec:** use the interface's names
    (`filter`, `query`), and rename the local dataset to `dataset` (see 42).
42. **SQL and dataset local names.** `Find` names the dataset `query` and
    the SQL string `sql` (`security/pgxgoqu/role_repository.go:290`, `:314`).
    Elsewhere the SQL is `query` and the dataset is `dataset`
    (`languages/pgxgoqu/language_repository.go:392-402`). **Rec:**
    `dataset` and `query`.
43. **Converted UUID locals.** `storedID` (`security/pgxgoqu/role_repository.go:180`,
    `:220`), `rawID` (`user_repository.go:244`), and `id` / `roleID`
    (`user_repository.go:134`, `role_repository.go:369`). **Rec:**
    `storedID`, plus `stored<Field>ID` for other ID fields.
44. **Step-error names.** `execErr` is used in languages and user
    (`languages/pgxgoqu/language_repository.go:157`), but role uses
    `deleteErr` (`security/pgxgoqu/role_repository.go:193`). Role uses
    `filterErr` (`:258`) where other code uses `validationErr`. **Rec:**
    name the error after the call, not the operation: `execErr`,
    `validationErr`.
45. **Result slice name.** `result` in `FindAll`
    (`languages/pgxgoqu/language_repository.go:326`), `roles` in `Find`
    (`security/pgxgoqu/role_repository.go:282`). **Rec:** use the entity's
    plural.
46. **Receivers.** `Translation` uses `tc` (`internal/languages/text.go:22`);
    every other type uses its first letter. `RoleName` uses `r`, the same as
    `Role` and the repositories (`internal/security/role.go:97`). **Rec:**
    `t` for `Translation` and `n` for `RoleName`, matching the other
    `…Name` types.
47. **How enum constants are declared.** `RoleSortID = RoleSort("id")`
    (`internal/security/role_repository.go:87`) vs
    `PermissionManageUser Permission = "manage_user"`
    (`internal/security/role.go:26`). **Rec:** the typed form,
    `RoleSortID RoleSort = "id"`. Also, `domquery.SortOrderAsc` and
    `SortOrderDesc` are untyped `bool` constants (`pkg/domquery/domquery.go`).

### Imports and formatting

48. **Import grouping.** `internal/security/role.go:10-11` puts
    `github.com/google/uuid` and `github.com/radynsade/faryengo/internal/languages`
    in one group. Every other file separates them
    (`security/pgxgoqu/user_repository.go:9-17`). **Rec:** use separate groups.
49. **Postgres dialect import.** It's in the languages and role adapters
    (`languages/pgxgoqu/language_repository.go:9`,
    `security/pgxgoqu/role_repository.go:12`) but not the user adapter, which
    relies on the role file in the same package. **Rec:** import it once per
    package, in `db.go`.
50. **Constructor signature breaking.** `NewRole` fits on one line with four
    parameters (`internal/security/role.go:132`). `NewLanguage` is broken
    across lines, also with four (`internal/languages/language.go:122-127`).
    **Rec:** always break `New<Entity>`.
51. **Long lines.** The `mapUserError` case condition
    (`security/pgxgoqu/user_repository.go:419`), the name-sort literal
    (`security/pgxgoqu/role_repository.go:297`) and the name filter literal
    (`:514`) run past 140 characters. **Rec:** pick a line limit, or move the
    long SQL into named constants.
