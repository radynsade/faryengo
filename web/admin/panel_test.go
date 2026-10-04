package admin

import (
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/security"
)

func TestAdminPanel(t *testing.T) {
	for _, tt := range []struct {
		name, language string
		fragment       bool
	}{
		{name: "document", language: "en"},
		{name: "fragment", language: "en", fragment: true},
		{name: "language path", language: "lv"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, _, _ := httpFixture(t)
			cookies := login(t, mux)
			root := "/admin/" + tt.language
			request := httptest.NewRequest(http.MethodGet, "https://admin.example.com"+root, nil)

			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}

			if tt.fragment {
				request.Header.Set("HX-Request", "true")
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			body := response.Body.String()

			if response.Code != http.StatusOK {
				t.Fatalf("panel status = %d: %s", response.Code, body)
			}

			for _, markup := range []string{
				`class="panel-layout"`, `aria-label="Admin navigation"`, "First Last",
				`href="` + root + `/users"`, `href="` + root + `/roles"`,
				`method="post" action="` + root + `/sign-out"`, "Sign out",
				`id="panel-main"`, `href="#panel-main"`, `aria-label="Admin panel"`,
			} {
				if !strings.Contains(body, markup) {
					t.Fatalf("panel is missing %q: %s", markup, body)
				}
			}

			if strings.Count(body, `class="panel-menu__link"`) != 2 || strings.Count(body, `id="page-content"`) != 1 {
				t.Fatal("panel must have two menu items and one HTMX swap target")
			}

			if strings.Contains(body, "<!doctype html>") == tt.fragment ||
				(tt.fragment && (strings.Contains(body, "<script") || strings.Contains(body, "<link"))) {
				t.Fatal("incorrect full-document or fragment response")
			}
		})
	}
}

func TestAdminPanelSections(t *testing.T) {
	for _, tt := range []struct {
		name, section string
		permissions   []security.Permission
		isSuper       bool
		anonymous     bool
		status        int
	}{
		{name: "users allowed", section: "users", permissions: []security.Permission{security.PermissionViewUser}, status: http.StatusOK},
		{name: "roles allowed", section: "roles", permissions: []security.Permission{security.PermissionViewRole}, status: http.StatusOK},
		{name: "users denied", section: "users", status: http.StatusForbidden},
		{name: "roles without role permissions", section: "roles", permissions: []security.Permission{security.PermissionViewUser}, status: http.StatusOK},
		{name: "roles without any permissions", section: "roles", status: http.StatusOK},
		{name: "manage user does not grant view", section: "users", permissions: []security.Permission{security.PermissionManageUser}, status: http.StatusForbidden},
		{name: "roles with manage only", section: "roles", permissions: []security.Permission{security.PermissionManageRole}, status: http.StatusOK},
		{name: "super users", section: "users", isSuper: true, status: http.StatusOK},
		{name: "super roles", section: "roles", isSuper: true, status: http.StatusOK},
		{name: "anonymous users", section: "users", anonymous: true, status: http.StatusSeeOther},
		{name: "anonymous roles", section: "roles", anonymous: true, status: http.StatusSeeOther},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)

			if err := repository.role.SetPermissions(tt.permissions); err != nil {
				t.Fatal(err)
			}

			repository.role.SetIsSuper(tt.isSuper)
			var cookies []*http.Cookie

			if !tt.anonymous {
				cookies = login(t, mux)
			}

			response := httpRequest(mux, http.MethodGet, "/admin/en/"+tt.section, "", cookies)
			body := response.Body.String()

			if response.Code != tt.status {
				t.Fatalf("section status = %d, want %d: %s", response.Code, tt.status, body)
			}

			if tt.anonymous {
				if response.Header().Get("Location") != "/admin/en/sign-in" || strings.Contains(body, "First Last") {
					t.Fatal("anonymous section did not redirect safely")
				}
			} else {
				if strings.Count(body, `aria-current="page"`) != 1 || !strings.Contains(body, `/`+tt.section+`" aria-current="page"`) {
					t.Fatal("section did not mark its active navigation item")
				}

				if tt.status == http.StatusForbidden && (!strings.Contains(body, "You do not have permission") || strings.Contains(body, "coming soon")) {
					t.Fatal("denied section exposed its content")
				}
			}
		})
	}
}

func TestAdminPanelCurrentName(t *testing.T) {
	mux, repository, _ := httpFixture(t)
	cookies := login(t, mux)

	for _, tt := range []struct {
		name, first, last string
	}{
		{name: "updated", first: "New", last: "Name"},
		{name: "escaped", first: "<script>alert(1)</script>", last: "O'Connor & Co"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := repository.credentials.User.SetFirstName(security.FirstName(tt.first)); err != nil {
				t.Fatal(err)
			}

			if err := repository.credentials.User.SetLastName(security.LastName(tt.last)); err != nil {
				t.Fatal(err)
			}

			response := httpRequest(mux, http.MethodGet, "/admin/en", "", cookies)
			body := response.Body.String()

			if response.Code != http.StatusOK || !strings.Contains(body, html.EscapeString(tt.first+" "+tt.last)) ||
				strings.Contains(body, "First Last") || strings.Contains(body, "<script>alert(1)</script>") {
				t.Fatalf("panel did not render the escaped current name: %s", body)
			}
		})
	}
}
