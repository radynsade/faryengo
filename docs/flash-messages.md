# Admin flash messages

`pkg/flashmsg` contains the generic message bag, JSON serialization, context
helpers, and the `FlashSessionStorage` interface with its `Session` reference type.
`pkg/flashmsg/redis` owns atomic session persistence through its `Store`,
configured with an application name and anonymous-session lifetime.
`web/admin/flash_messages.go` selects session keys from the request's identity,
manages anonymous cookies, and maps storage errors to transport responses.
`cmd/server` constructs the store with the existing Redis client and passes it to
`admin.NewHandler` through `flashmsg.FlashSessionStorage` (`Add` and `Take`).
Domain and application services do not depend on
flash messages.

The bag follows the consumption behavior of [Symfony's flash
messages](https://symfony.com/doc/current/session.html#flash-messages): append
messages by type, keep them in the session until retrieved, and remove them when
consumed. `Get` consumes one type, `All` consumes the complete bag, and `Peek` /
`PeekAll` leave messages available. See the [package API](../pkg/flashmsg/README.md).

## Storage and lifetime

Authenticated messages are JSON in the `admin:flashes` field of the existing
`faryen:security:{userID}:session:sessionID` Redis hash. No additional authenticated
session cookie is needed. Other applications can use their own session fields.
The admin uses the authenticated principal to locate the device session;
URL parameters never choose the session or supply a notification.

Flashes share the session's existing TTL and survive refresh-token rotation.
Adding or consuming them does not extend either the idle or absolute session
lifetime. Lua checks session generation and absolute expiration before every
operation. A missing, expired, or revoked session cannot be recreated by a flash
write. The normal authentication checks still verify durable credentials and
current permissions before protected handlers run.

Before authentication, error notifications use an anonymous admin session at
`faryen:web:admin:session:{randomID}` with a 15-minute TTL. Only adding a message
creates this session. An opaque HttpOnly cookie identifies it, using the admin's
Secure and SameSite Strict policy. It stores no authentication tokens, submitted
passwords, or email addresses. These anonymous sessions cannot access
an authenticated session's messages.

All additions and reads are atomic Lua operations using the same Redis client
and storage as authentication. The client has automatic retries disabled because
replaying an ambiguous add or consume can duplicate or lose notifications.
Concurrent writers append without overwriting each other, and only one reader
can consume a queued message. Redis key hash tags keep each operation in one slot.

## Rendering

Successful role creation, updates, and deletion enqueue a success message before
redirecting to a clean view/list URL. The next rendered admin page consumes and
shows those messages in the existing success dialog. Multiple successes share
one dialog. There are no `notice` or `notice_name` query parameters to forge,
replay, or clean up in JavaScript.

Authentication, authorization, role validation, and role-loading errors use the
same bag. They are stored and consumed in the response that displays the error,
keeping the current status code and submitted form values. This preserves native
form behavior and avoids persisting passwords or invalid form input. Deletion
failures consume only the error type and stay in the confirmation dialog;
pending success messages remain available for a normal page render.

Full documents and HTMX fragments consume messages server-side. Reload and Back
do not replay an already retrieved message, even without JavaScript. Notifications
are escaped by templ, and admin responses retain `Cache-Control: no-store`.
Transport-level errors such as rejected cross-origin requests and notification
storage failures use direct HTTP errors, since no usable flash session is
available to render them.

Storage failures return 503 without exposing Redis errors or reporting a false
success. A successful role write cannot be rolled back if storing its confirmation
subsequently fails; the response asks the user to reload before submitting again.
