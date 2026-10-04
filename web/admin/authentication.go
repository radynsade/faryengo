package admin

import (
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

var errSignInThrottled = errors.New("too many sign-in attempts")

func (h *Handler) signIn(writer http.ResponseWriter, request *http.Request) {
	mediaType, _, mediaErr := mime.ParseMediaType(request.Header.Get("Content-Type"))

	if mediaErr != nil || mediaType != "application/x-www-form-urlencoded" {
		h.renderSignIn(writer, request, http.StatusUnsupportedMediaType, "", "Submit the sign-in form to continue.")
	} else {
		request.Body = http.MaxBytesReader(writer, request.Body, 32768)
		parseErr := request.ParseForm()

		if parseErr != nil || len(request.PostForm) != 2 || len(request.PostForm["email"]) != 1 || len(request.PostForm["password"]) != 1 {
			h.renderSignIn(writer, request, http.StatusBadRequest, "", "Enter a valid email address and password.")
		} else {
			credentials := input.SignInInput{Email: request.PostForm.Get("email"), Password: request.PostForm.Get("password")}

			if validationErr := credentials.Validate(); validationErr != nil {
				h.renderSignIn(writer, request, http.StatusBadRequest, credentials.Email, "Enter a valid email address and password.")
			} else if limitErr := h.allowSignIn(request, credentials.Email); limitErr != nil {
				h.signInError(writer, request, credentials.Email, limitErr)
			} else {
				pair, err := h.service.SignIn(request.Context(), credentials)

				if err != nil {
					h.signInError(writer, request, credentials.Email, err)
				} else {
					h.setTokens(writer, pair)
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

func (h *Handler) refresh(writer http.ResponseWriter, request *http.Request) {
	pair, err := h.service.Refresh(request.Context(), h.cookie(request, "refresh"))

	if err == nil {
		_, err = h.service.Authenticate(request.Context(), pair.AccessToken)
	}

	if err != nil {
		// A rotation failure can be ambiguous; never invite a retry with the
		// old refresh token, which would revoke the session as a replay.
		h.clearTokens(writer)
		h.signInError(writer, request, "", err)
	} else {
		h.setTokens(writer, pair)
		http.Redirect(writer, request, adminPath(request), http.StatusSeeOther)
	}
}

func (h *Handler) signOut(writer http.ResponseWriter, request *http.Request) {
	raw, use := h.cookie(request, "refresh"), security.RefreshToken

	if raw == "" {
		raw, use = h.cookie(request, "access"), security.AccessToken
	}

	err := h.service.SignOut(request.Context(), raw, use)
	h.clearTokens(writer)
	status, _ := authenticationError(err)

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

func (h *Handler) setTokens(writer http.ResponseWriter, pair security.TokenPair) {
	for _, token := range []struct {
		kind, value string
		expires     time.Time
	}{
		{"access", pair.AccessToken, pair.AccessExpiresAt},
		{"refresh", pair.RefreshToken, pair.RefreshExpiresAt},
	} {
		lifetime := int(time.Until(token.expires).Seconds())
		http.SetCookie(writer, &http.Cookie{Name: h.cookieName(token.kind), Value: token.value,
			Path: "/", HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteStrictMode,
			Expires: token.expires, MaxAge: max(1, lifetime)})
	}
}

func (h *Handler) clearTokens(writer http.ResponseWriter) {
	for _, kind := range []string{"access", "refresh"} {
		http.SetCookie(writer, &http.Cookie{Name: h.cookieName(kind), Path: "/", HttpOnly: true,
			Secure: h.secureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	}
}

func (h *Handler) signInError(writer http.ResponseWriter, request *http.Request, email string, err error) {
	status, message := authenticationError(err)

	if errors.Is(err, app.ErrAuthenticationBusy) {
		writer.Header().Set("Retry-After", "1")
	} else if errors.Is(err, errSignInThrottled) {
		writer.Header().Set("Retry-After", "900")
	}

	h.renderSignIn(writer, request, status, email, message)
}

func authenticationError(err error) (int, string) {
	status, message := http.StatusServiceUnavailable, "Sign-in is temporarily unavailable. Please try again."

	if errors.Is(err, security.ErrInvalidCredentials) || errors.Is(err, security.ErrInvalidToken) ||
		errors.Is(err, security.ErrSessionRevoked) || errors.Is(err, security.ErrRefreshTokenReused) || errors.Is(err, security.ErrInvalidSession) {
		status, message = http.StatusUnauthorized, "The email, password, or session is invalid. Please sign in again."
	} else if errors.Is(err, security.ErrPermissionDenied) || errors.Is(err, security.ErrInvalidPermission) {
		status, message = http.StatusForbidden, "You do not have permission to access this page."
	} else if errors.Is(err, errSignInThrottled) {
		status, message = http.StatusTooManyRequests, "Too many sign-in attempts. Please try again in 15 minutes."
	}

	return status, message
}
