// Package assets serves the admin build and exposes its template URL helpers.
package assets

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/radynsade/faryengo/web/vite"
)

const URLPrefix = "/assets/admin/"

var ErrNilServeMux = errors.New("nil admin asset ServeMux")

//go:embed all:dist
var embeddedFiles embed.FS

var server = vite.New()

// BuiltAsset resolves a source path to its compiled asset URL.
var BuiltAsset = server.BuiltAsset

// BuiltCSS resolves a stylesheet entry path to its compiled CSS URL.
var BuiltCSS = server.BuiltCSS

func init() {
	built, err := fs.Sub(embeddedFiles, "dist")

	if err != nil {
		panic(fmt.Errorf("open embedded admin assets: %w", err))
	}

	if err := server.Load(context.Background(), built, URLPrefix); err != nil {
		panic(fmt.Errorf("load embedded admin assets: %w", err))
	}
}

// RegisterHandlers mounts the embedded admin build.
func RegisterHandlers(mux *http.ServeMux) error {
	var err error

	if mux == nil {
		err = ErrNilServeMux
	} else {
		mux.Handle(URLPrefix, server)
	}

	return err
}
