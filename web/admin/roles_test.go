package admin

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

func addHTTPRole(t *testing.T, repository *httpCredentials, name string, permissions []security.Permission, super bool) *security.Role {
	t.Helper()
	translation, err := languages.NewTranslation("en", name)

	if err != nil {
		t.Fatal(err)
	}

	role, err := security.NewRole(security.RoleID(uuid.New()), languages.Text{"en": translation}, permissions)

	if err != nil {
		t.Fatal(err)
	}

	role.SetIsSuper(super)
	repository.otherRoles = append(repository.otherRoles, role)
	return role
}

func TestRolesCRUD(t *testing.T) {
	mux, repository, _ := httpFixture(t)
	cookies := login(t, mux)
	root := "/admin/lv/roles"
	create := httpRequest(mux, http.MethodGet, root+"/create", "", cookies)

	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), `name="name[en]"`) || !strings.Contains(create.Body.String(), `name="name[lv]"`) {
		t.Fatalf("create form = %d %s", create.Code, create.Body.String())
	}

	values := url.Values{"name[en]": {"Editors"}, "name[lv]": {"Redaktori"}, "permissions": {"view_role"}}
	response := httpRequest(mux, http.MethodPost, root+"/create", values.Encode(), cookies)

	if response.Code != http.StatusSeeOther || len(repository.otherRoles) != 1 || repository.roleWrites != 1 {
		t.Fatalf("create = %d %s", response.Code, response.Body.String())
	}

	role := repository.otherRoles[0]
	path := root + "/" + uuid.UUID(role.ID()).String()

	if response.Header().Get("Location") != path+"/view" || role.Name()["lv"].Content() != "Redaktori" {
		t.Fatal("creation did not persist translations or retain the language path")
	}

	view := httpRequest(mux, http.MethodGet, response.Header().Get("Location"), "", cookies)

	if view.Code != http.StatusOK || !strings.Contains(view.Body.String(), "created successfully.") || !strings.Contains(view.Body.String(), "Redaktori") {
		t.Fatalf("view = %d %s", view.Code, view.Body.String())
	}

	edit := httpRequest(mux, http.MethodGet, path+"/edit", "", cookies)

	if edit.Code != http.StatusOK || !strings.Contains(edit.Body.String(), `value="Editors"`) || !strings.Contains(edit.Body.String(), `value="Redaktori"`) {
		t.Fatalf("edit = %d %s", edit.Code, edit.Body.String())
	}

	values = url.Values{"name[en]": {"Reviewers"}, "name[lv]": {""}}
	response = httpRequest(mux, http.MethodPost, path+"/edit", values.Encode(), cookies)
	updated := repository.otherRoles[0]

	if response.Code != http.StatusSeeOther || updated.Name()["en"].Content() != "Reviewers" || len(updated.Name()) != 1 || len(updated.Permissions()) != 0 || updated.IsSuper() {
		t.Fatalf("update = %d %s, role = %+v", response.Code, response.Body.String(), updated)
	}

	confirm := httpRequest(mux, http.MethodGet, path+"/delete", "", cookies)

	if confirm.Code != http.StatusMethodNotAllowed || repository.roleWrites != 2 {
		t.Fatal("GET deletion must be unavailable and must not change roles")
	}

	repository.deleteErr = security.ErrRoleAlreadyInUse
	response = httpRequest(mux, http.MethodPost, path+"/delete", "confirm=delete", cookies)

	if response.Code != http.StatusConflict || len(repository.otherRoles) != 1 || !strings.Contains(response.Body.String(), "Reassign those users") {
		t.Fatalf("assigned delete = %d %s", response.Code, response.Body.String())
	}

	repository.deleteErr = nil
	response = httpRequest(mux, http.MethodPost, path+"/delete", "confirm=delete", cookies)

	if response.Code != http.StatusSeeOther || len(repository.otherRoles) != 0 || response.Header().Get("Location") != root {
		t.Fatalf("delete = %d %s", response.Code, response.Body.String())
	}

	missing := httpRequest(mux, http.MethodGet, path+"/view", "", cookies)

	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted role status = %d", missing.Code)
	}
}

func TestRoleSuccessDialog(t *testing.T) {
	name := `Review "Pārskatītāji" & <script>alert(1)</script>`

	for _, tt := range []struct {
		name, action, query string
		view                bool
	}{
		{name: "created", action: "created", view: true},
		{name: "updated", action: "updated", view: true},
		{name: "deleted", action: "deleted"},
		{name: "ordinary list"},
		{name: "ordinary view", view: true},
		{name: "forged success", view: true, query: "?notice=created"},
		{name: "forged deletion", query: "?notice=deleted&notice_name=Forged"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, mode := range []string{"document", "fragment"} {
				t.Run(mode, func(t *testing.T) {
					mux, repository, _ := httpFixture(t)
					role := addHTTPRole(t, repository, name, nil, false)
					path := "/admin/en/roles"
					rolePath := path + "/" + uuid.UUID(role.ID()).String()
					cookies := login(t, mux)
					message := ""

					if tt.action != "" {
						postPath := rolePath + "/edit"
						body := url.Values{"name[en]": {name}}.Encode()

						switch tt.action {
						case "created":
							postPath = path + "/create"
						case "deleted":
							postPath, body = rolePath+"/delete", "confirm=delete"
						}

						posted := httpRequest(mux, http.MethodPost, postPath, body, cookies)
						path = posted.Header().Get("Location")

						if posted.Code != http.StatusSeeOther || strings.Contains(path, "?") {
							t.Fatalf("flash redirect = %d %q", posted.Code, path)
						}

						message = `Role "` + name + `" ` + tt.action + " successfully."
					} else if tt.view {
						path = rolePath + "/view"
					}

					for render := range 2 {
						request := httptest.NewRequest(http.MethodGet, path+tt.query, nil)

						if mode == "fragment" {
							request.Header.Set("HX-Request", "true")
						}

						for _, cookie := range cookies {
							request.AddCookie(cookie)
						}

						response := httptest.NewRecorder()
						mux.ServeHTTP(response, request)
						body := response.Body.String()

						if response.Code != http.StatusOK || strings.Contains(body, "<script>alert(1)</script>") {
							t.Fatalf("success page = %d %s", response.Code, body)
						}

						if render > 0 || message == "" {
							if strings.Contains(body, `id="success-dialog"`) {
								t.Fatal("navigation replayed or forged a success dialog")
							}
						} else if !strings.Contains(body, `id="success-dialog"`) || !strings.Contains(body, "data-dialog-open data-success-dialog") || !strings.Contains(body, html.EscapeString(message)) {
							t.Fatalf("success dialog missing or message not escaped: %s", body)
						}
					}
				})
			}
		})
	}
}

func TestRolesAuthenticatedAccess(t *testing.T) {
	for _, tt := range []struct {
		name                               string
		permissions                        []security.Permission
		super, targetSuper, own, anonymous bool
	}{
		{name: "no permissions"},
		{name: "viewer", permissions: []security.Permission{security.PermissionViewRole}},
		{name: "manage without view", permissions: []security.Permission{security.PermissionManageRole}},
		{name: "manager", permissions: []security.Permission{security.PermissionManageRole, security.PermissionViewRole}},
		{name: "super", super: true, targetSuper: true},
		{name: "own role", own: true},
		{name: "super target", targetSuper: true},
		{name: "anonymous", anonymous: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)

			if err := repository.role.SetPermissions(tt.permissions); err != nil {
				t.Fatal(err)
			}

			repository.role.SetIsSuper(tt.super)
			role := addHTTPRole(t, repository, "Reader", []security.Permission{security.PermissionManageUser}, tt.targetSuper)

			if tt.own {
				role = repository.role
			}

			var cookies []*http.Cookie
			status := http.StatusOK

			if tt.anonymous {
				status = http.StatusSeeOther
			} else {
				cookies = login(t, mux)
			}

			root := "/admin/en/roles"
			path := root + "/" + uuid.UUID(role.ID()).String()

			for _, route := range []string{root, root + "/create", path + "/view", path + "/edit"} {
				response := httpRequest(mux, http.MethodGet, route, "", cookies)

				if response.Code != status {
					t.Fatalf("GET %s = %d, want %d: %s", route, response.Code, status, response.Body.String())
				}

				if !tt.anonymous && (route == root+"/create" || route == path+"/edit") {
					if !strings.Contains(response.Body.String(), `name="is_super"`) || strings.Contains(response.Body.String(), "disabled") {
						t.Fatal("role form restricted the super flag or permissions")
					}
				}

				if !tt.anonymous && (route == root || route == path+"/view") {
					if !strings.Contains(response.Body.String(), `href="`+path+`/edit"`) || !strings.Contains(response.Body.String(), `data-confirm-action="`+path+`/delete"`) {
						t.Fatal("role page hid the edit or delete action")
					}
				}
			}

			if tt.anonymous {
				for _, route := range []string{root + "/create", path + "/edit", path + "/delete"} {
					response := httpRequest(mux, http.MethodPost, route, "name[en]=Changed&confirm=delete", cookies)

					if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en/sign-in" || repository.roleWrites != 0 {
						t.Fatalf("anonymous POST %s = %d, writes = %d", route, response.Code, repository.roleWrites)
					}
				}
			}
		})
	}
}

func TestRoleDeleteModal(t *testing.T) {
	for _, tt := range []struct {
		name        string
		body        string
		roleMissing bool
		deleteErr   error
		status      int
		message     string
	}{
		{name: "confirmed", body: "confirm=delete", status: http.StatusOK},
		{name: "missing confirmation", body: "", status: http.StatusBadRequest, message: "Submit a valid role form"},
		{name: "wrong confirmation", body: "confirm=other", status: http.StatusBadRequest, message: "Submit a valid role form"},
		{name: "duplicate confirmation", body: "confirm=delete&confirm=delete", status: http.StatusBadRequest, message: "Submit a valid role form"},
		{name: "assigned role", body: "confirm=delete", deleteErr: security.ErrRoleAlreadyInUse, status: http.StatusConflict, message: "Reassign those users"},
		{name: "missing role", body: "confirm=delete", roleMissing: true, status: http.StatusNotFound, message: "Role not found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)
			role := addHTTPRole(t, repository, "Review <script>alert(1)</script>", nil, false)
			path := "/admin/en/roles/" + uuid.UUID(role.ID()).String()
			cookies := login(t, mux)

			for _, route := range []string{"/admin/en/roles", path + "/view"} {
				response := httpRequest(mux, http.MethodGet, route, "", cookies)
				body := response.Body.String()

				if response.Code != http.StatusOK || !strings.Contains(body, `<dialog id="confirm-delete"`) || !strings.Contains(body, `data-confirm-action="`+path+`/delete"`) || !strings.Contains(body, html.EscapeString(role.Name()["en"].Content())) || strings.Contains(body, `href="`+path+`/delete"`) {
					t.Fatalf("modal page %s = %d %s", route, response.Code, body)
				}
			}

			if tt.roleMissing {
				repository.otherRoles = nil
			}

			repository.deleteErr = tt.deleteErr
			request := httptest.NewRequest(http.MethodPost, "https://admin.example.com"+path+"/delete", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("HX-Request", "true")

			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != tt.status {
				t.Fatalf("delete = %d %s", response.Code, response.Body.String())
			}

			if tt.status == http.StatusOK {
				location := "/admin/en/roles"

				if response.Header().Get("HX-Redirect") != location || len(repository.otherRoles) != 0 || repository.roleWrites != 1 {
					t.Fatal("confirmed deletion did not delete the role and return to the list")
				}

				response = httpRequest(mux, http.MethodGet, location, "", cookies)

				if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `id="success-dialog"`) || !strings.Contains(response.Body.String(), html.EscapeString(role.Name()["en"].Content())) {
					t.Fatal("deleted role name did not reach the success dialog")
				}
			} else {
				body := response.Body.String()

				if response.Header().Get("HX-Retarget") != "#confirm-delete" || !strings.Contains(body, "data-dialog-open") || !strings.Contains(body, tt.message) || strings.Contains(body, `id="page-content"`) || strings.Contains(body, "<script>alert(1)</script>") {
					t.Fatalf("delete error did not stay in the modal: %s", body)
				}

				if !tt.roleMissing && len(repository.otherRoles) != 1 {
					t.Fatal("failed deletion removed the role")
				}
			}
		})
	}
}

func TestRoleMutationsWithoutAuthorization(t *testing.T) {
	for _, tt := range []struct {
		name             string
		permissions      []security.Permission
		own, targetSuper bool
	}{
		{name: "no permissions"},
		{name: "view only", permissions: []security.Permission{security.PermissionViewRole}},
		{name: "manage only", permissions: []security.Permission{security.PermissionManageRole}},
		{name: "own role", own: true},
		{name: "super target", targetSuper: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, action := range []string{"create", "edit", "delete"} {
				t.Run(action, func(t *testing.T) {
					mux, repository, _ := httpFixture(t)

					if err := repository.role.SetPermissions(tt.permissions); err != nil {
						t.Fatal(err)
					}

					role := repository.role

					if !tt.own {
						role = addHTTPRole(t, repository, "Stronger role", []security.Permission{security.PermissionManageUser}, tt.targetSuper)
					}

					path := "/admin/en/roles/create"
					body := "name[en]=Changed&permissions=manage_user&is_super=1"
					status := http.StatusSeeOther

					if action != "create" {
						path = "/admin/en/roles/" + uuid.UUID(role.ID()).String() + "/" + action
					}

					if action == "delete" {
						body = "confirm=delete"

						if tt.own {
							status = http.StatusConflict
						}
					}

					response := httpRequest(mux, http.MethodPost, path, body, login(t, mux))

					if response.Code != status || repository.roleWrites != 1 {
						t.Fatalf("POST %s = %d, writes = %d: %s", action, response.Code, repository.roleWrites, response.Body.String())
					}

					if action == "create" || action == "edit" {
						written := repository.role

						if action == "create" || !tt.own {
							written = repository.otherRoles[len(repository.otherRoles)-1]
						}

						if written.Name()["en"].Content() != "Changed" || !written.IsSuper() || !slices.Equal(written.Permissions(), []security.Permission{security.PermissionManageUser}) {
							t.Fatal("common role service did not save all submitted values")
						}
					} else if !tt.own && len(repository.otherRoles) != 0 {
						t.Fatal("common role service did not delete the role")
					}
				})
			}
		})
	}
}

func TestRoleFormValidation(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{name: "missing name", body: "permissions=view_role", status: http.StatusUnprocessableEntity},
		{name: "bad permission", body: "name[en]=Kept&permissions=unknown", status: http.StatusUnprocessableEntity},
		{name: "unknown language", body: "name[zz]=Unknown", status: http.StatusBadRequest},
		{name: "duplicate name", body: "name[en]=One&name[en]=Two", status: http.StatusBadRequest},
		{name: "invalid super", body: "name[en]=Kept&is_super=yes", status: http.StatusBadRequest},
		{name: "oversized", body: "name[en]=" + strings.Repeat("x", 65537), status: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)
			response := httpRequest(mux, http.MethodPost, "/admin/en/roles/create", tt.body, login(t, mux))

			if response.Code != tt.status || repository.roleWrites != 0 {
				t.Fatalf("form = %d, writes = %d: %s", response.Code, repository.roleWrites, response.Body.String())
			}

			if strings.Contains(tt.body, "Kept") && !strings.Contains(response.Body.String(), `value="Kept"`) {
				t.Fatal("invalid form lost the submitted name")
			}
		})
	}
}

func TestRolesFiltersAndFragments(t *testing.T) {
	mux, repository, _ := httpFixture(t)
	cookies := login(t, mux)
	role := addHTTPRole(t, repository, "Review <script>alert(1)</script>", []security.Permission{security.PermissionViewRole}, false)
	name := role.Name()
	translation, err := languages.NewTranslation("lv", "Pārskatītāji")

	if err != nil {
		t.Fatal(err)
	}

	name["lv"] = translation

	if err := role.SetName(name); err != nil {
		t.Fatal(err)
	}

	query := url.Values{"name": {"Pārskat"}, "super": {"false"}, "permissions": {"view_role"}, "sort": {"name"}, "order": {"desc"}, "page": {"999"}, "size": {"1"}}
	request := httptest.NewRequest(http.MethodGet, "https://admin.example.com/admin/lv/roles?"+query.Encode(), nil)
	request.Header.Set("HX-Request", "true")

	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	body := response.Body.String()

	if response.Code != http.StatusOK || strings.Contains(body, "<!doctype") || !strings.Contains(body, "Pārskatītāji") || !strings.Contains(body, "Total found: 1") || !strings.Contains(body, `aria-sort="descending"`) {
		t.Fatalf("filter fragment = %d %s", response.Code, body)
	}

	if repository.lastQuery.Page != 1 || repository.lastQuery.PageSize != 1 || repository.lastQuery.Language != "lv" || repository.lastQuery.Sort != security.RoleSortName || !repository.lastQuery.Descending || repository.lastFilters.IsSuper == nil || *repository.lastFilters.IsSuper {
		t.Fatalf("filters did not reach repository: %+v", repository.lastQuery)
	}

	view := httpRequest(mux, http.MethodGet, "/admin/en/roles", "", cookies)

	if !strings.Contains(view.Body.String(), html.EscapeString("Review <script>alert(1)</script>")) || strings.Contains(view.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("role name was not escaped")
	}

	for _, invalid := range []string{"page=0", "size=101", "sort=permissions", "super=maybe", "permissions=unknown", "page=1&page=2", "order=ascending", "name=%ZZ"} {
		bad := httpRequest(mux, http.MethodGet, "/admin/en/roles?"+invalid, "", cookies)

		if bad.Code != http.StatusBadRequest {
			t.Fatalf("invalid query %s = %d", invalid, bad.Code)
		}
	}
}

func TestRoleCrossOriginMutations(t *testing.T) {
	for _, action := range []string{"create", "edit", "delete"} {
		t.Run(action, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)
			repository.role.SetIsSuper(true)
			role := addHTTPRole(t, repository, "Other", nil, false)
			path := "/admin/en/roles/create"

			if action != "create" {
				path = "/admin/en/roles/" + uuid.UUID(role.ID()).String() + "/" + action
			}

			request := httptest.NewRequest(http.MethodPost, "https://admin.example.com"+path, strings.NewReader("name[en]=Changed&confirm=delete"))
			request.Header.Set("Origin", "https://untrusted.example")
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			for _, cookie := range login(t, mux) {
				request.AddCookie(cookie)
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden || repository.roleWrites != 0 {
				t.Fatal("cross-origin mutation reached the repository")
			}
		})
	}
}
