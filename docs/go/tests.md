# Writing Go tests

This guide defines test placement, test structure, leak detection, and test
commands. Follow [code style](style.md) for declarations and formatting and
[file layout](layout.md) for production files.
[Architecture](../architecture.md) takes precedence where guidance conflicts.

> **Important — temporary:** Do not write new integration tests for now.
> Write unit tests; use full domain-contract mocks for repository dependencies.

## Test placement

Keep tests beside the Go packages they exercise, in files ending in `_test.go`.

## Table-driven tests

- Write table-driven tests with `t.Run` subtests.
- Give cases descriptive names and define their inputs and expected outcomes
  in the table. Run the behavior and check its outcome inside each subtest.

## Production code and test doubles

Do not change production code to accommodate tests. Keep its constructors,
dependency types, and behavior intact; do not introduce interfaces or wrappers
solely to make infrastructure mockable.

Mock only complete domain interfaces. Never mock infrastructure dependencies
such as database pools, connections, transactions, or Redis clients. This rule
applies now and when integration testing resumes.

Tests of repository callers use a full mock of the domain repository contract,
named `mock.<Entity>Repository`. Implement every operation with the contract's
original parameter and return types, including its typed failure contracts;
do not substitute a partial interface tailored to one test.

For example, `pgxgoqu.LanguageRepository` accepts a `pgxdb.DB`, which exists
so production code can pass a pool or a transaction. Tests never implement
`pgxdb.DB` with a fake; unit tests of the repository's callers use
`mock.LanguageRepository`, implementing the complete
`languages.LanguageRepository` interface.

When integration testing resumes, use real implementations of domain
interfaces connected to real infrastructure services. Infrastructure
dependencies remain unmocked.

## Goroutine leak detection

Call `goleak.VerifyTestMain` from `TestMain` in every package that has tests.

```go
import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }
```

## Running checks

Run commands from the repository root:

| Check | Command |
| --- | --- |
| All required checks | `make check` |
| Full test suite | `go test -race -count=1 -timeout 60s ./...` |
| One test | `go test -race -count=1 -run TestName ./internal/pkg -v` |
| Benchmarks | `go test -race -count=1 -bench=. -benchmem ./internal/pkg` |

`make check` builds assets and generates templates before formatting, vet,
lint, and the race-enabled test suite. Always keep `-race` and `-count=1` on
Go test commands; do not remove them to make the suite faster.
