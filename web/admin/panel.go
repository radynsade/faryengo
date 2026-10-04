package admin

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
)

func (h *Handler) home(writer http.ResponseWriter, request *http.Request) {
	h.panel(writer, request, "", "")
}

func (h *Handler) users(writer http.ResponseWriter, request *http.Request) {
	h.panel(writer, request, "users", security.PermissionViewUser)
}

func (h *Handler) roles(writer http.ResponseWriter, request *http.Request) {
	h.panel(writer, request, "roles", security.PermissionViewRole)
}

func (h *Handler) panel(writer http.ResponseWriter, request *http.Request, section string, permission security.Permission) {
	principal, err := h.service.Authenticate(request.Context(), h.cookie(request, "access"))

	if errors.Is(err, security.ErrInvalidToken) || errors.Is(err, security.ErrSessionRevoked) || errors.Is(err, security.ErrInvalidSession) {
		if !errors.Is(err, security.ErrInvalidToken) {
			h.clearTokens(writer)
		}

		http.Redirect(writer, request, adminPath(request)+"/sign-in", http.StatusSeeOther)
	} else if err != nil {
		h.signInError(writer, request, "", err)
	} else {
		props := layouts.PanelProps{
			Title: "Admin panel", Description: "Welcome to your workspace.",
			BasePath: adminPath(request), ActiveSection: section,
			FirstName: string(principal.FirstName), LastName: string(principal.LastName),
		}

		switch section {
		case "users":
			props.Title, props.Description = "Users", "Manage the people who have access to your workspace."
		case "roles":
			props.Title, props.Description = "Roles", "Organize access and permissions for your workspace."
		}

		status := http.StatusOK
		content := pages.Home(props)

		if permission != "" {
			if principal.HasPermission(permission) {
				content = pages.PanelSection(props, props.Title+" management is coming soon.")
			} else {
				status = http.StatusForbidden
				content = pages.PanelSection(props, "You do not have permission to view this section.")
			}
		}

		renderPage(writer, request, props.Title+" · Faryen Admin", content, templ.WithStatus(status))
	}
}
