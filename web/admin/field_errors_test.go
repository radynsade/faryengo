package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func assertInvalidControl(t *testing.T, body, id, errorID string) {
	t.Helper()
	pattern := regexp.MustCompile(`<(?:input|select)\b[^>]*\bid="` + regexp.QuoteMeta(id) + `"[^>]*>`)
	control := pattern.FindString(body)
	description := regexp.MustCompile(`aria-describedby="([^"]*)"`).FindStringSubmatch(control)

	if !strings.Contains(control, `aria-invalid="true"`) || len(description) != 2 || !strings.Contains(" "+description[1]+" ", " "+errorID+" ") {
		t.Fatalf("control %s is not linked to %s: %s", id, errorID, control)
	}

	if !strings.Contains(body, `id="`+errorID+`" role="alert"`) || strings.Index(body, `id="`+errorID+`"`) < strings.Index(body, `id="`+id+`"`) {
		t.Fatalf("control %s has no following error message", id)
	}
}

func TestRoleFieldErrors(t *testing.T) {
	for _, tt := range []struct {
		name, body, control, errorID, message string
		status                                int
	}{
		{name: "missing translated name", body: "permissions=view_role", control: "role-name-en", errorID: "role-name-errors", message: "Enter a name in at least one language.", status: http.StatusUnprocessableEntity},
		{name: "permissions", body: "name[en]=Kept&permissions=unknown&permissions=also_unknown", control: "role-permissions-input", errorID: "role-permissions-errors", message: "Enter a valid value.", status: http.StatusUnprocessableEntity},
		{name: "super checkbox", body: "name[en]=Kept&is_super=yes", control: "role-super", errorID: "role-super-errors", message: "Enter a valid value.", status: http.StatusBadRequest},
		{name: "inactive translation", body: url.Values{"name[en]": {"Kept"}, "name[lv]": {"\xff"}}.Encode(), control: "role-name-lv", errorID: "role-name-lv-errors", message: "Enter a valid value.", status: http.StatusUnprocessableEntity},
	} {
		for _, edit := range []bool{false, true} {
			operation := "create"

			if edit {
				operation = "edit"
			}

			t.Run(tt.name+"/"+operation, func(t *testing.T) {
				mux, repository, _ := httpFixture(t)
				cookies := login(t, mux)
				path := "/admin/en/roles/create"

				if edit {
					path = "/admin/en/roles/" + uuid.UUID(repository.role.ID()).String() + "/edit"
				}

				response := httpRequest(mux, http.MethodPost, path, tt.body, cookies)
				body := response.Body.String()

				if response.Code != tt.status || repository.roleWrites != 0 || !strings.Contains(body, tt.message) {
					t.Fatalf("field response = %d, writes = %d", response.Code, repository.roleWrites)
				}

				assertInvalidControl(t, body, tt.control, tt.errorID)

				if strings.Contains(body, `class="notice notice--error"`) || strings.Contains(body, "name: invalid value") {
					t.Fatal("field error leaked into the notification banner")
				}

				if tt.control == "role-permissions-input" {
					assertInvalidControl(t, body, "role-permissions", tt.errorID)

					if strings.Count(body, tt.message) != 1 || strings.Contains(body, "permissions[0]") || strings.Contains(body, `id="role-name-errors"`) {
						t.Fatal("permission errors were duplicated or attached to the name")
					}
				}

				if tt.control == "role-name-lv" {
					if !strings.Contains(body, `data-translations-input data-language="lv"`) || strings.Contains(body, `id="role-name-panel-lv" data-language="lv" role="tabpanel" aria-labelledby="role-name-tab-lv" hidden`) {
						t.Fatal("the invalid translation remained hidden")
					}
				}

				if strings.Contains(tt.body, "Kept") && !strings.Contains(body, `value="Kept"`) {
					t.Fatal("field validation discarded valid submitted values")
				}

				reloaded := httpRequest(mux, http.MethodGet, path, "", cookies)

				if strings.Contains(reloaded.Body.String(), `aria-invalid="true"`) || strings.Contains(reloaded.Body.String(), `id="`+tt.errorID+`"`) {
					t.Fatal("field errors persisted beyond their submission")
				}
			})
		}
	}
}

func TestFilterFieldErrors(t *testing.T) {
	for _, tt := range []struct{ query, control, errorID string }{
		{"name=" + strings.Repeat("x", 501), "role-filter-name", "role-filter-name-errors"},
		{"super=maybe", "role-filter-super", "role-filter-super-errors"},
		{"permissions=unknown", "role-filter-permissions-input", "role-filter-permissions-errors"},
	} {
		t.Run(tt.control, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)
			cookies := login(t, mux)

			for _, path := range []string{"/admin/en/roles", "/admin/en/roles/table"} {
				request := httptest.NewRequest(http.MethodGet, "https://admin.example.com"+path+"?"+tt.query, nil)
				request.Header.Set("HX-Request", "true")

				for _, cookie := range cookies {
					request.AddCookie(cookie)
				}

				response := httptest.NewRecorder()
				mux.ServeHTTP(response, request)
				body := response.Body.String()

				if response.Code != http.StatusBadRequest || repository.roleReads != 0 {
					t.Fatalf("invalid filter = %d, reads = %d", response.Code, repository.roleReads)
				}

				assertInvalidControl(t, body, tt.control, tt.errorID)

				target := "#page-content"

				if path == "/admin/en/roles/table" {
					target = "#roles-list-table"
				}

				if response.Header().Get("HX-Retarget") != target {
					t.Fatal("HTMX validation response cannot replace its target")
				}

				if strings.Contains(body, `class="notice notice--error"`) || strings.Contains(body, "No roles found") {
					t.Fatal("invalid filters rendered a banner or an empty result message")
				}
			}
		})
	}
}
