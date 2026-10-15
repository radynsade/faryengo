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
| `TRUSTED_PROXIES` | Comma-separated IP addresses and CIDR prefixes of the reverse proxies in front of the server, such as `10.0.0.0/8,::1`. | None |

Keep `AUTH_COOKIE_SECURE=true` in production and provide HTTPS through a trusted
reverse proxy. For local use over plain HTTP, set `AUTH_COOKIE_SECURE=false`.

List every reverse proxy in `TRUSTED_PROXIES`. The server takes the client's
address from `X-Forwarded-For` only on connections from those proxies, reading
it from the right and stopping at the first untrusted address, so each proxy
must append the address it received the request from rather than pass the
header through. Without trusted proxies the connection's own address is the
client's, so behind an unlisted proxy every visitor shares the proxy's address
and its sign-in limit.

## Sign-in limits

Every sign-in attempt in the admin panel and the office counts against its
email and its client address, whether it succeeds or not. Within 15 minutes of
the first counted attempt, an email allows 10 attempts and a client address
100; later attempts are refused with `429 Too Many Requests` until those 15
minutes pass. A successful sign-in clears its email's count. Counts live in
Redis-compatible storage under digests of the email and address; when that
storage is unavailable, signing in is refused. At most four passwords are
verified at once, and an attempt that waits more than two seconds for its turn
is refused with `503 Service Unavailable`.

## Responses and logs

Every response forbids framing and carries a content security policy that
allows only the server's own scripts, styles, and connections, plus data URLs
for fonts and images. Each request is logged once answered, with its method,
path without the query string, status, size, duration, and client address. A
handler that fails unexpectedly is logged with its stack trace and answered
with the application's error page.

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
Transactions, Goals, Reports, and Settings pages, which are empty for now except
the Settings. A workspace's settings, at
`/office/<language>/workspaces/<workspace>/settings`, list its members and let
any member rename it, add another existing user to it by email, and remove
other members. A workspace the user does not belong to sends them back to
choosing one, and its settings are not found. The profile settings are served at
`/office/<language>/profile-settings`, without the workspace sidebar, and
inside each workspace at
`/office/<language>/workspaces/<workspace>/profile-settings`, where the
workspace's dashboard and sidebar stay. The Settings link in the account menu
leads to the profile settings, inside the current workspace when there is one.
There users change their names and email, and their password; changing
the email or the password signs them out on every other device and keeps the
current one signed in. A visitor who is
not signed in is sent to the sign-in page.

Press Ctrl+C or send SIGTERM to stop the server gracefully. Startup and runtime
failures are reported with a nonzero exit status.
