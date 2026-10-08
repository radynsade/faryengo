package admin

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

func (h *Handler) home(writer http.ResponseWriter, request *http.Request) {
	h.panel(writer, request, "", "")
}

func (h *Handler) users(writer http.ResponseWriter, request *http.Request) {
	h.panel(writer, request, "users", users.PermissionViewUser)
}

func (h *Handler) panel(writer http.ResponseWriter, request *http.Request, section string, permission users.Permission) {
	h.withPanel(writer, request, section, permission, func(_ users.Principal, props layouts.PanelProps) {
		content := pages.Home(props)

		if section != "" {
			content = pages.PanelSection(props, admini18n.T(request.Context(), "home.users_soon"))
		}

		h.renderPage(writer, request, props.Title+" · Faryen "+admini18n.T(request.Context(), "common.admin"), content)
	})
}

func (h *Handler) withPanel(writer http.ResponseWriter, request *http.Request, section string, permission users.Permission, handle func(users.Principal, layouts.PanelProps)) {
	principal, err := h.authenticate(writer, request)

	if invalidSession(err) {
		http.Redirect(writer, request, adminPath(request)+"/sign-in", http.StatusSeeOther)
	} else if err != nil {
		h.signInError(writer, request, "", err)
	} else {
		flashState(request).principal = &principal

		props := layouts.PanelProps{
			Title:    admini18n.T(request.Context(), "common.panel"),
			BasePath: adminPath(request), ActiveSection: section,
			FirstName: string(principal.FirstName), LastName: string(principal.LastName),
			Email: string(principal.Email),
		}

		switch section {
		case "users":
			props.Title = admini18n.T(request.Context(), "navigation.users")
		case "roles":
			props.Title = admini18n.T(request.Context(), "navigation.roles")
		}

		allowed := permission == "" || app.Authorize(principal, permission) == nil

		if allowed {
			handle(principal, props)
		} else {
			if err := h.addFlash(request.Context(), writer, request, flashmsg.Error, admini18n.T(request.Context(), "errors.section")); err != nil {
				h.flashUnavailable(writer, request, err)
			} else {
				h.renderPage(writer, request, props.Title+" · Faryen "+admini18n.T(request.Context(), "common.admin"), pages.PanelSection(props, ""), templ.WithStatus(http.StatusForbidden))
			}
		}
	}
}
