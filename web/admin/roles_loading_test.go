package admin

import (
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
)

func TestRolesLazyLoading(t *testing.T) {
	for _, tt := range []struct {
		name, language, loading string
		fragment, history       bool
	}{
		{name: "document", language: "en", loading: "Loading roles…"},
		{name: "navigation", language: "en", loading: "Loading roles…", fragment: true},
		{name: "history", language: "en", loading: "Loading roles…", fragment: true, history: true},
		{name: "Latvian", language: "lv", loading: "Ielādē lomas…"},
		{name: "Russian", language: "ru", loading: "Загрузка ролей…", fragment: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)
			cookies := login(t, mux)
			role := addHTTPRole(t, repository, "Lazy loaded role", []users.Permission{users.PermissionViewRole}, false)
			query := url.Values{"name": {"Lazy loaded"}, "permissions": {"view_role", "view_role"}, "page": {"999"}, "size": {"1"}, "sort": {"name"}, "order": {"desc"}}
			root := "/admin/" + tt.language + "/roles"
			request := httptest.NewRequest(http.MethodGet, root+"?"+query.Encode(), nil)

			if tt.fragment {
				request.Header.Set("HX-Request", "true")
			}

			if tt.history {
				request.Header.Set("HX-History-Restore-Request", "true")
			}

			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			body := response.Body.String()

			if response.Code != http.StatusOK || repository.roleReads != 0 {
				t.Fatalf("initial page = %d, list queries = %d", response.Code, repository.roleReads)
			}

			for _, markup := range []string{`id="roles-list-table"`, `aria-busy="true"`, `class="data-table__spinner"`, `role="status"`, tt.loading, `hx-trigger="load delay:100ms, htmx:afterSettle from:body delay:100ms"`, `hx-sync="body:abort"`, `hx-get="` + html.EscapeString(root+"/table?"+query.Encode()) + `"`} {
				if !strings.Contains(body, markup) {
					t.Fatalf("initial page missing %q: %s", markup, body)
				}
			}

			if strings.Contains(body, uuid.UUID(role.ID()).String()) || strings.Contains(body, `class="data-table__empty"`) || strings.Count(body, `aria-disabled="true"`) != 4 {
				t.Fatal("initial table included rows, an empty state, or enabled pagination")
			}

			request = httptest.NewRequest(http.MethodGet, root+"/table?"+query.Encode(), nil)
			request.Header.Set("HX-Request", "true")

			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}

			response = httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			body = response.Body.String()

			if response.Code != http.StatusOK || repository.roleReads != 2 || !strings.Contains(body, "Lazy loaded role") || !strings.Contains(body, `aria-busy="false"`) {
				t.Fatalf("loaded table = %d, list queries = %d: %s", response.Code, repository.roleReads, body)
			}

			for _, markup := range []string{"<!doctype", "<head", `id="page-content"`, `id="confirm-delete"`, `class="data-table__spinner"`, `hx-trigger="load`} {
				if strings.Contains(body, markup) {
					t.Fatalf("loaded table unexpectedly included %q", markup)
				}
			}

			if response.Header().Get("HX-Retarget") != "#roles-list-table" || repository.lastQuery.Page != 1 || string(repository.lastQuery.Language) != tt.language {
				t.Fatal("table target or clamped pagination is incorrect")
			}
		})
	}
}

func TestRolesLazyLoadingErrors(t *testing.T) {
	for _, tt := range []struct {
		name, query, message string
		listError            error
		status               int
		reads                int
	}{
		{name: "invalid query", query: "?page=0", message: "Check the filters, sort order, and page number.", status: http.StatusBadRequest},
		{name: "unavailable list", listError: errors.New("database unavailable"), message: "Roles are temporarily unavailable. Please try again.", status: http.StatusInternalServerError, reads: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)
			cookies := login(t, mux)
			repository.roleListErr = tt.listError
			request := httptest.NewRequest(http.MethodGet, "/admin/en/roles/table"+tt.query, nil)
			request.Header.Set("HX-Request", "true")

			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			body := response.Body.String()

			if response.Code != tt.status || repository.roleReads != tt.reads || !strings.Contains(body, tt.message) || !strings.Contains(body, `role="alert"`) || !strings.Contains(body, "Retry") {
				t.Fatalf("loading error = %d, list queries = %d: %s", response.Code, repository.roleReads, body)
			}

			if response.Header().Get("HX-Retarget") != "#roles-list-table" || response.Header().Get("HX-Reswap") != "outerHTML" || strings.Contains(body, `aria-busy="true"`) || strings.Contains(body, `id="page-content"`) || strings.Contains(body, "database unavailable") {
				t.Fatal("loading failure did not produce a safe table error fragment")
			}

			repository.roleListErr = nil
			next := httpRequest(mux, http.MethodGet, "/admin/en/roles", "", cookies)

			if strings.Contains(next.Body.String(), tt.message) {
				t.Fatal("table error was replayed on the next page")
			}
		})
	}
}
