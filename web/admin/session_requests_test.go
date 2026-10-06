package admin

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"github.com/radynsade/faryengo/internal/app"
	"github.com/radynsade/faryengo/internal/app/input"
	securityjwt "github.com/radynsade/faryengo/internal/security/jwt"
	securityredis "github.com/radynsade/faryengo/internal/security/redis"
)

func TestAdminSessionRequests(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, body string
		fragment                 bool
		status, writes           int
	}{
		{name: "SSR", method: http.MethodGet, path: "/admin/en", status: http.StatusOK},
		{name: "HTMX", method: http.MethodGet, path: "/admin/en/roles", fragment: true, status: http.StatusOK},
		{name: "localized SSR", method: http.MethodGet, path: "/admin/lv/roles", status: http.StatusOK},
		{name: "native form", method: http.MethodPost, path: "/admin/en/roles/create", body: "name[en]=Created", status: http.StatusSeeOther, writes: 1},
		{name: "authenticated sign-in", method: http.MethodGet, path: "/admin/en/sign-in", status: http.StatusSeeOther},
	} {
		t.Run(tt.name, func(t *testing.T) {
			instances, repository, _ := httpFixtureWithInstances(t, true, nil, 2)
			cookies := login(t, instances[0])
			request := httptest.NewRequest(tt.method, "https://admin.example.com"+tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.AddCookie(cookies[0])

			if tt.fragment {
				request.Header.Set("HX-Request", "true")
			}

			response := httptest.NewRecorder()
			instances[1].ServeHTTP(response, request)

			if response.Code != tt.status || repository.roleWrites != tt.writes || len(response.Result().Cookies()) != 0 {
				t.Fatalf("session request = %d, writes = %d, cookies = %v: %s", response.Code, repository.roleWrites, response.Result().Cookies(), response.Body.String())
			}

			if tt.fragment && (strings.Contains(response.Body.String(), "<!doctype") || !strings.Contains(response.Body.String(), `id="page-content"`)) {
				t.Fatal("session authentication did not preserve the HTMX fragment")
			}

			if tt.status == http.StatusOK && response.Header().Get("Location") != "" {
				t.Fatal("authenticated page request redirected")
			}
		})
	}
}

func TestAdminExpiredSessionRejectsMutation(t *testing.T) {
	mux, repository, mini := httpFixture(t)
	cookies := login(t, mux)
	mini.FastForward(24 * time.Hour)
	response := httpRequest(mux, http.MethodPost, "/admin/en/roles/create", "name[en]=Rejected", cookies)

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en/sign-in" || repository.roleWrites != 0 {
		t.Fatalf("expired session = %d, writes = %d", response.Code, repository.roleWrites)
	}

	cleared := response.Result().Cookies()

	if len(cleared) != 1 || cleared[0].Name != "__Host-faryen_session" || cleared[0].MaxAge != -1 {
		t.Fatal("expired session cookie was not removed")
	}
}

func TestAdminUsesOnlyOpaqueSessionCookies(t *testing.T) {
	mux, repository, mini := httpFixture(t)
	_, key, err := ed25519.GenerateKey(rand.Reader)

	if err != nil {
		t.Fatal(err)
	}

	tokens, err := securityjwt.NewManager(securityjwt.Config{PrivateKey: key, KeyID: "test", Issuer: "faryen", AccessAudience: "admin", RefreshAudience: "refresh", AccessTTL: 5 * time.Minute})

	if err != nil {
		t.Fatal(err)
	}

	client := redislib.NewClient(&redislib.Options{Addr: mini.Addr(), MaxRetries: -1})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := securityredis.NewSessionStore(client)

	if err != nil {
		t.Fatal(err)
	}

	authentication, err := app.NewAuthenticationService(t.Context(), app.AuthenticationDependencies{Credentials: repository, Invalidator: repository, Roles: repository, Hasher: httpHasher{}, Revoker: store})

	if err != nil {
		t.Fatal(err)
	}

	jwtService, err := app.NewJWTAuthenticationService(authentication, app.JWTAuthenticationDependencies{Tokens: tokens, Reissuer: tokens, Sessions: store, Rotator: store}, 24*time.Hour)

	if err != nil {
		t.Fatal(err)
	}

	pair, err := jwtService.SignIn(t.Context(), input.SignInInput{Email: "person@example.com", Password: "correct"})

	if err != nil {
		t.Fatal(err)
	}

	if _, err := jwtService.Authenticate(t.Context(), pair.AccessToken); err != nil {
		t.Fatalf("fixture JWT is not valid: %v", err)
	}

	for _, tt := range []struct {
		name    string
		cookies []*http.Cookie
		bearer  string
	}{
		{name: "JWT cookies", cookies: []*http.Cookie{{Name: "__Host-faryen_access", Value: pair.AccessToken}, {Name: "__Host-faryen_refresh", Value: pair.RefreshToken}}},
		{name: "Bearer header", bearer: pair.AccessToken},
		{name: "access token in session cookie", cookies: []*http.Cookie{{Name: "__Host-faryen_session", Value: pair.AccessToken}}},
		{name: "refresh token in session cookie", cookies: []*http.Cookie{{Name: "__Host-faryen_session", Value: pair.RefreshToken}}},
		{name: "unknown session", cookies: []*http.Cookie{{Name: "__Host-faryen_session", Value: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://admin.example.com/admin/en", nil)

			for _, cookie := range tt.cookies {
				request.AddCookie(cookie)
			}

			if tt.bearer != "" {
				request.Header.Set("Authorization", "Bearer "+tt.bearer)
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en/sign-in" || strings.Contains(response.Body.String(), "First Last") {
				t.Fatalf("non-session credential accessed admin: %d", response.Code)
			}
		})
	}

	if response := httpRequest(mux, http.MethodPost, "/admin/en/refresh", "", login(t, mux)); response.Code != http.StatusNotFound {
		t.Fatalf("removed admin refresh endpoint = %d", response.Code)
	}

	if _, err := jwtService.Refresh(t.Context(), pair.RefreshToken); err != nil {
		t.Fatalf("admin requests modified the API JWT session: %v", err)
	}

	// Password revocation must also affect sessions created through the admin.
	cookies := login(t, mux)
	repository.credentials.Version = newTestUUID(t)

	if response := httpRequest(mux, http.MethodGet, "/admin/en", "", cookies); response.Code != http.StatusSeeOther {
		t.Fatalf("credential revocation did not invalidate admin: %d", response.Code)
	}
}
