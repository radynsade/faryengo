package admini18n

import (
	"encoding/json"
	"maps"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCatalogCoverage(t *testing.T) {
	data, err := catalogs.ReadFile("locales/active.en.json")

	if err != nil {
		t.Fatal(err)
	}

	var english map[string]json.RawMessage

	if err := json.Unmarshal(data, &english); err != nil {
		t.Fatal(err)
	}

	ids := slices.Sorted(maps.Keys(english))

	for _, item := range supported {
		t.Run(item.code, func(t *testing.T) {
			data, err := catalogs.ReadFile("locales/active." + item.code + ".json")

			if err != nil {
				t.Fatal(err)
			}

			var messages map[string]json.RawMessage

			if err := json.Unmarshal(data, &messages); err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(ids, slices.Sorted(maps.Keys(messages))) {
				t.Fatal("catalog IDs differ from English")
			}

			localizer := goi18n.NewLocalizer(bundle, item.code)

			for _, id := range ids {
				for _, count := range []int{0, 1, 2, 5, 11, 21, 101} {
					config := &goi18n.LocalizeConfig{
						MessageID:    id,
						TemplateData: map[string]any{"Name": "Test", "Label": "Name", "Language": "English", "Count": count},
					}

					if id == "multiselect.selected" {
						config.PluralCount = count
					}

					text, tag, err := localizer.LocalizeWithTag(config)

					if err != nil || tag.String() != item.code || text == "" || strings.Contains(text, "<no value>") {
						t.Fatalf("%s count %d = %q (%s): %v", id, count, text, tag, err)
					}
				}
			}
		})
	}
}

func TestLanguageAndPlurals(t *testing.T) {
	for _, tt := range []struct {
		code  string
		count int
		want  string
	}{
		{"en", 0, "0 selected"}, {"en", 1, "1 selected"},
		{"lv", 0, "Atlasītas 0 opcijas"}, {"lv", 1, "Atlasīta 1 opcija"}, {"lv", 2, "Atlasītas 2 opcijas"},
		{"ru", 1, "Выбран 1 пункт"}, {"ru", 2, "Выбрано 2 пункта"}, {"ru", 5, "Выбрано 5 пунктов"},
		{"ru", 21, "Выбран 21 пункт"}, {"unknown", 1, "1 selected"},
	} {
		t.Run(tt.want+tt.code, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/admin/"+tt.code+"/sign-in", nil)
			request.SetPathValue("language", tt.code)
			ctx := WithRequest(request)

			if got := Count(ctx, "multiselect.selected", tt.count); got != tt.want {
				t.Fatalf("count = %q, want %q", got, tt.want)
			}

			if tt.code == "unknown" && Language(ctx) != "en" {
				t.Fatal("unknown locale did not fall back to English")
			}
		})
	}
}

func TestLanguageLinks(t *testing.T) {
	for _, tt := range []struct{ name, path, target string }{
		{"filtered collection", "/roles?permissions=view_user&permissions=view_role&name=%C4%80&page=2&sort=name&order=desc", "/roles"},
		{"edit", "/roles/abc/edit", "/roles/abc/edit"},
		{"create", "/roles/create", "/roles/create"},
		{"delete error", "/roles/abc/delete", "/roles/abc/view"},
		{"sign out error", "/sign-out", "/sign-in"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/admin/lv"+tt.path, nil)
			request.SetPathValue("language", "lv")
			links := LanguageLinks(WithRequest(request))

			if len(links) != 3 {
				t.Fatalf("links = %v", links)
			}

			for index, link := range links {
				parsed, err := url.Parse(link.Href)

				if err != nil {
					t.Fatal(err)
				}

				if link.Code != supported[index].code || parsed.Path != "/admin/"+link.Code+tt.target || parsed.RawQuery != request.URL.RawQuery || link.Active != (link.Code == "lv") || parsed.Host != "" {
					t.Fatalf("invalid link: %+v", link)
				}
			}
		})
	}
}
