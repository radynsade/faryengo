# Admin design system

The admin interface uses server-rendered templ components, SCSS compiled by
Vite, Nunito typography, and Tabler icons. This document describes the visual
system and stylesheet ownership.
[Technical requirements](../architecture/tech-requirements.md) govern the
application boundaries; [assets](assets.md) describes build and delivery.

The system preserves the current admin appearance: white surfaces over a pale
background, muted supporting text, green ordinary actions, and red destructive
actions. The authenticated shell supports full-width collection pages and inset
cards. Authentication pages use a centered, narrow panel.

## Stylesheet structure and dependencies

`web/admin/assets/src/style.scss` is the sole CSS entry point. It loads vendor
icons and fonts, foundations, components, layouts, pages, and utilities in that
order. Each emitting stylesheet has one owner and is registered there once.

```text
styles/
  abstracts/
    _settings.scss          Sass maps: tokens, breakpoints, button recipes
    _functions.scss         token(), space(), breakpoint()
    _mixins.scss            Shared visual and responsive recipes
    _index.scss             Public Sass API; emits no CSS
  foundations/
    _tokens.scss            Emits the token registry as :root CSS variables
    _base.scss              Reset, document typography, selection
  fonts/
    _nunito.scss            Local font-face registrations
  components/
    _brand.scss             Brand mark and name
    _buttons.scss           Text buttons and icon buttons
    _forms.scss             Fields, stacks, fieldsets, checkboxes
    _cards.scss             Inset content cards
    _page-title.scss        Page heading and breadcrumb navigation
    _collection.scss        Toolbars, action groups, tag lists, pagination
    _tables.scss            Scroll regions, tables, sort links
    _filter-menu.scss       Collapsible filter popover
    _details.scss           Responsive definition lists
    _copy-value.scss        Inline clipboard controls for scalar values
    _feedback.scss          Badges and inline notices
    _navigation-feedback.scss  Navigation failure toast
    _links.scss             Inline text links
    _multiselect.scss       Searchable multiple selection
    _language-switcher.scss Interface language links
    _translations-input.scss  Language tabs and translation fields
    _dialog.scss            Native dialog surface and close control
    _confirm-delete.scss    Destructive confirmation content
    _success-dialog.scss    Successful-operation content
  layouts/
    _auth.scss              Authentication shell
    _panel.scss             Sidebar, account, navigation, main area
  pages/
    _roles.scss             Role-page composition and UUID column width
  utilities/
    _accessibility.scss     Visually hidden accessible text
```

Components and layouts may import the CSS-free `abstracts` API. They do not
import other emitting component files. Pages compose shared classes and add
domain-specific rules. Foundations do not depend on components or pages.
Utilities have a narrow purpose and must not become a home for unrelated styles.

Use `@use` and namespaced mixins instead of global Sass imports or `@extend`.
Keep selector ownership explicit; a page stylesheet must not redefine `.button`
or another shared component.

## Tokens: Sass configuration and runtime CSS variables

`abstracts/_settings.scss` is the source of visual values. Sass maps organize
colors, type sizes, and a combined token registry. A spacing unit and step list
generate the spacing scale. `foundations/_tokens.scss` emits that registry as
CSS custom properties. Sass variables hold compile-time decisions; CSS variables
hold values used by rendered elements and allow scoped overrides.

| Category | Token examples | Purpose |
| --- | --- | --- |
| Surfaces and text | `--color-background`, `--color-surface`, `--color-ink`, `--color-muted` | Page background, surfaces, primary and supporting text |
| States | `--color-accent`, `--color-accent-hover`, `--color-accent-soft`, `--color-danger`, `--color-danger-soft` | Ordinary interactions and destructive feedback |
| Brand and success | `--color-brand`, `--color-success` | Branding and successful-operation icons |
| Spacing | `--space-1` through `--space-6`, `--space-8`, `--space-12` | Layout gaps, insets, larger empty states |
| Typography | `--font-family`, `--font-size-control`, `--font-size-title`, `--font-weight-semibold`, `--line-height-body` | Shared type roles |
| Geometry | `--radius-control`, `--radius-panel`, `--border-width`, `--control-height`, `--icon-action-size` | Controls, cards, borders, icon actions |
| Width limits | `--sidebar-width`, `--auth-width`, `--page-content-width`, `--table-min-width` | Layout and component constraints |
| Elevation | `--shadow-panel`, `--shadow-dropdown`, `--shadow-popover`, `--shadow-dialog` | Surface hierarchy |
| Focus and motion | `--focus-width`, `--focus-offset`, `--focus-halo`, `--duration-fast`, `--ease-standard` | Keyboard indication and transitions |
| Stacking | `--z-skip-link`, `--z-popover`, `--z-dropdown` | In-page overlays; native modal dialogs use the top layer |

The root font size remains 14px, with 16px body text. Spacing uses rem units;
relative type roles use em units. Caption, small, control, title, and heading
sizes are 0.8125em, 0.875em, 0.9375em, 1.25em, and 1.625em respectively. Table
text inherits its own control-size context, so nested captions scale with it.
Control geometry and icon sizes use pixel tokens to preserve their current
dimensions independently of the rem spacing scale.

Breakpoints are named Sass values: `compact` (360px), `phone` (480px), and
`panel` (720px). Media query thresholds are resolved at compile time through
`respond-to`; changing a CSS variable cannot change these thresholds.

Use existing tokens before adding a value. Add a token for a shared visual
decision, not every structural number: `0`, `100%`, grid fractions, column
proportions, and component-specific relative typography can remain local.
Keep raw color values in the registry. Name new tokens by their role rather than
their color or the page that first used them.

## Sass API

Import the public API relative to the stylesheet:

```scss
@use '../abstracts' as ui;

.example-panel {
  @include ui.surface(panel, surface, panel);
  padding: ui.space(6);

  &:focus-visible {
    @include ui.focus-ring;
  }

  @include ui.respond-to(panel) {
    padding: ui.space(4);
  }
}
```

| Helper | Contract |
| --- | --- |
| `token($name)` | Returns `var(--name)`; rejects names absent from the registry |
| `space($step)` | Returns a registered spacing variable; rejects unsupported steps |
| `breakpoint($name)` | Returns a named compile-time breakpoint; rejects unknown names |
| `respond-to($name)` | Wraps content in the named maximum-width media query |
| `focus-ring($offset: null)` | Shared accent outline; defaults to the focus-offset token, accepts inset offsets |
| `motion($properties...)` | Applies the shared duration and easing to listed properties; disables the transition for reduced motion |
| `surface($radius: panel, $background: surface, $shadow: null)` | Shared border, corners, background, and optional named shadow |
| `row($gap: 2, $wrap: nowrap, $justify: null)` | Aligned flex row with a spacing token and optional wrapping/alignment |
| `heading($size: title, $line-height: title)` | Shared heading size, bold weight, and line height |
| `button-base` | Shared button alignment, border, corners, cursor, and focus behavior |
| `button-variant($name)` | Applies a registered button color/state recipe |
| `badge($tone: accent, $solid: false)` | Compact status typography, spacing, corners, and tone |

CSS-free helpers allow reuse without depending on stylesheet import order or
emitting another component's selectors. Simple component declarations may use
`var(--token)` directly. Use `token()` when constructing token names in Sass;
unknown tokens, breakpoints, and button variants fail compilation.

## Component contracts

Use block, element, and modifier classes (`block`, `block__element`,
`block--modifier`). A class describes visual ownership; `data-*` attributes,
IDs, ARIA state, and form attributes describe behavior. Keep HTMX targets and
JavaScript hooks stable during styling changes. The `role-filters` class remains
an existing JavaScript hook alongside the generic `filter-menu` class.

| Pattern | Shared classes and usage |
| --- | --- |
| Text buttons | `.button` with `--primary`, `--secondary`, or `--danger`; optional `--compact` |
| Icon actions | `.icon-button`; use `--danger` for destructive row actions and accessible labels for icon-only controls |
| Forms | `.form-stack`, `.form-field`, `__label`, `__input`, `.form-checkbox`, `.form-fieldset` |
| Cards | `.card`, `__eyebrow`, `__links`; page content owns its heading and paragraphs |
| Page titles | `.page-title`, `__heading`, `.breadcrumbs`, `__list`, `__item`, `__link`, `__separator`; `PageTitle` renders the title and an ordered breadcrumb list |
| Collection layout | `.list-toolbar`, `__controls`, `.action-group`, `.tag-list`, `.pagination`, `__current` |
| Tables | `.data-table-scroll`, `.data-table`, `__actions`, `__empty`, `__loading`, `__spinner`, `.table-sort`; page rules own domain-specific column widths |
| Filters | `.filter-menu`, `__form`; native `details` and `summary` remain usable without JavaScript |
| Details | `.details-list`; semantic `dl`, `dt`, and `dd` elements; role-view permissions use the same badges as the role list |
| Copyable values | `CopyValue` and `.copy-value`; inline scalar values with Tabler `copy` on hover/focus and `copy-check` after successful copying; localized clipboard status and keyboard activation |
| Status | `.badge`, `--negative`; `.notice`, `--error` for inline feedback |
| Dialogs | `.admin-dialog` is the shell; `.confirm-delete` and `.success-dialog` own their content |

The primary variant is a filled accent button for actions such as Create and
Save. The secondary variant has muted text, a white surface, and a line border;
hover uses accent text and a soft accent background without changing the border.
Filter, Sign out, and ordinary row actions use this same recipe. Sign out keeps
the account block's full-width geometry. The filled danger variant is used in
confirmation, detail, and edit actions; the danger-outline recipe gives row Delete its
red text and pale red hover. Button dimensions and color variants are separate
decisions.

Toolbars wrap their controls at the panel breakpoint. Tables retain a minimum
width inside a horizontal scroll region; their action column stays compact and
does not wrap its button group. Badges distinguish positive and negative values
with text as well as color. Detail lists become a single column on narrow screens.

Cards and authentication panels share a surface recipe. Filter popovers,
multiselect dropdowns, and modal dialogs use their respective elevation tokens.
Dialog content owns its spacing and typography; the dialog shell owns positioning,
backdrop, overflow, and its close control.

Multiselects render their complete initial layout on the server from typed
options: search placeholder, selected tags, count badge, listbox options, and
ARIA state. The dropdown and native submission select are hidden in the initial
HTML. JavaScript hydrates the existing elements by attaching listeners; it does
not rebuild the layout or change visibility on load or after HTMX swaps. Later
selection, search, and reset interactions update that state. A `scripting: none`
CSS fallback shows the native select and its label when JavaScript is disabled.

Translation fields render the initial language selection on the server, falling
back to the first configured language when the requested language is unavailable.
The initial HTML renders the visible tab bar, selected tab, tab focus order,
panel ARIA attributes, and `hidden` on inactive panels. JavaScript hydrates
delegated interactions and changes state only in response to tab navigation,
form reset, or validation; it does not recompute or rewrite the initial state.
HTMX fragments carry the same complete markup and need no initialization pass.
The `scripting: none` CSS fallback hides the tab bar and reveals all native
fields when JavaScript is disabled.

Field validation uses `.form-field--invalid`, red labels and control borders,
and `.form-field__errors` messages directly below the relevant controls. Invalid
native inputs and enhanced multiselects carry `aria-invalid` and link to their
messages through `aria-describedby`; translation controls retain their help-text
reference too. Each invalid translation marks its language tab, and the server
opens the first tab with a field error. The fallback language's name field is
`required`, and its help text names the fallback language; a missing fallback
translation is an error on that field and its tab, and other translations remain
optional. When the catalog has no fallback language, supplying no name at all is
a group error beside the name input. Checkbox errors appear beneath the checkbox. Messages use the interface
catalogs and apply to full pages, HTMX fragments, and native forms without
JavaScript. Field errors live only in the submission response. Form-wide
failures, such as wrong credentials or a malformed form, appear inside the form
region: above the sign-in form with `.form-error`, and below the role form's
toolbar with `.notice--error`.

A failed submission during an in-page update returns only its form region, the
`#sign-in-form` wrapper or the `#role-form` section, retargeted and morphed in
place; the head, layout, and sidebar are not sent again, and the address does not
change. Without JavaScript, the same failure renders the whole page.

## Layout and page responsibilities

The reusable `LanguageSwitcher` renders native locale links in the sidebar and
authentication layout. It uses shared button tokens, with an accent treatment for
the selected locale, and wraps on narrow screens. Layouts own its placement:
the sidebar places it after navigation, and the authentication panel separates it
from the form with a border. Its complete layout is server-rendered. All interface
copy, including accessible labels and interactive widget messages, uses the
embedded go-i18n catalogs; see [admin-i18n.md](admin-i18n.md).

`layouts/_panel.scss` owns the authenticated shell: a sticky, scrollable sidebar,
the account card, navigation, skip link, main area, and optional `.panel-page`
inset. At 720px and below it becomes a header with a two-column menu. The main
area has no shared padding or width limit, allowing collection tables to fill it.

Inset detail and form pages use `.panel-page__content` inside `.panel-page` to
center a content column, with a 960px maximum controlled by
`--page-content-width`. The page title and card share this column so their edges
align. `PageTitle` takes a title and `BreadcrumbItem` values (label and URL);
the last item is the current page and renders as unlinked text with
`aria-current="page"`. Breadcrumb links use the shared HTMX navigation behavior.
The heading is the page's single `h1`, with ID `page-heading`; a content card can
reference it using `aria-labelledby`. Keep the title and breadcrumbs above the
card instead of placing a back link or a duplicate page heading inside it.

`layouts/_auth.scss` owns centering, panel width, auth heading, brand divider,
footer, and full-width form submission buttons. Compact screens reduce its inset.

`pages/_roles.scss` owns the full-width role list, total/supporting text, editor
composition, and UUID column proportion. View, create, and edit pages use the
centered content column with Admin → Roles → current-page breadcrumbs.
The role view uses a compact collection-style toolbar above bordered detail
rows, with muted label cells and permission badges matching the list. It retains
copyable Name and UUID values, the shared super-role badge, and the delete
confirmation dialog. These styles are scoped to `.role-view`; the role list,
sidebar, typography, and color tokens are unchanged.
Actions follow the user's authority: viewing the list and role details needs the
view-roles permission, and the Create, Edit, and Delete actions, together with the
confirmation dialog, appear only with the manage-roles permission. The sidebar and
home page link only to sections the user may view.
The roles collection initially renders its controls and a single loading row,
with unknown totals and disabled pagination. HTMX loads `/roles/table` after
the initial render and replaces only `#roles-list-table` with the populated
collection; the first page request does not query the role list. Sorting,
filtering, and pagination retain their ordinary page URLs and repeat this flow.
The loading row announces its localized status, and its spinner respects reduced
motion. A native link inside `noscript` opens the populated table as a full page.
Loading errors consume error flashes in the table fragment and provide a Retry
link. Background table swaps do not move keyboard focus.
Create and edit pages share `.role-form` with the same compact toolbar as the
role view and full-width fields using their existing labels. The edit toolbar
starts with Save, View, and List, with an outlined Delete icon at the end.
View links to the current role's details and List links to the roles collection.
Name translations, the super-role checkbox, and permission selection occupy
separate bordered rows; their existing
labels, help, submission names, and server-rendered widget states are preserved.
Compact control tokens and widget spacing are scoped to `.role-form`. The shared
confirmation dialog stays outside the edit form; deletion submits only the
confirmation form and uses the persisted, localized role name. The create page
uses Create and List in its toolbar, with no View or Delete action until a role
exists. Shared component rules belong in `components/`, even when Roles is
currently their only consumer. Future Users
pages should compose these components without importing Roles styles.

## Accessibility and interaction rules

- Preserve native forms, links, `details`, and dialogs; JavaScript enhances them.
- Use the common focus ring for interactive components, with inset offsets where
  scrolling or grouped tabs could clip an outer ring.
- Apply transitions through `motion()` so reduced-motion preferences are honored.
- Keep form inputs at least 16px on phone screens to avoid focus zoom on iOS.
- Preserve semantic labels, icon `aria-hidden`, active navigation state, table
  captions, and the skip-to-content target.
- Keep unbounded identity and message text wrapping inside its component.
- Keep the `.visually-hidden` utility available independently of any page.

## Extending and verifying the system

1. Reuse existing component classes and tokens; compose page-specific spacing in
   a page stylesheet.
2. Put a new repeated pattern in its own component partial. Import only the
   abstract API, and register the partial in `style.scss`.
3. Add state recipes or semantic tokens centrally when existing ones do not
   express the design. Document additions here.
4. Check ordinary, hover, keyboard focus, selected, disabled, error, and success
   states relevant to the changed component.
5. Compare desktop and narrow-screen rendering, including long content,
   popovers, dialog overflow, and pages with JavaScript disabled.
6. Run `make check`: it builds assets, generates templ code, checks formatting,
   runs Go vet and lint, and runs the race-enabled uncached test suite.

Generated CSS in `dist/` and generated `*_templ.go` files are build output.
Edit SCSS and `.templ` sources, then rebuild. Runtime CSS variable overrides can
be scoped to a component or layout; keep the documented component contracts
intact when doing so.
