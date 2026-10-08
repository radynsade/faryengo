# flashmsg

`pkg/flashmsg` provides a typed, JSON-serializable bag of flash messages and a
storage contract for persisting them between requests. The core package imports
only the standard library and has no dependency on HTTP, Redis, or the domain.
The `pkg/flashmsg/redis` subpackage implements atomic storage in Redis session
hashes.

Applications own everything around the bag: they choose the session key, set
cookies, pick the storage, and decide when messages are consumed. For how the
admin panel presents and persists flash messages, see
[flash-messages.md](../flash-messages.md) and
[`web/admin/flash_messages.go`](../../web/admin/flash_messages.go).

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
bag.Add(flashmsg.Error, "Enter a name.")
bag.Add(flashmsg.Error, "Choose at least one permission.")

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
data, err := json.Marshal(bag) // {"error":["Enter a name."]}
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
session := flashmsg.Session{Key: "admin:flash:" + anonymousID}

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
	Key:           "sessions:{" + userID + "}:session:" + sessionID,
	GenerationKey: "sessions:{" + userID + "}:generation",
}

bag, err := store.Take(ctx, session, "") // Consume every type.
if errors.Is(err, flashredis.ErrSessionRevoked) {
	// Treat the user as signed out.
}
```

A blank `Key` returns `flashredis.ErrInvalidSession`. All errors wrap their
causes for `errors.Is` and `errors.As`.

## Post/Redirect/Get example

A handler stores a message before redirecting; middleware on the next request
consumes the messages and attaches them to the context for rendering.

```go
func (h *Handler) saveRole(writer http.ResponseWriter, request *http.Request) {
	session := h.flashSession(request)
	err := h.roles.Save(request.Context(), role)

	if err == nil {
		err = h.flashes.Add(request.Context(), session, flashmsg.Success, "Role saved.")
	}

	if err != nil {
		http.Error(writer, "internal error", http.StatusInternalServerError)
	} else {
		http.Redirect(writer, request, "/roles", http.StatusSeeOther)
	}
}

func (h *Handler) withFlashes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bag, err := h.flashes.Take(request.Context(), h.flashSession(request), "")

		if err != nil {
			bag = flashmsg.New()
		}

		next.ServeHTTP(writer, request.WithContext(flashmsg.WithBag(request.Context(), bag)))
	})
}
```

To show only one type on a page, such as validation errors next to a form, pass
that type to `Take` and leave the others for a later request.
