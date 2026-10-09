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
	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/pages"
	"github.com/radynsade/faryengo/web/office/utils"
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

	if err != nil && utils.IsUnauthenticated(err) {
		if cookie != "" {
			h.cookies.Clear(writer, utils.SessionCookie)
		}

		err = fmt.Errorf("%w: %w", utils.ErrUnauthenticated, err)
	} else if err != nil {
		err = fmt.Errorf("authenticate an office request: %w", err)
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
		http.Redirect(writer, request, utils.OfficePath(request), http.StatusSeeOther)
	} else if utils.IsUnauthenticated(err) {
		h.renderSignIn(writer, request, http.StatusOK, pages.SignInProps{})
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
		h.renderSignIn(writer, request, http.StatusBadRequest, pages.SignInProps{
			Error: officei18n.T(request.Context(), "errors.form"),
		})
	} else if err != nil {
		h.renderSignIn(writer, request, http.StatusUnprocessableEntity, pages.SignInProps{
			Email:       form.Email,
			FieldErrors: utils.FieldErrors(request.Context(), err),
		})
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
			http.Redirect(writer, request, utils.OfficePath(request), http.StatusSeeOther)
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
		http.Redirect(writer, request, utils.OfficePath(request)+"/sign-in", http.StatusSeeOther)
	} else {
		slog.ErrorContext(request.Context(), "office sign-out", "error", err)
		utils.Fail(writer, request, http.StatusServiceUnavailable, "errors.sign_out")
	}
}

//
// Helpers
//

// A failed in-page submission replaces only the sign-in form; every other
// response is the whole page.

func (h *Handler) renderSignIn(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	props pages.SignInProps,
) {
	props.Action = utils.OfficePath(request) + "/sign-in"

	if request.Method == http.MethodPost && utils.IsPartial(request) {
		utils.RenderFormRegion(writer, request, status, pages.SignInFormID, pages.SignInForm(props))
	} else {
		utils.Render(writer, request, status, officei18n.T(request.Context(), "auth.sign_in"), pages.SignIn(props))
	}
}

// Wrong credentials and an ended Session share one message, so the response
// never tells which part was wrong. The password is never rendered back.

func (h *Handler) signInFailed(
	writer http.ResponseWriter,
	request *http.Request,
	email string,
	err error,
) {
	status, messageID := http.StatusServiceUnavailable, "errors.sign_in_unavailable"

	if utils.IsUnauthenticated(err) {
		status, messageID = http.StatusUnauthorized, "errors.invalid_credentials"
	} else if errors.Is(err, emailpass.ErrVerificationBusy) {
		status, messageID = http.StatusServiceUnavailable, "errors.sign_in_busy"
		writer.Header().Set("Retry-After", "1")
	} else {
		slog.ErrorContext(request.Context(), "office sign-in", "error", err)
	}

	h.renderSignIn(writer, request, status, pages.SignInProps{
		Email: email,
		Error: officei18n.T(request.Context(), messageID),
	})
}
