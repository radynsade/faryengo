// Package staticast resolves URLs and serves regular files from a static asset filesystem.
package staticast

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
)

var (
	ErrNotLoaded      = errors.New("static assets not loaded")
	ErrAssetNotFound  = errors.New("static asset not found")
	ErrInvalidBaseURL = errors.New("invalid asset base URL")
	ErrNilFilesystem  = errors.New("nil asset filesystem")
)

// Static owns an atomically replaceable index of public static assets.
// Use New to initialize its function field. The filesystem must remain unchanged
// while loaded, and its files must implement io.Seeker, as with http.FileServerFS.
type Static struct {
	// StaticAsset resolves a path relative to the filesystem root to its asset URL.
	// Applications can expose this function as a package-level alias.
	StaticAsset func(source string) (string, error)
	assets      atomic.Pointer[assets]
}

type assets struct {
	baseURL    *url.URL
	files      map[string]struct{}
	filesystem fs.FS
}

var _ http.Handler = (*Static)(nil)

// New creates a static asset server with a bound StaticAsset URL helper.
func New() *Static {
	s := &Static{}
	s.StaticAsset = s.staticAsset
	return s
}

// Load indexes regular files in a filesystem rooted at the static directory.
// Hidden paths and symlinks are excluded. baseURL is a root-relative URL prefix
// or an absolute HTTP(S) URL. A failed load leaves the previous index active.
func (s *Static) Load(ctx context.Context, files fs.FS, baseURL string) error {
	var err error

	if s == nil {
		err = ErrNotLoaded
	} else if files == nil {
		err = ErrNilFilesystem
	} else if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	} else {
		var base *url.URL
		base, err = parseBaseURL(baseURL)

		if err == nil {
			snapshot := &assets{baseURL: base, files: make(map[string]struct{}), filesystem: files}
			err = snapshot.index(ctx)

			if err == nil {
				s.assets.Store(snapshot)
			}
		}
	}

	if err != nil {
		err = fmt.Errorf("load static assets: %w", err)
	}

	return err
}

func parseBaseURL(value string) (*url.URL, error) {
	base, err := url.Parse(value)

	if err != nil {
		err = fmt.Errorf("%w: %w", ErrInvalidBaseURL, err)
	} else if value == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || base.Opaque != "" ||
		(base.IsAbs() && ((base.Scheme != "http" && base.Scheme != "https") || base.Host == "")) ||
		(!base.IsAbs() && (base.Host != "" || !strings.HasPrefix(base.Path, "/"))) {
		err = ErrInvalidBaseURL
	} else {
		if base.Path == "" {
			base.Path = "/"
		}

		cleaned := path.Clean(base.Path)

		if cleaned != strings.TrimSuffix(base.Path, "/") && base.Path != "/" || strings.Contains(base.Path, "\\") || strings.HasPrefix(base.Path, "//") {
			err = ErrInvalidBaseURL
		} else {
			base.Path = strings.TrimSuffix(cleaned, "/") + "/"
			base.RawPath = ""
		}
	}

	if err != nil {
		base = nil
	}

	return base, err
}

func (a *assets) index(ctx context.Context) error {
	err := fs.WalkDir(a.filesystem, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		var err error

		if contextErr := ctx.Err(); contextErr != nil {
			err = contextErr
		} else if walkErr != nil {
			err = walkErr
		} else if name == "." {
			if !entry.IsDir() {
				err = fmt.Errorf("filesystem root is not a directory: %w", fs.ErrInvalid)
			}
		} else if !validFile(name) {
			if entry.IsDir() {
				err = fs.SkipDir
			}
		} else if !entry.IsDir() {
			var info fs.FileInfo
			info, err = entry.Info()

			if err == nil && info.Mode().IsRegular() {
				a.files[name] = struct{}{}
			}
		}

		return err
	})

	if err == nil {
		err = ctx.Err()
	}

	return err
}

func validFile(file string) bool {
	valid := file != "." && fs.ValidPath(file) && !strings.Contains(file, "\\")

	if valid {
		for segment := range strings.SplitSeq(file, "/") {
			if strings.HasPrefix(segment, ".") {
				valid = false
				break
			}
		}
	}

	return valid
}

func (s *Static) staticAsset(source string) (string, error) {
	var result string
	var err error
	var snapshot *assets

	if s != nil {
		snapshot = s.assets.Load()
	}

	if snapshot == nil {
		err = ErrNotLoaded
	} else if _, exists := snapshot.files[source]; !exists {
		err = ErrAssetNotFound
	} else {
		assetURL := *snapshot.baseURL
		assetURL.Path += source
		result = assetURL.String()
	}

	if err != nil {
		err = fmt.Errorf("resolve static asset %q: %w", source, err)
	}

	return result, err
}

// ServeHTTP serves indexed files under the configured URL prefix.
// It supports GET, HEAD, conditional and range requests without directory listings.
func (s *Static) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	var snapshot *assets

	if s != nil {
		snapshot = s.assets.Load()
	}

	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
	} else if snapshot == nil {
		http.Error(writer, "assets not loaded", http.StatusServiceUnavailable)
	} else if file, matches := strings.CutPrefix(request.URL.Path, snapshot.baseURL.Path); !matches {
		http.NotFound(writer, request)
	} else if _, exists := snapshot.files[file]; !exists {
		http.NotFound(writer, request)
	} else {
		content, openErr := snapshot.filesystem.Open(file)

		if openErr != nil {
			http.NotFound(writer, request)
		} else {
			defer func() {
				// Closing a read-only asset cannot change the completed response.
				_ = content.Close()
			}()

			info, statErr := content.Stat()

			if statErr != nil || !info.Mode().IsRegular() {
				http.NotFound(writer, request)
			} else if seeker, ok := content.(io.ReadSeeker); !ok {
				http.Error(writer, "asset does not support seeking", http.StatusInternalServerError)
			} else {
				writer.Header().Set("X-Content-Type-Options", "nosniff")
				http.ServeContent(writer, request, info.Name(), info.ModTime(), seeker)
			}
		}
	}
}
