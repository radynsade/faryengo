package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
)

func TestPanelAuthorization(t *testing.T) {
	view := users.Permissions{users.PermissionViewRole}
	manage := users.Permissions{users.PermissionManageRole}

	tests := []struct {
		name        string
		permissions users.Permissions
		isSuper     bool
		want        map[string]int
	}{
		{"no permissions", nil, false, map[string]int{
			"GET /admin/en": 200, "GET /admin/en/users": 403, "GET /admin/en/roles": 403, "GET /admin/en/roles/table": 403,
			"GET view": 403, "GET /admin/en/roles/create": 403, "POST /admin/en/roles/create": 403, "POST edit": 403, "POST delete": 403,
		}},
		{"view roles", view, false, map[string]int{
			"GET /admin/en/users": 403, "GET /admin/en/roles": 200, "GET /admin/en/roles/table": 200, "GET view": 200,
			"GET /admin/en/roles/create": 403, "GET edit": 403, "POST /admin/en/roles/create": 403, "POST delete": 403,
		}},
		{"manage roles without viewing", manage, false, map[string]int{
			"GET /admin/en/roles": 403, "GET view": 403, "GET /admin/en/roles/create": 200, "GET edit": 200,
			"POST /admin/en/roles/create": 303, "POST edit": 303, "POST delete": 303,
		}},
		{"super role", nil, true, map[string]int{
			"GET /admin/en/users": 200, "GET /admin/en/roles": 200, "GET view": 200, "GET edit": 200,
			"POST /admin/en/roles/create": 303, "POST delete": 303,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, route := range slices.Sorted(func(yield func(string) bool) {
				for route := range tt.want {
					if !yield(route) {
						return
					}
				}
			}) {
				f := newFixture(t, tt.permissions, tt.isSuper)
				target := f.addRole(t, "Reader", users.Permissions{users.PermissionViewUser}, false)
				method, path, _ := strings.Cut(route, " ")
				base := "/admin/en/roles/" + roleID(target)
				body := ""

				switch path {
				case "view", "edit", "delete":
					path = base + "/" + path
				}

				if method == http.MethodPost {
					body = "name[en]=Changed&permissions=view_role"

					if strings.HasSuffix(path, "/delete") {
						body = "confirm=delete"
					}
				}

				response := f.do(request{method: method, path: path, body: body, cookies: f.signIn(t)})

				if response.Code != tt.want[route] {
					t.Fatalf("%s = %d, want %d: %s", route, response.Code, tt.want[route], response.Body.String())
				}

				if response.Code == http.StatusForbidden && f.writes != 0 {
					t.Fatalf("%s wrote despite being forbidden", route)
				}
			}
		})
	}
}

func TestPanelHidesActionsWithoutPermission(t *testing.T) {
	f := newFixture(t, users.Permissions{users.PermissionViewRole}, false)
	cookies := f.signIn(t)
	pages := []string{"/admin/en/roles", "/admin/en/roles/table", "/admin/en/roles/" + roleID(f.role) + "/view"}

	for _, path := range pages {
		body := f.do(request{path: path, cookies: cookies}).Body.String()

		for _, hidden := range []string{"/roles/create", "/edit", "data-confirm-delete", `href="/admin/en/users"`} {
			if strings.Contains(body, hidden) {
				t.Fatalf("GET %s shows %q to a viewer", path, hidden)
			}
		}
	}

	home := f.do(request{path: "/admin/en", cookies: cookies}).Body.String()

	if !strings.Contains(home, `href="/admin/en/roles"`) || strings.Contains(home, `href="/admin/en/users"`) {
		t.Fatal("the navigation does not follow the user's permissions")
	}
}

func TestRoleCreateAndUpdate(t *testing.T) {
	f := newFixture(t, nil, true)
	cookies := f.signIn(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/admin/en/roles/create",
		body:    "name[en]=Editor&name[lv]=Redaktors&name[ru]=&permissions=view_role&permissions=manage_role",
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || f.writes != 1 {
		t.Fatalf("create = %d, writes = %d: %s", response.Code, f.writes, response.Body.String())
	}

	var created *users.Role

	for _, role := range f.roles {
		if role.Name["en"] == "Editor" {
			created = role
		}
	}

	if created == nil {
		t.Fatal("the role was not created")
	}

	if want := (users.RoleName{"en": "Editor", "lv": "Redaktors"}); !equalNames(created.Name, want) || created.IsSuper {
		t.Fatalf("created role = %+v", created)
	}

	location := "/admin/en/roles/" + roleID(created) + "/view"

	if response.Header().Get("Location") != location {
		t.Fatalf("Location = %q, want %q", response.Header().Get("Location"), location)
	}

	view := f.do(request{path: location, cookies: cookies}).Body.String()

	if !strings.Contains(view, `Role &#34;Editor&#34; created successfully.`) && !strings.Contains(view, `Role "Editor" created successfully.`) {
		t.Fatalf("the success message is missing: %s", view)
	}

	if again := f.do(request{path: location, cookies: cookies}).Body.String(); strings.Contains(again, "created successfully") {
		t.Fatal("the success message was shown twice")
	}

	response = f.do(request{
		method:  http.MethodPost,
		path:    "/admin/en/roles/" + roleID(created) + "/edit",
		body:    "name[en]=Chief&is_super=1",
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || !f.roles[created.ID].IsSuper || len(f.roles[created.ID].Permissions) != 0 {
		t.Fatalf("update = %d, role = %+v", response.Code, f.roles[created.ID])
	}
}

func TestRoleFormValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{"no name", "name[en]=+&permissions=view_role", http.StatusUnprocessableEntity, "This translation is required."},
		{"only another language", "name[lv]=Redaktors", http.StatusUnprocessableEntity, "This translation is required."},
		{"name too long", "name[en]=" + strings.Repeat("a", users.MaxRoleNameLength+1), http.StatusUnprocessableEntity, "This value is too long."},
		{"unknown permission", "name[en]=Editor&permissions=everything", http.StatusUnprocessableEntity, "Enter a valid value."},
		{"unknown language", "name[de]=Redakteur", http.StatusBadRequest, "Submit a valid role form."},
		{"malformed checkbox", "name[en]=Editor&is_super=yes", http.StatusBadRequest, "Submit a valid role form."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, nil, true)
			response := f.do(request{
				method:  http.MethodPost,
				path:    "/admin/en/roles/create",
				body:    tt.body,
				cookies: f.signIn(t),
				headers: partial,
			})

			if response.Code != tt.wantStatus || f.writes != 0 {
				t.Fatalf("status = %d, writes = %d, want %d", response.Code, f.writes, tt.wantStatus)
			}

			if !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Fatalf("body does not contain %q: %s", tt.wantBody, response.Body.String())
			}

			if response.Header().Get("HX-Push-Url") != "false" {
				t.Fatal("a form shown again in place changes the address")
			}
		})
	}
}

func TestRoleFieldErrorsMarkControls(t *testing.T) {
	f := newFixture(t, nil, true)
	response := f.do(request{
		method:  http.MethodPost,
		path:    "/admin/en/roles/create",
		body:    "name[lv]=Redaktors",
		cookies: f.signIn(t),
	})
	body := response.Body.String()

	// The missing fallback translation is marked on the English field and tab,
	// which opens first; the Latvian value is kept.
	for _, want := range []string{
		`id="role-name-en-errors"`,
		`aria-describedby="role-name-help role-name-en-errors"`,
		`translations-input__tab translations-input__tab--invalid`,
		`data-translations-input data-language="en"`,
		`value="Redaktors"`,
		`aria-invalid="true"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body does not contain %s", want)
		}
	}

	if strings.Contains(body, "Enter a name in at least one language.") {
		t.Fatal("the form still asks for a name in any language")
	}
}

func TestRoleFormMarksTheFallbackTranslationRequired(t *testing.T) {
	f := newFixture(t, nil, true)
	body := f.do(request{path: "/admin/en/roles/create", cookies: f.signIn(t)}).Body.String()

	if !strings.Contains(body, "The translation in the fallback language (English) is required.") {
		t.Fatal("the help does not name the required fallback translation")
	}

	inputs := regexp.MustCompile(`<input [^>]*name="name\[(\w+)\]"[^>]*>`).FindAllStringSubmatch(body, -1)

	if len(inputs) != 3 {
		t.Fatalf("found %d name inputs", len(inputs))
	}

	for _, input := range inputs {
		if required := strings.Contains(input[0], " required"); required != (input[1] == "en") {
			t.Fatalf("name[%s] required = %t", input[1], required)
		}
	}
}

func TestRoleDelete(t *testing.T) {
	tests := []struct {
		name         string
		own          bool
		headers      map[string]string
		wantStatus   int
		wantLocation string
		wantHeader   string
		wantBody     string
	}{
		{"without scripts", false, nil, http.StatusSeeOther, "/admin/en/roles", "", ""},
		{"in place", false, partial, http.StatusOK, "", "/admin/en/roles", ""},
		{"assigned role without scripts", true, nil, http.StatusConflict, "", "", "data-dialog-open"},
		{"assigned role in place", true, partial, http.StatusConflict, "", "", `<dialog id="confirm-delete"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, nil, true)
			target := f.addRole(t, "Reader", nil, false)

			if tt.own {
				target = f.role
			}

			cookies := f.signIn(t)
			response := f.do(request{
				method:  http.MethodPost,
				path:    "/admin/en/roles/" + roleID(target) + "/delete",
				body:    "confirm=delete",
				cookies: cookies,
				headers: tt.headers,
			})

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}

			if location := response.Header().Get("Location"); location != tt.wantLocation {
				t.Fatalf("Location = %q, want %q", location, tt.wantLocation)
			}

			if response.Header().Get("HX-Redirect") != "" {
				t.Fatal("a deletion reloads the whole document")
			}

			if tt.wantHeader != "" {
				var location map[string]string

				must(t, json.Unmarshal([]byte(response.Header().Get("HX-Location")), &location))

				if location["path"] != tt.wantHeader || location["target"] != "#panel-main" || !strings.HasPrefix(location["swap"], "morph:") {
					t.Fatalf("HX-Location = %v", location)
				}
			}

			body := response.Body.String()

			if !strings.Contains(body, tt.wantBody) {
				t.Fatalf("body does not contain %q: %s", tt.wantBody, body)
			}

			if tt.own && !strings.Contains(body, "This role is assigned to users.") {
				t.Fatal("the conflict is not explained")
			}

			if tt.own && tt.headers != nil && strings.Contains(body, "<html") {
				t.Fatal("a failed in-place deletion returned a document")
			}

			_, exists := f.roles[target.ID]

			if exists != tt.own {
				t.Fatalf("role exists = %t, want %t", exists, tt.own)
			}

			if !tt.own {
				list := f.do(request{path: "/admin/en/roles", cookies: cookies}).Body.String()

				if !strings.Contains(list, "deleted successfully") {
					t.Fatal("the deletion was not confirmed on the next page")
				}
			}
		})
	}
}

func TestRolesList(t *testing.T) {
	f := newFixture(t, users.Permissions{users.PermissionViewRole}, false)

	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		f.addRole(t, name, nil, false)
	}

	cookies := f.signIn(t)

	t.Run("page renders without loading roles", func(t *testing.T) {
		f.lastFind = users.RoleQuery{}
		body := f.do(request{path: "/admin/en/roles?name=a&size=2", cookies: cookies}).Body.String()

		if f.lastFind.Limit != 0 {
			t.Fatal("the page queried the role list")
		}

		if !strings.Contains(body, `hx-get="/admin/en/roles/table?name=a&amp;order=asc&amp;page=1&amp;size=2&amp;sort=id"`) {
			t.Fatalf("the loading row does not request the table: %s", body)
		}
	})

	t.Run("table fragment keeps filters", func(t *testing.T) {
		response := f.do(request{path: "/admin/en/roles/table?name=a&size=2&sort=name&order=desc", cookies: cookies, headers: partial})
		body := response.Body.String()

		if response.Code != http.StatusOK || strings.Contains(body, "<head") || !strings.HasPrefix(body, `<div id="roles-list-table"`) {
			t.Fatalf("table = %d: %s", response.Code, body)
		}

		if f.lastFind.Filter.NameLike != "a" || f.lastFind.Limit != 2 || f.lastFind.SortBy != users.RoleSortName || f.lastFind.SortOrder.IsAsc() {
			t.Fatalf("query = %+v", f.lastFind)
		}

		if !strings.Contains(body, "Total found: 3") || !strings.Contains(body, "1 / 2") {
			t.Fatalf("the totals are wrong: %s", body)
		}
	})

	t.Run("invalid filter is shown beside its field", func(t *testing.T) {
		response := f.do(request{path: "/admin/en/roles?uuid=" + strings.Repeat("a", 40), cookies: cookies})

		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `id="role-filter-uuid"`) ||
			!strings.Contains(response.Body.String(), "This value is too long.") {
			t.Fatalf("invalid filter = %d: %s", response.Code, response.Body.String())
		}
	})

	t.Run("invalid paging is reported", func(t *testing.T) {
		response := f.do(request{path: "/admin/en/roles?page=0", cookies: cookies})

		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Check the filters, sort order, and page number.") {
			t.Fatalf("invalid page = %d: %s", response.Code, response.Body.String())
		}
	})

	t.Run("loading failure offers a retry", func(t *testing.T) {
		f.listErr = errors.New("connection refused")
		defer func() { f.listErr = nil }()

		response := f.do(request{path: "/admin/en/roles/table", cookies: cookies, headers: partial})

		if response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "Retry") {
			t.Fatalf("failed table = %d: %s", response.Code, response.Body.String())
		}
	})
}

func TestRoleNameLanguage(t *testing.T) {
	f := newFixture(t, nil, true)
	role := f.addRole(t, "Editor", nil, false)
	role.Name["lv"] = "Redaktors"

	cookies := f.signIn(t)

	for language, want := range map[string]string{"en": "Editor", "lv": "Redaktors", "ru": "Editor"} {
		body := f.do(request{path: "/admin/" + language + "/roles/" + roleID(role) + "/view", cookies: cookies}).Body.String()

		if !strings.Contains(body, `<h1 id="page-heading" class="page-title__heading">`+want+`</h1>`) {
			t.Fatalf("%s heading does not show %q", language, want)
		}
	}
}

func TestRoleNotFound(t *testing.T) {
	f := newFixture(t, nil, true)
	cookies := f.signIn(t)

	for path, want := range map[string]int{
		"/admin/en/roles/" + uuid.Must(uuid.NewV7()).String() + "/view": http.StatusNotFound,
		"/admin/en/roles/not-a-uuid/view":                               http.StatusBadRequest,
		"/admin/en/roles/" + uuid.Nil.String() + "/edit":                http.StatusBadRequest,
	} {
		if response := f.do(request{path: path, cookies: cookies}); response.Code != want {
			t.Fatalf("GET %s = %d, want %d", path, response.Code, want)
		}
	}
}

func equalNames(got, want users.RoleName) bool {
	equal := len(got) == len(want)

	for code, translation := range want {
		equal = equal && got[code] == translation
	}

	return equal
}

// Declared column widths keep the table's shape while it loads; the actions
// column is sized for the buttons the user may use.

func TestRolesTableDeclaresItsColumns(t *testing.T) {
	for _, tt := range []struct {
		name        string
		permissions users.Permissions
		wantManage  bool
	}{
		{"viewer", users.Permissions{users.PermissionViewRole}, false},
		{"manager", users.Permissions{users.PermissionViewRole, users.PermissionManageRole}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, tt.permissions, false)
			cookies := f.signIn(t)

			for _, path := range []string{"/admin/en/roles", "/admin/en/roles/table"} {
				body := f.do(request{path: path, cookies: cookies}).Body.String()
				table := regexp.MustCompile(`<table class="([^"]*)"`).FindStringSubmatch(body)

				if table == nil || !strings.Contains(table[1], "data-table--fixed") {
					t.Fatalf("GET %s table = %v", path, table)
				}

				if strings.Contains(table[1], "role-table--manage") != tt.wantManage {
					t.Fatalf("GET %s table classes = %q", path, table[1])
				}

				if cols := strings.Count(body, `<col class="role-table__col--`); cols != 5 {
					t.Fatalf("GET %s declares %d columns", path, cols)
				}
			}
		})
	}
}
