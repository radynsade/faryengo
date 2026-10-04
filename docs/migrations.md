# Database migrations

The `cmd/migrate` binary embeds the paired SQL files in `db/migrations`. Build it
with `make build` or `go build -o bin/migrate ./cmd/migrate`.

Set `DATABASE_URL` in the environment or in an optional `.env` file in the
working directory, then run one of these
commands manually:

```sh
bin/migrate status
bin/migrate up
bin/migrate down
```

Existing environment variables take precedence over `.env` values. The
`-database URL` flag can supply the connection string instead. Put flags
before the command. `up` applies every pending migration in version order;
`down` rolls back the latest applied migration. `status` lists all bundled
migrations and whether each is applied. A fresh database has no history table;
`status` reports every migration as pending without creating one.

Each migration and its history record run in one transaction. The command uses
a PostgreSQL advisory lock to prevent concurrent migration commands, and it
rejects changed, missing, or out-of-order applied migrations. A migration must
have matching, nonempty `<version>_<name>.up.sql` and `.down.sql` files. Keep
SQL compatible with PostgreSQL transactions. Add a new version to change an
already applied schema; do not edit an applied migration. Version `000000`
creates `public.schema_migration`; rolling it back drops the table. If a database
has the previous `public.schema_migrations` table, the command stops so its
history can be reconciled before using the new table.

Migration `000003_add_language_fallback` adds the language fallback flag, defaults existing languages to `false`, and enforces a single fallback. It protects a fallback with translations from deletion or clearing its flag. Apply it before using the updated language repository or the CLI's `--fallback` option.

Migration `000005_add_role_super` adds the role's `is_super` flag, defaulting
existing roles to `false`. Apply it before deploying the updated server or using
the role commands. Its down migration removes the flag.

Migration `000006_add_role_permissions` adds `manage_role` and `view_role` to the
PostgreSQL `permission` enum. Apply it before assigning either permission to a
role. Existing grants are preserved, and regular roles receive no new grants
automatically. New enum values become available after the migration commits.

Its down migration recreates the enum with only `manage_user` and `view_user`,
preserving the permissions column's default and validation constraint. Remove
`manage_role` and `view_role` from all role permission lists before rollback;
otherwise the conversion fails and the migration transaction leaves the schema,
grants, and migration history unchanged. The rollback takes an exclusive lock
on the role table while converting its permissions column. See
[PostgreSQL enum limitations](https://www.postgresql.org/docs/current/datatype-enum.html).
