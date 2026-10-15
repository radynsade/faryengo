package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover turns a panicking handler into a logged failure. While nothing has
// been written yet, fail answers the request, or a plain 500 response when
// fail is nil. A response already under way cannot be replaced, so its
// connection is aborted instead and the client sees it incomplete rather than
// corrupted. A handler that aborts on purpose with http.ErrAbortHandler is
// not logged.

func Recover(fail http.HandlerFunc, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		recorder := newResponseRecorder(writer)

		defer func() {
			if recovered := recover(); recovered != nil {
				failPanicked(fail, recorder, request, recovered)
			}
		}()

		next.ServeHTTP(recorder, request)
	})
}

//
// Helpers
//

func failPanicked(fail http.HandlerFunc, recorder *responseRecorder, request *http.Request, recovered any) {
	aborted := recovered == http.ErrAbortHandler

	if !aborted {
		slog.ErrorContext(
			request.Context(),
			"handler panicked",
			"method", request.Method,
			"path", request.URL.Path,
			"panic", recovered,
			"stack", string(debug.Stack()),
		)
	}

	if aborted || recorder.started() {
		panic(http.ErrAbortHandler)
	} else if fail != nil {
		fail(recorder, request)
	} else {
		http.Error(recorder, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
