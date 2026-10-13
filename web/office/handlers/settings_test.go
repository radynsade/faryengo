package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/workspaces"
)

func TestSettingsPage(t *testing.T) {
	for _, tt := range []struct {
		name     string
		path     string
		contains []string
		excludes []string
	}{
		{
			name: "without a workspace",
			path: "/office/en/settings",
			contains: []string{
				`<div class="standalone"`,
				`<form id="profile-form" class="form" method="post" action="/office/en/settings/profile" novalidate`,
				`<form id="password-form" class="form" method="post" action="/office/en/settings/password" novalidate`,
			},
			excludes: []string{"sidebar", "workspace-form", "workspace-title", "workspace-member-form", "member-list"},
		},
		{
			name: "inside a workspace",
			path: designPath + "/settings",
			contains: []string{
				`<aside id="dashboard-sidebar" class="sidebar">`,
				`<main id="dashboard-main" class="dashboard__main dashboard__main--narrow"`,
				`action="` + designPath + `/settings/profile"`,
				`action="` + designPath + `/settings/password"`,
				`<h2 id="workspace-title">Workspace</h2>`,
				`<form id="workspace-form" class="form" method="post" action="` + designPath + `/settings/workspace" novalidate`,
				`name="name" type="text" placeholder autocomplete="off" value="Design" required`,
				`<h3 id="member-title">Add a member</h3>`,
				`<form id="workspace-member-form" class="form" method="post" action="` + designPath + `/settings/members" novalidate aria-labelledby="member-title"`,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)

			body := f.do(request{path: tt.path, cookies: cookies}).Body.String()

			for _, want := range append(tt.contains,
				`<h2 id="profile-title">Profile</h2>`,
				`<h2 id="password-title">Password change</h2>`,
				`name="first_name" type="text"`,
				`value="Ada"`,
				`value="Lovelace"`,
				`value="`+testEmail+`"`,
				`<input type="hidden" name="updated_at" value="`+f.user.UpdatedAt.Format(time.RFC3339Nano)+`">`,
				`data-confirm-password="email"`,
				`data-confirm-password="always"`,
				`<dialog id="profile-confirm" class="confirm-dialog" aria-labelledby="profile-confirm-title" aria-describedby="profile-confirm-description" data-confirm-dialog>`,
				`<dialog id="password-confirm" class="confirm-dialog" aria-labelledby="password-confirm-title" aria-describedby="password-confirm-description" data-confirm-dialog>`,
				`<p id="profile-confirm-description">Enter your current password to save the new email.</p>`,
				`<button class="button" type="submit" name="confirm" value="1" data-confirm-submit>`,
				`name="new_password_repeat"`,
				`required data-must-match="new_password"`,
				`autocomplete="new-password"`,
				`<p class="field__hint" id="password-new-hint">Use at least 6 characters.</p>`,
			) {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %s:\n%s", want, body)
				}
			}

			for _, unwanted := range append(tt.excludes, `class="message"`, `hx-swap-oob`, `data-confirm-dialog open`, `id="password-current"`) {
				if strings.Contains(body, unwanted) {
					t.Fatalf("body contains %s", unwanted)
				}
			}
		})
	}
}

// Names alone keep every Session; the confirmation shows once.

func TestUpdateProfileNames(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	version := f.version

	response := f.do(request{
		method:  http.MethodPost,
		path:    designPath + "/settings/profile",
		body:    profileBody(f, "Augusta", "King", testEmail, ""),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" {
		t.Fatalf("save = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if f.user.FirstName != "Augusta" || f.user.LastName != "King" || f.version != version || len(response.Result().Cookies()) != 0 {
		t.Fatalf("user %s %s, version replaced %t, cookies %v", f.user.FirstName, f.user.LastName, f.version != version, response.Result().Cookies())
	}

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	if !strings.Contains(body, `<p>Your profile has been saved.</p>`) || strings.Contains(body, "other devices") {
		t.Fatalf("settings lack only the profile confirmation:\n%s", body)
	}

	if strings.Contains(f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String(), `class="message"`) {
		t.Fatal("the confirmation shows again after a reload")
	}
}

// A new email ends every Session; this device signs in again with it.

func TestUpdateProfileEmail(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	version := f.version

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/office/en/settings/profile",
		body:    profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", testPassword),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/settings" {
		t.Fatalf("save = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if f.user.Email != "ada@lovelace.example" || f.version == version {
		t.Fatalf("email = %q, version replaced %t", f.user.Email, f.version != version)
	}

	assertSessionRenewed(t, f, cookies, response.Result().Cookies(), "Your profile has been saved.")
}

// A new password ends every Session; this device signs in again with it.

func TestChangePassword(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    designPath + "/settings/password",
		body:    passwordBody(testPassword, "battery staple", "battery staple"),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" {
		t.Fatalf("change = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if f.user.PasswordHash != "hash:battery staple" {
		t.Fatalf("password hash = %q", f.user.PasswordHash)
	}

	assertSessionRenewed(t, f, cookies, response.Result().Cookies(), "Your password has been changed.")

	response = f.do(request{
		method: http.MethodPost,
		path:   "/office/en/sign-in",
		body:   "email=" + url.QueryEscape(testEmail) + "&password=battery+staple",
	})

	if response.Code != http.StatusSeeOther {
		t.Fatalf("sign in with the new password = %d", response.Code)
	}
}

func TestSettingsRejected(t *testing.T) {
	for _, tt := range []struct {
		name     string
		path     string
		body     func(f *fixture) string
		status   int
		contains []string
		excludes []string
	}{
		{
			name:     "blank first name",
			path:     "/office/en/settings/profile",
			body:     func(f *fixture) string { return profileBody(f, " ", "Lovelace", testEmail, "") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<p class="field__error" id="profile-first-name-error">Please fill in this field.</p>`},
		},
		{
			name:     "too long last name in characters",
			path:     "/office/en/settings/profile",
			body:     func(f *fixture) string { return profileBody(f, "Ada", strings.Repeat("ā", 101), testEmail, "") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="profile-last-name-error">Use at most 100 characters.</p>`},
		},
		{
			name:     "invalid email",
			path:     "/office/en/settings/profile",
			body:     func(f *fixture) string { return profileBody(f, "Ada", "Lovelace", "not-an-email", testPassword) },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="profile-email-error">Enter a valid email address.</p>`, `value="not-an-email"`},
		},
		{
			name:     "new email asks for the password",
			path:     "/office/en/settings/profile",
			body:     func(f *fixture) string { return profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`data-confirm-dialog open>`, `value="ada@lovelace.example"`},
			excludes: []string{`id="profile-confirm-password-error"`},
		},
		{
			name: "new email confirmed without the password",
			path: "/office/en/settings/profile",
			body: func(f *fixture) string {
				return profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "") + "&confirm=1"
			},
			status:   http.StatusUnprocessableEntity,
			contains: []string{`data-confirm-dialog open>`, `id="profile-confirm-password-error">Please fill in this field.</p>`},
		},
		{
			name: "new email with a wrong password",
			path: "/office/en/settings/profile",
			body: func(f *fixture) string {
				return profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "wrong horse") + "&confirm=1"
			},
			status:   http.StatusUnprocessableEntity,
			contains: []string{`data-confirm-dialog open>`, `id="profile-confirm-password-error">The password is incorrect.</p>`},
		},
		{
			name: "outdated profile",
			path: "/office/en/settings/profile",
			body: func(f *fixture) string {
				return strings.Replace(profileBody(f, "Augusta", "King", testEmail, ""),
					url.QueryEscape(f.user.UpdatedAt.Format(time.RFC3339Nano)),
					url.QueryEscape(f.user.UpdatedAt.Add(-time.Hour).Format(time.RFC3339Nano)), 1)
			},
			status:   http.StatusConflict,
			contains: []string{"Your profile changed in the meantime. Reload the page and try again."},
		},
		{
			name:     "malformed profile",
			path:     "/office/en/settings/profile",
			body:     func(f *fixture) string { return profileBody(f, "Ada", "Lovelace", testEmail, "") + "&role=admin" },
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:     "different new passwords",
			path:     "/office/en/settings/password",
			body:     func(*fixture) string { return passwordBody(testPassword, "battery staple", "battery stable") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="password-repeat-error">The passwords do not match.</p>`},
		},
		{
			name:     "new password asks for the current one",
			path:     "/office/en/settings/password",
			body:     func(*fixture) string { return passwordBody("", "battery staple", "battery staple") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<dialog id="password-confirm"`, `data-confirm-dialog open>`},
			excludes: []string{`id="password-confirm-password-error"`},
		},
		{
			name: "wrong current password",
			path: "/office/en/settings/password",
			body: func(*fixture) string {
				return passwordBody("wrong horse", "battery staple", "battery staple") + "&confirm=1"
			},
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="password-confirm-password-error">The password is incorrect.</p>`},
		},
		{
			name:     "short new password",
			path:     "/office/ru/settings/password",
			body:     func(*fixture) string { return passwordBody(testPassword, "short", "short") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="password-new-error">Используйте не менее 6 символов.</p>`},
		},
		{
			name:     "empty password form",
			path:     "/office/en/settings/password",
			body:     func(*fixture) string { return passwordBody("", "", "") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="password-new-error">Please fill in this field.</p>`, `id="password-repeat-error">Please fill in this field.</p>`},
			excludes: []string{`data-confirm-dialog open`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			before, version := *f.user, f.version

			response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body(f), cookies: cookies})
			body := response.Body.String()

			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d:\n%s", response.Code, tt.status, body)
			}

			if *f.user != before || f.version != version || len(response.Result().Cookies()) != 0 {
				t.Fatal("a rejected change altered the user, the credentials, or the cookie")
			}

			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %s:\n%s", want, body)
				}
			}

			for _, unwanted := range tt.excludes {
				if strings.Contains(body, unwanted) {
					t.Fatalf("body contains %s", unwanted)
				}
			}

			for _, secret := range []string{testPassword, "wrong horse", "battery"} {
				if strings.Contains(body, secret) {
					t.Fatalf("body renders a submitted password %q", secret)
				}
			}
		})
	}
}

// A failed in-page submission replaces only its form, keeping the address.

func TestSettingsReplaceOnlyTheForm(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	// A missing or wrong current password replaces only the confirmation, so
	// the form keeps what was typed, the new passwords included.
	for _, tt := range []struct {
		path   string
		body   string
		formID string
		tag    string
	}{
		{path: designPath + "/settings/profile", body: profileBody(f, "", "Lovelace", testEmail, ""), formID: "profile-form", tag: "form"},
		{path: designPath + "/settings/password", body: passwordBody("", "", ""), formID: "password-form", tag: "form"},
		{path: designPath + "/settings/workspace", body: "name=", formID: "workspace-form", tag: "form"},
		{path: designPath + "/settings/members", body: "email=nobody%40example.com", formID: "workspace-member-form", tag: "form"},
		{
			path:   designPath + "/settings/profile",
			body:   profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "wrong horse") + "&confirm=1",
			formID: "profile-confirm",
			tag:    "dialog",
		},
		{path: designPath + "/settings/password", body: passwordBody("", "battery staple", "battery staple"), formID: "password-confirm", tag: "dialog"},
	} {
		response := f.do(request{
			method:  http.MethodPost,
			path:    tt.path,
			body:    tt.body,
			cookies: cookies,
			headers: map[string]string{"HX-Request": "true", "HX-Target": "dashboard-main"},
		})
		body := response.Body.String()

		if response.Header().Get("HX-Retarget") != "#"+tt.formID || !strings.HasPrefix(body, `<`+tt.tag+` id="`+tt.formID+`"`) || strings.Contains(body, `<head hx-head`) {
			t.Fatalf("POST %s = %d %v, body is not only the form:\n%s", tt.path, response.Code, response.Header(), body)
		}

		// An earlier confirmation must not stand beside the new errors.
		if !strings.HasSuffix(body, `<div id="messages" class="messages" hx-swap-oob="morph"></div>`) {
			t.Fatalf("POST %s does not clear the confirmations:\n%s", tt.path, body)
		}
	}
}

func TestSettingsVisitors(t *testing.T) {
	f := newFixture(t)

	for _, path := range []string{"/office/en/settings/profile", designPath + "/settings/password"} {
		response := f.do(request{method: http.MethodPost, path: path, body: "current_password=x"})

		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
			t.Fatalf("POST %s as a visitor = %d %q", path, response.Code, response.Header().Get("Location"))
		}
	}

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/office/en/workspaces/" + otherID + "/settings/profile",
		body:    profileBody(f, "Ada", "Lovelace", testEmail, ""),
		cookies: f.signIn(t),
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/workspaces" || f.user.FirstName != "Ada" {
		t.Fatalf("POST to another user's workspace = %d %q", response.Code, response.Header().Get("Location"))
	}
}

//
// Helpers
//

func profileBody(f *fixture, firstName, lastName, email, password string) string {
	values := url.Values{
		"first_name":       {firstName},
		"last_name":        {lastName},
		"email":            {email},
		"current_password": {password},
		"updated_at":       {f.user.UpdatedAt.Format(time.RFC3339Nano)},
	}

	return values.Encode()
}

func passwordBody(current, next, repeat string) string {
	values := url.Values{
		"current_password":    {current},
		"new_password":        {next},
		"new_password_repeat": {repeat},
	}

	return values.Encode()
}

// The old Session no longer authenticates, the new cookie does, and the
// settings show the change's confirmation with the sign-out notice once.

func assertSessionRenewed(t *testing.T, f *fixture, old, renewed []*http.Cookie, confirmation string) {
	t.Helper()

	if len(renewed) != 1 || renewed[0].Value == "" || renewed[0].Value == old[0].Value {
		t.Fatalf("cookies = %v, want one new session cookie", renewed)
	}

	if response := f.do(request{path: "/office/en/settings", cookies: old}); response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("old session = %d %q, want sign-in", response.Code, response.Header().Get("Location"))
	}

	response := f.do(request{path: "/office/en/settings", cookies: renewed})
	body := response.Body.String()

	for _, want := range []string{"<p>" + confirmation + "</p>", "<p>You were signed out on your other devices.</p>"} {
		if response.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("settings with the new session = %d, lack %s:\n%s", response.Code, want, body)
		}
	}
}

// A member renames the workspace for everyone; the top bar shows the new name.

func TestRenameWorkspace(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    designPath + "/settings/workspace",
		body:    "name=" + url.QueryEscape("  Studio  "),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" {
		t.Fatalf("rename = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if name := workspaceName(f, designID); name != "Studio" {
		t.Fatalf("workspace name = %q, want the trimmed new name", name)
	}

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	for _, want := range []string{
		"<p>The workspace has been saved.</p>",
		`<span class="topbar__workspace-name">Studio</span>`,
		`value="Studio"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings lack %s:\n%s", want, body)
		}
	}
}

func TestRenameWorkspaceRejected(t *testing.T) {
	for _, tt := range []struct {
		name      string
		path      string
		body      string
		updateErr error
		status    int
		location  string
		contains  []string
	}{
		{
			name:     "blank name",
			path:     designPath + "/settings/workspace",
			body:     "name=+++",
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<p class="field__error" id="workspace-name-error">Please fill in this field.</p>`},
		},
		{
			name:     "too long name in characters",
			path:     "/office/ru/workspaces/" + designID + "/settings/workspace",
			body:     "name=" + url.QueryEscape(strings.Repeat("ā", 101)),
			status:   http.StatusUnprocessableEntity,
			contains: []string{"Используйте не более 100 символов."},
		},
		{
			name:     "malformed form",
			path:     designPath + "/settings/workspace",
			body:     "name=Studio&owner=me",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:      "unavailable storage",
			path:      designPath + "/settings/workspace",
			body:      "name=Studio",
			updateErr: errors.New("storage unavailable"),
			status:    http.StatusServiceUnavailable,
			contains:  []string{"Your settings could not be loaded or saved.", `value="Studio"`},
		},
		{
			name:     "another user's workspace",
			path:     "/office/en/workspaces/" + otherID + "/settings/workspace",
			body:     "name=Studio",
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			f.updateErr = tt.updateErr

			response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body, cookies: cookies})
			body := response.Body.String()

			if response.Code != tt.status || response.Header().Get("Location") != tt.location {
				t.Fatalf("status = %d %q, want %d %q:\n%s", response.Code, response.Header().Get("Location"), tt.status, tt.location, body)
			}

			if workspaceName(f, designID) != "Design" || workspaceName(f, otherID) != "Someone else's" {
				t.Fatal("a rejected rename changed a workspace")
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

	f := newFixture(t)
	response := f.do(request{method: http.MethodPost, path: designPath + "/settings/workspace", body: "name=Studio"})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("rename as a visitor = %d %q", response.Code, response.Header().Get("Location"))
	}
}

func workspaceName(f *fixture, id string) workspaces.WorkspaceName {
	var name workspaces.WorkspaceName

	for _, workspace := range f.workspaces {
		if uuid.UUID(workspace.ID).String() == id {
			name = workspace.Name
		}
	}

	return name
}

// A member adds an existing User by email, ignoring letter case; the
// confirmation names the added User.

func TestAddWorkspaceMember(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	grace := f.others[0]

	response := f.do(request{
		method:  http.MethodPost,
		path:    designPath + "/settings/members",
		body:    "email=" + url.QueryEscape("GRACE@example.com"),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" {
		t.Fatalf("add = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if !slices.Contains(f.members[workspaces.WorkspaceID(uuid.MustParse(designID))], grace.ID) {
		t.Fatalf("members = %v, want Grace added", f.members)
	}

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	if !strings.Contains(body, "<p>Grace Hopper (grace@example.com) has been added to the workspace.</p>") {
		t.Fatalf("settings lack the confirmation:\n%s", body)
	}
}

func TestAddWorkspaceMemberRejected(t *testing.T) {
	for _, tt := range []struct {
		name     string
		path     string
		body     string
		status   int
		location string
		contains []string
	}{
		{
			name:     "invalid email",
			path:     designPath + "/settings/members",
			body:     "email=not-an-email",
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<p class="field__error" id="member-email-error">Enter a valid email address.</p>`, `value="not-an-email"`},
		},
		{
			name:     "blank email",
			path:     designPath + "/settings/members",
			body:     "email=",
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="member-email-error">Please fill in this field.</p>`},
		},
		{
			name:     "unknown email",
			path:     designPath + "/settings/members",
			body:     "email=nobody%40example.com",
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="member-email-error">No user has this email address.</p>`},
		},
		{
			name:     "already a member",
			path:     designPath + "/settings/members",
			body:     "email=" + url.QueryEscape(testEmail),
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="member-email-error">This user is already a member of the workspace.</p>`},
		},
		{
			name:     "malformed form",
			path:     designPath + "/settings/members",
			body:     "email=grace%40example.com&role=owner",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:     "another user's workspace",
			path:     "/office/en/workspaces/" + otherID + "/settings/members",
			body:     "email=grace%40example.com",
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			before := len(f.members[workspaces.WorkspaceID(uuid.MustParse(designID))]) + len(f.members[workspaces.WorkspaceID(uuid.MustParse(otherID))])

			response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body, cookies: cookies})
			body := response.Body.String()

			if response.Code != tt.status || response.Header().Get("Location") != tt.location {
				t.Fatalf("status = %d %q, want %d %q:\n%s", response.Code, response.Header().Get("Location"), tt.status, tt.location, body)
			}

			if after := len(f.members[workspaces.WorkspaceID(uuid.MustParse(designID))]) + len(f.members[workspaces.WorkspaceID(uuid.MustParse(otherID))]); after != before {
				t.Fatal("a rejected addition changed the memberships")
			}

			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %s:\n%s", want, body)
				}
			}
		})
	}

	f := newFixture(t)
	response := f.do(request{method: http.MethodPost, path: designPath + "/settings/members", body: "email=grace%40example.com"})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("add as a visitor = %d %q", response.Code, response.Header().Get("Location"))
	}
}

// The members are listed in name order, the signed-in User marked; an added
// member appears in the list.

func TestWorkspaceMembers(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	grace := f.others[0]

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()
	ada := `<li id="member-` + uuid.UUID(f.user.ID).String() + `" class="member-list__item">`

	for _, want := range []string{
		`<h3 id="members-title">Members</h3>`,
		`<ul class="member-list" aria-labelledby="members-title">`,
		ada,
		`<span class="member-list__name">Ada Lovelace</span>`,
		`<span class="member-list__badge">You</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings lack %s:\n%s", want, body)
		}
	}

	if strings.Contains(body, "Grace Hopper") {
		t.Fatal("the list shows a user who is not a member")
	}

	f.do(request{method: http.MethodPost, path: designPath + "/settings/members", body: "email=grace%40example.com", cookies: cookies})
	body = f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()
	graceRow := `<li id="member-` + uuid.UUID(grace.ID).String() + `" class="member-list__item">`

	if !strings.Contains(body, graceRow) || strings.Index(body, ada) > strings.Index(body, graceRow) || strings.Count(body, "member-list__badge") != 1 {
		t.Fatalf("settings do not list Ada, then Grace, with only Ada marked:\n%s", body)
	}
}

// A member list that cannot be loaded shows its error; the rest of the
// settings stay usable.

func TestWorkspaceMembersUnavailable(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	f.membersErr = errors.New("storage unavailable")

	response := f.do(request{path: designPath + "/settings", cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK || !strings.Contains(body, "The members could not be loaded.") || !strings.Contains(body, `id="workspace-member-form"`) {
		t.Fatalf("settings = %d:\n%s", response.Code, body)
	}

	if strings.Contains(body, "storage unavailable") || strings.Contains(body, `class="member-list"`) {
		t.Fatal("settings expose the cause or show a list")
	}
}

// A member removes another one; their own row has no remove button.

func TestRemoveWorkspaceMember(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	grace := f.others[0]
	design := workspaces.WorkspaceID(uuid.MustParse(designID))
	f.members[design] = append(f.members[design], grace.ID)
	removePath := designPath + "/settings/members/" + uuid.UUID(grace.ID).String() + "/remove"

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	for _, want := range []string{
		`<form class="member-list__remove" method="post" action="` + removePath + `"`,
		`aria-label="Remove Grace Hopper from the workspace"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings lack %s:\n%s", want, body)
		}
	}

	if strings.Count(body, `class="member-list__remove"`) != 1 {
		t.Fatal("the signed-in user's own row has a remove button")
	}

	response := f.do(request{method: http.MethodPost, path: removePath, cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" || slices.Contains(f.members[design], grace.ID) {
		t.Fatalf("remove = %d %q, members %v", response.Code, response.Header().Get("Location"), f.members[design])
	}

	body = f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	if !strings.Contains(body, "<p>Grace Hopper has been removed from the workspace.</p>") || strings.Contains(body, `id="member-`+uuid.UUID(grace.ID).String()+`"`) {
		t.Fatalf("settings do not confirm the removal or still list Grace:\n%s", body)
	}
}

// Removing oneself, a user who is no longer a member, or a member of another
// user's workspace changes nothing.

func TestRemoveWorkspaceMemberIgnored(t *testing.T) {
	for _, tt := range []struct {
		name     string
		path     func(f *fixture) string
		body     string
		status   int
		location string
	}{
		{
			name: "oneself",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.user.ID).String() + "/remove"
			},
			status:   http.StatusSeeOther,
			location: designPath + "/settings",
		},
		{
			name: "not a member",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/remove"
			},
			status:   http.StatusSeeOther,
			location: designPath + "/settings",
		},
		{
			name:     "unknown user",
			path:     func(*fixture) string { return designPath + "/settings/members/" + otherID + "/remove" },
			status:   http.StatusSeeOther,
			location: designPath + "/settings",
		},
		{
			name: "another user's workspace",
			path: func(f *fixture) string {
				return "/office/en/workspaces/" + otherID + "/settings/members/" + uuid.UUID(f.user.ID).String() + "/remove"
			},
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
		{
			name: "malformed form",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/remove"
			},
			body:   "user=me",
			status: http.StatusBadRequest,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			before := fmt.Sprint(f.members)

			response := f.do(request{method: http.MethodPost, path: tt.path(f), body: tt.body, cookies: cookies})

			if response.Code != tt.status || response.Header().Get("Location") != tt.location {
				t.Fatalf("remove = %d %q, want %d %q", response.Code, response.Header().Get("Location"), tt.status, tt.location)
			}

			if fmt.Sprint(f.members) != before {
				t.Fatalf("members = %v, want unchanged %v", f.members, before)
			}
		})
	}
}
