package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/web/admin/assets"
)

func TestAdminAssetURLs(t *testing.T) {
	mux := http.NewServeMux()

	if err := RegisterHandlers(mux); err != nil {
		t.Fatal(err)
	}

	scriptURL, err := assets.BuiltAsset("src/main.ts")
	if err != nil {
		t.Fatal(err)
	}

	cssURL, err := assets.BuiltCSS("src/style.scss")
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/en/sign-in", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `src="`+scriptURL+`"`) || !strings.Contains(response.Body.String(), `href="`+cssURL+`"`) {
		t.Fatalf("sign-in response = %d %q", response.Code, response.Body.String())
	}

	for _, tt := range []struct {
		name string
		url  string
	}{
		{name: "script alias", url: scriptURL},
		{name: "CSS alias", url: cssURL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.url, nil))
			if response.Code != http.StatusOK || response.Body.Len() == 0 {
				t.Fatalf("asset URL %q returned %d %q", tt.url, response.Code, response.Body.String())
			}
		})
	}
}

func TestAdminPageResponses(t *testing.T) {
	mux := http.NewServeMux()

	if err := RegisterHandlers(mux); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name           string
		path           string
		datastarHeader string
		fragment       bool
	}{
		{name: "sign-in document", path: "/admin/en/sign-in"},
		{name: "restore document", path: "/admin/en/restore-password"},
		{name: "sign-in fragment", path: "/admin/en/sign-in", datastarHeader: "true", fragment: true},
		{name: "restore fragment", path: "/admin/en/restore-password", datastarHeader: "true", fragment: true},
		{name: "language path fragment", path: "/admin/lv/restore-password", datastarHeader: "true", fragment: true},
		{name: "false header document", path: "/admin/en/sign-in", datastarHeader: "false"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Header.Set("Datastar-Request", tt.datastarHeader)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}

			if response.Header().Get("Vary") != "Datastar-Request" {
				t.Fatalf("Vary = %q", response.Header().Get("Vary"))
			}

			if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
			}

			body := response.Body.String()
			isDocument := strings.Contains(body, "<!doctype html>")

			if isDocument == tt.fragment {
				t.Fatalf("document = %t, fragment requested = %t", isDocument, tt.fragment)
			}

			for _, target := range []string{`id="page-title"`, `id="page-content"`} {
				if strings.Count(body, target) != 1 {
					t.Fatalf("expected one patch target %s in %s", target, body)
				}
			}

			if tt.fragment && (strings.Contains(body, "<script") || strings.Contains(body, "<link")) {
				t.Fatalf("fragment reloads assets: %s", body)
			}
		})
	}
}
