# CLI

Use `bin/cli` to manage languages, roles, and users. Set `DATABASE_URL` in the
environment or an optional `.env` file in the working directory. The database
must be prepared by an operator before running management commands.

Replace arguments in angle brackets with your values; arguments in square
brackets are optional. Quote arguments containing spaces or shell separators.

## Available commands

| Command | Purpose |
| --- | --- |
| `bin/cli help` | Display command syntax. Also available as `bin/cli -h` or `bin/cli --help`. |
| `bin/cli languages create-language <code> <englishName> <nativeName> [--fallback]` | Create a language. |
| `bin/cli languages delete-language <code>` | Delete a language. |
| `bin/cli users create-role <nameTranslations> [--super] [--permission <permission>]...` | Create a role. |
| `bin/cli users delete-role <roleUUID>` | Delete a role. |
| `bin/cli users create-user <email> <firstName> <lastName> <password> <phone> <roleUUID>` | Create a user with an existing role. |
| `bin/cli users delete-user <userUUID>` | Delete a user. |

## Languages

Provide a two-letter lowercase language code, its English name, and its native
name. Use `--fallback` or `-f` to make it the fallback for missing translations;
only one fallback language can exist.

Language codes must be unique. A language with existing translations cannot be
deleted. Both commands print the affected language code on success.

## Roles

Supply translated names as one quoted argument in the format
`<languageCode>:<name>|<languageCode>:<name>`. Each code must identify an existing
language, and names must be nonblank. Names cannot contain `:` or `|`. When a
fallback language exists, the name in that language is required; other languages
are optional.

- `--super` or `-s` grants all permissions, including those added later.
- `--permission <permission>` or `-p <permission>` grants one permission.
  Repeat the option to grant several. Available permissions are `manage_user`,
  `view_user`, `manage_role`, and `view_role`.
- Without either option, the role has no permissions.

Options may appear before or after the name. Use `--` to end option parsing.
Creation prints the new role UUID. Use that UUID to assign the role to users or
delete it. Roles assigned to users cannot be deleted.

## Users

Provide an email, first name, last name, password, international phone number,
and the UUID of an existing role, in that order. Emails must be unique and
passwords must contain at least six characters.

Creation prints the new user UUID. Use that UUID to delete the user. User and
role UUID arguments must use the standard hyphenated format and cannot be the
all-zero UUID.

Commands report errors and return a nonzero exit status when they fail.
