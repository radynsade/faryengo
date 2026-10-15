package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// AccessLog logs every request once it has been answered, including one whose
// handler panicked. Only the path is logged, never the query string, so values
// passed in it stay out of the logs.

func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		recorder := newResponseRecorder(writer)
		started := time.Now()

		// A handler that writes nothing is answered with 200 by net/http.
		defer func() {
			status := recorder.status

			if status == 0 {
				status = http.StatusOK
			}

			slog.InfoContext(
				request.Context(),
				"http request",
				"method", request.Method,
				"path", request.URL.Path,
				"status", status,
				"bytes", recorder.bytes,
				"duration", time.Since(started),
				"client", RemoteHost(request),
			)
		}()

		next.ServeHTTP(recorder, request)
	})
}
