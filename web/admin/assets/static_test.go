package assets

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testStaticFont = "nunito-v32-cyrillic_cyrillic-ext_latin_latin-ext-regular.woff2"

func TestStaticAsset(t *testing.T) {
	for _, tt := range []struct {
		name   string
		source string
		want   string
		err    error
	}{
		{name: "font", source: testStaticFont, want: staticURLPrefix + testStaticFont},
		{name: "missing", source: "missing.woff2", err: ErrStaticAssetNotFound},
		{name: "empty", err: ErrStaticAssetNotFound},
		{name: "directory", source: ".", err: ErrStaticAssetNotFound},
		{name: "absolute", source: "/" + testStaticFont, err: ErrStaticAssetNotFound},
		{name: "parent traversal", source: "../dist/.vite/manifest.json", err: ErrStaticAssetNotFound},
		{name: "nested traversal", source: "fonts/../" + testStaticFont, err: ErrStaticAssetNotFound},
		{name: "backslash", source: `fonts\font.woff2`, err: ErrStaticAssetNotFound},
		{name: "hidden", source: ".secret", err: ErrStaticAssetNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StaticAsset(tt.source)

			if got != tt.want || !errors.Is(err, tt.err) {
				t.Fatalf("StaticAsset(%q) = (%q, %v), want (%q, %v)", tt.source, got, err, tt.want, tt.err)
			}
		})
	}
}

func TestServeStaticAssets(t *testing.T) {
	font, err := fs.ReadFile(embeddedFiles, "static/"+testStaticFont)

	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()

	if err := RegisterHandlers(mux); err != nil {
		t.Fatal(err)
	}

	assetURL, err := StaticAsset(testStaticFont)

	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, method, url, byteRange string
		status                       int
		body                         string
	}{
		{name: "font", method: http.MethodGet, url: assetURL, status: http.StatusOK, body: string(font)},
		{name: "HEAD", method: http.MethodHead, url: assetURL, status: http.StatusOK},
		{name: "range", method: http.MethodGet, url: assetURL, byteRange: "bytes=0-3", status: http.StatusPartialContent, body: string(font[:4])},
		{name: "missing", method: http.MethodGet, url: staticURLPrefix + "missing.woff2", status: http.StatusNotFound},
		{name: "directory", method: http.MethodGet, url: staticURLPrefix, status: http.StatusNotFound},
		{name: "hidden", method: http.MethodGet, url: staticURLPrefix + ".secret", status: http.StatusNotFound},
		{name: "unsupported method", method: http.MethodPost, url: assetURL, status: http.StatusMethodNotAllowed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.url, nil)
			request.Header.Set("Range", tt.byteRange)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != tt.status {
				t.Fatalf("ServeHTTP() status = %d, want %d", response.Code, tt.status)
			}

			if tt.status < http.StatusBadRequest {
				if response.Body.String() != tt.body {
					t.Fatal("ServeHTTP() returned incorrect static content")
				}

				if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Content-Type") != "font/woff2" {
					t.Fatalf("ServeHTTP() headers = %v, want a font response with nosniff", response.Header())
				}
			}

			if tt.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("ServeHTTP() Allow = %q, want GET, HEAD", response.Header().Get("Allow"))
			}
		})
	}
}
