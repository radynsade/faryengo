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
	"github.com/radynsade/faryengo/pkg/flashmsg"
	"github.com/radynsade/faryengo/web/admin/assets"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

var ErrInvalidHandlerConfig = errors.New("invalid admin handler configuration")

type RateLimiter interface {
	Allow(context.Context, string, int, time.Duration) (bool, error)
}

type Handler struct {
	service       *app.SessionAuthenticationService
	roleService   *app.RoleService
	languages     *app.LanguageService
	limiter       RateLimiter
	flashStorage  flashmsg.FlashSessionStorage
	secureCookies bool
}

func NewHandler(service *app.SessionAuthenticationService, roles *app.RoleService, languages *app.LanguageService, limiter RateLimiter, flashStorage flashmsg.FlashSessionStorage, secureCookies bool) (*Handler, error) {
	var handler *Handler
	var err error

	if service == nil || roles == nil || languages == nil || limiter == nil || flashStorage == nil {
		err = ErrInvalidHandlerConfig
	} else {
		handler = &Handler{service: service, roleService: roles, languages: languages, limiter: limiter, flashStorage: flashStorage, secureCookies: secureCookies}
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
			{"POST /admin/{language}/sign-out", h.signOut},
			{"GET /admin/{language}/restore-password", h.handleRestorePassword},
			{"GET /admin/{language}", h.home},
			{"GET /admin/{language}/users", h.users},
			{"GET /admin/{language}/roles", h.roles},
			{"GET /admin/{language}/roles/table", h.rolesTable},
			{"GET /admin/{language}/roles/create", h.roleCreate},
			{"POST /admin/{language}/roles/create", h.roleCreate},
			{"GET /admin/{language}/roles/{role}/view", h.roleView},
			{"GET /admin/{language}/roles/{role}/edit", h.roleEdit},
			{"POST /admin/{language}/roles/{role}/edit", h.roleEdit},
			{"POST /admin/{language}/roles/{role}/delete", h.roleDelete},
		} {
			mux.Handle(route.pattern, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Cache-Control", "no-store")
				writer.Header().Set("X-Content-Type-Options", "nosniff")
				request = request.WithContext(admini18n.WithRequest(request))
				writer.Header().Set("Content-Language", admini18n.Language(request.Context()))
				request = request.WithContext(context.WithValue(request.Context(), flashRequestKey{}, &flashRequest{}))

				if checkErr := protection.Check(request); checkErr != nil {
					http.Error(writer, admini18n.T(request.Context(), "errors.cross_origin"), http.StatusForbidden)
				} else {
					route.handler(writer, request)
				}
			}))
		}
	}

	return err
}

func adminPath(request *http.Request) string {
	return "/admin/" + url.PathEscape(request.PathValue("language"))
}

func (h *Handler) handleSignIn(writer http.ResponseWriter, request *http.Request) {
	_, err := h.authenticate(writer, request)

	if err == nil {
		http.Redirect(writer, request, adminPath(request), http.StatusSeeOther)
	} else if invalidSession(err) {
		h.renderSignIn(writer, request, http.StatusOK, "", "")
	} else {
		h.signInError(writer, request, "", err)
	}
}

func (h *Handler) renderSignIn(writer http.ResponseWriter, request *http.Request, status int, email, message string) {
	var err error

	if message != "" {
		err = h.addFlash(request.Context(), writer, request, flashmsg.Error, message)
	}

	if err != nil {
		h.flashUnavailable(writer, request, err)
	} else {
		h.renderPage(writer, request, admini18n.T(request.Context(), "actions.sign_in")+" · Faryen "+admini18n.T(request.Context(), "common.admin"), pages.SignIn(pages.SignInProps{
			Action:             adminPath(request) + "/sign-in",
			RestorePasswordURL: adminPath(request) + "/restore-password",
			Email:              email,
		}), templ.WithStatus(status))
	}
}

func (h *Handler) handleRestorePassword(writer http.ResponseWriter, request *http.Request) {
	h.renderPage(writer, request, admini18n.T(request.Context(), "auth.restore")+" · Faryen "+admini18n.T(request.Context(), "common.admin"), pages.RestorePassword())
}

func (h *Handler) renderPage(writer http.ResponseWriter, request *http.Request, title string, content templ.Component, options ...func(*templ.ComponentHandler)) {
	bag, err := h.readFlashes(request.Context(), request, "")

	if err != nil {
		h.flashUnavailable(writer, request, err)
	} else {
		writer.Header().Add("Vary", "HX-Request")
		writer.Header().Add("Vary", "HX-History-Restore-Request")
		page := layouts.Page(title, content)

		if request.Header.Get("HX-Request") == "true" && request.Header.Get("HX-History-Restore-Request") != "true" {
			page = layouts.PageFragment(title, content)
		}

		request = request.WithContext(flashmsg.WithBag(request.Context(), bag))
		templ.Handler(page, options...).ServeHTTP(writer, request)
	}
}
