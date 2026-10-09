# flashmsg

`pkg/flashmsg` provides a typed, JSON-serializable bag of flash messages and a
storage contract for persisting them between requests. The core package imports
only the standard library and has no dependency on HTTP, Redis, or the domain.
The `pkg/flashmsg/redis` subpackage implements atomic storage in Redis session
hashes.

Applications own everything around the bag: they choose the session key, set
cookies, pick the storage, and decide when messages are consumed. The admin
integration in [`web/admin/utils/flashes.go`](../../web/admin/utils/flashes.go)
selects sessions, manages guest cookies, and maps storage errors to HTTP
responses. `cmd/server` constructs the store with the shared Redis client and
passes it to `handlers.NewHandler` in `web/admin/handlers` as a
`FlashSessionStorage`.

Imports:

```go
import (
	"github.com/radynsade/faryengo/pkg/flashmsg"
	flashredis "github.com/radynsade/faryengo/pkg/flashmsg/redis"
)
```

## Bag

`Bag` groups messages by type. Types are arbitrary strings; `flashmsg.Success`,
`flashmsg.Error`, `flashmsg.Info`, and `flashmsg.Warning` are conventional
labels. The zero value is ready to use, and `New()` returns an empty bag.
Messages remain available until consumed, following
[Symfony's flash-message behavior](https://symfony.com/doc/current/session.html#flash-messages).

| Method | Behavior |
| --- | --- |
| `Add(kind, message)` | Appends a message. Earlier messages of the same type are kept in insertion order. |
| `Peek(kind)` | Returns a copy of one type's messages without consuming them. |
| `Get(kind)` | Returns and removes one type's messages. |
| `Types()` | Returns the types present, sorted for deterministic rendering. |
| `PeekAll()` | Returns a deep copy of all messages without consuming them. |
| `All()` | Returns and removes all messages. |

```go
bag := flashmsg.New()
bag.Add(flashmsg.Success, "Role created.")
bag.Add(flashmsg.Error, "Unable to load roles.")
bag.Add(flashmsg.Error, "Unable to delete the role.")

bag.Types()               // ["error", "success"]
bag.Peek(flashmsg.Error)  // Both errors; the bag still holds them.
bag.Get(flashmsg.Success) // ["Role created."]; success messages are removed.
bag.All()                 // map[error:[...]]; the bag is now empty.
```

Returned slices and maps are independent copies, so callers may modify them
freely. A bag belongs to one request and must not be shared between goroutines.

### JSON

`json.Marshal` encodes a bag as an object of message arrays, and
`json.Unmarshal` decodes it back. A failed decode returns a wrapped error and
leaves the bag's previous contents intact.

```go
data, err := json.Marshal(bag)
if err != nil {
	return err
}

restored := flashmsg.New()
if err := json.Unmarshal(data, restored); err != nil {
	return err
}
```

### Request context

`WithBag` attaches a bag to a context, and `FromContext` retrieves it. When no
bag is attached, `FromContext` returns a new empty bag, so rendering code never
has to check for `nil`.

```go
ctx = flashmsg.WithBag(ctx, bag)

// Later, in a template or component:
for _, kind := range flashmsg.FromContext(ctx).Types() {
	// Render each type's messages.
}
```

## Session storage

`FlashSessionStorage` is the contract for persisting messages across requests:

```go
type FlashSessionStorage interface {
	Add(ctx context.Context, session Session, kind, message string) error
	Take(ctx context.Context, session Session, kind string) (*Bag, error)
}
```

`Take` atomically consumes one type, or every type when `kind` is empty.
Implementations must merge additions and consume messages atomically: a plain
read followed by a separate write can drop messages or show them twice when
requests run concurrently.

`Session` identifies where messages live. `Key` names the message session.
`GenerationKey` is empty for anonymous sessions; for authenticated sessions it
names the key holding the session's current generation, so revoked sessions
cannot receive or reveal messages.

Depend on the interface in handlers so tests can substitute an in-memory fake:

```go
type Handler struct {
	flashes flashmsg.FlashSessionStorage
}
```

## Redis storage

`flashredis.Store` implements `FlashSessionStorage`. Each operation is a single
Lua script, so concurrent additions are merged and a consumed message cannot be
replayed. Messages are stored as JSON in the `<application>:flashes` field of the
session hash; other fields are left untouched.

```go
client := redis.NewClient(&redis.Options{
	Addr:       "localhost:6379",
	MaxRetries: -1, // Never replay an ambiguous add or take.
})
defer client.Close()

store, err := flashredis.NewStore(client, "admin", 15*time.Minute)
if err != nil {
	return fmt.Errorf("configure flash store: %w", err)
}
```

`NewStore` requires a non-nil client, a nonblank application name, and an
anonymous-session lifetime of at least one second (whole seconds are used).
Otherwise it returns `flashredis.ErrInvalidConfig`. The caller owns the client
and closes it.

### Anonymous sessions

Leave `GenerationKey` empty. `Add` creates the hash if needed, records the
application name, and sets the configured TTL. `Take` on a missing or expired
session returns an empty bag without creating a key, and reads never extend the
lifetime. Use keys that are unique per application.

```go
session := flashmsg.Session{Key: "faryen:web:admin:session:{" + anonymousID + "}"}

if err := store.Add(ctx, session, flashmsg.Error, "Invalid email or password."); err != nil {
	return fmt.Errorf("store notification: %w", err)
}
```

### Authenticated sessions

Set both keys; they must share a Redis Cluster hash slot, so use a common hash
tag. The session hash must contain a `generation` field equal to the value at
`GenerationKey` and an `expires` field holding the absolute expiration in Unix
milliseconds. Messages share the session's existing expiration. If the session
is missing, expired, or its generation no longer matches, both methods return
`flashredis.ErrSessionRevoked` without creating or extending anything.

```go
session := flashmsg.Session{
	Key:           "faryen:users:{" + userID + "}:session:" + sessionID,
	GenerationKey: "faryen:users:{" + userID + "}:generation",
}

bag, err := store.Take(ctx, session, "") // Consume every type.
if errors.Is(err, flashredis.ErrSessionRevoked) {
	// Treat the user as signed out.
}
```

A blank `Key` returns `flashredis.ErrInvalidSession`. All errors wrap their
causes for `errors.Is` and `errors.As`.

## Usage conventions

Flash handling belongs to the transport layer. Domain and application services
do not depend on flashes. The conventions below describe the project's usage;
the generic bag does not enforce them.

### Session naming and lifetime

The admin uses these Redis names:

| Purpose | Name | Meaning |
| --- | --- | --- |
| Signed-in `Session.Key` | `faryen:web:admin:flashes:session:{sessionID}` | Messages of one device Session, selected from the verified Session's internal identity. |
| Guest `Session.Key` | `faryen:web:admin:flashes:guest:{randomID}` | Messages of a visitor who has not signed in, identified by a random UUIDv7. |
| Message hash field | `admin:flashes` | JSON bag in the flash hash; other applications use their own `<application>:flashes` field. |

Replace `sessionID` and `randomID` with their identifier values; the braces are
literal Redis hash tags, not placeholder delimiters. The Session identity is the
internal device ID, never the opaque authentication cookie value. Both kinds of
flash session use the store's anonymous mode, so `GenerationKey` stays empty and
each addition renews a 15-minute TTL. The distinct prefixes keep a guest cookie
from ever naming a device Session's messages. The storage accepts
application-selected keys; these prefixes are admin conventions.

Select a signed-in flash session from the request's verified identity, never
from URL parameters. Do not use `notice` or `notice_name` query parameters to
supply notifications. Flashes survive navigation and form submissions with the
existing opaque session cookie; no extra authenticated cookie is needed.
Authentication runs before a signed-in flash session is used, so the messages of
an ended, expired, or revoked Session can no longer be read, and they expire
with their TTL.

Before sign-in, use a separate guest session with an opaque HttpOnly cookie
named `faryen_flash`, or `__Host-faryen_flash` with secure cookies. The admin uses
SameSite Strict, its Secure policy, and a 15-minute TTL. Only adding a message
creates the session; reading an empty guest page creates nothing. Guest
sessions cannot access signed-in messages. Never store authentication tokens,
submitted passwords, email addresses, or other submitted form values in
flashes.

### Choosing and consuming messages

| Situation | Convention |
| --- | --- |
| Successful role creation, update, or deletion | Store a success message after the change succeeds, then redirect to a clean view or list URL. The next rendered admin page consumes it; group multiple successes in the existing success dialog. |
| Authentication, authorization, operational, malformed-request, or loading failure | Store and consume an error in the response displaying it. Preserve the HTTP status and submitted form values, except passwords. |
| Editable field validation | Pass localized errors directly to the form, without flash storage or a notification banner. Mark invalid controls with labels and borders and show messages beneath them. Never render or persist submitted passwords. |
| Deletion failure | Stay in the confirmation dialog and consume only errors, leaving pending successes for a normal page render. |

Consume messages server-side for both full documents and HTMX fragments. A
consumed message must not reappear on Reload or Back, even without JavaScript.
Escape notification text with templ and send admin responses with `Cache-Control: no-store`.
Use atomic additions and consumption; disable automatic storage retries because
replaying an ambiguous operation can duplicate or lose messages.

### Failure handling

Use direct HTTP errors for transport rejections, including rejected cross-origin
requests, and when flash storage is unavailable. Storage failures return 503
without exposing storage errors or reporting a false success.

If a change succeeded but storing its confirmation failed, that change remains
committed. Tell the user to reload before submitting again; do not imply that
the change was rolled back.

## Post/Redirect/Get example

A handler stores a message before redirecting; middleware on the next request
consumes the messages and attaches them to the context for rendering. The
application-specific `renderRoleFailure` helper stores and consumes errors in
the current response, preserving the status and form values as described above.

```go
func (h *Handler) saveRole(writer http.ResponseWriter, request *http.Request) {
	session := h.flashSession(request)
	err := h.roles.Save(request.Context(), role)

	if err != nil {
		h.renderRoleFailure(writer, request, err)
		return
	}

	if err := h.flashes.Add(request.Context(), session, flashmsg.Success, "Role saved."); err != nil {
		http.Error(writer, "Changes were saved, but the confirmation could not be stored. Reload before submitting again.", http.StatusServiceUnavailable)
		return
	}

	http.Redirect(writer, request, "/roles", http.StatusSeeOther)
}

func (h *Handler) withFlashes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bag, err := h.flashes.Take(request.Context(), h.flashSession(request), "")

		if err != nil {
			http.Error(writer, "Unable to load notifications.", http.StatusServiceUnavailable)
			return
		}

		next.ServeHTTP(writer, request.WithContext(flashmsg.WithBag(request.Context(), bag)))
	})
}
```

For an error response or deletion confirmation, pass `flashmsg.Error` to `Take`
and leave the other types for a later page render.
