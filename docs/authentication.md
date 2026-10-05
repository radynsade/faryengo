# Authentication and authorization

The application service is `internal/app.AuthenticationService`. JWT encoding is
implemented in `internal/security/jwt` using `github.com/golang-jwt/jwt/v5`.
Session state and sign-in throttling use `github.com/redis/go-redis/v9` in
`internal/security/redis`. Admin web transport is in `web/admin`, outside the
domain directories. There is no authentication REST API.
Dependencies are wired explicitly in `cmd/server`.

## Token policy

- Access tokens expire after 5 minutes by default; configuration cannot exceed
  15 minutes. Refresh tokens have an absolute 30-day lifetime by default, bounded
  at 90 days. A session also expires after 7 days without a refresh.
- Both tokens are signed with Ed25519 (`EdDSA`). Verification pins the algorithm,
  key ID, issuer, audience, token purpose, and header type. It requires expiration,
  not-before, issued-at, subject, session ID, and a random token ID. Clock skew is
  limited to 30 seconds. Access and refresh tokens cannot substitute for each other.
- Each device login has a random session ID. Refreshing rotates the token using
  an atomic Redis compare-and-swap. A reused signed refresh token revokes that
  device's entire session, including access tokens previously issued to it.
  Refreshing never extends the absolute session lifetime.
- Clients must serialize refresh requests. Do not retry an old refresh token
  after an ambiguous response or run parallel refreshes: reuse deliberately
  requires signing in again. The server's Redis client disables automatic retries.
- Tokens carry only identifiers and timing information. Permissions, password
  hashes, and credential versions are not embedded. Raw tokens are not stored
  server-side or written to logs.

These policies use the algorithm and token-type separation guidance in
[RFC 8725](https://www.rfc-editor.org/rfc/rfc8725.html) and refresh replay detection
in [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.14.2).

## State and immediate revocation

| Store | Data |
| --- | --- |
| PostgreSQL | Users, Argon2id password hashes, current role/permissions, unique case-insensitive email addresses, and a random `credential_version` per user. |
| Redis/Dragonfly | Per-device session credential version, SHA-256 refresh-token digest, absolute expiration, and a user session generation. Session keys expire automatically; one generation key per user remains until explicitly cleaned up. Rate-limit keys expire after their window. |

Every access-token authentication checks the Redis session and the current
PostgreSQL credential version, and loads the user's current role. Tokens are
therefore intentionally stateful. PostgreSQL and Redis must be available;
storage errors fail closed with HTTP 503 rather than accepting tokens without
revocation or authorization checks. Authentication must read PostgreSQL primary
state; replica lag would delay credential and permission changes.

Migration `000004_add_authentication` adds a database trigger that changes the
credential version whenever the password hash or email changes. Existing
`UserService.Update` password changes therefore invalidate access and refresh
tokens on **all devices, including the current one**, without a separate Redis
operation. User updates compare the originally loaded password hash atomically
in SQL; a concurrent password change rejects the stale update with
`security.ErrUserConflict`, preventing a profile update from restoring the old
password. To change passwords through the repository, load the existing user,
call `SetPasswordHash`, and then `Update`; reload the user before another update.
This also works for direct SQL password updates. Existing in-flight
requests may finish; subsequent authentication checks reject the old version.

`SignOut` revokes one device session. `SignOutAll` and trusted application calls
to `RevokeUserSessions` first change the durable PostgreSQL version, then change
the Redis generation. Old sessions cannot become valid simply by signing in
again, recreating a missing Redis generation, or resetting a password back to a
previous value. User deletion also prevents authentication. Permission and role
changes take effect on the next authentication check without issuing new tokens.

Redis uses atomic Lua scripts and primary reads. User/session keys use the same
Redis hash tag. A missing key always denies access. Use a dedicated authenticated
Redis/Dragonfly database with TLS and appropriate persistence/failover guarantees;
it holds security state. Do not restore older Redis session snapshots into a
live deployment: flush session state or rotate the signing key after such a
restore so individual logout and refresh replay revocations cannot be undone.
Account-wide revocation and password changes remain guarded by PostgreSQL.

## Configuration and migrations

Start from [`.env.example`](../.env.example). `config.Load` reads `.env` with
`godotenv`; existing environment variables take precedence.

Generate a stable signing seed with `openssl rand -base64 32` and store it in
`JWT_SIGNING_SEED` through your secret management system. There is no generated
startup key or insecure signing fallback. Keep this seed consistent across
instances and restarts. Do not commit it. Set `JWT_KEY_ID` to identify the key.
The JWT manager can accept previous public keys through `VerificationKeys` for
planned key rotation; the server configuration currently loads one signing key.
Changing that key without retaining its public key invalidates existing JWTs.

`DATABASE_URL`, `REDIS_URL`, issuer/audience settings, token TTLs, cookie flags,
and the listen address are listed in `.env.example`. PostgreSQL 13 or later is
required for the built-in `gen_random_uuid()` used by the migration. An operator
must apply the migration before deploying this server. Migration execution is
never part of application startup. Resolve duplicate `lower(email)` values in
existing users before applying the new unique index.

Production must use HTTPS, including across untrusted proxy hops, and keep
`AUTH_COOKIE_SECURE=true`. Secure cookies use the `__Host-` prefix, `Path=/`,
`HttpOnly`, and `SameSite=Strict`, with no Domain attribute. For local HTTP
only, `AUTH_COOKIE_SECURE=false` uses ordinary cookie names. Both tokens stay
in HttpOnly cookies; do not copy them into browser storage or client-side state.
No authentication responses may be cached.

Sign-in limits are shared across instances: 10 requests per account and 100 per
client address per 15 minutes. Successful attempts also count. Identifiers in
rate-limit keys are hashed. Forwarded address headers are not trusted; behind
a proxy, the address limit applies to the proxy unless a trusted deployment
layer supplies a verified client `RemoteAddr`. Password verification work is
limited to four concurrent operations per application instance; further requests
return 503 immediately rather than accumulating a password-verification queue. Unknown accounts
verify against a dummy Argon2id hash and return the same 401 as wrong passwords.

## Admin web transport

| Route | Request | Success |
| --- | --- | --- |
| `GET /admin/{language}/sign-in` | Browser navigation | HTML sign-in page |
| `POST /admin/{language}/sign-in` | Native `application/x-www-form-urlencoded` form with `email` and `password` | Both token cookies and a 303 redirect to `/admin/{language}` |
| `GET /admin/{language}` | Valid access cookie | Authenticated admin page with a sign-out form |
| `POST /admin/{language}/refresh` | Refresh cookie, submitted by the continuation form | Rotated cookies and a 303 redirect to `/admin/{language}` |
| `POST /admin/{language}/sign-out` | Refresh cookie, or an access cookie if unavailable | Device session revoked, cookies cleared, and a 303 redirect to sign-in |

The sign-in form works without JavaScript. It accepts exactly one email and
password in the request body, rejects unknown or repeated fields, and limits
the request to 32 KiB. Query parameters cannot supply credentials. Email and
password lengths are bounded. Failed submissions render an inline error and
retain the email, never the password. Authentication failures return 401,
permission failures 403, throttling 429, and storage failures 503. Responses
never disclose token validation details or account existence.

Web notifications use `pkg/flashmsg`, with persistence owned by `web/admin`.
Authenticated flashes occupy an `admin:flashes` field in the existing device
session hash, sharing its expiration and revocation. Sign-in errors are stored
in a separate anonymous admin session with a 15-minute TTL and an opaque
`faryen_flash` cookie (`__Host-faryen_flash` with secure cookies). This cookie is
HttpOnly, SameSite Strict, and carries only an identifier. Reading an empty guest
page does not create a session. Messages are atomically consumed on rendering;
see [flash messages](flash-messages.md).

The admin page calls `AuthenticationService.Authenticate` on every request,
checking session state, credential version, and current role. Missing or invalid
access cookies redirect to the sign-in page. If a refresh cookie is present,
the sign-in page offers a **Continue your session** form. Refreshing is an
explicit POST rather than a side effect of page navigation. A successful
refresh also checks the current role before setting cookies. A failed refresh
clears cookies and requires a new sign-in, preventing retries after an ambiguous
rotation. Sign-out also clears cookies when the presented session is already
invalid.

Browser requests use same-origin HttpOnly cookies; Bearer headers are not an
admin authentication mechanism. All admin routes are wrapped in Go's
`http.CrossOriginProtection`, checking Origin and Fetch Metadata for mutations;
no trusted cross-origin exceptions or CORS access are configured. Pages and
errors have `Cache-Control: no-store`. Native form success redirects load a
full document; GET page navigation also supports HTMX fragments. Password
restoration remains a presentation component without submission behavior.
HTMX history snapshots are disabled with `hx-history="false"`; Back and Forward
fetch full documents from the server so authentication is checked again.

Account-wide sign-out and authorization remain application use cases; they
have no public transport routes. Future admin handlers must call
`AuthenticationService.Authenticate` or `Authorize` before accessing protected
content or performing actions.

`PermissionManageUser`, `PermissionViewUser`, `PermissionManageRole`, and
`PermissionViewRole` are independent permissions. Managing users does not grant
role permissions, and managing roles does not imply permission to view roles.
No permission is inferred from a role name. `RevokeUserSessions` accepts a user
ID for trusted application callers; endpoints targeting another user must first
check the appropriate permission.

Admin role management currently requires authentication only. Role-specific
authorization checks are deferred, and its handlers use the common role service
methods. The role permissions remain defined for later authorization work.

## Verification

`make check` builds assets/templates and runs formatting, vet, lint, and the
race-enabled tests. Tests cover strict JWT validation, access/refresh separation,
key rotation, concurrent refresh replay, absolute and idle expiration, credential
changes, deletion, current permissions, Redis loss/outages, logout, secure cookie
flags, CSRF, request validation, and throttling. TestMain uses goleak.

An additional integration test uses an isolated real Redis instance:

```sh
FARYEN_TEST_REDIS_SOCKET=/path/to/isolated/redis.sock \
  go test -race -count=1 -timeout 60s -tags=integration \
  -run TestIntegrationRedisSessionRotation ./internal/security/redis
```

The integration test never connects to PostgreSQL or runs migrations. The
PostgreSQL migration has been written for an operator to apply; its trigger has
not been executed by the automated tests.
