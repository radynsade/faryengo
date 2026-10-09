package officei18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//
// Catalogs
//

// The catalogs are embedded, so a deployed binary needs no translation files.
// English is the fallback for unknown locales and missing messages.

const DefaultLanguage = "en"

//go:embed locales/*.json
var catalogs embed.FS

var (
	bundle    = goi18n.NewBundle(language.English)
	supported = []supportedLanguage{
		{code: "en", name: "English"},
		{code: "lv", name: "Latviešu"},
		{code: "ru", name: "Русский"},
	}
)

type supportedLanguage struct {
	code string
	name string
}

func init() {
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	for _, item := range supported {
		if _, err := bundle.LoadMessageFileFS(catalogs, "locales/active."+item.code+".json"); err != nil {
			panic(fmt.Errorf("load office %s translations: %w", item.code, err))
		}
	}
}

func Languages() []string {
	codes := make([]string, 0, len(supported))

	for _, item := range supported {
		codes = append(codes, item.code)
	}

	return codes
}

//
// Request locale
//

type contextKey struct{}

type requestLocale struct {
	code      string
	path      string
	query     string
	localizer *goi18n.Localizer
}

// The locale keeps the request's path and query, so language links can point
// to the same page.

func WithLocale(ctx context.Context, code string, requestURL *url.URL) context.Context {
	selected := DefaultLanguage

	for _, item := range supported {
		if item.code == code {
			selected = item.code
		}
	}

	locale := requestLocale{
		code:      selected,
		path:      "/office/" + selected,
		localizer: goi18n.NewLocalizer(bundle, selected),
	}

	if requestURL != nil {
		locale.path, locale.query = requestURL.Path, requestURL.RawQuery
	}

	return context.WithValue(ctx, contextKey{}, locale)
}

func Language(ctx context.Context) string {
	return fromContext(ctx).code
}

//
// Messages
//

func T(ctx context.Context, id string, data ...map[string]any) string {
	config := &goi18n.LocalizeConfig{MessageID: id}

	if len(data) > 0 {
		config.TemplateData = data[0]
	}

	return translate(ctx, config)
}

//
// Language links
//

type LanguageLink struct {
	Code   string
	Name   string
	Href   string
	Active bool
}

func LanguageLinks(ctx context.Context) []LanguageLink {
	locale := fromContext(ctx)
	parts := strings.Split(locale.path, "/")

	if len(parts) < 3 || parts[1] != "office" {
		parts = []string{"", "office", locale.code}
	}

	links := make([]LanguageLink, 0, len(supported))

	for _, item := range supported {
		parts[2] = item.code
		href := (&url.URL{Path: strings.Join(parts, "/"), RawQuery: locale.query}).String()

		links = append(links, LanguageLink{
			Code:   item.code,
			Name:   item.name,
			Href:   href,
			Active: item.code == locale.code,
		})
	}

	return links
}

//
// Helpers
//

func fromContext(ctx context.Context) requestLocale {
	locale, found := ctx.Value(contextKey{}).(requestLocale)

	if !found {
		locale = requestLocale{
			code:      DefaultLanguage,
			path:      "/office/" + DefaultLanguage,
			localizer: goi18n.NewLocalizer(bundle, DefaultLanguage),
		}
	}

	return locale
}

// go-i18n can return the fallback text together with a missing-translation
// error, so only an empty result falls back to the message ID.

func translate(ctx context.Context, config *goi18n.LocalizeConfig) string {
	text, err := fromContext(ctx).localizer.Localize(config)

	if err != nil && text == "" {
		text = config.MessageID
	}

	return text
}
