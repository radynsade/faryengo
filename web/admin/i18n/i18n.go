// Package admini18n owns the admin interface catalogs, independently of domain translations.
package admini18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var catalogs embed.FS

var bundle = goi18n.NewBundle(language.English)

type contextKey struct{}

type requestLocale struct {
	code      string
	path      string
	query     string
	localizer *goi18n.Localizer
}

type LanguageLink struct {
	Code   string
	Name   string
	Href   string
	Active bool
}

var supported = []struct{ code, name string }{{"en", "English"}, {"lv", "Latviešu"}, {"ru", "Русский"}}

func init() {
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	for _, item := range supported {
		if _, err := bundle.LoadMessageFileFS(catalogs, "locales/active."+item.code+".json"); err != nil {
			panic(fmt.Errorf("load admin %s translations: %w", item.code, err))
		}
	}
}

// WithRequest selects the interface locale from the route; unknown locales use English.
func WithRequest(request *http.Request) context.Context {
	code := "en"

	for _, item := range supported {
		if item.code == request.PathValue("language") {
			code = item.code
		}
	}

	return context.WithValue(request.Context(), contextKey{}, requestLocale{
		code: code, path: request.URL.Path, query: request.URL.RawQuery,
		localizer: goi18n.NewLocalizer(bundle, code),
	})
}

func fromContext(ctx context.Context) requestLocale {
	locale, found := ctx.Value(contextKey{}).(requestLocale)

	if !found {
		locale = requestLocale{code: "en", path: "/admin/en/sign-in", localizer: goi18n.NewLocalizer(bundle, "en")}
	}

	return locale
}

func Language(ctx context.Context) string {
	return fromContext(ctx).code
}

// T formats a catalog message, with optional named template values.
func T(ctx context.Context, id string, data ...map[string]any) string {
	config := &goi18n.LocalizeConfig{MessageID: id}

	if len(data) > 0 {
		config.TemplateData = data[0]
	}

	return translate(ctx, config)
}

func Count(ctx context.Context, id string, count int) string {
	return translate(ctx, &goi18n.LocalizeConfig{MessageID: id, PluralCount: count, TemplateData: map[string]any{"Count": count}})
}

func translate(ctx context.Context, config *goi18n.LocalizeConfig) string {
	text, err := fromContext(ctx).localizer.Localize(config)

	// go-i18n can return the English fallback together with a missing-translation error.
	if err != nil && text == "" {
		text = config.MessageID
	}

	return text
}

// LanguageLinks preserves route parameters and filters, using only navigable GET routes.
func LanguageLinks(ctx context.Context) []LanguageLink {
	locale := fromContext(ctx)
	parts := strings.Split(locale.path, "/")

	if len(parts) < 3 || parts[1] != "admin" {
		parts = []string{"", "admin", locale.code, "sign-in"}
	}

	last := len(parts) - 1

	switch parts[last] {
	case "delete":
		parts[last] = "view"
	case "refresh", "sign-out":
		parts[last] = "sign-in"
	}

	links := make([]LanguageLink, 0, len(supported))

	for _, item := range supported {
		parts[2] = item.code
		href := (&url.URL{Path: strings.Join(parts, "/"), RawQuery: locale.query}).String()
		links = append(links, LanguageLink{Code: item.code, Name: item.name, Href: href, Active: item.code == locale.code})
	}

	return links
}
