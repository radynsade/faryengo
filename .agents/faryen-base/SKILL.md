---
name: faryen-base
description: General Faryen project guidance.
---
## MUST DO

- Read every applicable `AGENTS.md` for the repository and changed paths.
- Treat every non-test database as read-only.
- Keep changes compatible with the affected application's stated stack.

## MUST NOT DO

- Manually apply changes to the database structure and data using direct SQL queries.
- Run migrations, seeders, database resets, or application commands that modify a non-test database.
- Execute migrations or seeders unless the user explicitly requests and authorizes that specific operation.
