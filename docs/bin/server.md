# Server

Use `bin/server` to serve the admin and office applications. Set configuration in the
environment or an optional `.env` file in the working directory. Existing
environment values take precedence over the file.

PostgreSQL and Redis-compatible storage must be available. An operator must
apply the database migrations before starting the server.

## Available command

| Command | Purpose |
| --- | --- |
| `bin/server` | Start the HTTP server and keep it running until stopped. |

Run the command without arguments; configure it through environment settings.

## Configuration

| Setting | Use | Default |
| --- | --- | --- |
| `DATABASE_URL` | PostgreSQL connection URL. Required. | None |
| `REDIS_URL` | Redis-compatible storage connection URL. | `redis://127.0.0.1:6379/0` |
| `HTTP_ADDRESS` | Listening address in `<host>:<port>` format. An omitted host listens on all interfaces. | `:8080` |
| `SESSION_TTL` | Browser session lifetime, from `1s` to `2160h`. | `168h` |
| `AUTH_COOKIE_SECURE` | Require HTTPS for authentication cookies. | `true` |

Keep `AUTH_COOKIE_SECURE=true` in production and provide HTTPS through a trusted
reverse proxy. For local use over plain HTTP, set `AUTH_COOKIE_SECURE=false`.

## Access and shutdown

Create the initial language, role, and user with the [CLI](cli.md). Open
`/admin/<language>/sign-in` on the server's public address, replacing
`<language>` with a supported interface language code, and sign in with the
user's email and password.

The office application is served at `/office/<language>`; `/office` redirects
to English. Users sign in at `/office/<language>/sign-in` with the same email
and password as the admin panel. The office keeps its own session cookie, so
signing in to one application does not sign in to the other. After signing in,
users choose one of their workspaces at `/office/<language>/workspaces`, or
create one at `/office/<language>/workspaces/new`; the creator becomes the new
workspace's first member and lands on its overview. A workspace's dashboard is served at
`/office/<language>/workspaces/<workspace>` and has Overview, Budgets,
Transactions, Goals, Reports, and Settings pages, which are empty for now. A
workspace the user does not belong to sends them back to choosing one. The
settings are also available outside any workspace at
`/office/<language>/settings`, without the workspace sidebar. There users
change their names and email, and their password; changing the email or the
password signs them out on every other device and keeps the current one
signed in. A visitor who is
not signed in is sent to the sign-in page.

Press Ctrl+C or send SIGTERM to stop the server gracefully. Startup and runtime
failures are reported with a nonzero exit status.
