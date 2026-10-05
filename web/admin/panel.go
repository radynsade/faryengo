package admin

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

func (h *Handler) home(writer http.ResponseWriter, request *http.Request) {
	h.panel(writer, request, "", "")
}

func (h *Handler) users(writer http.ResponseWriter, request *http.Request) {
	h.panel(writer, request, "users", security.PermissionViewUser)
}

func (h *Handler) panel(writer http.ResponseWriter, request *http.Request, section string, permission security.Permission) {
	h.withPanel(writer, request, section, permission, func(_ security.Principal, props layouts.PanelProps) {
		content := pages.Home(props)

		if section != "" {
			content = pages.PanelSection(props, props.Title+" management is coming soon.")
		}

		h.renderPage(writer, request, props.Title+" · Faryen Admin", content)
	})
}

func (h *Handler) withPanel(writer http.ResponseWriter, request *http.Request, section string, permission security.Permission, handle func(security.Principal, layouts.PanelProps)) {
	principal, err := h.service.Authenticate(request.Context(), h.cookie(request, "access"))

	if errors.Is(err, security.ErrInvalidToken) || errors.Is(err, security.ErrSessionRevoked) || errors.Is(err, security.ErrInvalidSession) {
		if !errors.Is(err, security.ErrInvalidToken) {
			h.clearTokens(writer)
		}

		http.Redirect(writer, request, adminPath(request)+"/sign-in", http.StatusSeeOther)
	} else if err != nil {
		h.signInError(writer, request, "", err)
	} else {
		flashState(request).principal = &principal

		props := layouts.PanelProps{
			Title:    "Admin panel",
			BasePath: adminPath(request), ActiveSection: section,
			FirstName: string(principal.FirstName), LastName: string(principal.LastName),
			Email: string(principal.Email),
		}

		switch section {
		case "users":
			props.Title = "Users"
		case "roles":
			props.Title = "Roles"
		}

		allowed := permission == "" || principal.HasPermission(permission)

		if allowed {
			handle(principal, props)
		} else {
			if err := h.addFlash(request.Context(), writer, request, flashmsg.Error, "You do not have permission to view this section."); err != nil {
				h.flashUnavailable(writer, request, err)
			} else {
				h.renderPage(writer, request, props.Title+" · Faryen Admin", pages.PanelSection(props, ""), templ.WithStatus(http.StatusForbidden))
			}
		}
	}
}
