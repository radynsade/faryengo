package staticast

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"go.uber.org/goleak"
	"golang.org/x/sync/errgroup"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func testFiles() fstest.MapFS {
	return fstest.MapFS{
		"fonts/font.woff2":      {Data: []byte("font data")},
		"images/logo image.svg": {Data: []byte("<svg></svg>")},
		"style.css":             {Data: []byte("body{color:red}"), ModTime: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)},
		"index.html":            {Data: []byte("<h1>Static</h1>")},
		".secret":               {Data: []byte("secret")},
		".hidden/file.txt":      {Data: []byte("secret")},
		"images/.secret":        {Data: []byte("secret")},
		`images\private.txt`:    {Data: []byte("private")},
		"link":                  {Mode: fs.ModeSymlink, Data: []byte("style.css")},
		"pipe":                  {Mode: fs.ModeNamedPipe},
	}
}

func TestStaticAsset(t *testing.T) {
	for _, tt := range []struct {
		name, baseURL, source, want string
		wantErr                     error
	}{
		{name: "nested file", baseURL: "/assets/static/", source: "fonts/font.woff2", want: "/assets/static/fonts/font.woff2"},
		{name: "escaped filename", baseURL: "/assets/static/", source: "images/logo image.svg", want: "/assets/static/images/logo%20image.svg"},
		{name: "escaped prefix", baseURL: "/my%20assets/", source: "style.css", want: "/my%20assets/style.css"},
		{name: "no trailing slash", baseURL: "/assets/static", source: "style.css", want: "/assets/static/style.css"},
		{name: "absolute URL", baseURL: "https://cdn.example.com/static/", source: "style.css", want: "https://cdn.example.com/static/style.css"},
		{name: "absolute root", baseURL: "http://cdn.example.com", source: "style.css", want: "http://cdn.example.com/style.css"},
		{name: "root prefix", baseURL: "/", source: "style.css", want: "/style.css"},
		{name: "index file", baseURL: "/static/", source: "index.html", want: "/static/index.html"},
		{name: "missing", baseURL: "/static/", source: "missing", wantErr: ErrAssetNotFound},
		{name: "empty", baseURL: "/static/", wantErr: ErrAssetNotFound},
		{name: "root directory", baseURL: "/static/", source: ".", wantErr: ErrAssetNotFound},
		{name: "directory", baseURL: "/static/", source: "fonts", wantErr: ErrAssetNotFound},
		{name: "absolute path", baseURL: "/static/", source: "/style.css", wantErr: ErrAssetNotFound},
		{name: "parent traversal", baseURL: "/static/", source: "../style.css", wantErr: ErrAssetNotFound},
		{name: "nested traversal", baseURL: "/static/", source: "fonts/../style.css", wantErr: ErrAssetNotFound},
		{name: "backslash", baseURL: "/static/", source: `images\private.txt`, wantErr: ErrAssetNotFound},
		{name: "hidden file", baseURL: "/static/", source: ".secret", wantErr: ErrAssetNotFound},
		{name: "hidden directory", baseURL: "/static/", source: ".hidden/file.txt", wantErr: ErrAssetNotFound},
		{name: "nested hidden file", baseURL: "/static/", source: "images/.secret", wantErr: ErrAssetNotFound},
		{name: "symlink", baseURL: "/static/", source: "link", wantErr: ErrAssetNotFound},
		{name: "special file", baseURL: "/static/", source: "pipe", wantErr: ErrAssetNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := New()

			if err := server.Load(t.Context(), testFiles(), tt.baseURL); err != nil {
				t.Fatal(err)
			}

			got, err := server.StaticAsset(tt.source)

			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Fatalf("StaticAsset(%q) = (%q, %v), want (%q, %v)", tt.source, got, err, tt.want, tt.wantErr)
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

	if err := server.Load(t.Context(), testFiles(), "/static/"); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, method, url, byteRange, modifiedSince, body, contentType string
		status                                                         int
	}{
		{name: "font", method: http.MethodGet, url: "/static/fonts/font.woff2", status: http.StatusOK, body: "font data", contentType: "font/woff2"},
		{name: "escaped filename", method: http.MethodGet, url: "/static/images/logo%20image.svg", status: http.StatusOK, body: "<svg></svg>", contentType: "image/svg+xml"},
		{name: "CSS", method: http.MethodGet, url: "/static/style.css", status: http.StatusOK, body: "body{color:red}", contentType: "text/css"},
		{name: "index file", method: http.MethodGet, url: "/static/index.html", status: http.StatusOK, body: "<h1>Static</h1>", contentType: "text/html"},
		{name: "HEAD", method: http.MethodHead, url: "/static/style.css", status: http.StatusOK, contentType: "text/css"},
		{name: "range", method: http.MethodGet, url: "/static/style.css", byteRange: "bytes=0-3", status: http.StatusPartialContent, body: "body", contentType: "text/css"},
		{name: "conditional", method: http.MethodGet, url: "/static/style.css", modifiedSince: "Thu, 01 Jan 2026 00:00:00 GMT", status: http.StatusNotModified},
		{name: "missing", method: http.MethodGet, url: "/static/missing", status: http.StatusNotFound},
		{name: "root directory", method: http.MethodGet, url: "/static/", status: http.StatusNotFound},
		{name: "directory", method: http.MethodGet, url: "/static/fonts/", status: http.StatusNotFound},
		{name: "hidden file", method: http.MethodGet, url: "/static/.secret", status: http.StatusNotFound},
		{name: "hidden directory", method: http.MethodGet, url: "/static/.hidden/file.txt", status: http.StatusNotFound},
		{name: "traversal", method: http.MethodGet, url: "/static/%2e%2e/style.css", status: http.StatusNotFound},
		{name: "symlink", method: http.MethodGet, url: "/static/link", status: http.StatusNotFound},
		{name: "wrong prefix", method: http.MethodGet, url: "/other/style.css", status: http.StatusNotFound},
		{name: "prefix boundary", method: http.MethodGet, url: "/static-extra/style.css", status: http.StatusNotFound},
		{name: "unsupported method", method: http.MethodPost, url: "/static/style.css", status: http.StatusMethodNotAllowed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.url, nil)
			request.Header.Set("Range", tt.byteRange)
			request.Header.Set("If-Modified-Since", tt.modifiedSince)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			if response.Code != tt.status {
				t.Fatalf("ServeHTTP() status = %d, want %d", response.Code, tt.status)
			}

			if tt.status < http.StatusBadRequest {
				if response.Body.String() != tt.body {
					t.Fatalf("ServeHTTP() body = %q, want %q", response.Body.String(), tt.body)
				}

				if response.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal("response is missing nosniff")
				}
			}

			if tt.contentType != "" && !strings.HasPrefix(response.Header().Get("Content-Type"), tt.contentType) {
				t.Fatalf("Content-Type = %q, want %q", response.Header().Get("Content-Type"), tt.contentType)
			}

			if tt.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("Allow = %q, want GET, HEAD", response.Header().Get("Allow"))
			}
		})
	}
}

func TestLoadFailures(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	indexContext, cancelIndex := context.WithCancel(t.Context())
	defer cancelIndex()

	for _, tt := range []struct {
		name    string
		ctx     context.Context
		files   fs.FS
		baseURL string
		wantErr error
	}{
		{name: "nil filesystem", files: nil, baseURL: "/static/", wantErr: ErrNilFilesystem},
		{name: "cancelled", ctx: cancelled, files: testFiles(), baseURL: "/static/", wantErr: context.Canceled},
		{name: "cancelled during indexing", ctx: indexContext, files: cancellingFS{FS: testFiles(), cancel: cancelIndex}, baseURL: "/static/", wantErr: context.Canceled},
		{name: "filesystem error", files: failingFS{}, baseURL: "/static/", wantErr: fs.ErrPermission},
		{name: "empty base", files: testFiles(), wantErr: ErrInvalidBaseURL},
		{name: "malformed URL", files: testFiles(), baseURL: "%", wantErr: ErrInvalidBaseURL},
		{name: "relative base", files: testFiles(), baseURL: "static/", wantErr: ErrInvalidBaseURL},
		{name: "query", files: testFiles(), baseURL: "/static/?x=1", wantErr: ErrInvalidBaseURL},
		{name: "empty query", files: testFiles(), baseURL: "/static/?", wantErr: ErrInvalidBaseURL},
		{name: "fragment", files: testFiles(), baseURL: "/static/#x", wantErr: ErrInvalidBaseURL},
		{name: "credentials", files: testFiles(), baseURL: "https://user:pass@example.com/static/", wantErr: ErrInvalidBaseURL},
		{name: "protocol relative", files: testFiles(), baseURL: "//example.com/static/", wantErr: ErrInvalidBaseURL},
		{name: "unsafe scheme", files: testFiles(), baseURL: "javascript:alert(1)", wantErr: ErrInvalidBaseURL},
		{name: "missing host", files: testFiles(), baseURL: "https:///static/", wantErr: ErrInvalidBaseURL},
		{name: "parent traversal", files: testFiles(), baseURL: "/static/../files/", wantErr: ErrInvalidBaseURL},
		{name: "duplicate slash", files: testFiles(), baseURL: "/static//", wantErr: ErrInvalidBaseURL},
		{name: "backslash", files: testFiles(), baseURL: `/static\files/`, wantErr: ErrInvalidBaseURL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := New()
			lookup := server.StaticAsset

			if err := server.Load(t.Context(), testFiles(), "/original/"); err != nil {
				t.Fatal(err)
			}

			ctx := tt.ctx

			if ctx == nil {
				ctx = t.Context()
			}

			if err := server.Load(ctx, tt.files, tt.baseURL); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Load() error = %v, want %v", err, tt.wantErr)
			}

			if got, err := lookup("style.css"); err != nil || got != "/original/style.css" {
				t.Fatalf("failed reload changed lookup: (%q, %v)", got, err)
			}

			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/original/style.css", nil))

			if response.Code != http.StatusOK {
				t.Fatalf("failed reload changed handler: status %d", response.Code)
			}
		})
	}
}

func TestUnloaded(t *testing.T) {
	for _, tt := range []struct {
		name   string
		server *Static
	}{
		{name: "new", server: New()},
		{name: "zero value", server: &Static{}},
		{name: "nil"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.server.staticAsset("style.css"); !errors.Is(err, ErrNotLoaded) {
				t.Fatalf("lookup error = %v, want ErrNotLoaded", err)
			}

			response := httptest.NewRecorder()
			tt.server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/static/style.css", nil))

			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("unloaded status = %d, want 503", response.Code)
			}
		})
	}

	var server *Static

	if err := server.Load(t.Context(), testFiles(), "/static/"); !errors.Is(err, ErrNotLoaded) {
		t.Fatalf("nil Load error = %v, want ErrNotLoaded", err)
	}
}

func TestReload(t *testing.T) {
	server := New()
	lookup := server.StaticAsset

	if err := server.Load(t.Context(), fstest.MapFS{"old.txt": {Data: []byte("old")}}, "/old/"); err != nil {
		t.Fatal(err)
	}

	if err := server.Load(t.Context(), testFiles(), "/new/"); err != nil {
		t.Fatal(err)
	}

	if got, err := lookup("style.css"); err != nil || got != "/new/style.css" {
		t.Fatalf("alias after reload = (%q, %v)", got, err)
	}

	if _, err := lookup("old.txt"); !errors.Is(err, ErrAssetNotFound) {
		t.Fatalf("old file error = %v, want ErrAssetNotFound", err)
	}

	if err := server.Load(t.Context(), fstest.MapFS{}, "/empty/"); err != nil {
		t.Fatalf("empty static directory: %v", err)
	}
}

func TestConcurrentReload(t *testing.T) {
	server := New()
	files := testFiles()

	if err := server.Load(t.Context(), files, "/static/"); err != nil {
		t.Fatal(err)
	}

	group, ctx := errgroup.WithContext(t.Context())
	group.Go(func() error {
		var err error

		for range 100 {
			err = server.Load(ctx, files, "/static/")

			if err != nil {
				break
			}
		}

		return err
	})

	for range 3 {
		group.Go(func() error {
			var err error

			for range 100 {
				var got string
				got, err = server.StaticAsset("style.css")

				if err == nil && got != "/static/style.css" {
					err = fmt.Errorf("unexpected asset URL %q", got)
				}

				if err == nil {
					response := httptest.NewRecorder()
					server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, got, nil))

					if response.Code != http.StatusOK || response.Body.String() != "body{color:red}" {
						err = fmt.Errorf("unexpected asset response %d: %q", response.Code, response.Body.String())
					}
				}

				if err != nil {
					break
				}
			}

			return err
		})
	}

	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
}

type failingFS struct{}

func (failingFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}

type cancellingFS struct {
	fs.FS
	cancel context.CancelFunc
}

func (f cancellingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(f.FS, name)

	if name == "fonts" {
		f.cancel()
	}

	return entries, err
}
