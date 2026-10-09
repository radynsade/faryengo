package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/users"
)

func TestSignIn(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		contentType  string
		wantStatus   int
		wantLocation string
		wantBody     string
	}{
		{"valid credentials", "email=ada@example.com&password=correct+horse", "", http.StatusSeeOther, "/admin/en", ""},
		{"wrong password", "email=ada@example.com&password=wrong", "", http.StatusUnauthorized, "", "The email, password, or session is invalid."},
		{"unknown email", "email=bob@example.com&password=correct+horse", "", http.StatusUnauthorized, "", "The email, password, or session is invalid."},
		{"invalid email", "email=not-an-email&password=secret", "", http.StatusUnprocessableEntity, "", "Enter a valid email address."},
		{"missing password", "email=ada@example.com", "", http.StatusUnprocessableEntity, "", "This field is required."},
		{"unknown field", "email=ada@example.com&password=x&role=super", "", http.StatusBadRequest, "", "Enter a valid email address and password."},
		{"repeated field", "email=ada@example.com&email=b@example.com&password=x", "", http.StatusBadRequest, "", "Enter a valid email address and password."},
		{"wrong content type", "email=ada@example.com&password=x", "text/plain", http.StatusBadRequest, "", "Enter a valid email address and password."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, nil, false)
			headers := map[string]string{}

			if tt.contentType != "" {
				headers["Content-Type"] = tt.contentType
			}

			response := f.do(request{method: http.MethodPost, path: "/admin/en/sign-in", body: tt.body, headers: headers})

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}

			if location := response.Header().Get("Location"); location != tt.wantLocation {
				t.Fatalf("Location = %q, want %q", location, tt.wantLocation)
			}

			if !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Fatalf("body does not contain %q: %s", tt.wantBody, response.Body.String())
			}

			if strings.Contains(response.Body.String(), "correct horse") || strings.Contains(response.Body.String(), `value="x"`) {
				t.Fatal("the submitted password was rendered")
			}
		})
	}
}

func TestSignInSetsOpaqueSessionCookie(t *testing.T) {
	f := newFixture(t, nil, false)
	cookies := f.signIn(t)

	if len(cookies) != 1 || cookies[0].Name != "faryen_session" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookies = %+v", cookies)
	}

	if strings.Contains(cookies[0].Value, string(f.user.Email)) {
		t.Fatal("the session cookie carries account details")
	}
}

func TestAuthenticationRequired(t *testing.T) {
	tests := []struct {
		name        string
		cookies     func(t *testing.T, f *fixture) []*http.Cookie
		wantCleared bool
	}{
		{"no cookie", func(*testing.T, *fixture) []*http.Cookie { return nil }, false},
		{"malformed cookie", func(*testing.T, *fixture) []*http.Cookie {
			return []*http.Cookie{{Name: "faryen_session", Value: "forged"}}
		}, true},
		{"revoked session", func(t *testing.T, f *fixture) []*http.Cookie {
			cookies := f.signIn(t)
			must(t, f.credentials().RotateVersion(t.Context(), f.user.ID))

			return cookies
		}, true},
		{"ended session", func(t *testing.T, f *fixture) []*http.Cookie {
			cookies := f.signIn(t)
			f.redis.FlushAll()

			return cookies
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, users.AllPermissions(), false)
			cookies := tt.cookies(t, f)

			for _, path := range []string{"/admin/en", "/admin/en/roles", "/admin/en/roles/create"} {
				response := f.do(request{path: path, cookies: cookies})

				if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en/sign-in" {
					t.Fatalf("GET %s = %d, Location %q", path, response.Code, response.Header().Get("Location"))
				}

				cleared := strings.Contains(response.Header().Get("Set-Cookie"), "faryen_session=;")

				if cleared != tt.wantCleared {
					t.Fatalf("GET %s cleared the cookie = %t, want %t", path, cleared, tt.wantCleared)
				}
			}

			response := f.do(request{method: http.MethodPost, path: "/admin/en/roles/create", body: "name[en]=Changed", cookies: cookies})

			if response.Code != http.StatusSeeOther || f.writes != 0 {
				t.Fatalf("POST without a session = %d, writes = %d", response.Code, f.writes)
			}
		})
	}
}

func TestSignInPageRedirectsSignedInUser(t *testing.T) {
	f := newFixture(t, nil, false)
	response := f.do(request{path: "/admin/en/sign-in", cookies: f.signIn(t)})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en" {
		t.Fatalf("GET sign-in = %d, Location %q", response.Code, response.Header().Get("Location"))
	}
}

func TestSignOut(t *testing.T) {
	f := newFixture(t, nil, false)
	cookies := f.signIn(t)
	response := f.do(request{method: http.MethodPost, path: "/admin/en/sign-out", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en/sign-in" {
		t.Fatalf("sign out = %d, Location %q", response.Code, response.Header().Get("Location"))
	}

	if !strings.Contains(response.Header().Get("Set-Cookie"), "faryen_session=;") {
		t.Fatal("sign out kept the session cookie")
	}

	if response := f.do(request{path: "/admin/en", cookies: cookies}); response.Code != http.StatusSeeOther {
		t.Fatalf("the ended session still authenticates: %d", response.Code)
	}
}

func TestCrossOriginRequestsRejected(t *testing.T) {
	f := newFixture(t, users.AllPermissions(), false)
	cookies := f.signIn(t)

	for _, language := range []string{"en", "lv"} {
		response := f.do(request{
			method:  http.MethodPost,
			path:    "/admin/" + language + "/roles/create",
			body:    "name[en]=Changed",
			cookies: cookies,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
		})

		if response.Code != http.StatusForbidden || f.writes != 0 {
			t.Fatalf("%s cross-origin POST = %d, writes = %d", language, response.Code, f.writes)
		}

		if language == "lv" && strings.Contains(response.Body.String(), "This request was blocked") {
			t.Fatal("the rejection was not localized")
		}
	}
}
