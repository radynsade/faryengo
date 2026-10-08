# Migrate

> **Important:** Only a human may run migrations. Agents must never apply or
> roll back migrations, including during development or testing. Agents may
> prepare migration files and instructions for a human to execute manually.

Use `bin/migrate` to apply, roll back, or inspect database migrations. Set
`DATABASE_URL` in the environment or an optional `.env` file in the working
directory. Existing environment values take precedence over the file.

Operators run migrations explicitly before using the CLI or server.

## Available commands

| Command | Purpose |
| --- | --- |
| `bin/migrate [-database <URL>] up` | Apply all pending migrations in order. |
| `bin/migrate [-database <URL>] down` | Roll back the most recently applied migration. |
| `bin/migrate [-database <URL>] status` | List migration versions, names, and whether each is applied or pending. |
| `bin/migrate -h` | Display command syntax and options. Also available as `bin/migrate --help`. |

Arguments in square brackets are optional. Replace `<URL>` with the PostgreSQL
connection URL and quote it when it contains shell separators.

## Options and usage

Use `-database <URL>` to override `DATABASE_URL`. Place the option before the
command. A database URL is required for `up`, `down`, and `status`.

Run `status` to inspect the current state. Use `up` to apply every pending
migration; use `down` once for each migration you want to roll back. Rollbacks
can remove data according to the migration being reversed.

The tool reports each applied or rolled-back migration, or reports that there
are none to process. Commands report errors and return a nonzero exit status
when they fail.
