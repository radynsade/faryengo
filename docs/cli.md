# CLI

Build the CLI with `make build` or `go build -o bin/cli ./cmd/cli`.
Set `DATABASE_URL` in the environment or in an optional `.env` file in the
working directory. Apply the database migrations before creating a language.

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
