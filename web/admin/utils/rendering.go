package utils

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/a-h/templ"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"

	"github.com/radynsade/faryengo/pkg/flashmsg"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
)

//
// Paths
//

func AdminPath(request *http.Request) string {
	return "/admin/" + url.PathEscape(request.PathValue("language"))
}

// The location tells the client to navigate within the dashboard, as a link
// would, when the request targeted something other than the content region.

func NavigationLocation(path string) string {
	location, _ := json.Marshal(map[string]string{
		"path":   path,
		"target": "#" + layouts.ContentRegionID,
		"swap":   "morph:outerHTML show:none",
	})

	return string(location)
}

//
// Response forms
//

// A request for an in-page update receives the partial form of a page. A
// history restoration asks for the complete document, because there is no
// cached page to restore into.

func IsPartial(request *http.Request) bool {
	return request.Header.Get("HX-Request") == "true" && request.Header.Get("HX-History-Restore-Request") != "true"
}

func varyByForm(writer http.ResponseWriter) {
	writer.Header().Add("Vary", "HX-Request")
	writer.Header().Add("Vary", "HX-History-Restore-Request")
	writer.Header().Add("Vary", "HX-Target")
}

// A partial request that targets the dashboard's content region already shows
// the dashboard, so only the content is sent.

func targetsContent(request *http.Request) bool {
	return IsPartial(request) && request.Header.Get("HX-Target") == layouts.ContentRegionID
}

//
// Rendering
//

// The page shows the flash messages in the bag, which the caller has already
// consumed from storage. A request from inside the dashboard receives only the
// page's content; if the page turns out to use another layout, such as the
// sign-in page after a session ended, the response replaces the whole page
// region instead.

func Render(
	writer http.ResponseWriter,
	request *http.Request,
	bag *flashmsg.Bag,
	status int,
	pageTitle string,
	content templ.Component,
) {
	title := admini18n.T(request.Context(), "document.title", map[string]any{"Page": pageTitle})
	page := layouts.Page(title, content)
	state := &layouts.RenderState{ContentOnly: targetsContent(request)}

	varyByForm(writer)

	if IsPartial(request) {
		page = layouts.PageFragment(title, content)

		// A form shown again in place keeps the address of the page it was
		// submitted from.
		if request.Method != http.MethodGet {
			writer.Header().Set("HX-Push-Url", "false")
		}
	}

	ctx := layouts.WithRenderState(flashmsg.WithBag(request.Context(), bag), state)
	body, err := render(request.WithContext(ctx), page)

	if err == nil && state.ContentOnly && !state.PanelRendered {
		writer.Header().Set("HX-Retarget", "#"+layouts.PageRegionID)
		writer.Header().Set("HX-Reswap", "morph:outerHTML show:none")
	}

	send(writer, request, status, body, err)
}

func RenderFragment(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	content templ.Component,
) {
	varyByForm(writer)

	body, err := render(request, content)

	send(writer, request, status, body, err)
}

// A failed in-page submission replaces only its form region, which the page
// already has, so neither the head nor the rest of the page is sent again.
// The region is morphed, keeping focus and typed values, and the address
// stays that of the page the form was submitted from.

func RenderFormRegion(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	regionID string,
	content templ.Component,
) {
	writer.Header().Set("HX-Retarget", "#"+regionID)
	writer.Header().Set("HX-Reswap", "morph:outerHTML")
	writer.Header().Set("HX-Push-Url", "false")
	RenderFragment(writer, request, status, content)
}

// Transport rejections and unavailable storage are plain-text errors; the
// client shows its translated navigation error and keeps the page usable.

func Fail(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	messageID string,
) {
	http.Error(writer, admini18n.T(request.Context(), messageID), status)
}

//
// Helpers
//

// Rendering completes before anything is written, so headers can depend on
// what was rendered and a failed render is reported with an error status
// instead of a truncated page.

func render(request *http.Request, content templ.Component) ([]byte, error) {
	var rendered, minified bytes.Buffer

	err := content.Render(request.Context(), &rendered)

	if err == nil {
		err = minifier.Minify("text/html", &minified, &rendered)
	}

	return minified.Bytes(), err
}

func send(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	body []byte,
	err error,
) {
	if err != nil {
		slog.ErrorContext(request.Context(), "render an admin page", "error", err)
		Fail(writer, request, http.StatusInternalServerError, "navigation.error")
	} else {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.WriteHeader(status)

		if _, writeErr := writer.Write(body); writeErr != nil {
			slog.DebugContext(request.Context(), "write an admin page", "error", writeErr)
		}
	}
}

// Whitespace and comments go, but every attribute, including defaults and
// quotes, stays as rendered, so morphing sees the same attributes and values
// as in the unminimized markup.

var minifier = newMinifier()

func newMinifier() *minify.M {
	m := minify.New()

	m.Add("text/html", &html.Minifier{
		KeepDefaultAttrVals: true,
		KeepDocumentTags:    true,
		KeepEndTags:         true,
		KeepQuotes:          true,
	})

	return m
}
