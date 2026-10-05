# Flash messages

`flashmsg` provides a generic, serializable message bag and session storage
contract. It imports only the standard library and does not depend on HTTP,
Redis, or an application's domain. Applications supply session keys and select
storage and consumption timing. The
[`redis` subpackage](redis/README.md) provides atomic Redis persistence; the admin
integration lives in [`web/admin`](../../web/admin/flash_messages.go).

```go
bag := flashmsg.New()
bag.Add(flashmsg.Success, "Your changes were saved.")
bag.Add(flashmsg.Error, "Enter a name.")

errors := bag.Peek(flashmsg.Error) // Reads without consuming.
successes := bag.Get(flashmsg.Success) // Reads and consumes this type.
remaining := bag.All() // Reads and consumes all remaining messages.
```

The zero value of `Bag` is ready to use. Types are arbitrary strings; `Success`,
`Error`, `Info`, and `Warning` are conventional labels. Multiple messages of a
single type retain insertion order. `Types` sorts labels for deterministic
rendering. `Peek`, `PeekAll`, `Get`, and `All` return independent copies.

`json.Marshal` and `json.Unmarshal` serialize a bag as an object of message
arrays. A failed decode leaves the previous bag intact. Persist the bag in a
server-side session, atomically merge additions, and atomically remove messages
when reading them. A plain read followed by a separate write can lose messages
or show them twice under concurrent requests.

`WithBag(ctx, bag)` and `FromContext(ctx)` let applications pass a request's bag
to rendering code. A missing bag produces an empty bag. Each bag belongs to one
request; it is not designed for shared access by concurrent goroutines.

`FlashSessionStorage` defines `Add(ctx, session, kind, message)` and
`Take(ctx, session, kind)`. `Take` atomically consumes one type, or all types when
`kind` is empty. Both methods use `flashmsg.Session`, whose `Key` identifies the
message session and optional `GenerationKey` identifies its authenticated
generation. The Redis implementation satisfies this interface.
