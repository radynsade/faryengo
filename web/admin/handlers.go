package admin

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/web/admin/assets"
	"github.com/radynsade/faryengo/web/admin/templates"
)

func RegisterHandlers(mux *http.ServeMux) error {
	var err error

	if registerErr := assets.RegisterHandlers(mux); registerErr != nil {
		err = fmt.Errorf("register admin assets: %w", registerErr)
	} else {
		adminRoot := "/admin"
		mux.HandleFunc(fmt.Sprintf("GET %s/{language}/sign-in", adminRoot), handleSignIn)
		mux.HandleFunc(fmt.Sprintf("GET %s/{language}/restore-password", adminRoot), handleRestorePassword)
	}

	return err
}

func handleSignIn(writer http.ResponseWriter, request *http.Request) {
	renderPage(writer, request, "Sign in · Faryen Admin", templates.SignIn())
}

func handleRestorePassword(writer http.ResponseWriter, request *http.Request) {
	renderPage(writer, request, "Restore password · Faryen Admin", templates.RestorePassword())
}

func renderPage(writer http.ResponseWriter, request *http.Request, title string, content templ.Component) {
	writer.Header().Add("Vary", "Datastar-Request")
	page := templates.Page(title, content)

	if request.Header.Get("Datastar-Request") == "true" {
		page = templates.PageFragment(title, content)
	}

	templ.Handler(page).ServeHTTP(writer, request)
}
