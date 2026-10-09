package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/users"
)

// Pages reachable by a signed-in super user, with the requests that render
// them.

func panelPages(f *fixture) []string {
	base := "/admin/en/roles/" + roleID(f.role)

	return []string{
		"/admin/en",
		"/admin/en/users",
		"/admin/en/roles",
		"/admin/en/roles/create",
		base + "/view",
		base + "/edit",
	}
}

func TestResponseForms(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		wantPartial bool
	}{
		{"direct load", nil, false},
		{"in-page update", partial, true},
		{"history restoration", map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, nil, true)
			cookies := f.signIn(t)
			paths := append(panelPages(f), "/admin/en/restore-password")

			for _, path := range paths {
				response := f.do(request{path: path, cookies: cookies, headers: tt.headers})
				body := response.Body.String()

				if response.Code != http.StatusOK {
					t.Fatalf("GET %s = %d", path, response.Code)
				}

				if vary := response.Header().Values("Vary"); !slices.Contains(vary, "HX-Request") || !slices.Contains(vary, "HX-History-Restore-Request") {
					t.Fatalf("GET %s Vary = %v", path, vary)
				}

				if isDocument := strings.HasPrefix(body, "<!doctype html>"); isDocument == tt.wantPartial {
					t.Fatalf("GET %s returned a document = %t: %.80s", path, isDocument, body)
				}

				if !strings.Contains(body, `<head hx-head="merge">`) || !strings.Contains(body, `id="page-content"`) {
					t.Fatalf("GET %s lacks the head or the content region", path)
				}

				if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Language") != "en" {
					t.Fatalf("GET %s headers = %v", path, response.Header())
				}
			}
		})
	}
}

// Every link and form inside the content region updates the page in place
// with morphing, keeping a native href or action for browsers without
// JavaScript. Language links are the one deliberate full navigation.

var (
	tagPattern       = regexp.MustCompile(`<(a|form)\s[^>]*>`)
	noscriptPattern  = regexp.MustCompile(`<noscript>.*?</noscript>`)
	attributePattern = regexp.MustCompile(`([a-z-]+)=(?:"([^"]*)"|'([^']*)')`)
)

func TestLinksAndFormsUpdateInPlace(t *testing.T) {
	f := newFixture(t, nil, true)
	cookies := f.signIn(t)
	paths := append(panelPages(f), "/admin/en/roles/table")
	guest := []string{"/admin/en/sign-in", "/admin/en/restore-password"}

	check := func(path, body string) {
		// Content for browsers without JavaScript is never enhanced.
		body = noscriptPattern.ReplaceAllString(body, "")

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
			case attributes["hx-post"] != "":
				if attributes["action"] != attributes["hx-post"] {
					t.Fatalf("GET %s: form action differs from its request: %s", path, tag)
				}
			case attributes["hx-target"] == "#confirm-delete":
			default:
				morphs := strings.HasPrefix(attributes["hx-swap"], "morph:")

				if !morphs || attributes["hx-target"] != "#page-content" || attributes["action"] == "" || attributes["method"] == "" {
					t.Fatalf("GET %s: form is not an in-page submission: %s", path, tag)
				}
			}
		}
	}

	for _, path := range paths {
		check(path, f.do(request{path: path, cookies: cookies}).Body.String())
	}

	for _, path := range guest {
		check(path, f.do(request{path: path}).Body.String())
	}
}

func TestResponsesAreMinimized(t *testing.T) {
	f := newFixture(t, nil, true)
	cookies := f.signIn(t)

	for _, headers := range []map[string]string{nil, partial} {
		for _, path := range append(panelPages(f), "/admin/en/roles/table") {
			body := f.do(request{path: path, cookies: cookies, headers: headers}).Body.String()

			if strings.ContainsAny(body, "\n\t") || strings.Contains(body, "<!--") || strings.Contains(body, ">  <") {
				t.Fatalf("GET %s is not minimized: %.200s", path, body)
			}
		}
	}

	// Behavior attributes and their values survive minimization unchanged.
	form := f.do(request{path: "/admin/en/roles/create", cookies: cookies}).Body.String()

	for _, want := range []string{
		`hx-swap="morph:outerHTML show:none"`,
		`hx-disabled-elt="find button[type='submit']"`,
		`data-translations-input`,
		`data-language="en"`,
		`id="role-name-panel-lv"`,
	} {
		if !strings.Contains(form, want) {
			t.Fatalf("minimized form lost %s", want)
		}
	}

	var statuses []string

	match := regexp.MustCompile(`data-selected-status='([^']*)'`).FindStringSubmatch(form)

	if match == nil || json.Unmarshal([]byte(match[1]), &statuses) != nil || len(statuses) != len(users.AllPermissions())+1 {
		t.Fatalf("multiselect statuses were changed: %v", match)
	}
}

// A page in Latvian or Russian must not contain an English catalog message
// that the language translates differently.

func TestPagesUseOnlyTheirLanguage(t *testing.T) {
	english := catalogMessages(t, "en")

	for _, language := range []string{"lv", "ru"} {
		t.Run(language, func(t *testing.T) {
			translated := catalogMessages(t, language)
			f := newFixture(t, nil, true)
			cookies := f.signIn(t)
			base := "/admin/" + language

			var bodies []string

			for _, path := range []string{"/sign-in", "/restore-password"} {
				bodies = append(bodies, f.do(request{path: base + path}).Body.String())
			}

			for _, path := range panelPages(f) {
				bodies = append(bodies, f.do(request{path: strings.Replace(path, "/admin/en", base, 1), cookies: cookies}).Body.String())
			}

			bodies = append(bodies, f.do(request{path: base + "/roles/table", cookies: cookies}).Body.String())
			bodies = append(bodies, f.do(request{method: http.MethodPost, path: base + "/roles/create", body: "name[en]=+", cookies: cookies}).Body.String())

			for id, message := range english {
				if strings.Contains(message, "{{") || message == translated[id] || len(message) < 4 {
					continue
				}

				for _, body := range bodies {
					if strings.Contains(body, ">"+message+"<") || strings.Contains(body, `"`+message+`"`) {
						t.Fatalf("%s page contains the English %s message %q", language, id, message)
					}
				}
			}

			if !strings.Contains(bodies[0], `<html lang="`+language+`">`) {
				t.Fatal("the document language does not match the route")
			}
		})
	}
}

func catalogMessages(t *testing.T, language string) map[string]string {
	t.Helper()

	var catalog map[string]map[string]string

	data, err := os.ReadFile("../i18n/locales/active." + language + ".json")
	must(t, err)
	must(t, json.Unmarshal(data, &catalog))

	messages := make(map[string]string, len(catalog))

	for id, forms := range catalog {
		messages[id] = forms["other"]
	}

	return messages
}
