# Faryen Office Assets

Vite, TypeScript, and SCSS assets for the office interface. The toolchain,
Prettier configuration, and HTMX setup match the
[admin assets](../../admin/assets/README.md).

From `web/office/assets`:

```sh
npm ci
npm run dev
npm run build
```

`src/main.ts` and `src/style.scss` are manifest entries. The build is embedded
into the Go binary and served at `/assets/office/`; templates resolve compiled
URLs with `BuiltAsset("main.ts")` and `BuiltCSS("style.scss")`.

The office has its own visual design, separate from the admin's: Manrope from
`@fontsource-variable/manrope` (bundled, with Latin, Latvian, and Cyrillic
subsets), a warm canvas, and a violet primary color. CSS variables live in
`src/styles/foundations/_tokens.scss`; components, layouts, and pages each have
their own stylesheet under `src/styles/`. Icons are inline SVG.

## Technical requirements

The office follows the [admin technical requirements](../../../docs/admin/tech-requirements.md):
no full page reloads, DOM morphing, minimized HTML, and translatable text.
The mechanics match the admin's:

- Links and forms use HTMX boosting with `morph:outerHTML` swaps and keep their
  native `href` and `action`. Language links are the one full navigation.
- In-page requests receive a partial response: the shared `<head>` and the page
  region (`#page-content`). Inside the dashboard, navigation targets only the
  main region (`#dashboard-main`), and the menu and account area of the sidebar
  arrive as out-of-band morphs. A failed sign-in replaces only the form.
- `src/navigation.ts` updates the document language, moves focus to the new
  content, lets modified clicks open new tabs, and shows the translated
  navigation error when a request fails. Opening a page from the narrow-screen
  drawer closes it.
- Icons are symbols of `static/icons.svg`, referenced with `<use>`, so no icon
  markup repeats in a response. The sprite URL carries a content version.
  Built files and the versioned sprite are served with long-lived, immutable
  caching, so in-page navigation never fetches them again.
