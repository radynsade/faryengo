package utils

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/a-h/templ"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"

	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/layouts"
)

//
// Paths
//

func OfficePath(request *http.Request) string {
	return "/office/" + url.PathEscape(officei18n.Language(request.Context()))
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

// A partial request that targets the dashboard's main region already shows
// the dashboard, so only the main region is sent.

func targetsContent(request *http.Request) bool {
	return IsPartial(request) && request.Header.Get("HX-Target") == layouts.ContentRegionID
}

//
// Rendering
//

// A request from inside the dashboard receives only the page's main region;
// if the page turns out to use another layout, such as the sign-in page after
// a session ended, the response replaces the whole page region instead.

func Render(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	pageTitle string,
	content templ.Component,
) {
	title := officei18n.T(request.Context(), "document.title", map[string]any{"Page": pageTitle})
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

	body, err := render(request.WithContext(layouts.WithRenderState(request.Context(), state)), page)

	if err == nil && state.ContentOnly && !state.DashboardRendered {
		writer.Header().Set("HX-Retarget", "#"+layouts.PageRegionID)
		writer.Header().Set("HX-Reswap", "morph:outerHTML show:none")
	}

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
	varyByForm(writer)

	body, err := render(request, content)

	send(writer, request, status, body, err)
}

// Transport rejections and unavailable services are plain-text errors; the
// client shows its translated navigation error and keeps the page usable.

func Fail(writer http.ResponseWriter, request *http.Request, status int, messageID string) {
	http.Error(writer, officei18n.T(request.Context(), messageID), status)
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
		slog.ErrorContext(request.Context(), "render an office page", "error", err)
		Fail(writer, request, http.StatusInternalServerError, "navigation.error")
	} else {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.WriteHeader(status)

		if _, writeErr := writer.Write(body); writeErr != nil {
			slog.DebugContext(request.Context(), "write an office page", "error", writeErr)
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
