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

Use `npm run preview` to serve the compiled assets locally.

## Admin UI components

The sign-in and password restoration pages are available at
`/admin/en/sign-in` and `/admin/en/restore-password`. Their navigation links keep
the current language path. The sign-in form posts credentials to the admin web
handler and redirects to
the authenticated admin page. It works without JavaScript and renders inline
errors for failed attempts. Password recovery remains presentation only.

Templates in `../templates` are organized into three packages:

- `pages/`: sign-in, password restoration, and the authenticated home page.
- `layouts/`: the `Root` document, full-page and HTMX fragment wrappers, and `AuthLayout`.
- `components/`: reusable `Brand`, `TextInput`, `PrimaryButton`, and `PageLink` components.

Pages compose layouts and components using templ's children blocks. Layouts may
use components; components do not depend on pages or layouts. `Root` currently
describes English page content.

`src/style.scss` loads the shared tokens and base styles from `src/styles/`,
followed by component and layout partials. Reuse the CSS color, spacing, radius,
and shadow variables when adding screens to keep data-heavy admin views compact
and consistent. There are no external font or image requests.

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
focuses the page heading after navigation, and shows the navigation error when
a request fails. Inline evaluation and response scripts are disabled.

`hx-history="false"` prevents admin page snapshots from entering HTMX's browser
storage cache. Back and Forward fetch the page from the server and recheck
access. Authentication forms use native submissions and browser redirects.
