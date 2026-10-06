package admin

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
)

var errSignInThrottled = errors.New("too many sign-in attempts")

func (h *Handler) signIn(writer http.ResponseWriter, request *http.Request) {
	mediaType, _, mediaErr := mime.ParseMediaType(request.Header.Get("Content-Type"))

	if mediaErr != nil || mediaType != "application/x-www-form-urlencoded" {
		h.renderSignIn(writer, request, http.StatusUnsupportedMediaType, "", admini18n.T(request.Context(), "errors.sign_in_form"), nil)
	} else {
		request.Body = http.MaxBytesReader(writer, request.Body, 32768)
		parseErr := request.ParseForm()

		if parseErr != nil || !validSignInForm(request.PostForm) {
			h.renderSignIn(writer, request, http.StatusBadRequest, "", admini18n.T(request.Context(), "errors.credentials_form"), nil)
		} else {
			credentials := signInRequest{Email: request.PostForm.Get("email"), Password: request.PostForm.Get("password")}

			if validationErr := requestvalidation.Validate(request.Context(), credentials); validationErr != nil {
				h.renderSignIn(writer, request, http.StatusBadRequest, credentials.Email, "", requestFieldErrors(request.Context(), validationErr))
			} else if limitErr := h.allowSignIn(request, credentials.Email); limitErr != nil {
				h.signInError(writer, request, credentials.Email, limitErr)
			} else {
				session, err := h.service.SignIn(request.Context(), input.SignInInput{Email: credentials.Email, Password: credentials.Password})

				if err != nil {
					h.signInError(writer, request, credentials.Email, err)
				} else {
					h.setSession(writer, session)
					http.Redirect(writer, request, adminPath(request), http.StatusSeeOther)
				}
			}
		}
	}
}

func (h *Handler) allowSignIn(request *http.Request, email string) error {
	var err error
	address, _, addressErr := net.SplitHostPort(request.RemoteAddr)

	if addressErr != nil {
		address = request.RemoteAddr
	}

	// Forwarded headers are not trusted: a proxy deployment must supply a
	// verified RemoteAddr upstream if it needs individual client IP limits.
	for _, policy := range []struct {
		key   string
		limit int
	}{
		{"ip:" + address, 100},
		{"account:" + strings.ToLower(email), 10},
	} {
		if err == nil {
			accepted, limitErr := h.limiter.Allow(request.Context(), policy.key, policy.limit, 15*time.Minute)

			if limitErr != nil {
				err = fmt.Errorf("limit admin sign-in: %w", limitErr)
			} else if !accepted {
				err = errSignInThrottled
			}
		}
	}

	return err
}

// authenticate uses the same opaque cookie for SSR and HTMX requests.
func (h *Handler) authenticate(writer http.ResponseWriter, request *http.Request) (security.Principal, error) {
	raw := h.cookie(request, "session")
	principal, err := h.service.Authenticate(request.Context(), raw)

	if invalidSession(err) && raw != "" {
		h.clearSession(writer)
	}

	return principal, err
}

func invalidSession(err error) bool {
	return errors.Is(err, security.ErrSessionRevoked) || errors.Is(err, security.ErrInvalidSession)
}

func (h *Handler) signOut(writer http.ResponseWriter, request *http.Request) {
	err := h.service.SignOut(request.Context(), h.cookie(request, "session"))
	h.clearSession(writer)
	status, _ := authenticationError(request.Context(), err)

	if err == nil || status == http.StatusUnauthorized {
		http.Redirect(writer, request, adminPath(request)+"/sign-in", http.StatusSeeOther)
	} else {
		h.signInError(writer, request, "", err)
	}
}

func (h *Handler) cookieName(kind string) string {
	prefix := "faryen_"

	if h.secureCookies {
		prefix = "__Host-faryen_"
	}

	return prefix + kind
}

func (h *Handler) cookie(request *http.Request, kind string) string {
	var value string
	cookie, err := request.Cookie(h.cookieName(kind))

	if err == nil {
		value = cookie.Value
	}

	return value
}

func (h *Handler) setSession(writer http.ResponseWriter, session security.SessionGrant) {
	lifetime := int(time.Until(session.ExpiresAt).Seconds())
	http.SetCookie(writer, &http.Cookie{Name: h.cookieName("session"), Value: session.ID,
		Path: "/", HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteStrictMode,
		Expires: session.ExpiresAt, MaxAge: max(1, lifetime)})
}

func (h *Handler) clearSession(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{Name: h.cookieName("session"), Path: "/", HttpOnly: true,
		Secure: h.secureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func (h *Handler) signInError(writer http.ResponseWriter, request *http.Request, email string, err error) {
	status, message := authenticationError(request.Context(), err)

	if errors.Is(err, app.ErrAuthenticationBusy) {
		writer.Header().Set("Retry-After", "1")
	} else if errors.Is(err, errSignInThrottled) {
		writer.Header().Set("Retry-After", "900")
	}

	h.renderSignIn(writer, request, status, email, message, nil)
}

func authenticationError(ctx context.Context, err error) (int, string) {
	status, message := http.StatusServiceUnavailable, admini18n.T(ctx, "errors.sign_in_unavailable")

	if errors.Is(err, security.ErrInvalidCredentials) || invalidSession(err) {
		status, message = http.StatusUnauthorized, admini18n.T(ctx, "errors.invalid_session")
	} else if errors.Is(err, security.ErrPermissionDenied) || errors.Is(err, security.ErrInvalidPermission) {
		status, message = http.StatusForbidden, admini18n.T(ctx, "errors.access")
	} else if errors.Is(err, errSignInThrottled) {
		status, message = http.StatusTooManyRequests, admini18n.T(ctx, "errors.throttled")
	}

	return status, message
}

// Missing fields are handled by DTO validation; duplicates and unknown fields
// are malformed forms rather than editable field values.
func validSignInForm(values url.Values) bool {
	valid := len(values["email"]) <= 1 && len(values["password"]) <= 1

	for _, key := range slices.Sorted(maps.Keys(values)) {
		if key != "email" && key != "password" {
			valid = false
		}
	}

	return valid
}
