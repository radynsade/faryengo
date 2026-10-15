package middleware

import "net/http"

// A responseRecorder remembers the status and size of the response written
// through it. Unwrap lets http.ResponseController reach the original writer,
// so flushing and deadlines keep working.

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func newResponseRecorder(writer http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: writer}
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}

	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}

	written, err := r.ResponseWriter.Write(body)
	r.bytes += written

	return written, err
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *responseRecorder) started() bool {
	return r.status != 0
}
