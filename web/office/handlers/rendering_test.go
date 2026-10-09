package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Pages reachable by a signed-in user, and pages for visitors.

var (
	dashboardPages = []string{
		"/office/en",
		"/office/en/budgets",
		"/office/en/transactions",
		"/office/en/goals",
		"/office/en/reports",
		"/office/en/settings",
	}
	guestPages = []string{"/office/en/sign-in"}
	partial    = map[string]string{"HX-Request": "true"}
	toContent  = map[string]string{"HX-Request": "true", "HX-Target": "dashboard-main"}
)

func TestResponseForms(t *testing.T) {
	for _, tt := range []struct {
		name        string
		headers     map[string]string
		wantPartial bool
	}{
		{"direct load", nil, false},
		{"in-page update", partial, true},
		{"history restoration", map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)

			for _, path := range append(slices.Clone(dashboardPages), guestPages...) {
				requestCookies := cookies

				if slices.Contains(guestPages, path) {
					requestCookies = nil
				}

				response := f.do(request{path: path, cookies: requestCookies, headers: tt.headers})
				body := response.Body.String()

				if response.Code != http.StatusOK {
					t.Fatalf("GET %s = %d", path, response.Code)
				}

				if vary := response.Header().Values("Vary"); !slices.Contains(vary, "HX-Request") || !slices.Contains(vary, "HX-History-Restore-Request") || !slices.Contains(vary, "HX-Target") {
					t.Fatalf("GET %s Vary = %v", path, vary)
				}

				if isDocument := strings.HasPrefix(body, "<!doctype html>"); isDocument == tt.wantPartial {
					t.Fatalf("GET %s returned a document = %t: %.80s", path, isDocument, body)
				}

				if !strings.Contains(body, `<head hx-head="merge">`) || !strings.Contains(body, `id="page-content"`) {
					t.Fatalf("GET %s lacks the head or the page region", path)
				}
			}
		})
	}
}

// Every link and form updates the page in place with morphing, keeping a
// native href or action for browsers without JavaScript. Language links are
// the one deliberate full navigation.

var (
	tagPattern       = regexp.MustCompile(`<(a|form)\s[^>]*>`)
	attributePattern = regexp.MustCompile(`([a-z-]+)=(?:"([^"]*)"|'([^']*)')`)
)

func TestLinksAndFormsUpdateInPlace(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	check := func(path, body string) {
		for _, tag := range tagPattern.FindAllString(body, -1) {
			attributes := make(map[string]string)

			for _, match := range attributePattern.FindAllStringSubmatch(tag, -1) {
				attributes[match[1]] = match[2] + match[3]
			}

			switch {
			case attributes["hreflang"] != "":
				if attributes["hx-boost"] != "false" {
					t.Fatalf("GET %s: language link is boosted: %s", path, tag)
				}
			case strings.HasPrefix(attributes["href"], "#"):
			case strings.HasPrefix(tag, "<a"):
				if attributes["hx-boost"] != "true" || !strings.HasPrefix(attributes["hx-swap"], "morph:") || attributes["href"] == "" {
					t.Fatalf("GET %s: link is not an in-page navigation: %s", path, tag)
				}
			default:
				if !strings.HasPrefix(attributes["hx-swap"], "morph:") || attributes["action"] == "" || attributes["method"] == "" || attributes["hx-sync"] != "this:drop" {
					t.Fatalf("GET %s: form is not an in-page submission: %s", path, tag)
				}
			}
		}
	}

	// Links and forms inherit their target: the whole page region from the
	// document, the main region inside the dashboard. Signing out leaves the
	// dashboard, so it names the page region itself.
	for _, path := range dashboardPages {
		body := f.do(request{path: path, cookies: cookies}).Body.String()

		for _, want := range []string{
			`<body hx-ext="head-support, morph" hx-history="false" hx-target="#page-content">`,
			`<div class="dashboard" hx-target="#dashboard-main"`,
			`action="/office/en/sign-out" hx-target="#page-content"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("GET %s lacks %s", path, want)
			}
		}

		check(path, body)
	}

	for _, path := range guestPages {
		check(path, f.do(request{path: path}).Body.String())
	}
}

// Inside the dashboard only the main region changes; the sidebar parts that
// depend on the page arrive as out-of-band morphs.

func TestDashboardNavigationReturnsOnlyContent(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{path: "/office/en/goals", cookies: cookies, headers: toContent})
	body := response.Body.String()

	for _, want := range []string{
		`<head hx-head="merge">`,
		`<title>Goals · Faryen</title>`,
		`<main id="dashboard-main"`,
		`<h1 id="page-title">Goals</h1>`,
		`<nav id="dashboard-menu" class="sidebar__nav" aria-label="Main menu" hx-swap-oob="morph">`,
		`<div id="dashboard-account" class="sidebar__foot" hx-swap-oob="morph">`,
		`href="/office/en/goals" aria-current="page"`,
		`href="/office/lv/goals"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("partial lacks %s:\n%s", want, body)
		}
	}

	for _, unwanted := range []string{`id="page-content"`, "dashboard__topbar", "sidebar__head"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("partial contains %s", unwanted)
		}
	}

	if response.Header().Get("HX-Retarget") != "" {
		t.Fatalf("HX-Retarget = %q", response.Header().Get("HX-Retarget"))
	}
}

// A request from inside the dashboard that ends on another layout, such as
// the sign-in page after the session ended, replaces the whole page region.

func TestLeavingTheDashboardReplacesThePage(t *testing.T) {
	f := newFixture(t)

	response := f.do(request{path: "/office/en", headers: toContent})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("GET /office/en = %d %q", response.Code, response.Header().Get("Location"))
	}

	// The browser follows the redirect with the same headers.
	response = f.do(request{path: "/office/en/sign-in", headers: toContent})

	if response.Header().Get("HX-Retarget") != "#page-content" || response.Header().Get("HX-Reswap") != "morph:outerHTML show:none" {
		t.Fatalf("headers = %v", response.Header())
	}

	if !strings.Contains(response.Body.String(), `<div class="auth" data-navigation-error=`) {
		t.Fatalf("body lacks the page region:\n%s", response.Body.String())
	}
}

func TestResponsesAreMinimized(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	for _, tt := range []struct {
		path    string
		cookies []*http.Cookie
		headers map[string]string
	}{
		{path: "/office/en", cookies: cookies},
		{path: "/office/en/budgets", cookies: cookies, headers: toContent},
		{path: "/office/en/sign-in"},
		{path: "/office/en/sign-in", headers: partial},
	} {
		body := f.do(request{path: tt.path, cookies: tt.cookies, headers: tt.headers}).Body.String()

		if strings.Contains(body, "\n") || strings.Contains(body, "\t") || strings.Contains(body, "<!--") {
			t.Fatalf("GET %s is not minimized", tt.path)
		}

		if !strings.Contains(body, `hx-swap="morph:outerHTML show:none"`) {
			t.Fatalf("GET %s lost its behavior attributes", tt.path)
		}
	}
}

// A failed in-page sign-in replaces only the form, keeping the address.

func TestFailedSubmissionsReplaceOnlyTheForm(t *testing.T) {
	f := newFixture(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    "/office/en/sign-in",
		body:    "email=" + testEmail + "&password=wrong",
		headers: map[string]string{"HX-Request": "true", "HX-Target": "page-content"},
	})
	body := response.Body.String()

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}

	for header, want := range map[string]string{
		"HX-Retarget":  "#sign-in-form",
		"HX-Reswap":    "morph:outerHTML",
		"HX-Push-Url":  "false",
		"Content-Type": "text/html; charset=utf-8",
	} {
		if response.Header().Get(header) != want {
			t.Fatalf("%s = %q, want %q", header, response.Header().Get(header), want)
		}
	}

	if !strings.HasPrefix(body, `<form id="sign-in-form"`) || strings.Contains(body, "<head") {
		t.Fatalf("body is not only the form:\n%s", body)
	}

	if !strings.Contains(body, "The email or password is incorrect.") {
		t.Fatalf("body lacks the error:\n%s", body)
	}
}

// Every page renders in every language without English text, apart from
// application data and texts that read the same in both languages.

func TestPagesUseOnlyTheirLanguage(t *testing.T) {
	english := readMessages(t, "en")

	for _, code := range []string{"lv", "ru"} {
		t.Run(code, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			translated := readMessages(t, code)

			for _, page := range append(slices.Clone(dashboardPages), guestPages...) {
				path := strings.Replace(page, "/en", "/"+code, 1)
				body := f.do(request{path: path, cookies: cookies}).Body.String()

				if slices.Contains(guestPages, page) {
					body = f.do(request{path: path}).Body.String()
				}

				for id, text := range english {
					if text != translated[id] && !strings.Contains(text, "{{") && strings.Contains(body, ">"+text+"<") {
						t.Fatalf("GET %s shows English %s: %q", path, id, text)
					}
				}
			}
		})
	}
}

func readMessages(t *testing.T, code string) map[string]string {
	t.Helper()

	var catalog map[string]map[string]string

	data, err := os.ReadFile("../i18n/locales/active." + code + ".json")

	if err == nil {
		err = json.Unmarshal(data, &catalog)
	}

	if err != nil {
		t.Fatal(err)
	}

	messages := make(map[string]string, len(catalog))

	for id, forms := range catalog {
		messages[id] = forms["other"]
	}

	return messages
}
