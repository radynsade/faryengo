package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/web/admin/assets"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

var ErrInvalidHandlerConfig = errors.New("invalid admin handler configuration")

type RateLimiter interface {
	Allow(context.Context, string, int, time.Duration) (bool, error)
}

type Handler struct {
	service       *app.AuthenticationService
	limiter       RateLimiter
	secureCookies bool
}

func NewHandler(service *app.AuthenticationService, limiter RateLimiter, secureCookies bool) (*Handler, error) {
	var handler *Handler
	var err error

	if service == nil || limiter == nil {
		err = ErrInvalidHandlerConfig
	} else {
		handler = &Handler{service: service, limiter: limiter, secureCookies: secureCookies}
	}

	return handler, err
}

func (h *Handler) RegisterHandlers(mux *http.ServeMux) error {
	var err error

	if registerErr := assets.RegisterHandlers(mux); registerErr != nil {
		err = fmt.Errorf("register admin assets: %w", registerErr)
	} else {
		protection := http.NewCrossOriginProtection()

		for _, route := range []struct {
			pattern string
			handler http.HandlerFunc
		}{
			{"GET /admin/{language}/sign-in", h.handleSignIn},
			{"POST /admin/{language}/sign-in", h.signIn},
			{"POST /admin/{language}/refresh", h.refresh},
			{"POST /admin/{language}/sign-out", h.signOut},
			{"GET /admin/{language}/restore-password", handleRestorePassword},
			{"GET /admin/{language}", h.home},
		} {
			mux.Handle(route.pattern, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Cache-Control", "no-store")
				writer.Header().Set("X-Content-Type-Options", "nosniff")
				protection.Handler(route.handler).ServeHTTP(writer, request)
			}))
		}
	}

	return err
}

func adminPath(request *http.Request) string {
	return "/admin/" + url.PathEscape(request.PathValue("language"))
}

func (h *Handler) handleSignIn(writer http.ResponseWriter, request *http.Request) {
	h.renderSignIn(writer, request, http.StatusOK, "", "")
}

func (h *Handler) renderSignIn(writer http.ResponseWriter, request *http.Request, status int, email, message string) {
	renderPage(writer, request, "Sign in · Faryen Admin", pages.SignIn(pages.SignInProps{
		Action:             adminPath(request) + "/sign-in",
		RefreshAction:      adminPath(request) + "/refresh",
		RestorePasswordURL: adminPath(request) + "/restore-password",
		Email:              email,
		Error:              message,
		CanContinue:        status == http.StatusOK && h.cookie(request, "refresh") != "",
	}), templ.WithStatus(status))
}

func handleRestorePassword(writer http.ResponseWriter, request *http.Request) {
	renderPage(writer, request, "Restore password · Faryen Admin", pages.RestorePassword())
}

func renderPage(writer http.ResponseWriter, request *http.Request, title string, content templ.Component, options ...func(*templ.ComponentHandler)) {
	writer.Header().Add("Vary", "HX-Request")
	writer.Header().Add("Vary", "HX-History-Restore-Request")
	page := layouts.Page(title, content)

	if request.Header.Get("HX-Request") == "true" && request.Header.Get("HX-History-Restore-Request") != "true" {
		page = layouts.PageFragment(title, content)
	}

	templ.Handler(page, options...).ServeHTTP(writer, request)
}
