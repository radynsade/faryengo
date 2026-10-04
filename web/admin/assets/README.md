# Faryen Admin Assets

Vite, TypeScript, and SCSS assets for the admin interface.

From `web/admin/assets`:

```sh
npm ci
npm run dev
```

`src/main.ts` and `src/style.scss` are manifest entries. The script also imports the stylesheet. During development, the Vite server serves this entry at `/src/main.ts`; server-rendered pages can load it with `/@vite/client` for hot reload.

Run `npm run build` to type-check and compile JavaScript, CSS, and imported assets directly into `dist/`. Output filenames include content hashes, and `.vite/manifest.json` maps source paths to those files. HTML is rendered by the backend.

The Go assets package exposes `BuiltAsset` and `BuiltCSS` as package-level aliases. The manifest and compiled files are embedded into the Go binary and initialized automatically. `adminHandler.RegisterHandlers(mux)` serves them at `/assets/admin/`. Run `make build` or `make check` from the project root to build assets before the Go compilation; rebuild the binary after changing assets. See [the asset integration documentation](../../../docs/assets.md) for wiring and template usage.

The helpers accept paths relative to `src/`: use `BuiltAsset("main.ts")` and
`BuiltCSS("style.scss")`. Include any subdirectories within `src/` in the path.

Place fonts, images, and other files that should retain their names in `static/`.
`StaticAsset("images/logo.svg")` returns the embedded file's URL and an error,
using a path relative to that directory. Files are served at
`/assets/admin/static/` and embedded directly into the Go binary. Rebuild the
binary after changing them. The helper reports missing files and rejects
directories and invalid paths.

Use `npm run preview` to serve the compiled assets locally.

## Code style

Prettier is installed locally at an exact version. Its configuration in
`.prettierrc.json` applies to JavaScript, TypeScript, CSS, and SCSS, including
the Vite configuration. Run these commands from `web/admin/assets`:

```sh
npm run format
npm run format:check
```

`format` rewrites files; `format:check` reports differences without changing
them and runs as part of the root `make check` command. Dependencies, compiled
assets, coverage output, and the npm lockfile are excluded by `.prettierignore`.
JSON configuration files use two spaces to match npm's package metadata.

The style follows the formatting conventions in the Symfony project's
`.php-cs-fixer.dist.php` where JavaScript and stylesheets have equivalents:

- Tabs for indentation, displayed at two columns, and LF line endings.
- Semicolons in JavaScript and TypeScript; single quotes where possible.
- Trailing commas in multiline JavaScript and TypeScript lists, including
  function arguments and parameters.
- Opening braces on the same line, consistent operator/comma spacing, and
  parentheses around arrow-function parameters.
- An 80-column wrapping target. Wrapped binary operators start the next line,
  using Prettier's `experimentalOperatorPosition` option.
- Standard Prettier formatting for stylesheet rules, declarations, and nesting.
  Declaration order is preserved because it can affect the cascade.

Use Sass's **SCSS syntax** (`.scss`), which Prettier supports directly. Indented
Sass (`.sass`) is not supported by Prettier and is outside these commands.
Prettier handles layout rather than semantic PHP-CS-Fixer equivalents such as
unused imports, member ordering, required braces, or author headers. It preserves
existing blank lines but does not insert blank lines around control flow.
See the [Prettier options](https://prettier.io/docs/options) for formatter behavior.

## Admin UI components

The sign-in and password restoration pages are available at
`/admin/en/sign-in` and `/admin/en/restore-password`. Their navigation links keep
the current language path. The sign-in form posts credentials to the admin web
handler and redirects to
the authenticated admin page. It works without JavaScript and renders inline
errors for failed attempts. Password recovery remains presentation only.

Templates in `../templates` are organized into three packages:

- `pages/`: sign-in, password restoration, and the authenticated home page.
- `layouts/`: the `Root` document, full-page and HTMX fragment wrappers,
  `AuthLayout`, and the authenticated `Panel` layout.
- `components/`: reusable `Brand`, `TextInput`, `PrimaryButton`, and `PageLink` components.

Pages compose layouts and components using templ's children blocks. Layouts may
use components; components do not depend on pages or layouts. `Root` currently
describes English page content.

Authenticated pages use `Panel`, with a sidebar containing Users and Roles links,
the current user's first and last name, and a native POST sign-out form. The
sidebar becomes a compact header on narrow screens. The links retain the
language path and mark the active section. `/admin/{language}/users` requires
`view_user`; `/admin/{language}/roles` requires authentication. Users
currently contains a placeholder; Roles provides the management table described
below.

`Panel` renders page content directly into the full-width main area, without a
shared title block, padding, or width limit. Pages can fill that area with tables
and toolbars; use the `panel-page` class when a page needs an inset. The main
area retains its accessible page label, skip link, and navigation focus target.

## Roles management

The Roles table fills the main area's width. Its toolbar contains Create,
the filtered total, a collapsible filter form, and pagination. Column links sort
by UUID, translated name, or super status. Filters match UUID and name substrings,
all selected permissions, and an optional super status; super roles satisfy all
permission selections. Sorting and pagination retain filters in the URL.

The list accepts `uuid`, `name`, repeated `permissions`, `super=true|false`,
`sort=uuid|name|super`, `order=asc|desc`, `page`, and `size` query parameters.
The default page size is 25; the maximum is 100. Invalid filters return a readable
400 response. Filters live in `internal/security/role_filters.go`, and PostgreSQL
applies them through `RoleRepository.Find` and `Count`.

Create and edit forms offer name fields for the configured languages, permissions,
and a super-role checkbox. Blank translations are omitted; at
least one name is required. The view page shows all translations and permissions.
Delete opens a confirmation page and only changes data on POST. Assigned roles
cannot be deleted. Form errors retain entered values; successful writes redirect
to a confirmation message. Forms and links work without JavaScript; HTMX enhances
filtering, sorting, pagination, and navigation.

Permissions in filters and create/edit forms use the shared `MultiSelect`
component. It follows Monoshop's searchable dropdown with highlighted selections,
a count badge, and removable tags. Arrow keys move through options, Enter toggles
the active option (or the only search result), and Escape closes the dropdown.
The native multiple select submits repeated values and remains available without
JavaScript. `src/multiselect.ts` initializes fields after HTMX swaps and removes
their listeners when content is replaced.

Role name translations use `TranslationsInput`, with grouped language tabs based
on Monoshop's language switcher. Tabs show native language names and initially
select the current admin language, falling back to the first available language.
Switching tabs preserves all values; every translation submits with the form.
Left/Right arrows, Home, and End move between tabs. JavaScript-free forms show
all language fields. `src/translations-input.ts` also initializes HTMX fragments.

Role management authorization is currently deferred. Signed-in users can view,
create, update, and delete roles through the common role service methods. All
permissions and the super flag are editable, including on your own role.
Every request reauthenticates the user; assigned roles cannot be deleted.
Mutation routes use the admin's same-origin protection and Strict session cookies.

`make check` runs the domain, service, repository, and HTTP tests. To also run the
PostgreSQL integration test, point `FARYEN_ROLE_TEST_DATABASE_URL` at a disposable
PostgreSQL instance before running `make check`. The test creates only temporary
tables and a temporary permission type on one connection; it never runs migrations
or accesses existing application tables.

`src/style.scss` loads the shared tokens and base styles from `src/styles/`,
followed by component and layout partials. Reuse the CSS color, spacing, radius,
and shadow variables when adding screens to keep data-heavy admin views compact
and consistent. Nunito is the default font throughout the admin interface,
including form controls and code text. Its normal and italic weights are loaded
from the bundled files in `static/`. There are no external font or image requests.

Icons use the installed `@tabler/icons-webfont` package's thin outline (300)
webfont. `src/style.scss` imports its stylesheet, so Vite bundles the icon fonts
into the embedded build. Use `<i class="ti ti-users" aria-hidden="true"></i>`
with the appropriate icon name and a visible text label for navigation and
buttons. Component styles set icon sizes independently of the text size.

## Navigation

The installed `htmx.org` NPM package is bundled by Vite into the embedded build,
with no runtime CDN request or additional extensions. `PageLink` uses
`hx-boost="true"` to enhance ordinary links. Native forms, modified clicks, and
JavaScript-free navigation retain standard browser behavior.

Handlers return HTML fragments for `HX-Request: true` and full documents for
ordinary navigation and `HX-History-Restore-Request: true`. Fragments include a
`<title>` and `#page-content`; HTMX updates the document title and swaps the page
content without reloading assets. Responses vary on both request headers.

HTMX manages browser history and `hx-sync="body:replace"` cancels a pending link
request when another starts. `hx-params="none"` keeps page navigation free of
form values. `navigation.ts` configures full-document history restoration,
focuses the panel's main area or the auth page heading after navigation, and
shows the navigation error when a request fails. Inline evaluation and response
scripts are disabled.

`hx-history="false"` prevents admin page snapshots from entering HTMX's browser
storage cache. Back and Forward fetch the page from the server and recheck
access. Authentication forms use native submissions and browser redirects.
