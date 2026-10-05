# Redis flash storage

`redis.Store` atomically appends and consumes `flashmsg.Bag` messages in a Redis
session hash. It has no dependency on HTTP or Faryen's internal packages. The
application supplies session keys, owns its cookies, and decides when to consume
notifications.

```go
import flashredis "github.com/radynsade/faryengo/pkg/flashmsg/redis"

store, err := flashredis.NewStore(client, "admin", 15*time.Minute)
if err != nil {
    return fmt.Errorf("configure flash store: %w", err)
}

session := flashmsg.Session{
    Key: "sessions:{user}:device",
    GenerationKey: "sessions:{user}:generation",
}
if err := store.Add(ctx, session, flashmsg.Success, "Saved."); err != nil {
    return fmt.Errorf("store notification: %w", err)
}

bag, err := store.Take(ctx, session, "") // Consume all types.
if err != nil {
    return fmt.Errorf("read notifications: %w", err)
}
successes := bag.Get(flashmsg.Success)
```

Messages occupy the `<application>:flashes` hash field, leaving other fields
untouched. `Take` consumes only the requested type when its final argument is
nonempty; an empty type consumes the complete bag. Additions preserve message
order within each type. Concurrent consumers cannot replay a message.

Authenticated sessions supply both keys, which must share a Redis Cluster hash
slot. The session hash must contain `generation` matching the generation key's
value and `expires` containing its absolute expiration as Unix milliseconds.
Every operation checks these values. Missing, expired, or revoked sessions return
`ErrSessionRevoked`, without creating the session or changing its TTL.

An empty `GenerationKey` selects an anonymous session. Adding a message creates
the hash, stores its `application`, and sets the configured TTL. Reading a missing
or expired anonymous session returns an empty bag without creating a key. Reads
do not extend the lifetime. Anonymous keys should be unique per application.

The constructor requires a non-nil client, a nonblank application name, and a
lifetime of at least one second. Lifetimes use whole seconds. An empty or blank
session key returns `ErrInvalidSession`; invalid constructor values return
`ErrInvalidConfig`. Errors wrap their causes for `errors.Is` and `errors.As`.

The caller owns and closes the Redis client. Disable automatic retries
(`redis.Options{MaxRetries: -1}`) so an ambiguous add or consume is not replayed.
