package handlers

import (
	"net/http"
	"strings"
	"testing"
)

func TestSignIn(t *testing.T) {
	for _, tt := range []struct {
		name     string
		path     string
		body     string
		headers  map[string]string
		status   int
		location string
		cookie   bool
		contains []string
		excludes []string
	}{
		{
			name:     "signs in and opens the dashboard in the same language",
			path:     "/office/ru/sign-in",
			body:     "email=" + testEmail + "&password=correct+horse",
			status:   http.StatusSeeOther,
			location: "/office/ru",
			cookie:   true,
		},
		{
			name:     "rejects a wrong password without telling which part was wrong",
			path:     "/office/en/sign-in",
			body:     "email=" + testEmail + "&password=wrong+horse",
			status:   http.StatusUnauthorized,
			contains: []string{"The email or password is incorrect.", `value="` + testEmail + `"`},
			excludes: []string{"wrong horse"},
		},
		{
			name:     "rejects an unknown email with the same message",
			path:     "/office/en/sign-in",
			body:     "email=nobody@example.com&password=correct+horse",
			status:   http.StatusUnauthorized,
			contains: []string{"The email or password is incorrect."},
		},
		{
			name:   "shows field errors for empty fields",
			path:   "/office/lv/sign-in",
			body:   "email=&password=",
			status: http.StatusUnprocessableEntity,
			contains: []string{
				`<p class="field__error" id="email-error">Lūdzu, aizpildiet šo lauku.</p>`,
				`<p class="field__error" id="password-error">Lūdzu, aizpildiet šo lauku.</p>`,
				`aria-invalid="true"`,
			},
		},
		{
			name:     "shows a field error for an invalid email",
			path:     "/office/en/sign-in",
			body:     "email=not-an-email&password=x",
			status:   http.StatusUnprocessableEntity,
			contains: []string{"Enter a valid email address.", `value="not-an-email"`},
		},
		{
			name:     "rejects a malformed form",
			path:     "/office/en/sign-in",
			body:     "email=" + testEmail + "&password=x&extra=1",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:    "rejects a cross-origin submission",
			path:    "/office/en/sign-in",
			body:    "email=" + testEmail + "&password=correct+horse",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			status:  http.StatusForbidden,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body, headers: tt.headers})

			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.status, response.Body.String())
			}

			if location := response.Header().Get("Location"); location != tt.location {
				t.Fatalf("Location = %q, want %q", location, tt.location)
			}

			hasCookie := false

			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == "faryen_office_session" && cookie.Value != "" {
					hasCookie = true
				}
			}

			if hasCookie != tt.cookie {
				t.Fatalf("session cookie set = %v, want %v", hasCookie, tt.cookie)
			}

			for _, fragment := range tt.contains {
				if !strings.Contains(response.Body.String(), fragment) {
					t.Fatalf("body lacks %q:\n%s", fragment, response.Body.String())
				}
			}

			for _, fragment := range tt.excludes {
				if strings.Contains(response.Body.String(), fragment) {
					t.Fatalf("body contains %q", fragment)
				}
			}
		})
	}
}

func TestSignOut(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{method: http.MethodPost, path: "/office/lv/sign-out", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/lv/sign-in" {
		t.Fatalf("sign out = %d %q", response.Code, response.Header().Get("Location"))
	}

	// The ended Session no longer opens the dashboard, even with the old cookie.
	response = f.do(request{path: "/office/lv", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/lv/sign-in" {
		t.Fatalf("dashboard after sign out = %d %q", response.Code, response.Header().Get("Location"))
	}
}
