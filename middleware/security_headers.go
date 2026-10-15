package middleware

import "net/http"

// The policy allows only the server's own scripts, styles, and connections,
// so injected markup cannot run code or load anything from elsewhere. Fonts
// and images may also be inlined as data URLs by the asset build. No page may
// be framed, which keeps it from being overlaid by another site.

const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' data:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

// SecurityHeaders sets the browser protections shared by every response.
// X-Frame-Options repeats frame-ancestors for browsers that predate it.

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := writer.Header()

		header.Set("Content-Security-Policy", contentSecurityPolicy)
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "same-origin")

		next.ServeHTTP(writer, request)
	})
}
