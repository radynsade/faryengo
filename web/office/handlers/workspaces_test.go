package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/workspaces"
)

func TestSelectWorkspace(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{path: "/office/en/workspaces", cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, body)
	}

	for _, want := range []string{
		"<title>Select workspace · Faryen</title>",
		`<h1 id="page-title">Select workspace</h1>`,
		`<main id="standalone-main" class="standalone__main" tabindex="-1" aria-labelledby="page-title">`,
		`<li id="workspace-` + designID + `">`,
		`<li id="workspace-` + familyID + `">`,
		`<span class="workspace-select__name">Design</span>`,
		`<span class="workspace-select__mark" aria-hidden="true">F</span>`,
		`<a class="workspace-select__item" href="` + designPath + `"`,
		`<a class="button button--secondary" href="/office/en/workspaces/new" hx-boost="true"`,
		"Create a workspace",
		`href="/office/lv/workspaces"`,
		`<a class="dropdown__item" href="/office/en/profile-settings"`,
		`action="/office/en/sign-out"`,
		"Sign out",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	if strings.Index(body, "workspace-"+designID) > strings.Index(body, "workspace-"+familyID) {
		t.Fatal("workspaces are not in name order")
	}

	for _, unwanted := range []string{otherID, "Someone else", "sidebar", "dashboard", "workspace-select__empty"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body contains %s", unwanted)
		}
	}
}

func TestSelectWorkspaceWithoutWorkspaces(t *testing.T) {
	f := newFixture(t)
	f.members = nil
	cookies := f.signIn(t)

	body := f.do(request{path: "/office/lv/workspaces", cookies: cookies}).Body.String()

	for _, want := range []string{
		`<p class="workspace-select__empty">Jūs vēl neesat nevienas darbvietas dalībnieks.</p>`,
		"Izveidot darbvietu",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	if strings.Contains(body, "workspace-select__list") {
		t.Fatal("an empty selection renders a list")
	}
}

// A list that cannot be loaded still renders the page, as a minimized HTML
// document or partial with an error status, so the client shows it in place
// instead of a bare error, and the controls and the create button stay.

func TestSelectWorkspaceUnavailable(t *testing.T) {
	for _, tt := range []struct {
		name         string
		headers      map[string]string
		wantDocument bool
	}{
		{name: "direct load", wantDocument: true},
		{name: "in-page update", headers: partial},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			f.findErr = errors.New("storage unavailable")

			response := f.do(request{path: "/office/en/workspaces", cookies: cookies, headers: tt.headers})
			body := response.Body.String()

			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
				t.Fatalf("response = %d %q", response.Code, response.Header().Get("Content-Type"))
			}

			if isDocument := strings.HasPrefix(body, "<!doctype html>"); isDocument != tt.wantDocument {
				t.Fatalf("returned a document = %t, want %t", isDocument, tt.wantDocument)
			}

			for _, want := range []string{
				`<div class="standalone" data-navigation-error=`,
				`<div class="form-alert" role="alert">`,
				"Your workspaces could not be loaded. Please try again later.",
				`<a class="workspace-select__retry" href="/office/en/workspaces" hx-boost="true"`,
				">Try again</a>",
				"Create a workspace",
				`action="/office/en/sign-out"`,
			} {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %s:\n%s", want, body)
				}
			}

			for _, unwanted := range []string{"storage unavailable", "workspace-select__list", "workspace-select__empty", "\n", "<!--"} {
				if strings.Contains(body, unwanted) {
					t.Fatalf("body contains %q", unwanted)
				}
			}
		})
	}
}

// Leaving the dashboard for the selection, and the selection for the
// dashboard, replace the whole page region, because the layouts differ.

func TestSelectWorkspaceSwitchesLayouts(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{path: "/office/en/workspaces", cookies: cookies, headers: toContent})

	if response.Header().Get("HX-Retarget") != "#page-content" || !strings.Contains(response.Body.String(), `<div class="standalone"`) {
		t.Fatalf("from the dashboard: headers = %v", response.Header())
	}

	response = f.do(request{
		path:    designPath,
		cookies: cookies,
		headers: map[string]string{"HX-Request": "true", "HX-Target": "page-content"},
	})

	if response.Header().Get("HX-Retarget") != "" || !strings.Contains(response.Body.String(), `<div class="dashboard"`) {
		t.Fatalf("to the dashboard: headers = %v", response.Header())
	}
}

func TestNewWorkspacePage(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{path: "/office/en/workspaces/new", cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}

	for _, want := range []string{
		"<title>Create a workspace · Faryen</title>",
		`<h1 id="page-title">Create a workspace</h1>`,
		`<form id="create-workspace-form" class="form" method="post" action="/office/en/workspaces" novalidate`,
		`name="name"`,
		`<a class="form__cancel" href="/office/en/workspaces" hx-boost="true"`,
		`<div class="standalone"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	if strings.Contains(body, "sidebar") {
		t.Fatal("the creation page shows the sidebar")
	}

	for _, visit := range []request{
		{path: "/office/en/workspaces/new"},
		{method: http.MethodPost, path: "/office/en/workspaces", body: "name=Design"},
	} {
		response = f.do(visit)

		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
			t.Fatalf("%s %s as a visitor = %d %q", visit.method, visit.path, response.Code, response.Header().Get("Location"))
		}
	}
}

// A created workspace has the creator as its member and opens its overview.

func TestCreateWorkspace(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	existing := len(f.workspaces)

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/office/en/workspaces",
		body:    "name=" + url.QueryEscape("  Holiday trip  "),
		cookies: cookies,
		headers: map[string]string{"HX-Request": "true", "HX-Target": "page-content"},
	})

	if response.Code != http.StatusSeeOther || len(f.workspaces) != existing+1 {
		t.Fatalf("create = %d, workspaces %d: %s", response.Code, len(f.workspaces), response.Body.String())
	}

	created := f.workspaces[existing]
	location := "/office/en/workspaces/" + uuid.UUID(created.ID).String()

	if created.Name != "Holiday trip" || response.Header().Get("Location") != location {
		t.Fatalf("created %q, Location = %q, want %q", created.Name, response.Header().Get("Location"), location)
	}

	if members := f.members[created.ID]; len(members) != 1 || members[0] != f.user.ID {
		t.Fatalf("members = %v, want only the creator", members)
	}

	// The browser follows the redirect with the same headers, and the
	// overview replaces the whole page.
	response = f.do(request{path: location, cookies: cookies, headers: map[string]string{"HX-Request": "true", "HX-Target": "page-content"}})

	for _, want := range []string{
		`<span class="topbar__workspace-name">Holiday trip</span>`,
		`<h1 id="page-title">Overview</h1>`,
	} {
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), want) {
			t.Fatalf("overview = %d, lacks %s", response.Code, want)
		}
	}
}

func TestCreateWorkspaceRejected(t *testing.T) {
	for _, tt := range []struct {
		name      string
		path      string
		body      string
		headers   map[string]string
		createErr error
		status    int
		contains  []string
	}{
		{
			name:     "blank name",
			path:     "/office/en/workspaces",
			body:     "name=+++",
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<p class="field__error" id="workspace-name-error">Please fill in this field.</p>`, `aria-invalid="true"`},
		},
		{
			name:     "too long name in characters",
			path:     "/office/ru/workspaces",
			body:     "name=" + url.QueryEscape(strings.Repeat("ā", workspaces.MaxWorkspaceNameLength+1)),
			status:   http.StatusUnprocessableEntity,
			contains: []string{"Используйте не более 100 символов."},
		},
		{
			name:     "malformed form",
			path:     "/office/en/workspaces",
			body:     "name=Design&extra=1",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:      "unavailable storage",
			path:      "/office/en/workspaces",
			body:      "name=Design",
			createErr: errors.New("storage unavailable"),
			status:    http.StatusServiceUnavailable,
			contains:  []string{"The workspace could not be created. Please try again later.", `value="Design"`},
		},
		{
			name:    "cross-origin submission",
			path:    "/office/en/workspaces",
			body:    "name=Design",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			status:  http.StatusForbidden,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			f.createErr = tt.createErr
			existing := len(f.workspaces)

			response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body, cookies: cookies, headers: tt.headers})
			body := response.Body.String()

			if response.Code != tt.status || len(f.workspaces) != existing {
				t.Fatalf("status = %d, want %d; workspaces %d, want %d:\n%s", response.Code, tt.status, len(f.workspaces), existing, body)
			}

			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %s:\n%s", want, body)
				}
			}

			if strings.Contains(body, "storage unavailable") {
				t.Fatal("response exposes the cause")
			}
		})
	}
}

// A failed in-page submission replaces only the form, keeping the address.

func TestCreateWorkspaceReplacesOnlyTheForm(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/office/en/workspaces",
		body:    "name=",
		cookies: cookies,
		headers: map[string]string{"HX-Request": "true", "HX-Target": "page-content"},
	})
	body := response.Body.String()

	for header, want := range map[string]string{
		"HX-Retarget": "#create-workspace-form",
		"HX-Reswap":   "morph:outerHTML",
		"HX-Push-Url": "false",
	} {
		if response.Header().Get(header) != want {
			t.Fatalf("%s = %q, want %q", header, response.Header().Get(header), want)
		}
	}

	if response.Code != http.StatusUnprocessableEntity || !strings.HasPrefix(body, `<form id="create-workspace-form"`) || strings.Contains(body, `<head hx-head`) {
		t.Fatalf("response = %d, body is not only the form:\n%s", response.Code, body)
	}
}
