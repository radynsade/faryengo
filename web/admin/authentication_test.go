package admin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	otherRoles  []*security.Role
	roleErr     error
	deleteErr   error
	lastQuery   security.RoleQuery
	lastFilters security.RoleFilters
	roleWrites  int
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
func (r *httpCredentials) FindByID(_ context.Context, id security.RoleID) (*security.Role, error) {
	var role *security.Role
	var err error
	if r.roleErr != nil {
		err = r.roleErr
	} else if r.role != nil && id == r.role.ID() {
		role = r.role
	} else {
		for _, candidate := range r.otherRoles {
			if candidate.ID() == id {
				role = candidate
			}
		}
		if role == nil {
			err = security.ErrRoleNotFound
		}
	}
	return role, err
}

type httpHasher struct{}

func (httpHasher) Hash(context.Context, string) (security.PasswordHash, error) { return "dummy", nil }
func (httpHasher) Verify(_ context.Context, password string, hash security.PasswordHash) (bool, error) {
	return password == "correct" && hash == "stored", nil
}

func httpFixture(t *testing.T) (*http.ServeMux, *httpCredentials, *miniredis.Miniredis) {
	t.Helper()
	return httpFixtureWithCookies(t, true)
}

func httpFixtureWithCookies(t *testing.T, secure bool) (*http.ServeMux, *httpCredentials, *miniredis.Miniredis) {
	t.Helper()
	return httpFixtureWithFlashStorage(t, secure, nil)
}

func httpFixtureWithFlashStorage(t *testing.T, secure bool, flashStorage FlashSessionStorage) (*http.ServeMux, *httpCredentials, *miniredis.Miniredis) {
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

	roleService, err := app.NewRoleService(repository)

	if err != nil {
		t.Fatal(err)
	}

	languageService, err := app.NewLanguageService(httpLanguages{})

	if err != nil {
		t.Fatal(err)
	}

	if flashStorage == nil {
		flashStorage = client
	}

	handler, err := NewHandler(service, roleService, languageService, limiter, flashStorage, secure)

	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()

	if err := handler.RegisterHandlers(mux); err != nil {
		t.Fatal(err)
	}

	return mux, repository, mini
}

func httpRequest(mux *http.ServeMux, method, path, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://admin.example.com"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func login(t *testing.T, mux *http.ServeMux) []*http.Cookie {
	t.Helper()
	response := httpRequest(mux, http.MethodPost, "/admin/en/sign-in", url.Values{"email": {"person@example.com"}, "password": {"correct"}}.Encode(), nil)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("sign-in status = %d, body = %s", response.Code, response.Body.String())
	}

	return response.Result().Cookies()
}

func TestAdminSignIn(t *testing.T) {
	for _, tt := range []struct {
		name, language string
		secure         bool
	}{
		{name: "secure cookies", language: "en", secure: true},
		{name: "local HTTP cookies", language: "en"},
		{name: "language path", language: "lv", secure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, _, _ := httpFixtureWithCookies(t, tt.secure)
			root := "/admin/" + tt.language
			page := httpRequest(mux, http.MethodGet, root+"/sign-in", "", nil)

			for _, markup := range []string{`method="post" action="` + root + `/sign-in"`, `type="submit"`, `name="email"`, `name="password"`, `required`} {
				if !strings.Contains(page.Body.String(), markup) {
					t.Fatalf("sign-in form is missing %q: %s", markup, page.Body.String())
				}
			}

			body := url.Values{"email": {"person@example.com"}, "password": {"correct"}}.Encode()
			response := httpRequest(mux, http.MethodPost, root+"/sign-in", body, nil)

			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != root {
				t.Fatalf("sign-in = %d, Location = %q", response.Code, response.Header().Get("Location"))
			}

			cookies := response.Result().Cookies()

			if len(cookies) != 2 {
				t.Fatalf("cookies = %d", len(cookies))
			}

			for _, cookie := range cookies {
				if cookie.Secure != tt.secure || strings.HasPrefix(cookie.Name, "__Host-") != tt.secure ||
					!cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge < 1 {
					t.Fatalf("unsafe cookie flags: %s", cookie)
				}

				if strings.Contains(response.Body.String(), cookie.Value) {
					t.Fatal("token exposed in HTML")
				}
			}

			home := httpRequest(mux, http.MethodGet, root, "", cookies)

			if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), "You are signed in.") ||
				!strings.Contains(home.Body.String(), `action="`+root+`/sign-out"`) {
				t.Fatalf("admin home = %d: %s", home.Code, home.Body.String())
			}

			for _, response := range []*httptest.ResponseRecorder{page, home, response} {
				if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal("authentication response lacks protection headers")
				}
			}
		})
	}
}

func TestAdminInvalidSignIn(t *testing.T) {
	mux, _, _ := httpFixture(t)

	for _, tt := range []struct {
		name, body, contentType, query string
		status                         int
	}{
		{name: "unknown account", body: "email=unknown%40example.com&password=correct", status: http.StatusUnauthorized},
		{name: "wrong password", body: "email=person%40example.com&password=wrong-secret", status: http.StatusUnauthorized},
		{name: "invalid email", body: "email=bad&password=correct", status: http.StatusBadRequest},
		{name: "empty password", body: "email=person%40example.com&password=", status: http.StatusBadRequest},
		{name: "missing email", body: "password=correct", status: http.StatusBadRequest},
		{name: "duplicate email", body: "email=person%40example.com&email=other%40example.com&password=correct", status: http.StatusBadRequest},
		{name: "duplicate password", body: "email=person%40example.com&password=correct&password=other", status: http.StatusBadRequest},
		{name: "unknown field", body: "email=person%40example.com&password=correct&role=admin", status: http.StatusBadRequest},
		{name: "malformed encoding", body: "email=%zz&password=correct", status: http.StatusBadRequest},
		{name: "query credentials ignored", body: "password=correct", query: "?email=person%40example.com", status: http.StatusBadRequest},
		{name: "oversized body", body: "email=person%40example.com&password=" + strings.Repeat("x", 32768), status: http.StatusBadRequest},
		{name: "JSON rejected", body: `{}`, contentType: "application/json", status: http.StatusUnsupportedMediaType},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://admin.example.com/admin/en/sign-in"+tt.query, strings.NewReader(tt.body))
			contentType := "application/x-www-form-urlencoded"

			if tt.contentType != "" {
				contentType = tt.contentType
			}

			request.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != tt.status || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") ||
				!strings.Contains(response.Body.String(), `class="form-error" role="alert"`) || len(response.Result().Cookies()) != 1 {
				t.Fatalf("invalid sign-in = %d: %s", response.Code, response.Body.String())
			}

			flashCookie := response.Result().Cookies()[0]

			if flashCookie.Name != "__Host-faryen_flash" || !flashCookie.HttpOnly || !flashCookie.Secure || flashCookie.SameSite != http.SameSiteStrictMode {
				t.Fatalf("unsafe anonymous flash cookie: %s", flashCookie)
			}

			reloaded := httpRequest(mux, http.MethodGet, "/admin/en/sign-in", "", []*http.Cookie{flashCookie})

			if reloaded.Code != http.StatusOK || strings.Contains(reloaded.Body.String(), `class="form-error"`) {
				t.Fatal("sign-in error was replayed after rendering")
			}

			if strings.Contains(response.Body.String(), "wrong-secret") {
				t.Fatal("password echoed in response")
			}

			if tt.name == "wrong password" && !strings.Contains(response.Body.String(), `value="person@example.com"`) {
				t.Fatal("email not preserved after failed sign-in")
			}
		})
	}
}

func TestAdminCSRF(t *testing.T) {
	mux, _, _ := httpFixture(t)
	cookies := login(t, mux)

	for _, tt := range []struct {
		name, path, origin, site string
		status                   int
	}{
		{name: "sign-in cross origin", path: "sign-in", origin: "https://attacker.example", status: http.StatusForbidden},
		{name: "sign-in cross site", path: "sign-in", site: "cross-site", status: http.StatusForbidden},
		{name: "refresh cross origin", path: "refresh", origin: "https://attacker.example", status: http.StatusForbidden},
		{name: "sign-out cross origin", path: "sign-out", origin: "https://attacker.example", status: http.StatusForbidden},
		{name: "same origin", path: "sign-in", origin: "https://admin.example.com", status: http.StatusSeeOther},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://admin.example.com/admin/en/"+tt.path,
				strings.NewReader("email=person%40example.com&password=correct"))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", tt.origin)
			request.Header.Set("Sec-Fetch-Site", tt.site)

			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != tt.status {
				t.Fatalf("CSRF status = %d: %s", response.Code, response.Body.String())
			}

			if tt.status == http.StatusForbidden && (len(response.Result().Cookies()) != 0 || response.Header().Get("Cache-Control") != "no-store") {
				t.Fatal("rejected request changed cookies or may be cached")
			}
		})
	}

	if response := httpRequest(mux, http.MethodGet, "/admin/en", "", cookies); response.Code != http.StatusOK {
		t.Fatalf("CSRF requests invalidated the session: %d", response.Code)
	}
}

func TestAdminThrottling(t *testing.T) {
	mux, _, _ := httpFixture(t)

	for attempt := range 11 {
		response := httpRequest(mux, http.MethodPost, "/admin/en/sign-in", "email=person%40example.com&password=wrong", nil)
		expected := http.StatusUnauthorized

		if attempt == 10 {
			expected = http.StatusTooManyRequests

			if response.Header().Get("Retry-After") != "900" || !strings.Contains(response.Body.String(), "Too many sign-in attempts") {
				t.Fatal("missing throttling feedback")
			}
		}

		if response.Code != expected {
			t.Fatalf("attempt %d = %d", attempt, response.Code)
		}
	}
}

func TestAdminSessionRenewalAndSignOut(t *testing.T) {
	for _, tt := range []struct {
		name   string
		replay bool
	}{
		{name: "renewal and logout"},
		{name: "refresh replay", replay: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, _, _ := httpFixture(t)
			old := login(t, mux)
			var refreshOnly []*http.Cookie

			for _, cookie := range old {
				if cookie.Name == "__Host-faryen_refresh" {
					refreshOnly = append(refreshOnly, cookie)
				}
			}

			response := httpRequest(mux, http.MethodGet, "/admin/en", "", refreshOnly)

			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en/sign-in" {
				t.Fatal("missing access cookie did not redirect to sign-in")
			}

			page := httpRequest(mux, http.MethodGet, "/admin/en/sign-in", "", refreshOnly)

			if !strings.Contains(page.Body.String(), "Continue your session") || !strings.Contains(page.Body.String(), `action="/admin/en/refresh"`) {
				t.Fatal("missing session renewal form")
			}

			response = httpRequest(mux, http.MethodPost, "/admin/en/refresh", "", refreshOnly)

			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en" {
				t.Fatalf("renewal = %d: %s", response.Code, response.Body.String())
			}

			next := response.Result().Cookies()

			if len(next) != 2 || next[0].Value == old[0].Value || next[1].Value == old[1].Value {
				t.Fatal("renewal did not rotate both tokens")
			}

			if tt.replay {
				response = httpRequest(mux, http.MethodPost, "/admin/en/refresh", "", old)

				if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "Continue your session") {
					t.Fatal("refresh replay accepted or retry offered")
				}
			} else {
				if response := httpRequest(mux, http.MethodGet, "/admin/en", "", next); response.Code != http.StatusOK {
					t.Fatalf("renewed session rejected: %d", response.Code)
				}

				response = httpRequest(mux, http.MethodPost, "/admin/en/sign-out", "", next)

				if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en/sign-in" {
					t.Fatalf("sign-out = %d", response.Code)
				}
			}

			cleared := 0

			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == "__Host-faryen_access" || cookie.Name == "__Host-faryen_refresh" {
					if cookie.MaxAge != -1 || !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" {
						t.Fatal("session cookies not cleared safely")
					}

					cleared++
				}
			}

			if cleared != 2 {
				t.Fatal("both session cookies must be cleared")
			}

			if response := httpRequest(mux, http.MethodGet, "/admin/en", "", next); response.Code != http.StatusSeeOther {
				t.Fatalf("revoked session remained active: %d", response.Code)
			}
		})
	}
}

func TestAdminAuthenticationState(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*httpCredentials, *miniredis.Miniredis, []*http.Cookie) []*http.Cookie
		status int
	}{
		{name: "anonymous", change: func(_ *httpCredentials, _ *miniredis.Miniredis, _ []*http.Cookie) []*http.Cookie { return nil }, status: http.StatusSeeOther},
		{name: "password change", change: func(repo *httpCredentials, _ *miniredis.Miniredis, cookies []*http.Cookie) []*http.Cookie {
			repo.credentials.Version = uuid.New()

			return cookies
		}, status: http.StatusSeeOther},
		{name: "role removed", change: func(repo *httpCredentials, _ *miniredis.Miniredis, cookies []*http.Cookie) []*http.Cookie {
			repo.role = nil

			return cookies
		}, status: http.StatusForbidden},
		{name: "Redis outage", change: func(_ *httpCredentials, mini *miniredis.Miniredis, cookies []*http.Cookie) []*http.Cookie {
			mini.Close()

			return cookies
		}, status: http.StatusServiceUnavailable},
		{name: "refresh cannot authorize", change: func(_ *httpCredentials, _ *miniredis.Miniredis, cookies []*http.Cookie) []*http.Cookie {
			cookies[0].Value = cookies[1].Value

			return cookies
		}, status: http.StatusSeeOther},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, repo, mini := httpFixture(t)
			cookies := tt.change(repo, mini, login(t, mux))
			response := httpRequest(mux, http.MethodGet, "/admin/en", "", cookies)

			if response.Code != tt.status || strings.Contains(response.Body.String(), "You are signed in.") {
				t.Fatalf("access = %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAdminLimiterFailure(t *testing.T) {
	mux, _, mini := httpFixture(t)
	mini.Close()
	response := httpRequest(mux, http.MethodPost, "/admin/en/sign-in", "email=person%40example.com&password=correct", nil)

	if response.Code != http.StatusServiceUnavailable || len(response.Result().Cookies()) != 0 || !strings.Contains(response.Body.String(), "temporarily unavailable") {
		t.Fatalf("limiter failure = %d: %s", response.Code, response.Body.String())
	}
}

func TestSecurityAPIRemoved(t *testing.T) {
	mux, _, _ := httpFixture(t)

	for _, tt := range []struct{ method, path string }{
		{http.MethodPost, "/api/auth/sign-in"},
		{http.MethodPost, "/api/auth/refresh"},
		{http.MethodPost, "/api/auth/sign-out"},
		{http.MethodPost, "/api/auth/sign-out-all"},
		{http.MethodGet, "/api/auth/me"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			if response := httpRequest(mux, tt.method, tt.path, "", nil); response.Code != http.StatusNotFound {
				t.Fatalf("removed API returned %d", response.Code)
			}
		})
	}
}

func TestAdminInvalidHandlerConfig(t *testing.T) {
	if handler, err := NewHandler(nil, nil, nil, nil, nil, true); handler != nil || !errors.Is(err, ErrInvalidHandlerConfig) {
		t.Fatalf("invalid config = %v, %v", handler, err)
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
