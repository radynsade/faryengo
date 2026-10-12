package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Every signed-in user may open their settings, even with no permissions.

func TestSettingsPage(t *testing.T) {
	f := newFixture(t, nil, false)
	cookies := f.signIn(t)

	response := f.do(request{path: "/admin/en/settings", cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}

	for _, want := range []string{
		"<title>Settings · Faryen Admin</title>",
		`<h1 id="page-heading" class="page-title__heading">Settings</h1>`,
		`<h2 id="profile-title" class="settings-page__title">Profile</h2>`,
		`<h2 id="password-title" class="settings-page__title">Password change</h2>`,
		`<form id="profile-form" class="settings-form" method="post" action="/admin/en/settings/profile" novalidate data-confirm-password="email"`,
		`<form id="password-form" class="settings-form" method="post" action="/admin/en/settings/password" novalidate data-confirm-password="always"`,
		`value="Ada"`,
		`value="Lovelace"`,
		`value="` + testEmail + `"`,
		`<input type="hidden" name="updated_at" value="` + f.user.UpdatedAt.UTC().Format(time.RFC3339Nano) + `">`,
		`<dialog id="profile-confirm" class="admin-dialog password-confirm" aria-labelledby="profile-confirm-title" aria-describedby="profile-confirm-message" data-dialog data-confirm-dialog>`,
		`<dialog id="password-confirm" class="admin-dialog password-confirm"`,
		`<button class="button button--primary" type="submit" name="confirm" value="1" data-confirm-submit>`,
		`<p class="form-field__help" id="password-new-help">Use at least 6 characters.</p>`,
		`name="new_password_repeat" type="password" placeholder autocomplete="new-password" required data-must-match="new_password"`,
		`autocomplete="current-password" required autofocus`,
		`<a class="panel-sidebar__settings" href="/admin/en/settings" aria-label="Settings" title="Settings"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	for _, unwanted := range []string{`data-confirm-dialog open`, `data-dialog-open`, `name="current_password" type="password" placeholder autocomplete="current-password" value=`} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body contains %s", unwanted)
		}
	}

	// The account row is sent again only by the settings page, which a saved
	// profile redirects to.
	partial := f.do(request{path: "/admin/en/settings", cookies: cookies, headers: map[string]string{"HX-Request": "true", "HX-Target": "panel-main"}}).Body.String()

	if !strings.Contains(partial, `<section id="panel-account" class="panel-sidebar__account" aria-label="Your account" hx-swap-oob="morph">`) {
		t.Fatalf("settings do not refresh the account row:\n%s", partial)
	}
}

// Names alone keep every Session; the confirmation shows once, in the success
// dialog, and the sidebar shows the new name.

func TestUpdateProfileNames(t *testing.T) {
	f := newFixture(t, nil, false)
	cookies := f.signIn(t)
	version := f.version

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/admin/en/settings/profile",
		body:    adminProfileBody(f, "Augusta", "King", testEmail, ""),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en/settings" {
		t.Fatalf("save = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if f.user.FirstName != "Augusta" || f.version != version || len(response.Result().Cookies()) != 0 {
		t.Fatalf("user %s, version replaced %t, cookies %v", f.user.FirstName, f.version != version, response.Result().Cookies())
	}

	body := f.do(request{path: "/admin/en/settings", cookies: cookies}).Body.String()

	for _, want := range []string{`<p class="success-dialog__message">Your profile has been saved.</p>`, `<p class="panel-sidebar__name">Augusta King</p>`} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings lack %s:\n%s", want, body)
		}
	}

	if strings.Contains(f.do(request{path: "/admin/en/settings", cookies: cookies}).Body.String(), "success-dialog") {
		t.Fatal("the confirmation shows again after a reload")
	}
}

func TestUpdateProfileEmail(t *testing.T) {
	f := newFixture(t, nil, false)
	cookies := f.signIn(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/admin/en/settings/profile",
		body:    adminProfileBody(f, "Ada", "Lovelace", "ada@lovelace.example", testPassword) + "&confirm=1",
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || f.user.Email != "ada@lovelace.example" {
		t.Fatalf("save = %d, email %q: %s", response.Code, f.user.Email, response.Body.String())
	}

	assertAdminSessionRenewed(t, f, cookies, response.Result().Cookies(), "Your profile has been saved.")
}

func TestChangePassword(t *testing.T) {
	f := newFixture(t, nil, false)
	cookies := f.signIn(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/admin/en/settings/password",
		body:    adminPasswordBody(testPassword, "battery staple", "battery staple") + "&confirm=1",
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || f.user.PasswordHash != "hash:battery staple" {
		t.Fatalf("change = %d, hash %q: %s", response.Code, f.user.PasswordHash, response.Body.String())
	}

	assertAdminSessionRenewed(t, f, cookies, response.Result().Cookies(), "Your password has been changed.")
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
			path:     "/admin/en/settings/profile",
			body:     func(f *fixture) string { return adminProfileBody(f, " ", "Lovelace", testEmail, "") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<p class="form-field__error">This field is required.</p>`, `aria-describedby="profile-first-name-errors"`},
		},
		{
			name:     "new email asks for the password",
			path:     "/admin/en/settings/profile",
			body:     func(f *fixture) string { return adminProfileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`data-confirm-dialog open data-dialog-open>`, `value="ada@lovelace.example"`},
			excludes: []string{`id="profile-confirm-password-errors"`},
		},
		{
			name: "new email confirmed without the password",
			path: "/admin/en/settings/profile",
			body: func(f *fixture) string {
				return adminProfileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "") + "&confirm=1"
			},
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="profile-confirm-password-errors"`, "This field is required."},
		},
		{
			name: "new email with a wrong password",
			path: "/admin/en/settings/profile",
			body: func(f *fixture) string {
				return adminProfileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "wrong horse") + "&confirm=1"
			},
			status:   http.StatusUnprocessableEntity,
			contains: []string{`data-confirm-dialog open data-dialog-open>`, "The password is incorrect."},
		},
		{
			name: "outdated profile",
			path: "/admin/en/settings/profile",
			body: func(f *fixture) string {
				return strings.Replace(adminProfileBody(f, "Augusta", "King", testEmail, ""),
					url.QueryEscape(f.user.UpdatedAt.UTC().Format(time.RFC3339Nano)),
					url.QueryEscape(f.user.UpdatedAt.Add(-time.Hour).UTC().Format(time.RFC3339Nano)), 1)
			},
			status:   http.StatusConflict,
			contains: []string{`<p class="notice notice--error" role="alert">Your profile changed in the meantime. Reload the page and try again.</p>`},
		},
		{
			name:     "malformed profile",
			path:     "/admin/en/settings/profile",
			body:     func(f *fixture) string { return adminProfileBody(f, "Ada", "Lovelace", testEmail, "") + "&role=admin" },
			status:   http.StatusBadRequest,
			contains: []string{"Submit a valid form."},
		},
		{
			name:     "different new passwords",
			path:     "/admin/en/settings/password",
			body:     func(*fixture) string { return adminPasswordBody("", "battery staple", "battery stable") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{"The passwords do not match."},
			excludes: []string{`data-dialog-open`},
		},
		{
			name:     "short new password",
			path:     "/admin/ru/settings/password",
			body:     func(*fixture) string { return adminPasswordBody("", "short", "short") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="password-new-errors"`},
			excludes: []string{`data-dialog-open`},
		},
		{
			name:     "new password asks for the current one",
			path:     "/admin/en/settings/password",
			body:     func(*fixture) string { return adminPasswordBody("", "battery staple", "battery staple") },
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<dialog id="password-confirm"`, `data-confirm-dialog open data-dialog-open>`},
		},
		{
			name: "wrong current password",
			path: "/admin/en/settings/password",
			body: func(*fixture) string {
				return adminPasswordBody("wrong horse", "battery staple", "battery staple") + "&confirm=1"
			},
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="password-confirm-password-errors"`, "The password is incorrect."},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, nil, false)
			cookies := f.signIn(t)
			before, version := *f.user, f.version

			response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body(f), cookies: cookies})
			body := response.Body.String()

			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d:\n%s", response.Code, tt.status, body)
			}

			if *f.user != before || f.version != version {
				t.Fatal("a rejected change altered the user or the credentials")
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

// A failed in-page submission replaces only its form, or only its
// confirmation when the current password is missing or wrong, so the form
// keeps what was typed.

func TestSettingsReplaceOnlyTheForm(t *testing.T) {
	f := newFixture(t, nil, false)
	cookies := f.signIn(t)

	for _, tt := range []struct {
		path   string
		body   string
		region string
		tag    string
	}{
		{path: "/admin/en/settings/profile", body: adminProfileBody(f, "", "Lovelace", testEmail, ""), region: "profile-form", tag: "form"},
		{path: "/admin/en/settings/password", body: adminPasswordBody("", "", ""), region: "password-form", tag: "form"},
		{
			path:   "/admin/en/settings/profile",
			body:   adminProfileBody(f, "Ada", "Lovelace", "ada@lovelace.example", "wrong horse") + "&confirm=1",
			region: "profile-confirm",
			tag:    "dialog",
		},
		{path: "/admin/en/settings/password", body: adminPasswordBody("", "battery staple", "battery staple"), region: "password-confirm", tag: "dialog"},
	} {
		response := f.do(request{
			method:  http.MethodPost,
			path:    tt.path,
			body:    tt.body,
			cookies: cookies,
			headers: map[string]string{"HX-Request": "true", "HX-Target": "panel-main"},
		})
		body := response.Body.String()

		if response.Header().Get("HX-Retarget") != "#"+tt.region || !strings.HasPrefix(body, `<`+tt.tag+` id="`+tt.region+`"`) || strings.Contains(body, `<head hx-head`) {
			t.Fatalf("POST %s = %d %v, body is not only the region:\n%s", tt.path, response.Code, response.Header(), body)
		}
	}
}

func TestSettingsVisitors(t *testing.T) {
	f := newFixture(t, nil, false)

	for _, path := range []string{"/admin/en/settings", "/admin/en/settings/profile", "/admin/en/settings/password"} {
		method := http.MethodPost

		if path == "/admin/en/settings" {
			method = http.MethodGet
		}

		response := f.do(request{method: method, path: path, body: "current_password=x"})

		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/en/sign-in" {
			t.Fatalf("%s %s as a visitor = %d %q", method, path, response.Code, response.Header().Get("Location"))
		}
	}
}

//
// Helpers
//

func adminProfileBody(f *fixture, firstName, lastName, email, password string) string {
	values := url.Values{
		"first_name":       {firstName},
		"last_name":        {lastName},
		"email":            {email},
		"current_password": {password},
		"updated_at":       {f.user.UpdatedAt.UTC().Format(time.RFC3339Nano)},
	}

	return values.Encode()
}

func adminPasswordBody(current, next, repeat string) string {
	values := url.Values{
		"current_password":    {current},
		"new_password":        {next},
		"new_password_repeat": {repeat},
	}

	return values.Encode()
}

// The old Session no longer authenticates, the new cookie does, and the
// settings show the change's confirmation with the sign-out notice once.

func assertAdminSessionRenewed(t *testing.T, f *fixture, old, renewed []*http.Cookie, confirmation string) {
	t.Helper()

	if len(renewed) != 1 || renewed[0].Value == "" || renewed[0].Value == old[0].Value {
		t.Fatalf("cookies = %v, want one new session cookie", renewed)
	}

	if response := f.do(request{path: "/admin/en/settings", cookies: old}); response.Header().Get("Location") != "/admin/en/sign-in" {
		t.Fatalf("old session = %d %q, want sign-in", response.Code, response.Header().Get("Location"))
	}

	response := f.do(request{path: "/admin/en/settings", cookies: renewed})
	body := response.Body.String()

	for _, want := range []string{
		`<p class="success-dialog__message">` + confirmation + `</p>`,
		`<p class="success-dialog__message">You were signed out on your other devices.</p>`,
	} {
		if response.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("settings with the new session = %d, lack %s:\n%s", response.Code, want, body)
		}
	}
}
