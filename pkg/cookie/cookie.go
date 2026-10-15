package cookie

import (
	"net/http"
	"time"
)

//
// Jar
//

// A Jar reads and writes the web applications' cookies, which are always
// HTTP-only, limited to same-site requests, and valid for the whole host.
// Cookies are named by kind, such as "session". Secure cookies use the
// __Host- prefix, which binds them to this host and path and requires HTTPS.

type Jar struct {
	Secure bool
}

func (j Jar) Name(kind string) string {
	prefix := "faryen_"

	if j.Secure {
		prefix = "__Host-faryen_"
	}

	return prefix + kind
}

func (j Jar) Get(request *http.Request, kind string) string {
	var value string

	if cookie, err := request.Cookie(j.Name(kind)); err == nil {
		value = cookie.Value
	}

	return value
}

func (j Jar) Set(writer http.ResponseWriter, kind, value string, expiresAt time.Time) {
	http.SetCookie(writer, &http.Cookie{
		Name:     j.Name(kind),
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   j.Secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
		MaxAge:   max(1, int(time.Until(expiresAt).Seconds())),
	})
}

func (j Jar) Clear(writer http.ResponseWriter, kind string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     j.Name(kind),
		Path:     "/",
		HttpOnly: true,
		Secure:   j.Secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
	})
}
