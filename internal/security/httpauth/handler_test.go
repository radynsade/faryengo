package httpauth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
	securityjwt "github.com/radynsade/faryengo/internal/security/jwt"
	securityredis "github.com/radynsade/faryengo/internal/security/redis"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type httpCredentials struct {
	credentials security.Credentials
	role        *security.Role
}

func (r *httpCredentials) FindByEmail(_ context.Context, email security.Email) (*security.Credentials, error) {
	var credentials *security.Credentials
	var err error

	if strings.EqualFold(string(email), string(r.credentials.User.Email())) {
		copy := r.credentials
		credentials = &copy
	} else {
		err = security.ErrUserNotFound
	}

	return credentials, err
}
func (r *httpCredentials) FindByUserID(_ context.Context, id security.UserID) (*security.Credentials, error) {
	var credentials *security.Credentials
	var err error

	if id == r.credentials.User.ID() {
		copy := r.credentials
		credentials = &copy
	} else {
		err = security.ErrUserNotFound
	}

	return credentials, err
}
func (r *httpCredentials) FindByID(context.Context, security.RoleID) (*security.Role, error) {
	return r.role, nil
}

type httpHasher struct{}

func (httpHasher) Hash(context.Context, string) (security.PasswordHash, error) { return "dummy", nil }
func (httpHasher) Verify(_ context.Context, password string, hash security.PasswordHash) (bool, error) {
	return password == "correct" && hash == "stored", nil
}

func httpFixture(t *testing.T) (*http.ServeMux, *httpCredentials, *miniredis.Miniredis) {
	t.Helper()
	roleID := security.RoleID(uuid.New())
	user, err := security.NewUser(security.UserID(uuid.New()), roleID, "person@example.com", "+37123456789", "stored", "First", "Last")

	if err != nil {
		t.Fatal(err)
	}

	translation, err := languages.NewTranslation("en", "Admin")

	if err != nil {
		t.Fatal(err)
	}

	role, err := security.NewRole(roleID, languages.Text{"en": translation}, []security.Permission{security.PermissionViewUser})

	if err != nil {
		t.Fatal(err)
	}

	repository := &httpCredentials{credentials: security.Credentials{User: user, Version: uuid.New()}, role: role}
	mini := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: mini.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	sessions, err := securityredis.NewSessionStore(client)

	if err != nil {
		t.Fatal(err)
	}

	limiter, err := securityredis.NewRateLimiter(client)

	if err != nil {
		t.Fatal(err)
	}

	_, key, err := ed25519.GenerateKey(rand.Reader)

	if err != nil {
		t.Fatal(err)
	}

	tokens, err := securityjwt.NewManager(securityjwt.Config{PrivateKey: key, KeyID: "test", Issuer: "faryen", AccessAudience: "admin", RefreshAudience: "refresh", AccessTTL: 5 * time.Minute})

	if err != nil {
		t.Fatal(err)
	}

	service, err := app.NewAuthenticationService(context.Background(), app.AuthenticationDependencies{Credentials: repository, Invalidator: repository, Roles: repository, Hasher: httpHasher{}, Tokens: tokens, Sessions: sessions, Rotator: sessions, Revoker: sessions}, 24*time.Hour)

	if err != nil {
		t.Fatal(err)
	}

	handler, err := NewHandler(service, limiter, true)

	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()

	if err := handler.RegisterHandlers(mux); err != nil {
		t.Fatal(err)
	}

	protected := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, found := PrincipalFromContext(request.Context()); !found {
			t.Error("principal missing")
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("GET /protected/view", handler.RequirePermission(security.PermissionViewUser, protected))
	mux.Handle("GET /protected/manage", handler.RequirePermission(security.PermissionManageUser, protected))
	mux.Handle("GET /protected/invalid", handler.RequirePermission("", protected))
	mux.Handle("POST /protected/view", handler.RequirePermission(security.PermissionViewUser, protected))
	return mux, repository, mini
}

func httpRequest(mux *http.ServeMux, method, path, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://admin.example.com"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func login(t *testing.T, mux *http.ServeMux) []*http.Cookie {
	t.Helper()
	response := httpRequest(mux, http.MethodPost, "/api/auth/sign-in", `{"email":"person@example.com","password":"correct"}`, nil)

	if response.Code != http.StatusNoContent {
		t.Fatalf("sign-in status = %d, body = %s", response.Code, response.Body.String())
	}

	return response.Result().Cookies()
}

func TestHTTPCookiesPermissionsAndPasswordChange(t *testing.T) {
	mux, repository, _ := httpFixture(t)
	cookies := login(t, mux)

	if len(cookies) != 2 {
		t.Fatalf("cookies = %d", len(cookies))
	}

	for _, cookie := range cookies {
		if !strings.HasPrefix(cookie.Name, "__Host-") || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge < 1 {
			t.Fatalf("unsafe cookie flags: %s", cookie.Name)
		}
	}

	for _, tt := range []struct {
		name, path string
		status     int
	}{
		{"authenticated identity", "/api/auth/me", http.StatusOK},
		{"allowed permission", "/protected/view", http.StatusNoContent},
		{"denied permission", "/protected/manage", http.StatusForbidden},
		{"invalid permission", "/protected/invalid", http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httpRequest(mux, http.MethodGet, tt.path, "", cookies)

			if response.Code != tt.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}

	repository.credentials.Version = uuid.New()

	for _, tt := range []struct{ name, method, path string }{
		{"access denied", http.MethodGet, "/api/auth/me"}, {"refresh denied", http.MethodPost, "/api/auth/refresh"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httpRequest(mux, tt.method, tt.path, "", cookies)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}

			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("authentication response may be cached")
			}
		})
	}
}

func TestHTTPRefreshReplayAndLogoutAll(t *testing.T) {
	mux, _, _ := httpFixture(t)
	old := login(t, mux)
	response := httpRequest(mux, http.MethodPost, "/api/auth/refresh", "", old)

	if response.Code != http.StatusNoContent {
		t.Fatalf("refresh = %d %s", response.Code, response.Body.String())
	}

	next := response.Result().Cookies()
	replay := httpRequest(mux, http.MethodPost, "/api/auth/refresh", "", old)

	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay = %d", replay.Code)
	}

	if response := httpRequest(mux, http.MethodGet, "/api/auth/me", "", next); response.Code != http.StatusUnauthorized {
		t.Fatalf("replayed family remained active: %d", response.Code)
	}

	first, second := login(t, mux), login(t, mux)
	response = httpRequest(mux, http.MethodPost, "/api/auth/sign-out-all", "", first)

	if response.Code != http.StatusNoContent {
		t.Fatalf("logout all = %d", response.Code)
	}

	for _, cookies := range [][]*http.Cookie{first, second} {
		if response := httpRequest(mux, http.MethodGet, "/api/auth/me", "", cookies); response.Code != http.StatusUnauthorized {
			t.Fatalf("logged-out device remained active: %d", response.Code)
		}
	}
}

func TestHTTPInvalidInputCSRFAndThrottling(t *testing.T) {
	for _, tt := range []struct {
		name, body, origin, site, contentType string
		status                                int
	}{
		{name: "unknown account", body: `{"email":"unknown@example.com","password":"correct"}`, status: http.StatusUnauthorized},
		{name: "wrong password", body: `{"email":"person@example.com","password":"wrong"}`, status: http.StatusUnauthorized},
		{name: "unknown field", body: `{"email":"person@example.com","password":"correct","role":"admin"}`, status: http.StatusBadRequest},
		{name: "trailing JSON", body: `{"email":"person@example.com","password":"correct"}{}`, status: http.StatusBadRequest},
		{name: "malformed", body: `{`, status: http.StatusBadRequest},
		{name: "wrong content type", body: `{}`, contentType: "text/plain", status: http.StatusUnsupportedMediaType},
		{name: "cross origin", body: `{"email":"person@example.com","password":"correct"}`, origin: "https://attacker.example", status: http.StatusForbidden},
		{name: "cross site metadata", body: `{"email":"person@example.com","password":"correct"}`, site: "cross-site", status: http.StatusForbidden},
		{name: "same origin", body: `{"email":"person@example.com","password":"correct"}`, origin: "https://admin.example.com", status: http.StatusNoContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, _, _ := httpFixture(t)
			request := httptest.NewRequest(http.MethodPost, "https://admin.example.com/api/auth/sign-in", strings.NewReader(tt.body))
			contentType := "application/json"

			if tt.contentType != "" {
				contentType = tt.contentType
			}

			request.Header.Set("Content-Type", contentType)
			request.Header.Set("Origin", tt.origin)
			request.Header.Set("Sec-Fetch-Site", tt.site)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != tt.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}

	mux, _, _ := httpFixture(t)

	for index := 0; index < 11; index++ {
		response := httpRequest(mux, http.MethodPost, "/api/auth/sign-in", `{"email":"person@example.com","password":"wrong"}`, nil)
		expected := http.StatusUnauthorized

		if index == 10 {
			expected = http.StatusTooManyRequests
		}

		if response.Code != expected {
			t.Fatalf("attempt %d: status = %d", index, response.Code)
		}
	}
}

func TestHTTPStorageFailureAndTokenTypes(t *testing.T) {
	mux, _, mini := httpFixture(t)
	cookies := login(t, mux)
	request := httptest.NewRequest(http.MethodGet, "https://admin.example.com/api/auth/me", nil)

	for _, cookie := range cookies {
		request.AddCookie(cookie)

		if cookie.Name == "__Host-faryen_refresh" {
			request.Header.Set("Authorization", "Bearer "+cookie.Value)
		}
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatal("refresh token authorized as access")
	}

	mini.Close()
	response = httpRequest(mux, http.MethodGet, "/api/auth/me", "", cookies)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("Redis failure = %d", response.Code)
	}
}

func TestHTTPProtectedMutationCSRF(t *testing.T) {
	mux, _, _ := httpFixture(t)
	cookies := login(t, mux)
	request := httptest.NewRequest(http.MethodPost, "https://admin.example.com/protected/view", nil)
	request.Header.Set("Origin", "https://attacker.example")

	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("protected mutation accepted cross-origin request: %d", response.Code)
	}
}

func (r *httpCredentials) Invalidate(ctx context.Context, id security.UserID) error {
	var err error

	if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	} else if id != r.credentials.User.ID() {
		err = security.ErrUserNotFound
	} else {
		r.credentials.Version = uuid.New()
	}

	return err
}
