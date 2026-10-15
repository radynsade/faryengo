package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	"github.com/radynsade/faryengo/middleware"
	"github.com/radynsade/faryengo/pkg/cookie"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	"github.com/radynsade/faryengo/web/office/assets"
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Handler
//

var (
	ErrHandlerConfigInvalid = errors.New("invalid office handler configuration")
	ErrServeMuxNil          = errors.New("office ServeMux is nil")
)

type Handler struct {
	passwords  *emailpass.Authenticator
	sessions   *sessionid.Authenticator
	identities *security.IdentityResolver
	workspaces *app.WorkspaceService
	categories *app.CategoryService
	accounts   *app.AccountService
	budgets    *app.BudgetService
	currencies *app.CurrencyService
	users      *app.UserService
	flashes    *utils.Flashes
	cookies    cookie.Jar
}

func NewHandler(
	passwords *emailpass.Authenticator,
	sessions *sessionid.Authenticator,
	identities *security.IdentityResolver,
	workspaces *app.WorkspaceService,
	categoryService *app.CategoryService,
	accountService *app.AccountService,
	budgetService *app.BudgetService,
	currencyService *app.CurrencyService,
	userService *app.UserService,
	flashStorage flashmsg.FlashSessionStorage,
	secureCookies bool,
) (*Handler, error) {
	var (
		handler *Handler
		flashes *utils.Flashes
		err     error
	)

	if passwords == nil || sessions == nil || identities == nil || workspaces == nil || categoryService == nil || accountService == nil ||
		budgetService == nil || currencyService == nil || userService == nil {
		err = ErrHandlerConfigInvalid
	} else if flashes, err = utils.NewFlashes(flashStorage); err != nil {
		err = fmt.Errorf("%w: %w", ErrHandlerConfigInvalid, err)
	} else {
		handler = &Handler{
			passwords:  passwords,
			sessions:   sessions,
			identities: identities,
			workspaces: workspaces,
			categories: categoryService,
			accounts:   accountService,
			budgets:    budgetService,
			currencies: currencyService,
			users:      userService,
			flashes:    flashes,
			cookies:    cookie.Jar{Secure: secureCookies},
		}
	}

	return handler, err
}

// Every office route shares one wrapper: responses are never cached, the
// interface language comes from the route before anything renders, and
// cross-origin unsafe requests are rejected before a handler runs. Any other
// address or method under the office is answered with the office's own error
// page, in the language its address names.

func (h *Handler) RegisterHandlers(mux *http.ServeMux) error {
	var err error

	if mux == nil {
		err = ErrServeMuxNil
	} else if registerErr := assets.RegisterHandlers(mux); registerErr != nil {
		err = fmt.Errorf("register office assets: %w", registerErr)
	} else {
		protection := http.NewCrossOriginProtection()

		for _, route := range []struct {
			pattern string
			handler http.HandlerFunc
		}{
			{"GET /office", h.root},
			{"GET /office/{language}", h.root},
			{"GET /office/{language}/profile-settings", h.profileSettings},
			{"POST /office/{language}/profile-settings/profile", h.updateProfile},
			{"POST /office/{language}/profile-settings/password", h.changePassword},
			{"GET /office/{language}/workspaces", h.selectWorkspace},
			{"POST /office/{language}/workspaces", h.createWorkspace},
			{"GET /office/{language}/workspaces/new", h.newWorkspacePage},
			{"GET /office/{language}/workspaces/{workspace}", h.section("overview")},
			{"GET /office/{language}/workspaces/{workspace}/categories", h.listCategories},
			{"POST /office/{language}/workspaces/{workspace}/categories", h.createCategory},
			{"GET /office/{language}/workspaces/{workspace}/categories/new", h.newCategory},
			{"GET /office/{language}/workspaces/{workspace}/categories/{category}", h.editCategory},
			{"POST /office/{language}/workspaces/{workspace}/categories/{category}", h.updateCategory},
			{"POST /office/{language}/workspaces/{workspace}/categories/{category}/delete", h.deleteCategory},
			{"GET /office/{language}/workspaces/{workspace}/accounts", h.listAccounts},
			{"POST /office/{language}/workspaces/{workspace}/accounts", h.createAccount},
			{"GET /office/{language}/workspaces/{workspace}/accounts/new", h.newAccount},
			{"GET /office/{language}/workspaces/{workspace}/accounts/{account}", h.editAccount},
			{"POST /office/{language}/workspaces/{workspace}/accounts/{account}", h.updateAccount},
			{"POST /office/{language}/workspaces/{workspace}/accounts/{account}/delete", h.deleteAccount},
			{"GET /office/{language}/workspaces/{workspace}/budgets", h.listBudgets},
			{"POST /office/{language}/workspaces/{workspace}/budgets", h.createBudget},
			{"GET /office/{language}/workspaces/{workspace}/budgets/new", h.newBudget},
			{"GET /office/{language}/workspaces/{workspace}/budgets/{budget}", h.editBudget},
			{"POST /office/{language}/workspaces/{workspace}/budgets/{budget}", h.updateBudget},
			{"POST /office/{language}/workspaces/{workspace}/budgets/{budget}/delete", h.deleteBudget},
			{"GET /office/{language}/workspaces/{workspace}/profile-settings", h.profileSettings},
			{"POST /office/{language}/workspaces/{workspace}/profile-settings/profile", h.updateProfile},
			{"POST /office/{language}/workspaces/{workspace}/profile-settings/password", h.changePassword},
			{"GET /office/{language}/workspaces/{workspace}/settings", h.workspaceSettings},
			{"POST /office/{language}/workspaces/{workspace}/settings/workspace", h.renameWorkspace},
			{"POST /office/{language}/workspaces/{workspace}/settings/members", h.addWorkspaceMember},
			{"POST /office/{language}/workspaces/{workspace}/settings/members/{user}/role", h.changeWorkspaceMemberRole},
			{"POST /office/{language}/workspaces/{workspace}/settings/members/{user}/remove", h.removeWorkspaceMember},
			{"GET /office/{language}/sign-in", h.signInPage},
			{"POST /office/{language}/sign-in", h.signIn},
			{"POST /office/{language}/sign-out", h.signOut},
			{"/office", h.notFound},
			{"/office/{language}", h.notFound},
			{"/office/{language}/{path...}", h.notFound},
		} {
			mux.Handle(route.pattern, wrapRoute(protection, route.handler))
		}
	}

	return err
}

//
// Not found
//

func (h *Handler) notFound(writer http.ResponseWriter, request *http.Request) {
	utils.Fail(writer, request, http.StatusNotFound, "errors.not_found")
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
		ctx := officei18n.WithLocale(request.Context(), request.PathValue("language"), request.URL)
		request = request.WithContext(ctx)

		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Language", officei18n.Language(ctx))

		guarded.ServeHTTP(writer, request)
	})
}

// A panicking handler is answered with the error page, in the request's
// language, when nothing has been written yet.

func unexpected(writer http.ResponseWriter, request *http.Request) {
	utils.Fail(writer, request, http.StatusInternalServerError, "errors.unexpected")
}
