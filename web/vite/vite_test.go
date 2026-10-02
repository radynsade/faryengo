package vite

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/sync/errgroup"
)

func testBuild() fstest.MapFS {
	return fstest.MapFS{
		manifestPath: {Data: []byte(`{
			"src/main.ts": {"file":"main-123.js", "src":"src/main.ts", "css":["style-123.css"], "imports":["_shared.js"], "dynamicImports":["src/lazy.ts"], "assets":["logo image.svg"]},
			"src/style.scss": {"file":"style-123.css", "src":"src/style.scss"},
			"_shared.js": {"file":"shared-123.js"},
			"src/lazy.ts": {"file":"lazy-123.js"},
			"src/logo.svg": {"file":"logo image.svg"}
		}`)},
		"main-123.js":    {Data: []byte("console.log('admin');")},
		"style-123.css":  {Data: []byte("body{color:red}")},
		"shared-123.js":  {Data: []byte("export const shared=1;")},
		"lazy-123.js":    {Data: []byte("export const lazy=1;")},
		"logo image.svg": {Data: []byte("<svg></svg>")},
		"unlisted.txt":   {Data: []byte("not a public asset")},
	}
}

func TestBuiltURLs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		baseURL string
		source  string
		css     bool
		want    string
		wantErr error
	}{
		{name: "script", baseURL: "/admin/assets/", source: "src/main.ts", want: "/admin/assets/main-123.js"},
		{name: "stylesheet", baseURL: "/admin/assets/", source: "src/style.scss", css: true, want: "/admin/assets/style-123.css"},
		{name: "absolute URL", baseURL: "https://cdn.example.com/admin", source: "src/main.ts", want: "https://cdn.example.com/admin/main-123.js"},
		{name: "root URL", baseURL: "/", source: "src/main.ts", want: "/main-123.js"},
		{name: "escaped output", baseURL: "/admin/assets/", source: "src/logo.svg", want: "/admin/assets/logo%20image.svg"},
		{name: "missing source", baseURL: "/admin/assets/", source: "src/missing.js", wantErr: ErrAssetNotFound},
		{name: "non-CSS entry", baseURL: "/admin/assets/", source: "src/main.ts", css: true, wantErr: ErrNotCSS},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := New()
			if err := server.Load(context.Background(), testBuild(), tt.baseURL); err != nil {
				t.Fatal(err)
			}

			lookup := server.BuiltAsset
			if tt.css {
				lookup = server.BuiltCSS
			}

			got, err := lookup(tt.source)
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Fatalf("lookup(%q) = (%q, %v), want (%q, %v)", tt.source, got, err, tt.want, tt.wantErr)
			}

			if tt.wantErr == nil {
				response := httptest.NewRecorder()
				server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, got, nil))
				if response.Code != http.StatusOK || response.Body.Len() == 0 {
					t.Fatalf("resolved URL %q returned %d: %q", got, response.Code, response.Body.String())
				}
			}
		})
	}
}

func TestServeHTTP(t *testing.T) {
	server := New()
	if err := server.Load(context.Background(), testBuild(), "/admin/assets/"); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name        string
		method      string
		url         string
		rangeHeader string
		status      int
		body        string
	}{
		{name: "script", method: http.MethodGet, url: "/admin/assets/main-123.js", status: http.StatusOK, body: "console.log('admin');"},
		{name: "CSS", method: http.MethodGet, url: "/admin/assets/style-123.css", status: http.StatusOK, body: "body{color:red}"},
		{name: "shared chunk", method: http.MethodGet, url: "/admin/assets/shared-123.js", status: http.StatusOK, body: "export const shared=1;"},
		{name: "HEAD", method: http.MethodHead, url: "/admin/assets/main-123.js", status: http.StatusOK},
		{name: "byte range", method: http.MethodGet, url: "/admin/assets/main-123.js", rangeHeader: "bytes=0-6", status: http.StatusPartialContent, body: "console"},
		{name: "unknown", method: http.MethodGet, url: "/admin/assets/unknown.js", status: http.StatusNotFound},
		{name: "unlisted", method: http.MethodGet, url: "/admin/assets/unlisted.txt", status: http.StatusNotFound},
		{name: "manifest", method: http.MethodGet, url: "/admin/assets/.vite/manifest.json", status: http.StatusNotFound},
		{name: "directory", method: http.MethodGet, url: "/admin/assets/", status: http.StatusNotFound},
		{name: "traversal", method: http.MethodGet, url: "/admin/assets/%2e%2e/main-123.js", status: http.StatusNotFound},
		{name: "wrong prefix", method: http.MethodGet, url: "/other/main-123.js", status: http.StatusNotFound},
		{name: "unsupported method", method: http.MethodPost, url: "/admin/assets/main-123.js", status: http.StatusMethodNotAllowed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.url, nil)
			request.Header.Set("Range", tt.rangeHeader)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			if response.Code != tt.status {
				t.Fatalf("ServeHTTP() status = %d, want %d", response.Code, tt.status)
			}

			if tt.status < http.StatusBadRequest && response.Body.String() != tt.body {
				t.Fatalf("ServeHTTP() body = %q, want %q", response.Body.String(), tt.body)
			}

			if tt.status == http.StatusOK && strings.HasSuffix(tt.url, ".css") && !strings.HasPrefix(response.Header().Get("Content-Type"), "text/css") {
				t.Fatalf("Content-Type = %q, want CSS", response.Header().Get("Content-Type"))
			}

			if tt.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("Allow = %q, want GET, HEAD", response.Header().Get("Allow"))
			}
		})
	}
}

func TestLoadFailures(t *testing.T) {
	for _, tt := range []struct {
		name     string
		manifest string
		baseURL  string
		wantErr  error
	}{
		{name: "missing manifest", baseURL: "/assets/", wantErr: fs.ErrNotExist},
		{name: "invalid JSON", manifest: "{", baseURL: "/assets/", wantErr: ErrInvalidManifest},
		{name: "empty manifest", manifest: "{}", baseURL: "/assets/", wantErr: ErrInvalidManifest},
		{name: "missing output", manifest: `{"src/main.ts":{"file":"missing.js"}}`, baseURL: "/assets/", wantErr: fs.ErrNotExist},
		{name: "traversing output", manifest: `{"src/main.ts":{"file":"../outside.js"}}`, baseURL: "/assets/", wantErr: ErrInvalidManifest},
		{name: "missing chunk", manifest: `{"src/main.ts":{"file":"main-123.js","imports":["_missing.js"]}}`, baseURL: "/assets/", wantErr: ErrInvalidManifest},
		{name: "relative base", manifest: "{}", baseURL: "assets/", wantErr: ErrInvalidBaseURL},
		{name: "base query", manifest: "{}", baseURL: "/assets/?key=value", wantErr: ErrInvalidBaseURL},
		{name: "unsafe scheme", manifest: "{}", baseURL: "javascript:alert(1)", wantErr: ErrInvalidBaseURL},
		{name: "protocol-relative base", manifest: "{}", baseURL: "//example.com/assets/", wantErr: ErrInvalidBaseURL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			built := fstest.MapFS{}
			if tt.manifest != "" {
				built[manifestPath] = &fstest.MapFile{Data: []byte(tt.manifest)}
			}

			server := New()
			if err := server.Load(context.Background(), built, tt.baseURL); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Load() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	server := New()
	if err := server.Load(context.Background(), nil, "/assets/"); !errors.Is(err, ErrNilFilesystem) {
		t.Fatalf("Load(nil FS) error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := server.Load(ctx, testBuild(), "/assets/"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load(canceled context) error = %v", err)
	}
}

func TestAliasesSurviveReload(t *testing.T) {
	server := New()
	asset, css := server.BuiltAsset, server.BuiltCSS

	for _, lookup := range []func(string) (string, error){asset, css} {
		if got, err := lookup("src/main.ts"); got != "" || !errors.Is(err, ErrNotLoaded) {
			t.Fatalf("lookup before Load() = (%q, %v), want ErrNotLoaded", got, err)
		}
	}

	for _, base := range []string{"/first/", "/second/"} {
		if err := server.Load(context.Background(), testBuild(), base); err != nil {
			t.Fatal(err)
		}

		got, err := asset("src/main.ts")
		if err != nil || got != base+"main-123.js" {
			t.Fatalf("captured alias after Load() = (%q, %v)", got, err)
		}
	}

	if err := server.Load(context.Background(), fstest.MapFS{}, "/third/"); err == nil {
		t.Fatal("Load(missing manifest) succeeded")
	}

	got, err := asset("src/main.ts")
	if err != nil || got != "/second/main-123.js" {
		t.Fatalf("failed Load changed active build: (%q, %v)", got, err)
	}
}

func TestConcurrentLookupAndReload(t *testing.T) {
	server := New()
	if err := server.Load(context.Background(), testBuild(), "/first/"); err != nil {
		t.Fatal(err)
	}

	group, ctx := errgroup.WithContext(context.Background())
	group.Go(func() error {
		var err error

		for range 20 {
			err = server.Load(ctx, testBuild(), "/second/")

			if err != nil {
				break
			}
		}

		return err
	})
	group.Go(func() error {
		var err error

		for range 100 {
			var got string
			got, err = server.BuiltAsset("src/main.ts")

			if err != nil {
				break
			}

			if !slices.Contains([]string{"/first/main-123.js", "/second/main-123.js"}, got) {
				err = fmt.Errorf("unexpected asset URL %q", got)
				break
			}
		}

		return err
	})

	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
}
