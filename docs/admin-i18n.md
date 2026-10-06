# Admin interface translations

The admin uses [go-i18n v2](https://github.com/nicksnyder/go-i18n) with embedded
JSON catalogs in `web/admin/i18n/locales/active.{en,lv,ru}.json`. English is the
fallback. The supported locales and native display names are defined in
`web/admin/i18n/i18n.go`. This follows Monoshop's English, Latvian, and Russian
interface and language links that retain the current route and query parameters.

The route `/admin/{language}/...` selects the interface language. The route
wrapper adds a localizer to the request context before authentication or rendering,
and sets `Content-Language`. Full documents set `html[lang]`; fragments carry
`data-ui-language` and the translated navigation error so HTMX navigation can
update the document metadata and feedback. Unknown interface locales use English
without changing the existing domain language selection.

Templates and handlers use `admini18n.T(ctx, "actions.sign_in")`. Message IDs are
grouped by responsibility: `common`, `navigation`, `actions`, `pagination`,
`fields`, `auth`, `home`, `roles`, `permissions`, `multiselect`, and `errors`.
Named values use go-i18n templates, for example `{{.Name}}`, and a
`map[string]any` passed to `T`. Translate complete sentences rather than
concatenating translated fragments. Templ escapes interpolated strings when
rendering HTML. Role names and user identity remain application data.

For plural messages, call `admini18n.Count(ctx, id, count)`. Catalogs provide the
language's plural categories (`one`/`other` for English, `zero`/`one`/`other` for
Latvian, and `one`/`few`/`many`/`other` for Russian). The multiselect renders its
initial status and translated removal labels on the server, and includes localized
statuses for every possible selection count. JavaScript reads these messages when
state changes; it contains no English copy or separate pluralization rules.

The shared `LanguageSwitcher` component renders native links in the panel sidebar
and authentication layout. It marks the selected language, uses native language
names, and preserves resource IDs and all query parameters, including repeated
permission filters. Mutation-only routes map to a navigable destination: role
deletion to role viewing, and sign-out to sign-in. Language changes use
full navigation so the entire document uses the new language. Links work without
JavaScript; switching pages does not save unsaved form values.

To add or update copy, add the same semantic ID to all catalogs, translate its
complete message and any plural forms, and reference the ID from the template or
handler. To add a locale, add its catalog and supported-language entry. The catalogs
are embedded at build time, so deployments need no external translation files.
`make check` verifies catalog coverage and formatting, plural forms, full and HTMX
pages in all three locales, authentication and role messages, language links,
filter preservation, and the English fallback.

Domain translations in `internal/languages` and the database are separate from
interface catalogs. They determine which editable role-name translations exist
and which translated role name to display; adding an interface locale does not
create a domain language or run a migration.

Role details display one Name field using the selected route language, matching
the heading and role list. When that translation is missing, the domain fallback
language is used, then the first available translation. Creation and editing keep
language tabs for entering all translations.
