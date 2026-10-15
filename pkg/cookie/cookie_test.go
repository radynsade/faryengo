package cookie

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestJar(t *testing.T) {
	tests := []struct {
		name     string
		jar      Jar
		wantName string
	}{
		{"plain HTTP", Jar{}, "faryen_session"},
		{"secure", Jar{Secure: true}, "__Host-faryen_session"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			test.jar.Set(recorder, "session", "value", time.Now().Add(time.Hour))

			cookies := recorder.Result().Cookies()

			if len(cookies) != 1 {
				t.Fatalf("got %d cookies, want 1", len(cookies))
			}

			set := cookies[0]

			if set.Name != test.wantName || set.Value != "value" || set.Path != "/" || !set.HttpOnly ||
				set.Secure != test.jar.Secure || set.SameSite != http.SameSiteStrictMode || set.MaxAge <= 0 {
				t.Fatalf("got cookie %+v", set)
			}

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.AddCookie(set)

			if got := test.jar.Get(request, "session"); got != "value" {
				t.Fatalf("Get() = %q, want %q", got, "value")
			}

			if got := test.jar.Get(httptest.NewRequest(http.MethodGet, "/", nil), "session"); got != "" {
				t.Fatalf("Get() without the cookie = %q, want empty", got)
			}

			recorder = httptest.NewRecorder()
			test.jar.Clear(recorder, "session")

			cleared := recorder.Result().Cookies()

			if len(cleared) != 1 || cleared[0].Name != test.wantName || cleared[0].MaxAge >= 0 {
				t.Fatalf("got cleared cookies %+v, want one expired %s", cleared, test.wantName)
			}
		})
	}
}
