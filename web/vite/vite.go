// Package vite resolves Vite manifest entries and serves the compiled assets.
package vite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync/atomic"
)

var (
	ErrNotLoaded       = errors.New("Vite build not loaded")
	ErrAssetNotFound   = errors.New("built asset not found")
	ErrNotCSS          = errors.New("built asset is not CSS")
	ErrInvalidManifest = errors.New("invalid Vite manifest")
	ErrInvalidBaseURL  = errors.New("invalid asset base URL")
	ErrNilFilesystem   = errors.New("nil asset filesystem")
)

const manifestPath = ".vite/manifest.json"

// Vite owns an immutable build snapshot. New binds the exported function fields
// to private methods, so apps can expose them as package-level aliases.
// Load can replace the snapshot while requests and template renders are running.
// Use New to initialize the function fields.
type Vite struct {
	BuiltAsset func(source string) (string, error)
	BuiltCSS   func(source string) (string, error)
	build      atomic.Pointer[build]
}

type build struct {
	baseURL *url.URL
	files   map[string]struct{}
	assets  map[string]string
	handler http.Handler
}

type manifestChunk struct {
	File           string   `json:"file"`
	Source         string   `json:"src"`
	CSS            []string `json:"css"`
	Assets         []string `json:"assets"`
	Imports        []string `json:"imports"`
	DynamicImports []string `json:"dynamicImports"`
}

var _ http.Handler = (*Vite)(nil)

func New() *Vite {
	v := &Vite{}
	v.BuiltAsset = v.builtAsset
	v.BuiltCSS = v.builtCSS
	return v
}

// Load reads .vite/manifest.json from a filesystem rooted at the build directory.
// baseURL is a root-relative URL prefix or an absolute HTTP(S) URL.
// A failed load leaves the previous build active.
func (v *Vite) Load(ctx context.Context, built fs.FS, baseURL string) error {
	var err error

	if v == nil {
		err = ErrNotLoaded
	} else if built == nil {
		err = ErrNilFilesystem
	} else if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	} else {
		var snapshot *build
		snapshot, err = loadBuild(ctx, built, baseURL)

		if err == nil {
			v.build.Store(snapshot)
		}
	}

	if err != nil {
		err = fmt.Errorf("load Vite build: %w", err)
	}

	return err
}

func loadBuild(ctx context.Context, built fs.FS, baseURL string) (*build, error) {
	base, err := parseBaseURL(baseURL)
	var snapshot *build

	if err == nil {
		var data []byte
		data, err = fs.ReadFile(built, manifestPath)

		if err != nil {
			err = fmt.Errorf("read %s: %w", manifestPath, err)
		} else if contextErr := ctx.Err(); contextErr != nil {
			err = contextErr
		} else {
			var manifest map[string]manifestChunk
			if decodeErr := json.Unmarshal(data, &manifest); decodeErr != nil {
				err = fmt.Errorf("%w: %w", ErrInvalidManifest, decodeErr)
			} else if len(manifest) == 0 {
				err = fmt.Errorf("empty manifest: %w", ErrInvalidManifest)
			} else {
				snapshot = &build{baseURL: base, assets: make(map[string]string), files: make(map[string]struct{})}
				err = snapshot.index(ctx, built, manifest)

				if err == nil {
					snapshot.handler = http.StripPrefix(strings.TrimSuffix(base.Path, "/"), http.FileServerFS(built))
				}
			}
		}
	}

	if err != nil {
		snapshot = nil
	}

	return snapshot, err
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

func (b *build) index(ctx context.Context, built fs.FS, manifest map[string]manifestChunk) error {
	var err error

	for _, key := range slices.Sorted(maps.Keys(manifest)) {
		chunk := manifest[key]
		err = b.addSource(key, chunk.File)

		if err == nil && chunk.Source != "" {
			err = b.addSource(chunk.Source, chunk.File)
		}

		if err == nil {
			for _, dependency := range append(slices.Clone(chunk.Imports), chunk.DynamicImports...) {
				if _, exists := manifest[dependency]; !exists {
					err = fmt.Errorf("entry %q references missing chunk %q: %w", key, dependency, ErrInvalidManifest)
					break
				}
			}
		}

		if err == nil {
			files := append([]string{chunk.File}, chunk.CSS...)
			files = append(files, chunk.Assets...)

			for _, file := range files {
				err = b.addFile(ctx, built, file)

				if err != nil {
					break
				}
			}
		}

		if err != nil {
			err = fmt.Errorf("manifest entry %q: %w", key, err)
			break
		}
	}

	if err == nil {
		err = ctx.Err()
	}

	return err
}

func (b *build) addSource(source, file string) error {
	var err error

	if existing, exists := b.assets[source]; exists && existing != file {
		err = fmt.Errorf("source %q maps to multiple files: %w", source, ErrInvalidManifest)
	} else {
		b.assets[source] = file
	}

	return err
}

func (b *build) addFile(ctx context.Context, built fs.FS, file string) error {
	var err error

	if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	} else if !validFile(file) {
		err = fmt.Errorf("invalid output path %q: %w", file, ErrInvalidManifest)
	} else if _, exists := b.files[file]; !exists {
		info, statErr := fs.Stat(built, file)

		if statErr != nil {
			err = fmt.Errorf("stat built asset %q: %w", file, statErr)
		} else if !info.Mode().IsRegular() {
			err = fmt.Errorf("built asset %q is not a regular file: %w", file, ErrInvalidManifest)
		} else {
			b.files[file] = struct{}{}
		}
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

func (v *Vite) builtAsset(source string) (string, error) {
	return v.assetURL(source, false)
}

func (v *Vite) builtCSS(source string) (string, error) {
	return v.assetURL(source, true)
}

func (v *Vite) assetURL(source string, cssOnly bool) (string, error) {
	var result string
	var err error
	var snapshot *build

	if v != nil {
		snapshot = v.build.Load()
	}

	if snapshot == nil {
		err = ErrNotLoaded
	} else if file, exists := snapshot.assets[source]; !exists {
		err = ErrAssetNotFound
	} else if cssOnly && !strings.EqualFold(path.Ext(file), ".css") {
		err = ErrNotCSS
	} else {
		assetURL := *snapshot.baseURL
		assetURL.Path += file
		result = assetURL.String()
	}

	if err != nil {
		err = fmt.Errorf("resolve built asset %q: %w", source, err)
	}

	return result, err
}

// ServeHTTP serves only manifest-referenced files under the configured URL prefix.
func (v *Vite) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	var snapshot *build

	if v != nil {
		snapshot = v.build.Load()
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
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		snapshot.handler.ServeHTTP(writer, request)
	}
}
