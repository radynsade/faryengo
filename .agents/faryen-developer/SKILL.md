---
name: faryen-developer
description: Implement, fix, refactor, or diagnose code in the Faryen project.
---

# Finassets Development

## MUST DO

### Skill Routing

- Read and apply `$faryen-base` before starting development work.

### Workflow

- Inspect the affected implementation and call sites before changing code.
- Reuse existing project mechanisms where they fit.
- Make the smallest complete change and avoid unrelated refactoring.

### SQL and Database Access

- Write SQL keywords in lowercase.
- Use distinct, consistent aliases across outer queries and subqueries.
- Wrap related writes in a transaction when partial persistence would be invalid.

### Validation

- Run the checks required by the applicable application-specific coding rules.

## MUST NOT DO

- Add a production dependency without an explicit need and approval.
- Normalize line endings or local formatting in an existing file when that would create an unrelated diff.
- Interpolate user-provided values into SQL.
- Introduce an N+1 query.
- Add comments that restate a method name or an obvious code action.

## PREFER

- Avoid early returns. An explicit control flow is much better than a random return statement in the middle of a function.
- The maximum length of a line is 100.
