package handlers

import (
	"net/http"
	"strings"
	"testing"
)

func TestHandlerConfiguration(t *testing.T) {
	if _, err := NewHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, false); err != ErrHandlerConfigInvalid {
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
		excludes []string
	}{
		{
			name:     "redirects the root to choosing a workspace in the default language",
			path:     "/office",
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
		{
			name:     "redirects a language root to choosing a workspace",
			path:     "/office/lv",
			status:   http.StatusSeeOther,
			location: "/office/lv/workspaces",
		},
		{
			name:     "sends visitors to the sign-in page",
			path:     designPath + "/accounts",
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
				`<nav class="language-switcher" aria-label="Language">`,
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
			name:     "sends a signed-in user from the sign-in page to the workspace selection",
			path:     "/office/lv/sign-in",
			cookies:  signedIn,
			status:   http.StatusSeeOther,
			location: "/office/lv/workspaces",
		},
		{
			name:     "sends visitors from the workspace selection to the sign-in page",
			path:     "/office/en/workspaces",
			status:   http.StatusSeeOther,
			location: "/office/en/sign-in",
		},
		{
			name:     "renders the workspace overview with the user's profile",
			path:     designPath,
			cookies:  signedIn,
			status:   http.StatusOK,
			language: "en",
			contains: []string{
				"<title>Overview · Faryen</title>",
				`<a class="sidebar__link" href="` + designPath + `" aria-current="page"`,
				`href="` + designPath + `/accounts"`,
				`<span class="topbar__workspace-name">Design</span>`,
				`<main id="dashboard-main" class="dashboard__main" tabindex="-1"`,
				`href="/office/en/workspaces"`,
				`action="/office/en/sign-out"`,
				"Ada Lovelace",
				testEmail,
				`<span class="avatar__initials" aria-hidden="true">AL</span>`,
			},
		},
		{
			name:     "shows a workspace's settings with its sidebar",
			path:     "/office/lv/workspaces/" + designID + "/settings",
			cookies:  signedIn,
			status:   http.StatusOK,
			language: "lv",
			contains: []string{
				`<h1 id="page-title">Darbvietas iestatījumi</h1>`,
				`<aside id="dashboard-sidebar" class="sidebar">`,
				`<a class="sidebar__link" href="/office/lv/workspaces/` + designID + `/settings" aria-current="page"`,
				`<span>Iestatījumi</span>`,
				`<a class="dropdown__item" href="/office/lv/workspaces/` + designID + `/profile-settings" hx-boost`,
				`href="/office/ru/workspaces/` + designID + `/settings"`,
			},
		},
		{
			name:     "shows the settings without a workspace and without the sidebar",
			path:     "/office/en/profile-settings",
			cookies:  signedIn,
			status:   http.StatusOK,
			language: "en",
			contains: []string{
				`<div class="standalone"`,
				`<h1 id="page-title">Settings</h1>`,
				`<a class="dropdown__item" href="/office/en/profile-settings" aria-current="page"`,
				`href="/office/lv/profile-settings"`,
			},
			excludes: []string{"sidebar", "dashboard"},
		},
		{
			name:     "sends visitors from the settings to the sign-in page",
			path:     "/office/en/profile-settings",
			status:   http.StatusSeeOther,
			location: "/office/en/sign-in",
		},
		{
			name:     "sends a user back to choosing from a workspace they do not belong to",
			path:     "/office/en/workspaces/" + otherID,
			cookies:  signedIn,
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
		{
			name:     "sends a user back to choosing from a missing workspace",
			path:     "/office/en/workspaces/01920000-0000-7000-8000-000000000000",
			cookies:  signedIn,
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
		{
			name:     "sends a user back to choosing from a malformed workspace ID",
			path:     "/office/en/workspaces/not-an-id",
			cookies:  signedIn,
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
		{
			name:     "sends a user back to choosing from the nil workspace ID",
			path:     "/office/en/workspaces/00000000-0000-0000-0000-000000000000",
			cookies:  signedIn,
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
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

			for _, fragment := range tt.excludes {
				if strings.Contains(response.Body.String(), fragment) {
					t.Fatalf("body contains %q", fragment)
				}
			}
		})
	}
}

// Sections without content yet are neither in the menu nor served.

func TestUnimplementedSectionsAbsent(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	menu := f.do(request{path: designPath, cookies: cookies}).Body.String()

	for _, section := range []string{"budgets", "transactions", "goals", "reports"} {
		if strings.Contains(menu, `href="`+designPath+"/"+section+`"`) {
			t.Fatalf("menu leads to %s:\n%s", section, menu)
		}

		if response := f.do(request{path: designPath + "/" + section, cookies: cookies}); response.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want %d", section, response.Code, http.StatusNotFound)
		}
	}
}
