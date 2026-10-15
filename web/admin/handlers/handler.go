package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	"github.com/radynsade/faryengo/middleware"
	"github.com/radynsade/faryengo/pkg/cookie"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	"github.com/radynsade/faryengo/web/admin/assets"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/utils"
)

//
// Handler
//

var (
	ErrHandlerConfigInvalid = errors.New("invalid admin handler configuration")
	ErrServeMuxNil          = errors.New("admin ServeMux is nil")
)

type Handler struct {
	passwords  *emailpass.Authenticator
	sessions   *sessionid.Authenticator
	identities *security.IdentityResolver
	users      *app.UserService
	roles      *app.RoleService
	languages  *app.LanguageService
	currencies *app.CurrencyService
	flashes    *utils.Flashes
	cookies    cookie.Jar
}

func NewHandler(
	passwords *emailpass.Authenticator,
	sessions *sessionid.Authenticator,
	identities *security.IdentityResolver,
	userService *app.UserService,
	roles *app.RoleService,
	languages *app.LanguageService,
	currencies *app.CurrencyService,
	flashStorage flashmsg.FlashSessionStorage,
	secureCookies bool,
) (*Handler, error) {
	var (
		handler *Handler
		flashes *utils.Flashes
		err     error
	)

	cookies := cookie.Jar{Secure: secureCookies}

	if passwords == nil || sessions == nil || identities == nil || userService == nil || roles == nil || languages == nil ||
		currencies == nil {
		err = ErrHandlerConfigInvalid
	} else if flashes, err = utils.NewFlashes(flashStorage, cookies); err != nil {
		err = fmt.Errorf("%w: %w", ErrHandlerConfigInvalid, err)
	} else {
		handler = &Handler{
			passwords:  passwords,
			sessions:   sessions,
			identities: identities,
			users:      userService,
			roles:      roles,
			languages:  languages,
			currencies: currencies,
			flashes:    flashes,
			cookies:    cookies,
		}
	}

	return handler, err
}

// Every admin route shares one wrapper: responses are never cached, the
// interface language comes from the route before anything renders, and
// cross-origin unsafe requests are rejected before a handler runs.

func (h *Handler) RegisterHandlers(mux *http.ServeMux) error {
	var err error

	if mux == nil {
		err = ErrServeMuxNil
	} else if registerErr := assets.RegisterHandlers(mux); registerErr != nil {
		err = fmt.Errorf("register admin assets: %w", registerErr)
	} else {
		protection := http.NewCrossOriginProtection()

		for _, route := range []struct {
			pattern string
			handler http.HandlerFunc
		}{
			{"GET /admin/{language}/sign-in", h.signInPage},
			{"POST /admin/{language}/sign-in", h.signIn},
			{"POST /admin/{language}/sign-out", h.signOut},
			{"GET /admin/{language}/restore-password", h.restorePasswordPage},
			{"GET /admin/{language}", h.home},
			{"GET /admin/{language}/settings", h.settings},
			{"POST /admin/{language}/settings/profile", h.updateProfile},
			{"POST /admin/{language}/settings/password", h.changePassword},
			{"GET /admin/{language}/users", h.usersPage},
			{"GET /admin/{language}/users/table", h.usersTable},
			{"GET /admin/{language}/users/create", h.userCreate},
			{"POST /admin/{language}/users/create", h.userCreate},
			{"GET /admin/{language}/users/{user}/view", h.userView},
			{"GET /admin/{language}/users/{user}/edit", h.userEdit},
			{"POST /admin/{language}/users/{user}/edit", h.userEdit},
			{"POST /admin/{language}/users/{user}/delete", h.userDelete},
			{"GET /admin/{language}/roles", h.rolesPage},
			{"GET /admin/{language}/roles/table", h.rolesTable},
			{"GET /admin/{language}/roles/create", h.roleCreate},
			{"POST /admin/{language}/roles/create", h.roleCreate},
			{"GET /admin/{language}/roles/{role}/view", h.roleView},
			{"GET /admin/{language}/roles/{role}/edit", h.roleEdit},
			{"POST /admin/{language}/roles/{role}/edit", h.roleEdit},
			{"POST /admin/{language}/roles/{role}/delete", h.roleDelete},
			{"GET /admin/{language}/currencies", h.currenciesPage},
			{"GET /admin/{language}/currencies/create", h.currencyCreate},
			{"POST /admin/{language}/currencies/create", h.currencyCreate},
			{"GET /admin/{language}/currencies/{currency}/view", h.currencyView},
			{"GET /admin/{language}/currencies/{currency}/edit", h.currencyEdit},
			{"POST /admin/{language}/currencies/{currency}/edit", h.currencyEdit},
			{"POST /admin/{language}/currencies/{currency}/delete", h.currencyDelete},
		} {
			mux.Handle(route.pattern, wrapRoute(protection, route.handler))
		}
	}

	return err
}

//
// Helpers
//

func wrapRoute(protection *http.CrossOriginProtection, next http.HandlerFunc) http.Handler {
	guarded := middleware.Recover(unexpected, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if checkErr := protection.Check(request); checkErr != nil {
			utils.Fail(writer, request, http.StatusForbidden, "errors.cross_origin")
		} else {
			next(writer, request)
		}
	}))

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := admini18n.WithLocale(request.Context(), request.PathValue("language"), request.URL)
		ctx = utils.WithState(ctx)
		request = request.WithContext(ctx)

		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Language", admini18n.Language(ctx))

		guarded.ServeHTTP(writer, request)
	})
}

// A panicking handler is answered with the error page, in the request's
// language, when nothing has been written yet.

func unexpected(writer http.ResponseWriter, request *http.Request) {
	utils.Fail(writer, request, http.StatusInternalServerError, "navigation.error")
}

// Rendering a page consumes every pending flash message, so a message shown
// once never reappears on reload or history navigation.

func (h *Handler) render(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	pageTitle string,
	content templ.Component,
) {
	bag, err := h.flashes.Take(request, "")

	if err != nil {
		utils.FlashUnavailable(writer, request, err)
	} else {
		utils.Render(writer, request, bag, status, pageTitle, content)
	}
}
