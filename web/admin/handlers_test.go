package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/web/admin/assets"
)

func TestAdminAssetURLs(t *testing.T) {
	mux, _, _ := httpFixture(t)

	scriptURL, err := assets.BuiltAsset("main.ts")
	if err != nil {
		t.Fatal(err)
	}

	cssURL, err := assets.BuiltCSS("style.scss")
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
	mux, _, _ := httpFixture(t)

	for _, tt := range []struct {
		name          string
		path          string
		htmxHeader    string
		historyHeader string
		fragment      bool
	}{
		{name: "sign-in document", path: "/admin/en/sign-in"},
		{name: "restore document", path: "/admin/en/restore-password"},
		{name: "sign-in fragment", path: "/admin/en/sign-in", htmxHeader: "true", fragment: true},
		{name: "restore fragment", path: "/admin/en/restore-password", htmxHeader: "true", fragment: true},
		{name: "language path fragment", path: "/admin/lv/restore-password", htmxHeader: "true", fragment: true},
		{name: "false header document", path: "/admin/en/sign-in", htmxHeader: "false"},
		{name: "history document", path: "/admin/en/sign-in", historyHeader: "true"},
		{name: "history document with request header", path: "/admin/en/sign-in", htmxHeader: "true", historyHeader: "true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Header.Set("HX-Request", tt.htmxHeader)
			request.Header.Set("HX-History-Restore-Request", tt.historyHeader)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}

			if vary := strings.Join(response.Header().Values("Vary"), ", "); vary != "HX-Request, HX-History-Restore-Request" {
				t.Fatalf("Vary = %q", vary)
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

			if !tt.fragment && !strings.Contains(body, `hx-history="false"`) {
				t.Fatal("admin document allows history snapshots")
			}
		})
	}
}
