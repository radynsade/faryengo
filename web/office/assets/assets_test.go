package assets

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestAssets(t *testing.T) {
	mux := http.NewServeMux()
	if err := RegisterHandlers(mux); err != nil {
		t.Fatal(err)
	}

	script, err := BuiltAsset("main.ts")
	if err != nil {
		t.Fatal(err)
	}

	sprite, err := fs.ReadFile(embeddedFiles, "static/icons.svg")
	if err != nil {
		t.Fatal(err)
	}

	unversioned, _, _ := strings.Cut(Icons, "?")

	for _, tt := range []struct {
		name  string
		url   string
		cache string
		body  string
	}{
		{name: "built files are immutable", url: script, cache: immutable},
		{name: "the versioned sprite is immutable", url: Icons, cache: immutable, body: string(sprite)},
		{name: "unversioned static files are not cached", url: unversioned, cache: "", body: string(sprite)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.url, nil))

			if response.Code != http.StatusOK {
				t.Fatalf("GET %s = %d", tt.url, response.Code)
			}

			if response.Header().Get("Cache-Control") != tt.cache {
				t.Fatalf("Cache-Control = %q, want %q", response.Header().Get("Cache-Control"), tt.cache)
			}

			if tt.body != "" && response.Body.String() != tt.body {
				t.Fatalf("GET %s served other content", tt.url)
			}
		})
	}

	if !strings.HasPrefix(Icons, staticURLPrefix+"icons.svg?v=") {
		t.Fatalf("Icons = %q", Icons)
	}
}
