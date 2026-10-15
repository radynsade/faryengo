# cookie

`pkg/cookie` reads and writes HTTP cookies with one fixed security policy, so
every application that uses it names and protects its cookies the same way.

Import `github.com/radynsade/faryengo/pkg/cookie`.

## Jar

| Method | Purpose |
| --- | --- |
| `Name(kind)` | Returns the cookie name for a kind, such as `session`. |
| `Get(request, kind)` | Returns the cookie's value, or an empty string when it is missing. |
| `Set(writer, kind, value, expiresAt)` | Writes the cookie until `expiresAt`. |
| `Clear(writer, kind)` | Tells the browser to delete the cookie. |

Every cookie is HTTP-only, `SameSite=Strict`, and valid for the path `/`. A
cookie is named `faryen_<kind>`. With `Secure` set, it is named
`__Host-faryen_<kind>` and is sent only over HTTPS; the `__Host-` prefix binds
it to the exact host. `Set` gives the cookie both an expiry time and a
`Max-Age` of at least one second.

A `Jar` holds no state besides `Secure`, so its zero value is usable and
copies are interchangeable.

## Usage conventions

Keep the cookie kinds as constants in the application that owns them, and
give each application its own kinds where its sessions must stay separate,
such as `session` for the admin panel and `office_session` for the office.
Set `Secure` from configuration; leave it off only for local development over
plain HTTP.
