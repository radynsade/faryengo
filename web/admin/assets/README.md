# Faryen Admin Assets

Vite, TypeScript, and SCSS assets for the admin interface.

From `web/admin/assets`:

```sh
npm ci
npm run dev
```

`src/main.ts` and `src/style.scss` are manifest entries. The script also imports the stylesheet. During development, the Vite server serves this entry at `/src/main.ts`; server-rendered pages can load it with `/@vite/client` for hot reload.

Run `npm run build` to type-check and compile JavaScript, CSS, and imported assets directly into `dist/`. Output filenames include content hashes, and `.vite/manifest.json` maps source paths to those files. HTML is rendered by the backend.

The Go assets package exposes `BuiltAsset` and `BuiltCSS` as package-level aliases. The manifest and compiled files are embedded into the Go binary and initialized automatically. `admin.RegisterHandlers(mux)` serves them at `/assets/admin/`. Run `make build` or `make check` from the project root to build assets before the Go compilation; rebuild the binary after changing assets. See [the asset integration documentation](../../../docs/assets.md) for wiring and template usage.

Use `npm run preview` to serve the compiled assets locally.

## Admin UI components

The static sign-in and password restoration pages are available at
`/admin/en/sign-in` and `/admin/en/restore-password`. Their navigation links keep
the current language path. Buttons are presentation controls; authentication,
password recovery, and submission are not implemented.

Templates in `../templates` share the `Root` document, `AuthLayout`, `Brand`,
`TextInput`, `PrimaryButton`, and `PageLink` components. Compose new page contents using
templ's children blocks. `Root` currently describes English page content.

`src/style.scss` loads the shared tokens and base styles from `src/styles/`,
followed by component and layout partials. Reuse the CSS color, spacing, radius,
and shadow variables when adding screens to keep data-heavy admin views compact
and consistent. There are no external font or image requests.

## Navigation

The pinned Datastar bundle in `src/vendor/` is included in the embedded Vite
build. `PageLink` enhances ordinary clicks with the `@navigate` action. Modified
clicks and JavaScript-free navigation retain standard link behavior.

Handlers return a full document for direct requests and HTML fragments for
requests with `Datastar-Request: true`. Fragments patch `#page-title` and
`#page-content`; `Vary: Datastar-Request` keeps the responses distinct for caches.
Templates for each screen describe the same content in both response modes.

Navigation updates browser history after a successful patch. Back and Forward
request the current URL through Datastar. Navigation cancels any previous page
request, excludes all signals from requests, moves focus to the page heading,
and shows an error message if loading fails. Page content and existing assets
stay in the current document. Authentication buttons remain presentation only.
