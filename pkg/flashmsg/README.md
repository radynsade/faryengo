# Flash messages

`flashmsg` is a generic, serializable message bag. It imports only the standard
library and does not know about HTTP, Redis, authentication, or an application's
session keys. Applications own storage, atomic updates, and consumption timing.
The admin integration lives in [`web/admin`](../../web/admin/flash_messages.go).

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
