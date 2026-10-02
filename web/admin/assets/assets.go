// Package assets serves the admin build and exposes its template URL helpers.
package assets

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/radynsade/faryengo/web/vite"
)

const URLPrefix = "/assets/admin/"

var ErrNilServeMux = errors.New("nil admin asset ServeMux")

var server = vite.New()

// BuiltAsset resolves a source path to its compiled asset URL.
var BuiltAsset = server.BuiltAsset

// BuiltCSS resolves a stylesheet entry path to its compiled CSS URL.
var BuiltCSS = server.BuiltCSS

// RegisterHandlers loads a filesystem rooted at dist and mounts its assets.
func RegisterHandlers(ctx context.Context, mux *http.ServeMux, built fs.FS) error {
	var err error

	if mux == nil {
		err = ErrNilServeMux
	} else if loadErr := server.Load(ctx, built, URLPrefix); loadErr != nil {
		err = fmt.Errorf("load admin assets: %w", loadErr)
	} else {
		mux.Handle(URLPrefix, server)
	}

	return err
}
