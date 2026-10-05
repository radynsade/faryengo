// Package assets serves the admin build and exposes its template URL helpers.
package assets

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/radynsade/faryengo/pkg/viteast"
)

const URLPrefix = "/assets/admin/"

var ErrNilServeMux = errors.New("nil admin asset ServeMux")

//go:embed all:dist static
var embeddedFiles embed.FS

var server = viteast.New()

// BuiltAsset resolves a path relative to src/ to its compiled asset URL.
var BuiltAsset = server.BuiltAsset

// BuiltCSS resolves a stylesheet entry relative to src/ to its compiled CSS URL.
var BuiltCSS = server.BuiltCSS

func init() {
	built, err := fs.Sub(embeddedFiles, "dist")

	if err != nil {
		panic(fmt.Errorf("open embedded admin assets: %w", err))
	}

	if err := server.Load(context.Background(), built, URLPrefix); err != nil {
		panic(fmt.Errorf("load embedded admin assets: %w", err))
	}

	static, err := fs.Sub(embeddedFiles, "static")

	if err != nil {
		panic(fmt.Errorf("open embedded admin static assets: %w", err))
	}

	if err := staticServer.Load(context.Background(), static, staticURLPrefix); err != nil {
		panic(fmt.Errorf("load embedded admin static assets: %w", err))
	}

	preloadAssets, err = loadPreloads(context.Background(), built)

	if err != nil {
		panic(err)
	}
}

// RegisterHandlers mounts the embedded admin build and static assets.
func RegisterHandlers(mux *http.ServeMux) error {
	var err error

	if mux == nil {
		err = ErrNilServeMux
	} else {
		mux.Handle(URLPrefix, server)
		mux.Handle(staticURLPrefix, staticServer)
	}

	return err
}
