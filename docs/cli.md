# CLI

Build the CLI with `make build` or `go build -o bin/cli ./cmd/cli`.
Set `DATABASE_URL` in the environment or in an optional `.env` file in the
working directory. An operator must apply the database migrations before using
the commands.

Commands use `<binary> <domain-scope> <command-name> ...`. User and role commands
belong to `security`; language commands belong to `languages`.

## Languages

```sh
bin/cli languages create-language lv Latvian Latviešu
bin/cli languages create-language en English English --fallback
bin/cli languages delete-language lv
```

Use `--fallback` (or `-f`) to mark the language as the fallback for missing translations. Without the option, the language is not a fallback. Only one fallback language can exist.

Pass names containing spaces as quoted arguments. Each command prints the
affected language code on success. Creating a language whose code already exists
returns an error and leaves the existing language unchanged. Deleting a language
with existing translations is restricted by the database. A fallback with translations also cannot have its fallback flag cleared. Run `bin/cli help`
to display the command syntax.

## Roles

```sh
bin/cli security create-role 'en:Administrator|lv:Administrators' --super
bin/cli security create-role 'en:User manager|lv:Lietotāju pārvaldnieks' --permission manage_user --permission view_user
bin/cli security create-role 'en:Guest|lv:Viesis'
bin/cli security delete-role <roleUUID>
```

Names use the format `en:Some text|lv:Kāds teksts`. Quote the whole name to
protect spaces and pipes from the shell. The parser trims language codes and
content; the last entry for a repeated code wins. Each entry must contain
exactly one colon, and separators cannot be escaped. Codes must be two lowercase
letters, content must be nonblank UTF-8, and every code must identify an existing
language in PostgreSQL.

`--super` (or `-s`) grants every defined permission, including permissions added
later. Regular roles default to an empty permission list. Repeat `--permission`
(or `-p`) to grant `manage_user` and/or `view_user`; `--permission=view_user` is
also accepted. Unknown permissions are rejected even for super roles. Options
may appear before or after the name; `--` ends option parsing.

Creation generates the role UUID and prints `created role <UUID>`. Deletion
requires a non-nil UUID in standard hyphenated format and prints
`deleted role <UUID>`. Missing roles and roles assigned to users return errors
and a nonzero exit status. Deleting an unused role also deletes its translated
name. Argument syntax and input values are validated before opening a database
connection.

Apply migration `000005_add_role_super` manually before using the updated role
repository or running the server. Existing roles default to non-super.

## Users

```sh
bin/cli security create-user <email> <firstName> <lastName> <password> <phone> <roleUUID>
bin/cli security delete-user <userUUID>
```

For example:

```sh
bin/cli security create-user person@example.com 'First Name' 'Last Name' 'example-password' +37123456789 8e35a76b-cc06-4b5b-8d8c-2c4d9144ff63
bin/cli security delete-user 3fc159c0-10b6-4c34-8459-c8544330d366
```

The role UUID must identify an existing role. UUID arguments use the standard
hyphenated format and cannot be nil. Creation validates the email, names,
international phone number, and password before connecting to PostgreSQL.
Passwords must contain at least eight characters, cannot be blank, and are
limited to 4096 bytes. The command generates the user UUID and stores an
Argon2id password hash through the application `UserService`.

Creation prints `created user <UUID>`; deletion prints `deleted user <UUID>`.
Neither command prints passwords or hashes. Duplicate emails, missing roles,
and missing users return errors and a nonzero exit status. Deleting a user
prevents subsequent access and refresh authentication for that user because
session checks require the current PostgreSQL user record.
