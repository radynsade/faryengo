package admin

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/web/admin/assets"
	"github.com/radynsade/faryengo/web/admin/templates"
)

func RegisterHandlers(ctx context.Context, mux *http.ServeMux, built fs.FS) error {
	var err error

	if registerErr := assets.RegisterHandlers(ctx, mux, built); registerErr != nil {
		err = fmt.Errorf("register admin assets: %w", registerErr)
	} else {
		adminRoot := "/admin"
		mux.HandleFunc(fmt.Sprintf("GET %s/{language}/sign-in", adminRoot), handleSignIn)
	}

	return err
}

func handleSignIn(writer http.ResponseWriter, request *http.Request) {
	templ.Handler(templates.Root()).ServeHTTP(writer, request)
}
