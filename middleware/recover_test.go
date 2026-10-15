package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecover(t *testing.T) {
	failPage := func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte("error page"))
	}

	tests := []struct {
		name       string
		fail       http.HandlerFunc
		handler    http.HandlerFunc
		wantStatus int
		wantBody   string
		wantAbort  bool
		wantLogged bool
	}{
		{
			name:       "no panic",
			fail:       failPage,
			handler:    func(writer http.ResponseWriter, _ *http.Request) { _, _ = writer.Write([]byte("ok")) },
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "panic before writing renders the failure",
			fail:       failPage,
			handler:    func(http.ResponseWriter, *http.Request) { panic("boom") },
			wantStatus: http.StatusInternalServerError,
			wantBody:   "error page",
			wantLogged: true,
		},
		{
			name:       "panic without a failure handler",
			handler:    func(http.ResponseWriter, *http.Request) { panic("boom") },
			wantStatus: http.StatusInternalServerError,
			wantBody:   http.StatusText(http.StatusInternalServerError),
			wantLogged: true,
		},
		{
			name: "panic after writing aborts",
			fail: failPage,
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte("partial"))
				panic("boom")
			},
			wantAbort:  true,
			wantLogged: true,
		},
		{
			name:      "deliberate abort is not logged",
			fail:      failPage,
			handler:   func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) },
			wantAbort: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer

			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })

			recorder := httptest.NewRecorder()
			aborted := false

			func() {
				defer func() {
					aborted = recover() == http.ErrAbortHandler
				}()

				Recover(test.fail, test.handler).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			}()

			if aborted != test.wantAbort {
				t.Fatalf("aborted = %v, want %v", aborted, test.wantAbort)
			}

			if !test.wantAbort && (recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), test.wantBody)) {
				t.Fatalf("got (%d, %q), want (%d, %q)", recorder.Code, recorder.Body.String(), test.wantStatus, test.wantBody)
			}

			if logged := strings.Contains(logs.String(), "handler panicked"); logged != test.wantLogged {
				t.Fatalf("logged = %v, want %v: %s", logged, test.wantLogged, logs.String())
			}
		})
	}
}

func TestAccessLog(t *testing.T) {
	var logs bytes.Buffer

	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	handler := AccessLog(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTeapot)
		_, _ = writer.Write([]byte("tea"))
	}))

	request := httptest.NewRequest(http.MethodPost, "/office/en/sign-in?token=secret", nil)
	request.RemoteAddr = "203.0.113.9:4000"

	handler.ServeHTTP(httptest.NewRecorder(), request)

	line := logs.String()

	for _, want := range []string{"method=POST", "path=/office/en/sign-in", "status=418", "bytes=3", "client=203.0.113.9"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log %q lacks %q", line, want)
		}
	}

	if strings.Contains(line, "secret") {
		t.Fatalf("log %q contains the query string", line)
	}
}

func TestSecurityHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()

	SecurityHeaders(http.NotFoundHandler()).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	for name, want := range map[string]string{
		"Content-Security-Policy": contentSecurityPolicy,
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "same-origin",
	} {
		if got := recorder.Header().Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}
