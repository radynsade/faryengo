package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/radynsade/faryengo/web/admin/assets"
)

func TestAdminAssetURLs(t *testing.T) {
	built := fstest.MapFS{
		".vite/manifest.json": {Data: []byte(`{
			"src/main.ts":{"file":"main-123.js"},
			"src/style.scss":{"file":"style-123.css"}
		}`)},
		"main-123.js":   {Data: []byte("console.log('admin');")},
		"style-123.css": {Data: []byte("body{color:red}")},
	}
	mux := http.NewServeMux()

	if err := RegisterHandlers(context.Background(), mux, built); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/en/sign-in", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `src="/assets/admin/main-123.js"`) || !strings.Contains(response.Body.String(), `href="/assets/admin/style-123.css"`) {
		t.Fatalf("sign-in response = %d %q", response.Code, response.Body.String())
	}

	for _, tt := range []struct {
		name   string
		lookup func(string) (string, error)
		source string
		body   string
	}{
		{name: "script alias", lookup: assets.BuiltAsset, source: "src/main.ts", body: "console.log('admin');"},
		{name: "CSS alias", lookup: assets.BuiltCSS, source: "src/style.scss", body: "body{color:red}"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, err := tt.lookup(tt.source)
			if err != nil {
				t.Fatal(err)
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url, nil))
			if response.Code != http.StatusOK || response.Body.String() != tt.body {
				t.Fatalf("asset URL %q returned %d %q", url, response.Code, response.Body.String())
			}
		})
	}
}
