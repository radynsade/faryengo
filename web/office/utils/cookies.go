package utils

import (
	"net/http"
	"time"
)

//
// Cookies
//

// The office keeps its own session cookie, so signing in to the office does
// not sign in to the admin panel or the other way round. Secure cookies use
// the __Host- prefix, which binds them to this host and path and requires
// HTTPS.

const SessionCookie = "office_session"

type Cookies struct {
	Secure bool
}

func (c Cookies) Name(kind string) string {
	prefix := "faryen_"

	if c.Secure {
		prefix = "__Host-faryen_"
	}

	return prefix + kind
}

func (c Cookies) Get(request *http.Request, kind string) string {
	var value string

	if cookie, err := request.Cookie(c.Name(kind)); err == nil {
		value = cookie.Value
	}

	return value
}

func (c Cookies) Set(writer http.ResponseWriter, kind, value string, expiresAt time.Time) {
	http.SetCookie(writer, &http.Cookie{
		Name:     c.Name(kind),
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
		MaxAge:   max(1, int(time.Until(expiresAt).Seconds())),
	})
}

func (c Cookies) Clear(writer http.ResponseWriter, kind string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     c.Name(kind),
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
	})
}
