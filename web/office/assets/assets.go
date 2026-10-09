// Package assets serves the office build and exposes its template URL helpers.
package assets

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/radynsade/faryengo/pkg/staticast"
	"github.com/radynsade/faryengo/pkg/viteast"
)

const URLPrefix = "/assets/office/"

var ErrNilServeMux = errors.New("nil office asset ServeMux")

//go:embed all:dist static
var embeddedFiles embed.FS

var server = viteast.New()

// BuiltAsset resolves a path relative to src/ to its compiled asset URL.
var BuiltAsset = server.BuiltAsset

// BuiltCSS resolves a stylesheet entry relative to src/ to its compiled CSS URL.
var BuiltCSS = server.BuiltCSS

const staticURLPrefix = URLPrefix + "static/"

var staticServer = staticast.New()

// StaticAsset resolves a path relative to static/ to its embedded asset URL.
var StaticAsset = staticServer.StaticAsset

// Icons is the URL of the icon sprite. Every icon on a page references it, so
// no icon's markup is repeated in a response. The URL carries a version of
// the sprite's content, so browsers can keep it until the sprite changes.
var Icons string

// Long-lived caching lets the browser reuse assets across in-page
// navigations: built files have content-hashed names, and static files are
// cached only when their URL carries a content version.
const immutable = "public, max-age=31536000, immutable"

func init() {
	built, err := fs.Sub(embeddedFiles, "dist")

	if err != nil {
		panic(fmt.Errorf("open embedded office assets: %w", err))
	}

	if err := server.Load(context.Background(), built, URLPrefix); err != nil {
		panic(fmt.Errorf("load embedded office assets: %w", err))
	}

	static, err := fs.Sub(embeddedFiles, "static")

	if err != nil {
		panic(fmt.Errorf("open embedded office static assets: %w", err))
	}

	if err := staticServer.Load(context.Background(), static, staticURLPrefix); err != nil {
		panic(fmt.Errorf("load embedded office static assets: %w", err))
	}

	if Icons, err = versioned(static, "icons.svg"); err != nil {
		panic(fmt.Errorf("resolve the office icon sprite: %w", err))
	}
}

// RegisterHandlers mounts the embedded office build and static assets.
func RegisterHandlers(mux *http.ServeMux) error {
	var err error

	if mux == nil {
		err = ErrNilServeMux
	} else {
		mux.Handle(URLPrefix, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Cache-Control", immutable)
			server.ServeHTTP(writer, request)
		}))
		mux.Handle(staticURLPrefix, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Query().Has("v") {
				writer.Header().Set("Cache-Control", immutable)
			}

			staticServer.ServeHTTP(writer, request)
		}))
	}

	return err
}

//
// Helpers
//

func versioned(static fs.FS, source string) (string, error) {
	data, err := fs.ReadFile(static, source)
	address := ""

	if err == nil {
		address, err = StaticAsset(source)
	}

	if err == nil {
		sum := sha256.Sum256(data)
		address += "?v=" + hex.EncodeToString(sum[:6])
	}

	return address, err
}
