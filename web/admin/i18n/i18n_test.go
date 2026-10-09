package admini18n

import (
	"context"
	"encoding/json"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

//
// Catalogs
//

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
		catalog := readCatalog(t, code)

		if !slices.Equal(slices.Sorted(maps.Keys(catalog)), slices.Sorted(maps.Keys(english))) {
			t.Fatalf("%s messages differ from English", code)
		}

		for id, forms := range catalog {
			if strings.TrimSpace(forms["other"]) == "" {
				t.Fatalf("%s %s has no other form", code, id)
			}
		}
	}
}

// A message whose English text has plural forms needs every plural category
// of each language.

func TestCatalogsDefinePluralForms(t *testing.T) {
	categories := map[string][]string{
		"en": {"one", "other"},
		"lv": {"zero", "one", "other"},
		"ru": {"one", "few", "many", "other"},
	}

	english := readCatalog(t, DefaultLanguage)

	for _, code := range Languages() {
		catalog := readCatalog(t, code)

		for id, forms := range english {
			if _, plural := forms["one"]; plural {
				for _, category := range categories[code] {
					if catalog[id][category] == "" {
						t.Fatalf("%s %s lacks the %s form", code, id, category)
					}
				}
			}
		}
	}
}

var messageIDPattern = regexp.MustCompile(
	`"((?:common|document|navigation|actions|clipboard|pagination|fields|multiselect|auth|home|roles|permissions|errors|validation)\.[a-z_]+)"`,
)

func TestReferencedMessagesExist(t *testing.T) {
	english := readCatalog(t, DefaultLanguage)

	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == "dist") {
			err = filepath.SkipDir
		} else if err == nil && (strings.HasSuffix(path, ".templ") || strings.HasSuffix(path, ".go")) &&
			!strings.HasSuffix(path, "_templ.go") && !strings.HasSuffix(path, "_test.go") {
			var data []byte

			data, err = os.ReadFile(path)

			for _, match := range messageIDPattern.FindAllStringSubmatch(string(data), -1) {
				if _, found := english[match[1]]; !found {
					t.Errorf("%s references the missing message %s", path, match[1])
				}
			}
		}

		return err
	})

	if err != nil {
		t.Fatal(err)
	}
}

//
// Messages
//

func TestMessages(t *testing.T) {
	tests := []struct {
		name string
		code string
		got  func(ctx context.Context) string
		want string
	}{
		{"English", "en", func(ctx context.Context) string { return T(ctx, "actions.sign_in") }, "Sign in"},
		{"Latvian", "lv", func(ctx context.Context) string { return T(ctx, "actions.sign_in") }, "Pieslēgties"},
		{"unknown locale uses English", "de", func(ctx context.Context) string { return T(ctx, "actions.sign_in") }, "Sign in"},
		{"named values", "en", func(ctx context.Context) string {
			return T(ctx, "roles.deleted", map[string]any{"Name": "Editor"})
		}, `Role "Editor" deleted successfully.`},
		{"Russian plural few", "ru", func(ctx context.Context) string { return Count(ctx, "multiselect.selected", 3) }, "Выбрано 3 пункта"},
		{"Russian plural many", "ru", func(ctx context.Context) string { return Count(ctx, "multiselect.selected", 5) }, "Выбрано 5 пунктов"},
		{"Latvian plural zero", "lv", func(ctx context.Context) string { return Count(ctx, "multiselect.selected", 0) }, "Atlasītas 0 opcijas"},
		{"missing message", "en", func(ctx context.Context) string { return T(ctx, "missing.message") }, "missing.message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithLocale(context.Background(), tt.code, nil)

			if got := tt.got(ctx); got != tt.want {
				t.Fatalf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

//
// Language links
//

func TestLanguageLinks(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"keeps the page and filters", "/admin/lv/roles?permissions=view_role&permissions=manage_role&page=2", "/admin/ru/roles?permissions=view_role&permissions=manage_role&page=2"},
		{"keeps resource IDs", "/admin/en/roles/0190a7c4-5c1e-7b4e-9a40-6b3c1e2d4f5a/edit", "/admin/ru/roles/0190a7c4-5c1e-7b4e-9a40-6b3c1e2d4f5a/edit"},
		{"deletion leads to the role", "/admin/en/roles/0190a7c4-5c1e-7b4e-9a40-6b3c1e2d4f5a/delete", "/admin/ru/roles/0190a7c4-5c1e-7b4e-9a40-6b3c1e2d4f5a/view"},
		{"sign-out leads to sign-in", "/admin/en/sign-out", "/admin/ru/sign-in"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestURL, err := url.Parse(tt.path)

			if err != nil {
				t.Fatal(err)
			}

			links := LanguageLinks(WithLocale(context.Background(), strings.Split(tt.path, "/")[2], requestURL))

			if len(links) != len(Languages()) || links[2].Code != "ru" || links[2].Href != tt.want {
				t.Fatalf("links = %+v, want ru link %q", links, tt.want)
			}

			active := 0

			for _, link := range links {
				if link.Active {
					active++
				}
			}

			if active != 1 {
				t.Fatalf("%d links are active", active)
			}
		})
	}
}
