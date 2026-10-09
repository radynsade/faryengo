package handlers

import (
	"net/http"
	"strings"
	"testing"
)

func TestHandlerConfiguration(t *testing.T) {
	if _, err := NewHandler(nil, nil, nil, false); err != ErrHandlerConfigInvalid {
		t.Fatalf("NewHandler(nil...) = %v, want %v", err, ErrHandlerConfigInvalid)
	}

	if err := (&Handler{}).RegisterHandlers(nil); err != ErrServeMuxNil {
		t.Fatalf("RegisterHandlers(nil) = %v, want %v", err, ErrServeMuxNil)
	}
}

func TestPages(t *testing.T) {
	f := newFixture(t)
	signedIn := f.signIn(t)

	for _, tt := range []struct {
		name     string
		path     string
		cookies  []*http.Cookie
		status   int
		location string
		language string
		contains []string
	}{
		{
			name:     "redirects the root to the default language",
			path:     "/office",
			status:   http.StatusSeeOther,
			location: "/office/en",
		},
		{
			name:     "sends visitors to the sign-in page",
			path:     "/office/en/budgets",
			status:   http.StatusSeeOther,
			location: "/office/en/sign-in",
		},
		{
			name:     "renders the sign-in page",
			path:     "/office/en/sign-in",
			status:   http.StatusOK,
			language: "en",
			contains: []string{
				`<html lang="en">`,
				`action="/office/en/sign-in"`,
				`autocomplete="current-password"`,
				`href="/office/lv/sign-in"`,
				"Welcome back",
			},
		},
		{
			name:     "translates the sign-in page",
			path:     "/office/ru/sign-in",
			status:   http.StatusOK,
			language: "ru",
			contains: []string{`<html lang="ru">`, "С возвращением", `action="/office/ru/sign-in"`},
		},
		{
			name:     "falls back to English for an unknown language",
			path:     "/office/xx/sign-in",
			status:   http.StatusOK,
			language: "en",
			contains: []string{`<html lang="en">`, "Welcome back"},
		},
		{
			name:     "sends a signed-in user from the sign-in page to the dashboard",
			path:     "/office/lv/sign-in",
			cookies:  signedIn,
			status:   http.StatusSeeOther,
			location: "/office/lv",
		},
		{
			name:     "renders the overview with the user's profile",
			path:     "/office/en",
			cookies:  signedIn,
			status:   http.StatusOK,
			language: "en",
			contains: []string{
				"<title>Overview · Faryen</title>",
				`<a class="sidebar__link" href="/office/en" aria-current="page"`,
				`href="/office/en/budgets"`,
				`action="/office/en/sign-out"`,
				"Ada Lovelace",
				testEmail,
				`<span class="avatar__initials" aria-hidden="true">AL</span>`,
			},
		},
		{
			name:     "marks the settings button on the settings page",
			path:     "/office/lv/settings",
			cookies:  signedIn,
			status:   http.StatusOK,
			language: "lv",
			contains: []string{
				`<h1 id="page-title">Iestatījumi</h1>`,
				`href="/office/lv/settings" aria-label="Iestatījumi" title="Iestatījumi" aria-current="page"`,
				`href="/office/ru/settings"`,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := f.do(request{path: tt.path, cookies: tt.cookies})

			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d", response.Code, tt.status)
			}

			if location := response.Header().Get("Location"); location != tt.location {
				t.Fatalf("Location = %q, want %q", location, tt.location)
			}

			if tt.language != "" && response.Header().Get("Content-Language") != tt.language {
				t.Fatalf("Content-Language = %q, want %q", response.Header().Get("Content-Language"), tt.language)
			}

			for _, fragment := range tt.contains {
				if !strings.Contains(response.Body.String(), fragment) {
					t.Fatalf("body lacks %q:\n%s", fragment, response.Body.String())
				}
			}
		})
	}
}
