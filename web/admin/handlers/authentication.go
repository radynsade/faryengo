package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/security/emailpass"
	"github.com/radynsade/faryengo/internal/security/sessionid"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
	"github.com/radynsade/faryengo/web/admin/utils"
)

//
// Authentication
//

// The Session's identity, with the User's current Role, is resolved on every
// request. A missing, invalid, revoked, or expired Session, and a deleted
// User, are reported as utils.ErrUnauthenticated, and a rejected cookie is
// cleared. Any other failure denies access without ending the Session.

func (h *Handler) authenticate(writer http.ResponseWriter, request *http.Request) (*security.Identity, error) {
	var (
		current *security.Identity
		session *security.Session
		err     error
	)

	cookie := h.cookies.Get(request, utils.SessionCookie)

	if cookie == "" {
		err = utils.ErrUnauthenticated
	} else {
		session, err = h.sessions.SignIn(request.Context(), sessionid.ID(cookie))
	}

	if err == nil {
		current, err = h.identities.Resolve(request.Context(), session)
	}

	if err == nil {
		utils.StateOf(request).Identity = current
	} else if utils.IsUnauthenticated(err) {
		if cookie != "" {
			h.cookies.Clear(writer, utils.SessionCookie)
		}

		err = fmt.Errorf("%w: %w", utils.ErrUnauthenticated, err)
	} else {
		err = fmt.Errorf("authenticate an admin request: %w", err)
	}

	return current, err
}

//
// Sign-in
//

// Sign in page

func (h *Handler) signInPage(writer http.ResponseWriter, request *http.Request) {
	_, err := h.authenticate(writer, request)

	if err == nil {
		http.Redirect(writer, request, utils.AdminPath(request), http.StatusSeeOther)
	} else if utils.IsUnauthenticated(err) {
		h.renderSignIn(writer, request, http.StatusOK, "", nil)
	} else {
		h.signInFailed(writer, request, "", err)
	}
}

// Sign in

func (h *Handler) signIn(writer http.ResponseWriter, request *http.Request) {
	form, err := utils.ParseSignInForm(writer, request)

	if err == nil {
		err = requestvalidation.Validate(request.Context(), form)
	}

	if errors.Is(err, utils.ErrFormInvalid) {
		h.showSignInError(writer, request, http.StatusBadRequest, "", "errors.credentials_form")
	} else if err != nil {
		h.renderSignIn(writer, request, http.StatusUnprocessableEntity, form.Email, utils.FieldErrors(request.Context(), err))
	} else {
		var (
			session *security.Session
			id      sessionid.ID
		)

		session, err = h.passwords.SignIn(request.Context(), emailpass.Credentials{
			Email:    users.Email(form.Email),
			Password: form.Password,
		})

		if err == nil {
			id, err = h.sessions.Issue(request.Context(), session)
		}

		if err == nil {
			h.cookies.Set(writer, utils.SessionCookie, string(id), session.ExpiresAt)
			http.Redirect(writer, request, utils.AdminPath(request), http.StatusSeeOther)
		} else {
			h.signInFailed(writer, request, form.Email, err)
		}
	}
}

// Sign out

// Only this device's Session ends. If it cannot be ended, the cookie stays,
// so the user can try again instead of leaving a Session that still works.

func (h *Handler) signOut(writer http.ResponseWriter, request *http.Request) {
	current, err := h.authenticate(writer, request)

	if err == nil {
		err = h.sessions.SignOut(request.Context(), current.Session, false)

		if err == nil {
			h.cookies.Clear(writer, utils.SessionCookie)
		}
	}

	if err == nil || utils.IsUnauthenticated(err) {
		http.Redirect(writer, request, utils.AdminPath(request)+"/sign-in", http.StatusSeeOther)
	} else if current == nil {
		h.signInFailed(writer, request, "", err)
	} else {
		slog.ErrorContext(request.Context(), "admin sign-out", "error", err)
		h.renderPanelError(writer, request, current, http.StatusServiceUnavailable, "errors.sign_out")
	}
}

// Restore password page

func (h *Handler) restorePasswordPage(writer http.ResponseWriter, request *http.Request) {
	h.render(
		writer,
		request,
		http.StatusOK,
		admini18n.T(request.Context(), "auth.restore"),
		pages.RestorePassword(utils.AdminPath(request)+"/sign-in"),
	)
}

//
// Helpers
//

func (h *Handler) renderSignIn(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	email string,
	fields components.FieldErrors,
) {
	h.render(
		writer,
		request,
		status,
		admini18n.T(request.Context(), "actions.sign_in"),
		pages.SignIn(pages.SignInProps{
			Action:             utils.AdminPath(request) + "/sign-in",
			RestorePasswordURL: utils.AdminPath(request) + "/restore-password",
			Email:              email,
			FieldErrors:        fields,
		}),
	)
}

func (h *Handler) showSignInError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	email string,
	messageID string,
) {
	if err := h.flashes.Add(writer, request, flashmsg.Error, admini18n.T(request.Context(), messageID)); err != nil {
		utils.FlashUnavailable(writer, request, err)
	} else {
		h.renderSignIn(writer, request, status, email, nil)
	}
}

// Wrong credentials and an ended Session share one message, so the response
// never tells which part was wrong.

func (h *Handler) signInFailed(
	writer http.ResponseWriter,
	request *http.Request,
	email string,
	err error,
) {
	status, messageID := http.StatusServiceUnavailable, "errors.sign_in_unavailable"

	if utils.IsUnauthenticated(err) {
		status, messageID = http.StatusUnauthorized, "errors.invalid_session"
	} else if errors.Is(err, emailpass.ErrVerificationBusy) {
		status, messageID = http.StatusServiceUnavailable, "errors.sign_in_busy"
		writer.Header().Set("Retry-After", "1")
	} else {
		slog.ErrorContext(request.Context(), "admin sign-in", "error", err)
	}

	h.showSignInError(writer, request, status, email, messageID)
}
