package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProfileSettingsPage(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{path: "/office/en/profile-settings", cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.Code, body)
	}

	for _, want := range []string{
		"<title>Settings · Faryen</title>",
		`<div class="standalone"`,
		`<section class="standalone__section settings settings--profile">`,
		`<h1 id="page-title">Settings</h1>`,
		`<h2 id="profile-title">Profile</h2>`,
		`<h2 id="password-title">Password change</h2>`,
		`<form id="profile-form" class="form" method="post" action="/office/en/profile-settings/profile" novalidate`,
		`<form id="password-form" class="form" method="post" action="/office/en/profile-settings/password" novalidate`,
		`name="first_name" type="text"`,
		`value="Ada"`,
		`value="Lovelace"`,
		`value="` + testEmail + `"`,
		`<input type="hidden" name="updated_at" value="` + f.user.UpdatedAt.Format(time.RFC3339Nano) + `">`,
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
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	for _, unwanted := range []string{
		"sidebar", "dashboard", "workspace-form", "workspace-title", "workspace-member-form", "member-list", "members-title",
		`class="message"`, `hx-swap-oob`, `data-confirm-dialog open`, `id="password-current"`,
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body contains %s", unwanted)
		}
	}
}

// Inside a workspace the same settings keep the workspace's dashboard, with
// no sidebar item marked, and post to the workspace's address for them.

func TestWorkspaceProfileSettingsPage(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{path: designPath + "/profile-settings", cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.Code, body)
	}

	for _, want := range []string{
		"<title>Settings · Faryen</title>",
		`<aside id="dashboard-sidebar" class="sidebar">`,
		`<main id="dashboard-main" class="dashboard__main dashboard__main--narrow"`,
		`<h1 id="page-title">Settings</h1>`,
		`<span class="topbar__workspace-name">Design</span>`,
		`<h2 id="profile-title">Profile</h2>`,
		`<h2 id="password-title">Password change</h2>`,
		`<form id="profile-form" class="form" method="post" action="` + designPath + `/profile-settings/profile" novalidate`,
		`<form id="password-form" class="form" method="post" action="` + designPath + `/profile-settings/password" novalidate`,
		`<button class="confirm-dialog__cancel" type="button" data-dialog-close>`,
		`value="Ada"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	for _, unwanted := range []string{
		`class="standalone"`, "workspace-form", "workspace-title", "workspace-member-form", "member-list",
		`class="sidebar__link" href="` + designPath + `/settings" aria-current`, `class="message"`,
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body contains %s", unwanted)
		}
	}
}

// Names alone keep every Session; the confirmation shows once.

func TestUpdateProfileNames(t *testing.T) {
	for _, settingsPath := range []string{"/office/en/profile-settings", designPath + "/profile-settings"} {
		t.Run(settingsPath, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			version := f.version

			response := f.do(request{
				method:  http.MethodPost,
				path:    settingsPath + "/profile",
				body:    profileBody(f, "Augusta", "King", testEmail, ""),
				cookies: cookies,
			})

			// The redirect keeps the address the form was posted from.
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != settingsPath {
				t.Fatalf("save = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
			}

			if f.user.FirstName != "Augusta" || f.user.LastName != "King" || f.version != version || len(response.Result().Cookies()) != 0 {
				t.Fatalf("user %s %s, version replaced %t, cookies %v", f.user.FirstName, f.user.LastName, f.version != version, response.Result().Cookies())
			}

			body := f.do(request{path: settingsPath, cookies: cookies}).Body.String()

			if !strings.Contains(body, `<p>Your profile has been saved.</p>`) || strings.Contains(body, "other devices") {
				t.Fatalf("settings lack only the profile confirmation:\n%s", body)
			}

			if strings.Contains(f.do(request{path: settingsPath, cookies: cookies}).Body.String(), `class="message"`) {
				t.Fatal("the confirmation shows again after a reload")
			}
		})
	}
}

// A new email ends every Session; this device signs in again with it.

func TestUpdateProfileEmail(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	version := f.version

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/office/en/profile-settings/profile",
		body:    profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", testPassword),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/profile-settings" {
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
		path:    designPath + "/profile-settings/password",
		body:    passwordBody(testPassword, "battery staple", "battery staple"),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/profile-settings" {
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

func TestProfileSettingsRejected(t *testing.T) {
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
			path:     "/office/en/profile-settings/profile",
			body:     func(f *fixture) string { return profileBody(f, " ", "Lovelace", testEmail, "") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<p class="field__error" id="profile-first-name-error">Please fill in this field.</p>`},
		},
		{
			name:     "too long last name in characters",
			path:     "/office/en/profile-settings/profile",
			body:     func(f *fixture) string { return profileBody(f, "Ada", strings.Repeat("ā", 101), testEmail, "") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="profile-last-name-error">Use at most 100 characters.</p>`},
		},
		{
			name:     "invalid email",
			path:     "/office/en/profile-settings/profile",
			body:     func(f *fixture) string { return profileBody(f, "Ada", "Lovelace", "not-an-email", testPassword) },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="profile-email-error">Enter a valid email address.</p>`, `value="not-an-email"`},
		},
		{
			name:     "new email asks for the password",
			path:     "/office/en/profile-settings/profile",
			body:     func(f *fixture) string { return profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<dialog id="profile-confirm"`, `data-confirm-dialog open>`},
			excludes: []string{`id="profile-confirm-password-error"`},
		},
		{
			name: "new email confirmed without the password",
			path: "/office/en/profile-settings/profile",
			body: func(f *fixture) string {
				return profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "") + "&confirm=1"
			},
			status:   http.StatusUnprocessableEntity,
			contains: []string{`data-confirm-dialog open>`, `id="profile-confirm-password-error">Please fill in this field.</p>`},
		},
		{
			name: "new email with a wrong password",
			path: "/office/en/profile-settings/profile",
			body: func(f *fixture) string {
				return profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "wrong horse") + "&confirm=1"
			},
			status:   http.StatusUnprocessableEntity,
			contains: []string{`data-confirm-dialog open>`, `id="profile-confirm-password-error">The password is incorrect.</p>`},
		},
		{
			name: "outdated profile",
			path: "/office/en/profile-settings/profile",
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
			path:     "/office/en/profile-settings/profile",
			body:     func(f *fixture) string { return profileBody(f, "Ada", "Lovelace", testEmail, "") + "&role=admin" },
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:     "different new passwords",
			path:     "/office/en/profile-settings/password",
			body:     func(*fixture) string { return passwordBody(testPassword, "battery staple", "battery stable") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="password-repeat-error">The passwords do not match.</p>`},
		},
		{
			name:     "new password asks for the current one",
			path:     "/office/en/profile-settings/password",
			body:     func(*fixture) string { return passwordBody("", "battery staple", "battery staple") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<dialog id="password-confirm"`, `data-confirm-dialog open>`},
			excludes: []string{`id="password-confirm-password-error"`},
		},
		{
			name: "wrong current password",
			path: "/office/en/profile-settings/password",
			body: func(*fixture) string {
				return passwordBody("wrong horse", "battery staple", "battery staple") + "&confirm=1"
			},
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="password-confirm-password-error">The password is incorrect.</p>`},
		},
		{
			name:     "short new password",
			path:     "/office/ru/profile-settings/password",
			body:     func(*fixture) string { return passwordBody(testPassword, "short", "short") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="password-new-error">Используйте не менее 6 символов.</p>`},
		},
		{
			name:     "empty password form",
			path:     "/office/en/profile-settings/password",
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

func TestProfileSettingsReplaceOnlyTheForm(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	// A missing or wrong current password replaces only the confirmation, so
	// the form keeps what was typed, the new passwords included.
	for _, tt := range []struct {
		path   string
		body   string
		formID string
		tag    string
		inside bool
	}{
		{path: "/office/en/profile-settings/profile", body: profileBody(f, "", "Lovelace", testEmail, ""), formID: "profile-form", tag: "form"},
		{path: "/office/en/profile-settings/password", body: passwordBody("", "", ""), formID: "password-form", tag: "form"},
		{
			path:   "/office/en/profile-settings/profile",
			body:   profileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "wrong horse") + "&confirm=1",
			formID: "profile-confirm",
			tag:    "dialog",
		},
		{path: "/office/en/profile-settings/password", body: passwordBody("", "battery staple", "battery staple"), formID: "password-confirm", tag: "dialog"},
		{path: designPath + "/profile-settings/profile", body: profileBody(f, "", "Lovelace", testEmail, ""), formID: "profile-form", tag: "form", inside: true},
		{path: designPath + "/profile-settings/password", body: passwordBody("", "battery staple", "battery staple"), formID: "password-confirm", tag: "dialog", inside: true},
	} {
		headers := partial

		if tt.inside {
			headers = toContent
		}

		response := f.do(request{
			method:  http.MethodPost,
			path:    tt.path,
			body:    tt.body,
			cookies: cookies,
			headers: headers,
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

func TestProfileSettingsVisitors(t *testing.T) {
	f := newFixture(t)

	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/office/en/profile-settings"},
		{http.MethodPost, "/office/en/profile-settings/profile"},
		{http.MethodPost, "/office/en/profile-settings/password"},
		{http.MethodGet, designPath + "/profile-settings"},
		{http.MethodPost, designPath + "/profile-settings/password"},
	} {
		response := f.do(request{method: tt.method, path: tt.path, body: "current_password=x"})

		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
			t.Fatalf("%s %s as a visitor = %d %q", tt.method, tt.path, response.Code, response.Header().Get("Location"))
		}
	}
}

// The profile settings moved from the settings' addresses, which a
// workspace's own settings keep; the old addresses are not found and change
// nothing.

func TestOldProfileSettingsRoutesRemoved(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	before, version := *f.user, f.version

	for _, tt := range []struct{ method, path, body string }{
		{http.MethodGet, "/office/en/settings", ""},
		{http.MethodPost, "/office/en/settings/profile", profileBody(f, "Augusta", "King", testEmail, "")},
		{http.MethodPost, "/office/en/settings/password", passwordBody(testPassword, "battery staple", "battery staple")},
		{http.MethodPost, designPath + "/settings/profile", profileBody(f, "Augusta", "King", testEmail, "")},
		{http.MethodPost, designPath + "/settings/password", passwordBody(testPassword, "battery staple", "battery staple")},
	} {
		response := f.do(request{method: tt.method, path: tt.path, body: tt.body, cookies: cookies})

		if response.Code != http.StatusNotFound {
			t.Fatalf("%s %s = %d %q, want 404", tt.method, tt.path, response.Code, response.Header().Get("Location"))
		}
	}

	if *f.user != before || f.version != version {
		t.Fatal("a removed route changed the user or the credentials")
	}
}

// The profile settings inside a workspace the user does not belong to are not
// found; a form posted to them leads back to choosing a workspace and changes
// nothing.

func TestWorkspaceProfileSettingsNotMember(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	before, version := *f.user, f.version
	otherPath := "/office/en/workspaces/" + otherID + "/profile-settings"

	for _, path := range []string{otherPath, "/office/en/workspaces/not-a-uuid/profile-settings"} {
		if response := f.do(request{path: path, cookies: cookies}); response.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d %q, want 404", path, response.Code, response.Header().Get("Location"))
		}
	}

	for _, tt := range []struct{ path, body string }{
		{otherPath + "/profile", profileBody(f, "Augusta", "King", testEmail, "")},
		{otherPath + "/password", passwordBody(testPassword, "battery staple", "battery staple")},
	} {
		response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body, cookies: cookies})

		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/workspaces" {
			t.Fatalf("POST %s = %d %q, want the workspace choice", tt.path, response.Code, response.Header().Get("Location"))
		}
	}

	if *f.user != before || f.version != version {
		t.Fatal("a non-member's request changed the user or the credentials")
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

	if response := f.do(request{path: "/office/en/profile-settings", cookies: old}); response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("old session = %d %q, want sign-in", response.Code, response.Header().Get("Location"))
	}

	response := f.do(request{path: "/office/en/profile-settings", cookies: renewed})
	body := response.Body.String()

	for _, want := range []string{"<p>" + confirmation + "</p>", "<p>You were signed out on your other devices.</p>"} {
		if response.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("settings with the new session = %d, lack %s:\n%s", response.Code, want, body)
		}
	}
}
