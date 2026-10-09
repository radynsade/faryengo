package officei18n

import (
	"context"
	"encoding/json"
	"maps"
	"net/url"
	"slices"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func readCatalog(t *testing.T, code string) map[string]map[string]string {
	t.Helper()

	var catalog map[string]map[string]string

	data, err := catalogs.ReadFile("locales/active." + code + ".json")

	if err == nil {
		err = json.Unmarshal(data, &catalog)
	}

	if err != nil {
		t.Fatal(err)
	}

	return catalog
}

func TestCatalogsDefineTheSameMessages(t *testing.T) {
	english := readCatalog(t, DefaultLanguage)

	for _, code := range Languages() {
		t.Run(code, func(t *testing.T) {
			catalog := readCatalog(t, code)

			if !slices.Equal(slices.Sorted(maps.Keys(catalog)), slices.Sorted(maps.Keys(english))) {
				t.Fatalf("%s messages differ from English", code)
			}

			for id, forms := range catalog {
				if strings.TrimSpace(forms["other"]) == "" {
					t.Fatalf("%s %s has no other form", code, id)
				}
			}
		})
	}
}

func TestLocale(t *testing.T) {
	for _, tt := range []struct {
		name     string
		code     string
		path     string
		language string
		message  string
		links    []string
	}{
		{
			name:     "selects a supported language",
			code:     "lv",
			path:     "/office/lv/sign-in?next=1",
			language: "lv",
			message:  "Pieslēgties",
			links:    []string{"/office/en/sign-in?next=1", "/office/lv/sign-in?next=1", "/office/ru/sign-in?next=1"},
		},
		{
			name:     "falls back to English",
			code:     "xx",
			path:     "/office/xx/sign-in",
			language: "en",
			message:  "Sign in",
			links:    []string{"/office/en/sign-in", "/office/lv/sign-in", "/office/ru/sign-in"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requestURL, err := url.Parse(tt.path)
			if err != nil {
				t.Fatal(err)
			}

			ctx := WithLocale(context.Background(), tt.code, requestURL)

			if Language(ctx) != tt.language {
				t.Fatalf("Language() = %q, want %q", Language(ctx), tt.language)
			}

			if T(ctx, "auth.sign_in") != tt.message {
				t.Fatalf("T() = %q, want %q", T(ctx, "auth.sign_in"), tt.message)
			}

			var hrefs []string

			for _, link := range LanguageLinks(ctx) {
				hrefs = append(hrefs, link.Href)

				if link.Active != (link.Code == tt.language) {
					t.Fatalf("link %s active = %v", link.Code, link.Active)
				}
			}

			if !slices.Equal(hrefs, tt.links) {
				t.Fatalf("LanguageLinks() = %v, want %v", hrefs, tt.links)
			}
		})
	}
}
