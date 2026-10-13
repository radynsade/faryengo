package handlers

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// Every failure is the office's error page in the address's language, never
// plain text.

func TestErrorPages(t *testing.T) {
	for _, tt := range []struct {
		name     string
		method   string
		path     string
		signedIn bool
		findErr  error
		headers  map[string]string
		status   int
		contains []string
		excludes []string
	}{
		{
			name:     "an unknown address",
			path:     "/office/en/nowhere/at/all",
			status:   http.StatusNotFound,
			contains: []string{`<html lang="en">`, "Page not found", "404", `href="/office/en"`, `href="/office/lv/nowhere/at/all"`},
			excludes: []string{"Try again"},
		},
		{
			name:     "an unknown address in another language",
			path:     "/office/ru/nowhere",
			status:   http.StatusNotFound,
			contains: []string{`<html lang="ru">`, "Страница не найдена", `href="/office/ru"`},
		},
		{
			name:     "an unsupported method",
			method:   http.MethodPost,
			path:     "/office/en/workspaces/new",
			status:   http.StatusNotFound,
			contains: []string{"Page not found"},
		},
		{
			name:     "a cross-origin submission",
			method:   http.MethodPost,
			path:     "/office/en/sign-in",
			headers:  map[string]string{"Sec-Fetch-Site": "cross-site"},
			status:   http.StatusForbidden,
			contains: []string{"Access denied", "This request came from another site and was blocked."},
			excludes: []string{"Try again"},
		},
		{
			name:     "an unavailable service",
			path:     designPath,
			signedIn: true,
			findErr:  errors.New("storage unavailable"),
			status:   http.StatusServiceUnavailable,
			contains: []string{"Temporarily unavailable", "Your workspaces could not be loaded.", "Try again", `href="` + designPath + `"`},
			excludes: []string{"storage unavailable"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			r := request{method: tt.method, path: tt.path, headers: tt.headers}

			if tt.signedIn {
				r.cookies = f.signIn(t)
			}

			f.findErr = tt.findErr
			response := f.do(r)
			body := response.Body.String()

			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.status, body)
			}

			if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
				t.Fatalf("Content-Type = %q", contentType)
			}

			for _, want := range append([]string{`<div class="error-page" data-navigation-error=`, `id="error-main"`}, tt.contains...) {
				if !strings.Contains(body, want) {
					t.Errorf("body lacks %q", want)
				}
			}

			for _, unwanted := range tt.excludes {
				if strings.Contains(body, unwanted) {
					t.Errorf("body contains %q", unwanted)
				}
			}
		})
	}
}

// An error inside the dashboard leaves the dashboard, so it replaces the
// whole page region; a failed submission keeps the address it was sent from.

func TestErrorPagesReplaceThePage(t *testing.T) {
	f := newFixture(t)

	response := f.do(request{path: designPath + "/missing", headers: toContent})

	if response.Header().Get("HX-Retarget") != "#page-content" || response.Header().Get("HX-Reswap") != "morph:outerHTML show:none" {
		t.Fatalf("headers = %v", response.Header())
	}

	if body := response.Body.String(); strings.HasPrefix(body, "<!doctype html>") || !strings.Contains(body, `class="error-page"`) {
		t.Fatalf("body is not the partial error page: %.120s", body)
	}

	response = f.do(request{method: http.MethodPost, path: "/office/en/nowhere", headers: partial})

	if response.Header().Get("HX-Push-Url") != "false" {
		t.Fatalf("HX-Push-Url = %q", response.Header().Get("HX-Push-Url"))
	}
}
