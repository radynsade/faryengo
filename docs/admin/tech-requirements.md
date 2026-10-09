# Technical requirements of the admin panel

These requirements apply to every page, form, and component of the admin panel.
A change that breaks one of them is a bug, even when the feature otherwise works.

| # | Requirement |
| --- | --- |
| 1 | Navigation and form submissions never trigger a full page reload. |
| 2 | Page updates morph the existing DOM instead of replacing it. |
| 3 | HTML responses are minimized. |
| 4 | Every static text is translatable and rendered through i18n. |

## 1. No full page reloads

Every link click and form submission inside the panel updates only the affected
part of the page. If an action makes the browser load a new document, it is a
bug.

A full document load is allowed only for:

- the first request to the panel (direct URL entry, bookmark, or browser refresh),
- history restoration when no cached state is available,
- opening a link in a new tab or window, and
- requests made with JavaScript disabled.

### Server

- Each page is available at one URL and can be returned in two forms: a complete
  document for full loads, and a partial response for in-page updates.
- Navigation inside the dashboard updates only the page's content. The partial
  response contains the content region and the head, never the sidebar or the
  rest of the shared layout; the parts of the sidebar that depend on the current
  page, such as the active section and the language links, are updated
  separately. A request that leaves the dashboard, such as signing out or a
  session that ended, replaces the whole page instead.
- Responses that differ by form or by the region they update declare it in the
  `Vary` header so caches never serve one form in place of the other.
- A successful form submission follows Post/Redirect/Get: the server redirects,
  and the client updates the page with the result of the redirected request.
- A failed validation returns the same form with its field errors and an error
  status, and the client shows it in place.
- The server can tell the client to update a different region than the one the
  request targeted, or to navigate elsewhere, without returning a full document.

### Client

- The URL in the address bar always matches the displayed content. Results that
  have their own URL, such as filtered or paginated lists, are added to the
  browser history.
- Back and forward navigation restore the matching content.
- Clicks with a modifier key (Ctrl, Cmd, Shift, Alt) and middle clicks keep the
  browser's default behavior.
- A form cannot be submitted twice while its request is pending. A newer
  navigation cancels an older one that is still in progress.
- A failed request shows a translated error message and leaves the current page
  usable. It never falls back to a full reload.
- After the main content changes, the document language and title are updated,
  and focus moves to the new content for keyboard and screen reader users.
- Every link and form keeps a working `href`, `method`, and `action`, so the panel
  stays usable without JavaScript.
- Content received from the server never executes inline scripts. Client
  behavior is loaded once and must keep working for content inserted later.

## 2. DOM morphing

Page updates morph the existing DOM: only the nodes that differ from the new
content are changed. Morphing keeps focus, text selection, input state, scroll
position, open disclosure elements, and running CSS transitions, and it avoids
visible flicker.

- Navigation and form results that replace the main content are morphed.
- On navigation, the document `<head>` is merged: shared stylesheets and scripts
  are kept, and only page-specific elements such as `<title>` change.
- Elements that hold client-side state have stable, unique IDs so they are
  matched between renders. List rows use the ID of the entity they display.
- A plain replacement without morphing is allowed only for self-contained
  regions with no state to keep, such as a lazily loaded table or a dialog body.
- Client components must keep working when morphing keeps their root element but
  replaces their children. They release stale references and re-initialize from
  the new markup.

## 3. Minimized HTML

Every HTML response, both full documents and partial responses, is minimized.
Partial responses are sent on every interaction, so their size directly affects
how fast the panel feels.

- Minimization is applied to the rendered output, so templates stay readable.
  Templates are not minimized by hand.
- Minimization never changes meaning or behavior:
  - whitespace inside `<pre>`, `<textarea>`, and between inline words is kept;
  - attribute values, IDs, and `data-*` and other behavior attributes are kept
    unchanged;
  - the morphed result is identical to the result of the unminimized response.
- Comments and whitespace between block elements are removed.
- Pages do not render unused markup, such as hidden alternative content,
  repeated inline SVGs, or inline styles that belong in a stylesheet.
- CSS and JavaScript are minified as part of the production build.

## 4. Translatable texts

Every text a user can see or hear comes from translation catalogs. This includes
visible text, `title`, `placeholder`, `aria-label`, and `alt` attributes, the
document title, validation and flash messages, error responses, and confirmation
prompts.

- Messages are complete sentences with named parameters. Sentences are never
  built by joining translated fragments.
- Messages that include a number use the plural rules of each language.
- Client-side scripts contain no user-facing text. The server renders localized
  strings into the markup, and scripts read them from there.
- The interface language is part of the URL. Switching the language keeps the
  current page and its query parameters.
- Not translated: application data (such as role names, emails, and IDs), the
  name of each language in the language switcher, and the product name.

Switching the interface language is the only click allowed to reload the full
document, because every text on the page, including the document language,
changes.

## Verification

Automated tests cover each requirement:

- Requests for in-page updates receive partial responses, other requests receive
  complete documents, and both declare `Vary`.
- Links and forms are wired for in-page updates with morphing.
- Every page and message renders in every supported language without text from
  another language.
- Responses are minimized and keep their behavior attributes.

Before merging a UI change, check it in the browser: open the network panel, use
every link and form on the changed page, and make sure no document request
appears after the first page load.
