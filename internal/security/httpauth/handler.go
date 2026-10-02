package httpauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/security"
)

var ErrInvalidHandlerConfig = errors.New("invalid authentication handler configuration")

type RateLimiter interface {
	Allow(context.Context, string, int, time.Duration) (bool, error)
}

type Handler struct {
	service       *app.AuthenticationService
	limiter       RateLimiter
	secureCookies bool
}

type principalContextKey struct{}

func NewHandler(service *app.AuthenticationService, limiter RateLimiter, secureCookies bool) (*Handler, error) {
	var handler *Handler
	var err error

	if service == nil || limiter == nil {
		err = ErrInvalidHandlerConfig
	} else {
		handler = &Handler{service: service, limiter: limiter, secureCookies: secureCookies}
	}

	return handler, err
}

func (h *Handler) RegisterHandlers(mux *http.ServeMux) error {
	var err error

	if mux == nil {
		err = ErrInvalidHandlerConfig
	} else {
		protection := http.NewCrossOriginProtection()
		routes := []struct {
			pattern string
			handler http.Handler
		}{
			{"POST /api/auth/sign-in", http.HandlerFunc(h.signIn)},
			{"POST /api/auth/refresh", http.HandlerFunc(h.refresh)},
			{"POST /api/auth/sign-out", http.HandlerFunc(h.signOut)},
			{"POST /api/auth/sign-out-all", http.HandlerFunc(h.signOutAll)},
			{"GET /api/auth/me", h.Authenticate(http.HandlerFunc(h.me))},
		}

		for _, route := range routes {
			mux.Handle(route.pattern, protection.Handler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Cache-Control", "no-store")
				writer.Header().Set("X-Content-Type-Options", "nosniff")
				route.handler.ServeHTTP(writer, request)
			})))
		}
	}

	return err
}

func (h *Handler) signIn(writer http.ResponseWriter, request *http.Request) {
	mediaType, _, mediaErr := mime.ParseMediaType(request.Header.Get("Content-Type"))

	if mediaErr != nil || mediaType != "application/json" {
		http.Error(writer, "Use application/json", http.StatusUnsupportedMediaType)
		return
	}

	request.Body = http.MaxBytesReader(writer, request.Body, 32768)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var credentials input.SignInInput
	decodeErr := decoder.Decode(&credentials)

	if decodeErr != nil || !errors.Is(decoder.Decode(new(any)), io.EOF) {
		http.Error(writer, "Invalid sign-in request", http.StatusBadRequest)
	} else if validationErr := credentials.Validate(); validationErr != nil {
		http.Error(writer, "Invalid sign-in request", http.StatusBadRequest)
	} else if h.allowSignIn(writer, request, credentials.Email) {
		pair, err := h.service.SignIn(request.Context(), credentials)

		if err != nil {
			h.writeError(writer, err)
		} else {
			h.setTokens(writer, pair)
			writer.WriteHeader(http.StatusNoContent)
		}
	}
}

func (h *Handler) allowSignIn(writer http.ResponseWriter, request *http.Request, email string) bool {
	allowed := true
	address, _, addressErr := net.SplitHostPort(request.RemoteAddr)

	if addressErr != nil {
		address = request.RemoteAddr
	}

	// Forwarded headers are not trusted: a proxy deployment must set a verified
	// RemoteAddr upstream if it needs individual client IP limits.
	for _, policy := range []struct {
		key   string
		limit int
	}{
		{"ip:" + address, 100},
		{"account:" + strings.ToLower(email), 10},
	} {
		if allowed {
			accepted, err := h.limiter.Allow(request.Context(), policy.key, policy.limit, 15*time.Minute)

			if err != nil {
				http.Error(writer, "Authentication unavailable", http.StatusServiceUnavailable)
				allowed = false
			} else if !accepted {
				writer.Header().Set("Retry-After", "900")
				http.Error(writer, "Too many sign-in attempts", http.StatusTooManyRequests)
				allowed = false
			}
		}
	}

	return allowed
}

func (h *Handler) refresh(writer http.ResponseWriter, request *http.Request) {
	raw := h.cookie(request, "refresh")
	pair, err := h.service.Refresh(request.Context(), raw)

	if err != nil {
		h.writeError(writer, err)
	} else {
		h.setTokens(writer, pair)
		writer.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) signOut(writer http.ResponseWriter, request *http.Request) {
	raw, use := h.cookie(request, "refresh"), security.RefreshToken

	if raw == "" {
		raw, use = h.accessToken(request), security.AccessToken
	}

	err := h.service.SignOut(request.Context(), raw, use)
	h.clearTokens(writer)

	if err != nil {
		h.writeError(writer, err)
	} else {
		writer.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) signOutAll(writer http.ResponseWriter, request *http.Request) {
	err := h.service.SignOutAll(request.Context(), h.accessToken(request))

	if err != nil {
		h.writeError(writer, err)
	} else {
		h.clearTokens(writer)
		writer.WriteHeader(http.StatusNoContent)
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

func (h *Handler) accessToken(request *http.Request) string {
	value := h.cookie(request, "access")

	if header := request.Header.Get("Authorization"); header != "" {
		fields := strings.Fields(header)
		value = ""

		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			value = fields[1]
		}
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

// Authenticate checks JWT, session state, durable credential version, and current role.
func (h *Handler) Authenticate(next http.Handler) http.Handler {
	return h.protect(next, "")
}

func (h *Handler) RequirePermission(permission security.Permission, next http.Handler) http.Handler {
	var handler http.Handler

	if err := permission.Validate(); err != nil {
		handler = http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, "Permission denied", http.StatusForbidden)
		})
	} else {
		handler = h.protect(next, permission)
	}

	return handler
}

func (h *Handler) protect(next http.Handler, permission security.Permission) http.Handler {
	// Also protect future cookie-authenticated mutation routes from CSRF.
	protection := http.NewCrossOriginProtection()
	return protection.Handler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		var principal security.Principal
		var err error

		if permission == "" {
			principal, err = h.service.Authenticate(request.Context(), h.accessToken(request))
		} else {
			principal, err = h.service.Authorize(request.Context(), h.accessToken(request), permission)
		}

		if err != nil {
			h.writeError(writer, err)
		} else {
			ctx := context.WithValue(request.Context(), principalContextKey{}, principal)
			next.ServeHTTP(writer, request.WithContext(ctx))
		}
	}))
}

func PrincipalFromContext(ctx context.Context) (security.Principal, bool) {
	principal, found := ctx.Value(principalContextKey{}).(security.Principal)
	return principal, found
}

func (h *Handler) me(writer http.ResponseWriter, request *http.Request) {
	principal, _ := PrincipalFromContext(request.Context())
	writer.Header().Set("Content-Type", "application/json")
	response := struct {
		UserID      string                `json:"user_id"`
		SessionID   string                `json:"session_id"`
		RoleID      string                `json:"role_id"`
		Permissions []security.Permission `json:"permissions"`
	}{uuid.UUID(principal.UserID).String(), principal.SessionID.String(), uuid.UUID(principal.RoleID).String(), principal.Permissions}

	if err := json.NewEncoder(writer).Encode(response); err != nil {
		slog.ErrorContext(request.Context(), "write authenticated identity", "error", err)
	}
}

func (h *Handler) writeError(writer http.ResponseWriter, err error) {
	status, message := http.StatusServiceUnavailable, "Authentication unavailable"

	if errors.Is(err, app.ErrAuthenticationBusy) {
		writer.Header().Set("Retry-After", "1")
	}

	if errors.Is(err, security.ErrInvalidCredentials) || errors.Is(err, security.ErrInvalidToken) ||
		errors.Is(err, security.ErrSessionRevoked) || errors.Is(err, security.ErrRefreshTokenReused) || errors.Is(err, security.ErrInvalidSession) {
		status, message = http.StatusUnauthorized, "Invalid credentials or session"
		writer.Header().Set("WWW-Authenticate", "Bearer")
	} else if errors.Is(err, security.ErrPermissionDenied) || errors.Is(err, security.ErrInvalidPermission) {
		status, message = http.StatusForbidden, "Permission denied"
	}

	http.Error(writer, message, status)
}
