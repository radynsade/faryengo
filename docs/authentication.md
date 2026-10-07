# Authentication and authorization

Authentication has a shared identity service and two credential mechanisms:

- `internal/app.AuthenticationService` verifies passwords, checks current
  PostgreSQL credential versions, resolves `security.Principal`, and coordinates
  account-wide revocation. It has no JWT, cookie, or HTTP dependencies.
- `internal/app.SessionAuthenticationService` creates and authenticates random
  opaque browser credentials against server-side sessions.
- `internal/app.JWTAuthenticationService` owns access/refresh issuance,
  verification, rotation, and token-based logout. JWT encoding stays in
  `internal/security/jwt` using `github.com/golang-jwt/jwt/v5`.
- `app.Authorize(principal, permission)` and `Principal.HasPermission` apply the
  same permission rules to principals from either mechanism. Application use
  cases accept the resolved principal rather than a raw credential.

Both mechanism services expose `Authenticate(context.Context, string)
(security.Principal, error)`. Transport consumers can declare a small interface
for that method, extract the appropriate credential, and pass the principal to
application code. The credential string is an opaque ID for sessions and an
access token for JWT; neither mechanism interprets HTTP headers or cookies.

`security.Session` contains only device identity, user identity, credential
version, and absolute expiration. `security.TokenSession` adds refresh-token
state. `internal/security/redis.SessionStore` implements persistence and shared
revocation for both, with no raw bearer credentials stored. Sign-in throttling
also uses `github.com/redis/go-redis/v9`. Dependencies are wired explicitly in
`cmd/server`, which currently serves only the session-based admin panel. The JWT
application flow remains available for future API transports; there are no
public authentication REST API routes.

## Browser session policy

Sign-in generates a new random 256-bit credential, encoded as URL-safe base64.
The browser receives only this opaque ID in `faryen_session` (or
`__Host-faryen_session` with secure cookies). No user ID, permissions, JWT, or
other claims appear in it. A separate internal UUIDv7 identifies the device
session in the server-side principal. Submitted or existing cookie values never
choose the new credential, preventing session fixation.

The Redis lookup key uses a SHA-256 digest of the opaque credential and maps it
to a user ID and internal session ID. The authoritative session hash uses the
same per-user generation, credential version, and expiration checks as JWT
sessions, so account-wide revocation covers both mechanisms. The lookup is
published before the session hash; a partial write cannot authenticate a missing
session. Lookup and session creation are separate writes because their keys may
occupy different Redis Cluster slots. Orphaned lookups, including those left by
logout, expire at the absolute deadline and never bypass session checks.

`SESSION_TTL` sets an absolute browser-session lifetime, defaulting to 7 days and
bounded between 1 second and 90 days. Authentication and flash operations never
extend it. Redis expiration and explicit server-side deadline checks enforce it
independently of the cookie's expiration. Logout deletes the authoritative hash
and clears the cookie. Expired, missing, revoked, or malformed sessions require
new sign-in; browser sessions do not refresh or issue JWTs.

## JWT token policy

- Access tokens expire after 5 minutes by default; configuration cannot exceed
  15 minutes. Refresh tokens have an absolute 30-day lifetime by default, bounded
  at 90 days. A JWT device session also expires after 7 days without a refresh.
- Both tokens are signed with Ed25519 (`EdDSA`). Verification pins the algorithm,
  key ID, issuer, audience, token purpose, and header type. It requires expiration,
  not-before, issued-at, subject, session ID, and a UUIDv7 token ID. Clock skew is
  limited to 30 seconds. Access and refresh tokens cannot substitute for each other.
- Each device login has a UUIDv7 session ID. Refreshing rotates the token using
  an atomic Redis compare-and-swap. Requests carrying the immediately previous
  refresh token within a fixed 10-second overlap window receive the identical
  replacement pair. This tolerates parallel API client requests across server
  instances. Older tokens, or reuse after the window, revoke that device's entire
  session, including access tokens previously issued to it. Refreshing never
  extends the absolute session lifetime.
- The overlap window starts at the first rotation using Redis time, never slides
  on duplicate requests, and ends no later than the replacement access token's
  expiration. Within it, a stolen previous token is indistinguishable from a
  legitimate duplicate and can obtain the same replacement pair. Outside it,
  strict replay revocation applies. Do not automatically retry ambiguous refresh
  failures; the server's Redis client disables automatic retries.
- Tokens carry only identifiers and timing information. Permissions, password
  hashes, and credential versions are not embedded. Raw tokens are not stored
  server-side or written to logs.

These policies use the algorithm and token-type separation guidance in
[RFC 8725](https://www.rfc-editor.org/rfc/rfc8725.html) and refresh replay detection
in [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.14.2).

## State and immediate revocation

| Store | Data |
| --- | --- |
| PostgreSQL | Users, Argon2id password hashes, current role/permissions, unique case-insensitive email addresses, and a UUIDv7 `authentication_snapshot_version` per user. |
| Redis/Dragonfly | Per-device credential version, absolute expiration, and a user session generation for both mechanisms. Browser credentials have digest lookup keys; JWT sessions additionally store current/previous refresh digests, public rotation claims, and a fixed overlap deadline. Session and lookup keys expire automatically; generation keys remain until explicitly cleaned up. Rate-limit keys expire after their window. |

Every browser-session or access-token authentication checks the Redis session
and the current PostgreSQL credential version, and loads the user's current
role. Both mechanisms therefore use server-side revocation state. PostgreSQL
and Redis must be available; storage errors fail closed with HTTP 503 rather
than accepting credentials without
revocation or authorization checks. Authentication must read PostgreSQL primary
state; replica lag would delay credential and permission changes.

Migration `000004_add_authentication` adds a database trigger that changes the
credential version whenever the password hash or email changes. Migration
`000008_security_refactor` renames the column, its constraints, the trigger, and
its function to use `authentication_snapshot_version`. Migration
`000009_add_user_timestamps` adds database-owned account timestamps and the
update timestamp used for optimistic writes. Password changes therefore
invalidate browser sessions and
access/refresh tokens on **all devices, including the current one**, without a
separate Redis operation. User updates compare the loaded `UpdatedAt` atomically
in SQL; a concurrent password change rejects the stale update with
`security.ErrUserConflict`, preventing a profile update from restoring the old
password. To change passwords through the repository, load the existing user,
assign `PasswordHash`, and then call `Update`; reload the user before another update.
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
live deployment: flush session state after such a restore so individual logout
and refresh replay revocations cannot be undone. Rotating only the JWT signing
key does not invalidate browser sessions.
Account-wide revocation and password changes remain guarded by PostgreSQL.

## Configuration and migrations

Start from [`.env.example`](../.env.example). `config.Load` reads `.env` with
`godotenv`; existing environment variables take precedence.

For an API transport using JWT, generate a stable signing seed with
`openssl rand -base64 32` and store it in `JWT_SIGNING_SEED` through your secret
management system. There is no generated
startup key or insecure signing fallback. Keep this seed consistent across
instances and restarts. Do not commit it. Set `JWT_KEY_ID` to identify the key.
The JWT manager can accept previous public keys through `VerificationKeys` for
planned key rotation; `Config.SigningKey` currently loads one signing key.
Changing that key without retaining its public key invalidates existing JWTs.
The admin-only server does not initialize JWT signing or require a signing seed.

`DATABASE_URL`, `REDIS_URL`, issuer/audience settings, token TTLs, cookie flags,
and the listen address are listed in `.env.example`. PostgreSQL 18 or later is
required for the built-in `uuidv7()` used by credential invalidation, defaults,
and rotation triggers. An operator must apply migration `000007_use_uuid_v7`
before deploying this server. Migration execution is
never part of application startup. Resolve duplicate `lower(email)` values in
existing users before applying the new unique index.

Production must use HTTPS, including across untrusted proxy hops, and keep
`AUTH_COOKIE_SECURE=true`. Secure cookies use the `__Host-` prefix, `Path=/`,
`HttpOnly`, and `SameSite=Strict`, with no Domain attribute. For local HTTP
only, `AUTH_COOKIE_SECURE=false` uses ordinary cookie names. The admin
authentication cookie contains only the opaque session ID.
JWT transport credentials are handled by API clients; the admin does not accept
or issue access or refresh token cookies.
No authentication responses may be cached.

Sign-in limits are shared across instances: 10 requests per account and 100 per
client address per 15 minutes. Successful attempts also count. Identifiers in
rate-limit keys are hashed. Forwarded address headers are not trusted; behind
a proxy, the address limit applies to the proxy unless a trusted deployment
layer supplies a verified client `RemoteAddr`. Password verification work is
limited to four concurrent operations per application instance; further requests
return 503 immediately rather than accumulating a password-verification queue.
Unknown accounts verify against a dummy Argon2id hash and return the same 401
as wrong passwords.

## Admin web transport

| Route | Request | Success |
| --- | --- | --- |
| `GET /admin/{language}/sign-in` | Browser navigation | HTML sign-in page, or redirect to admin home with a valid session cookie |
| `POST /admin/{language}/sign-in` | Native `application/x-www-form-urlencoded` form with `email` and `password` | New server-side session, one opaque session cookie, and a 303 redirect to `/admin/{language}` |
| `GET /admin/{language}` | Session cookie | Authenticated admin page with a sign-out form |
| `POST /admin/{language}/sign-out` | Session cookie | Device session revoked, session cookie cleared, and a 303 redirect to sign-in |

The former admin refresh route is removed. Existing JWT admin cookies no longer
authenticate the panel; users sign in again to establish an opaque session. The
JWT token policy and application flow remain unchanged for API consumers.

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

Every protected admin request calls `SessionAuthenticationService.Authenticate`
through the common admin authentication helper. Normal SSR navigation, native
form submissions, and HTMX requests automatically supply the same cookie. The
helper resolves the current principal and removes invalid session cookies before
redirecting to sign-in. Authorization consumes that principal through
`app.Authorize`; no JWT-specific permissions or duplicated authorization paths
exist. Sign-out also removes a cookie for an already invalid session; storage
failures return 503 instead of reporting successful revocation.

Browser requests use same-origin HttpOnly cookies; Bearer headers and JWT cookies
are not admin authentication mechanisms. All admin routes are wrapped in Go's
`http.CrossOriginProtection`, checking Origin and Fetch Metadata for mutations;
no trusted cross-origin exceptions or CORS access are configured. GET requests
do not renew authentication state. Pages and errors have `Cache-Control: no-store`.
Native form success redirects load a full document; GET page navigation also
supports HTMX fragments. Password restoration remains a presentation component
without submission behavior. HTMX history snapshots are disabled with
`hx-history="false"`; Back and Forward fetch full documents from the server so
authentication is checked again.

Account-wide sign-out remains an application use case with no public transport
route. Trusted callers can use `AuthenticationService.SignOutAll` with an
already authenticated principal or `RevokeUserSessions` with a user ID.
Future protected transports must authenticate with their selected mechanism
and check the returned principal before accessing content or performing actions.

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
race-enabled tests. Tests cover opaque session login, server-side expiration,
logout and account-wide invalidation, cookie protections, SSR and HTMX requests,
admin rejection of JWT credentials, shared authorization and current permissions,
strict JWT validation, access/refresh separation, key rotation, concurrent refresh
across instances, fixed-window replay detection, absolute and idle expiration,
credential changes, deletion, Redis loss/outages, CSRF, request validation, and
throttling. TestMain uses goleak.

Additional integration tests exercise opaque session persistence and JWT
rotation against an isolated real Redis instance:

```sh
FARYEN_TEST_REDIS_SOCKET=/path/to/isolated/redis.sock \
  go test -race -count=1 -timeout 60s -tags=integration \
  -run TestIntegrationRedis ./internal/security/redis
```

The integration test never connects to PostgreSQL or runs migrations. The
PostgreSQL migration has been written for an operator to apply; its trigger has
not been executed by the automated tests.
