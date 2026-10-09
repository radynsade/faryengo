package handlers

import (
	"net/http"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
	"github.com/radynsade/faryengo/web/admin/utils"
)

//
// Panel
//

// Home

func (h *Handler) home(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionHome, "", func(_ *security.Identity, panel layouts.PanelProps) {
		h.render(writer, request, http.StatusOK, panel.Title, pages.Home(panel))
	})
}

//
// Helpers
//

// Every panel page authenticates first: a visitor without a valid Session is
// sent to sign in, and a user without the section's permission sees the
// panel with an error instead of the section.

func (h *Handler) withPanel(
	writer http.ResponseWriter,
	request *http.Request,
	section string,
	permission users.Permission,
	handle func(current *security.Identity, panel layouts.PanelProps),
) {
	current, err := h.authenticate(writer, request)

	if utils.IsUnauthenticated(err) {
		http.Redirect(writer, request, utils.AdminPath(request)+"/sign-in", http.StatusSeeOther)
	} else if err != nil {
		h.signInFailed(writer, request, "", err)
	} else {
		panel := h.panelProps(request, current, section)

		if permission == "" || current.Can(permission) {
			handle(current, panel)
		} else {
			h.renderPanelError(writer, request, current, http.StatusForbidden, "errors.section")
		}
	}
}

func (h *Handler) panelProps(request *http.Request, current *security.Identity, section string) layouts.PanelProps {
	titleID := "common.panel"

	switch section {
	case layouts.SectionUsers:
		titleID = "navigation.users"
	case layouts.SectionRoles:
		titleID = "navigation.roles"
	}

	return layouts.PanelProps{
		Title:         admini18n.T(request.Context(), titleID),
		BasePath:      utils.AdminPath(request),
		ActiveSection: section,
		FirstName:     string(current.User.FirstName),
		LastName:      string(current.User.LastName),
		Email:         string(current.User.Email),
		CanViewUsers:  current.Can(users.PermissionViewUser),
		CanViewRoles:  current.Can(users.PermissionViewRole),
	}
}

// The error is shown inside the panel, so navigation stays available.

func (h *Handler) renderPanelError(
	writer http.ResponseWriter,
	request *http.Request,
	current *security.Identity,
	status int,
	messageID string,
) {
	panel := h.panelProps(request, current, layouts.SectionHome)

	if err := h.flashes.Add(writer, request, flashmsg.Error, admini18n.T(request.Context(), messageID)); err != nil {
		utils.FlashUnavailable(writer, request, err)
	} else {
		h.render(writer, request, status, panel.Title, pages.PanelSection(panel, ""))
	}
}
