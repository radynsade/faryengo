package handlers

import (
	"log/slog"
	"net/http"

	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
)

//
// Dashboard
//

func (h *Handler) root(writer http.ResponseWriter, request *http.Request) {
	http.Redirect(writer, request, utils.OfficePath(request), http.StatusSeeOther)
}

// Sections are empty for now; each shows the dashboard with its own menu
// item active. A visitor who is not signed in is sent to the sign-in page.

func (h *Handler) section(key string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		current, err := h.authenticate(writer, request)

		if err == nil {
			title := officei18n.T(request.Context(), "nav."+key)
			props := utils.DashboardProps(request, current, key, title)

			utils.Render(writer, request, http.StatusOK, title, pages.Section(props))
		} else if utils.IsUnauthenticated(err) {
			http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
		} else {
			slog.ErrorContext(request.Context(), "office authentication", "error", err)
			utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.sign_in_unavailable")
		}
	}
}
