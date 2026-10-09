package handlers

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
)

func TestUserAuthorization(t *testing.T) {
	view := users.Permissions{users.PermissionViewUser}
	manage := users.Permissions{users.PermissionManageUser}

	tests := []struct {
		name        string
		permissions users.Permissions
		isSuper     bool
		want        map[string]int
	}{
		{"no permissions", users.Permissions{users.PermissionViewRole}, false, map[string]int{
			"GET /admin/en/users": 403, "GET /admin/en/users/table": 403, "GET view": 403, "GET /admin/en/users/create": 403,
			"GET edit": 403, "POST /admin/en/users/create": 403, "POST edit": 403, "POST delete": 403,
		}},
		{"view users", view, false, map[string]int{
			"GET /admin/en/users": 200, "GET /admin/en/users/table": 200, "GET view": 200,
			"GET /admin/en/users/create": 403, "GET edit": 403, "POST /admin/en/users/create": 403, "POST edit": 403, "POST delete": 403,
		}},
		{"manage users without viewing", manage, false, map[string]int{
			"GET /admin/en/users": 403, "GET view": 403, "GET /admin/en/users/create": 200, "GET edit": 200,
			"POST /admin/en/users/create": 303, "POST edit": 303, "POST delete": 303,
		}},
		{"super role", nil, true, map[string]int{
			"GET /admin/en/users": 200, "GET /admin/en/users/table": 200, "GET view": 200, "GET edit": 200,
			"POST /admin/en/users/create": 303, "POST edit": 303, "POST delete": 303,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, route := range slices.Sorted(maps.Keys(tt.want)) {
				f := newFixture(t, tt.permissions, tt.isSuper)
				target := f.addUser(t, "grace@example.com", f.role)
				method, path, _ := strings.Cut(route, " ")
				base := "/admin/en/users/" + userID(target)
				body := ""

				switch path {
				case "view", "edit", "delete":
					path = base + "/" + path
				}

				if method == http.MethodPost {
					body = userBody(f.role, map[string]string{"email": "new@example.com", "password": "secret1"})

					if strings.HasSuffix(path, "/edit") {
						body = userBody(f.role, map[string]string{"updated_at": target.UpdatedAt.Format(time.RFC3339Nano)})
					} else if strings.HasSuffix(path, "/delete") {
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

func TestUserPagesHideActionsWithoutPermission(t *testing.T) {
	f := newFixture(t, users.Permissions{users.PermissionViewUser}, false)
	target := f.addUser(t, "grace@example.com", f.role)
	cookies := f.signIn(t)
	pages := []string{"/admin/en/users", "/admin/en/users/table", "/admin/en/users/" + userID(target) + "/view"}

	for _, path := range pages {
		body := f.do(request{path: path, cookies: cookies}).Body.String()

		for _, hidden := range []string{"/users/create", "/edit", "data-confirm-delete", "/roles/" + roleID(f.role) + "/view"} {
			if strings.Contains(body, hidden) {
				t.Fatalf("GET %s shows %q to a viewer", path, hidden)
			}
		}
	}
}

func TestUserCreateUpdateAndDelete(t *testing.T) {
	f := newFixture(t, nil, true)
	cookies := f.signIn(t)
	editor := f.addRole(t, "Editor", users.Permissions{users.PermissionViewUser}, false)

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/admin/en/users/create",
		body:    userBody(editor, map[string]string{"first_name": " Grace ", "email": "grace@example.com", "password": "secret1"}),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || f.writes != 1 {
		t.Fatalf("create = %d, writes = %d: %s", response.Code, f.writes, response.Body.String())
	}

	created, err := f.users().FindByEmail(t.Context(), "grace@example.com")
	must(t, err)

	if created.FirstName != "Grace" || created.RoleID != editor.ID || created.PasswordHash != "hash:secret1" || created.Phone != "+37120000001" {
		t.Fatalf("created user = %+v", created)
	}

	location := "/admin/en/users/" + userID(created) + "/view"

	if response.Header().Get("Location") != location {
		t.Fatalf("Location = %q, want %q", response.Header().Get("Location"), location)
	}

	view := f.do(request{path: location, cookies: cookies}).Body.String()

	if !strings.Contains(view, `User &#34;Grace Hopper&#34; created successfully.`) && !strings.Contains(view, `User "Grace Hopper" created successfully.`) {
		t.Fatalf("the success message is missing: %s", view)
	}

	if !strings.Contains(view, "Editor") || !strings.Contains(view, "grace@example.com") {
		t.Fatalf("the view lacks the user's details: %s", view)
	}

	if again := f.do(request{path: location, cookies: cookies}).Body.String(); strings.Contains(again, "created successfully") {
		t.Fatal("the success message was shown twice")
	}

	edit := "/admin/en/users/" + userID(created) + "/edit"
	form := f.do(request{path: edit, cookies: cookies}).Body.String()
	updatedAt := created.UpdatedAt.UTC().Format(time.RFC3339Nano)

	if !strings.Contains(form, `name="updated_at" value="`+updatedAt+`"`) || !strings.Contains(form, `value="grace@example.com"`) {
		t.Fatalf("the edit form lacks the user's details: %s", form)
	}

	response = f.do(request{
		method:  http.MethodPost,
		path:    edit,
		body:    userBody(f.role, map[string]string{"last_name": "Brewster", "email": "grace@example.com", "updated_at": updatedAt}),
		cookies: cookies,
	})

	updated := f.accounts[created.ID]

	if response.Code != http.StatusSeeOther || updated.LastName != "Brewster" || updated.RoleID != f.role.ID || updated.PasswordHash != "hash:secret1" {
		t.Fatalf("update = %d, user = %+v", response.Code, updated)
	}

	response = f.do(request{
		method:  http.MethodPost,
		path:    edit,
		body:    userBody(f.role, map[string]string{"last_name": "Brewster", "password": "changed", "updated_at": updated.UpdatedAt.UTC().Format(time.RFC3339Nano)}),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || f.accounts[created.ID].PasswordHash != "hash:changed" {
		t.Fatalf("password change = %d, user = %+v", response.Code, f.accounts[created.ID])
	}

	response = f.do(request{
		method:  http.MethodPost,
		path:    "/admin/en/users/" + userID(created) + "/delete",
		body:    "confirm=delete",
		cookies: cookies,
		headers: partial,
	})

	if _, found := f.accounts[created.ID]; found || response.Code != http.StatusOK || !strings.Contains(response.Header().Get("HX-Location"), `"/admin/en/users"`) {
		t.Fatalf("delete = %d, headers = %v", response.Code, response.Header())
	}

	list := f.do(request{path: "/admin/en/users", cookies: cookies}).Body.String()

	if !strings.Contains(list, `User &#34;Grace Brewster&#34; deleted successfully.`) && !strings.Contains(list, `User "Grace Brewster" deleted successfully.`) {
		t.Fatalf("the deletion message is missing: %s", list)
	}
}

func TestUserFormValidation(t *testing.T) {
	missing := users.NewRole(users.RoleID(uuid.Must(uuid.NewV7())), users.RoleName{"en": "Gone"}, nil, false)

	tests := []struct {
		name       string
		role       func(f *fixture) *users.Role
		values     map[string]string
		wantStatus int
		wantField  string
		wantBody   string
	}{
		{"invalid email", nil, map[string]string{"email": "nope"}, http.StatusUnprocessableEntity, "user-email", "Enter a valid email address."},
		{"taken email", nil, map[string]string{"email": testEmail}, http.StatusConflict, "user-email", "Another user already has this email address."},
		{"invalid phone", nil, map[string]string{"phone": "20000000"}, http.StatusUnprocessableEntity, "user-phone", "Enter a phone number in international format"},
		{"blank first name", nil, map[string]string{"first_name": "  "}, http.StatusUnprocessableEntity, "user-first-name", "This field is required."},
		{"long last name", nil, map[string]string{"last_name": strings.Repeat("a", users.MaxLastNameLength+1)}, http.StatusUnprocessableEntity, "user-last-name", "This value is too long."},
		{"missing password", nil, map[string]string{"password": ""}, http.StatusUnprocessableEntity, "user-password", "This field is required."},
		{"short password", nil, map[string]string{"password": "abc"}, http.StatusUnprocessableEntity, "user-password", "The password must be at least 6 characters long."},
		{"missing role", nil, map[string]string{"role": ""}, http.StatusUnprocessableEntity, "user-role", "Select a role."},
		{"deleted role", func(*fixture) *users.Role { return missing }, nil, http.StatusUnprocessableEntity, "user-role", "The selected role no longer exists."},
		{"malformed role", nil, map[string]string{"role": "admin"}, http.StatusUnprocessableEntity, "user-role", "Enter a valid UUID."},
		{"unknown field", nil, map[string]string{"nickname": "ada"}, http.StatusBadRequest, "", "Submit a valid user form."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, nil, true)
			role := f.role

			if tt.role != nil {
				role = tt.role(f)
			}

			values := map[string]string{"email": "grace@example.com", "password": "secret-value"}
			maps.Copy(values, tt.values)
			writes := 0

			if tt.wantStatus == http.StatusConflict {
				writes = 1
			} else if tt.role != nil {
				writes = 1
			}

			response := f.do(request{
				method:  http.MethodPost,
				path:    "/admin/en/users/create",
				body:    userBody(role, values),
				cookies: f.signIn(t),
				headers: partial,
			})
			body := response.Body.String()

			if response.Code != tt.wantStatus || f.writes != writes || len(f.accounts) != 1 {
				t.Fatalf("status = %d, writes = %d, want %d: %s", response.Code, f.writes, tt.wantStatus, body)
			}

			if !strings.Contains(body, tt.wantBody) {
				t.Fatalf("body does not contain %q: %s", tt.wantBody, body)
			}

			if tt.wantField != "" && !strings.Contains(body, `aria-describedby="`+tt.wantField) {
				t.Fatalf("the %s control is not linked to its error: %s", tt.wantField, body)
			}

			if strings.Contains(body, values["password"]) && values["password"] != "" {
				t.Fatal("the submitted password was rendered back")
			}
		})
	}
}

func TestUserUpdateConflict(t *testing.T) {
	f := newFixture(t, nil, true)
	target := f.addUser(t, "grace@example.com", f.role)
	stale := target.UpdatedAt.Add(-time.Minute).Format(time.RFC3339Nano)

	tests := []struct {
		name       string
		updatedAt  string
		wantStatus int
		wantBody   string
	}{
		{"outdated form", stale, http.StatusConflict, "This user was changed by someone else."},
		{"malformed token", "yesterday", http.StatusBadRequest, "Submit a valid user form."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := f.do(request{
				method:  http.MethodPost,
				path:    "/admin/en/users/" + userID(target) + "/edit",
				body:    userBody(f.role, map[string]string{"email": "grace@example.com", "first_name": "Changed", "updated_at": tt.updatedAt}),
				cookies: f.signIn(t),
				headers: partial,
			})

			if response.Code != tt.wantStatus || !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Fatalf("update = %d: %s", response.Code, response.Body.String())
			}

			if f.accounts[target.ID].FirstName != "Grace" {
				t.Fatal("an outdated form overwrote a newer change")
			}
		})
	}
}

func TestUserList(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		want       []string
		wantNot    []string
	}{
		{"every user", "", http.StatusOK, []string{"Total found: 3", "ada@example.com", "grace@example.com", "Editor", ">You<", "user-table__uuid"}, nil},
		{"email filter", "?email=grace", http.StatusOK, []string{"Total found: 1", "grace@example.com"}, []string{"ada@example.com"}},
		{"role filter", "?role=", http.StatusOK, []string{"Total found: 1", "linus@example.com"}, []string{"grace@example.com"}},
		{"unknown sort", "?sort=password", http.StatusBadRequest, []string{"Check the filters, sort order, and page number."}, nil},
		{"invalid role filter", "?role=everyone", http.StatusBadRequest, []string{"Enter a valid UUID."}, nil},
		{"unknown parameter", "?admin=1", http.StatusBadRequest, []string{"Check the filters, sort order, and page number."}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, nil, true)
			editor := f.addRole(t, "Editor", nil, false)
			f.addUser(t, "grace@example.com", f.role)
			f.addUser(t, "linus@example.com", editor)
			query := tt.query

			if query == "?role=" {
				query += roleID(editor)
			}

			response := f.do(request{path: "/admin/en/users/table" + query, cookies: f.signIn(t), headers: partial})
			body := response.Body.String()

			if response.Code != tt.wantStatus || !strings.HasPrefix(body, `<div id="users-list-table"`) {
				t.Fatalf("table = %d: %.300s", response.Code, body)
			}

			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Fatalf("the table lacks %q: %s", want, body)
				}
			}

			for _, unwanted := range tt.wantNot {
				if strings.Contains(body, unwanted) {
					t.Fatalf("the table contains %q: %s", unwanted, body)
				}
			}
		})
	}
}

func TestUserListPageDefersTheQuery(t *testing.T) {
	f := newFixture(t, nil, true)
	body := f.do(request{path: "/admin/en/users?email=ada&sort=email&order=asc", cookies: f.signIn(t)}).Body.String()

	if f.lastUserFind.Limit != 0 {
		t.Fatal("the page queried the user list before the table request")
	}

	if !strings.Contains(body, `hx-get="/admin/en/users/table?email=ada&amp;order=asc&amp;page=1&amp;size=20&amp;sort=email"`) {
		t.Fatalf("the loading row does not request the same list: %s", body)
	}
}

func TestUserDeleteFailureKeepsTheDialog(t *testing.T) {
	f := newFixture(t, nil, true)
	missing := uuid.Must(uuid.NewV7()).String()

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/admin/en/users/" + missing + "/delete",
		body:    "confirm=delete",
		cookies: f.signIn(t),
		headers: partial,
	})

	if response.Code != http.StatusNotFound || !strings.HasPrefix(response.Body.String(), `<dialog id="confirm-delete"`) ||
		!strings.Contains(response.Body.String(), "User not found.") {
		t.Fatalf("delete = %d: %s", response.Code, response.Body.String())
	}
}

// The signed-in user sees no way to delete their own account, and a crafted
// request is refused without a write.

func TestUserCannotDeleteThemselves(t *testing.T) {
	f := newFixture(t, nil, true)
	other := f.addUser(t, "grace@example.com", f.role)
	cookies := f.signIn(t)
	own := "/admin/en/users/" + userID(f.user)

	for _, path := range []string{own + "/view", own + "/edit"} {
		if body := f.do(request{path: path, cookies: cookies}).Body.String(); strings.Contains(body, "data-confirm-delete") {
			t.Fatalf("GET %s offers deleting the own account", path)
		}
	}

	table := f.do(request{path: "/admin/en/users/table", cookies: cookies}).Body.String()

	if strings.Contains(table, own+"/delete") || !strings.Contains(table, "/admin/en/users/"+userID(other)+"/delete") {
		t.Fatalf("the table's delete actions do not exclude the own account: %s", table)
	}

	response := f.do(request{method: http.MethodPost, path: own + "/delete", body: "confirm=delete", cookies: cookies, headers: partial})

	if response.Code != http.StatusConflict || f.writes != 0 || f.accounts[f.user.ID] == nil ||
		!strings.Contains(response.Body.String(), "You cannot delete your own account.") {
		t.Fatalf("delete own account = %d, writes = %d: %s", response.Code, f.writes, response.Body.String())
	}
}

//
// Helpers
//

// A user form body starts from valid values that the overrides replace; an
// empty override leaves its field empty.

func userBody(role *users.Role, overrides map[string]string) string {
	values := url.Values{
		"first_name": {"Grace"},
		"last_name":  {"Hopper"},
		"email":      {"grace@example.com"},
		"phone":      {"+37120000001"},
		"role":       {roleID(role)},
	}

	for key, value := range overrides {
		values.Set(key, value)
	}

	return values.Encode()
}
