package utils

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/a-h/templ"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"

	officei18n "github.com/radynsade/faryengo/web/office/i18n"
	"github.com/radynsade/faryengo/web/office/templates/layouts"
	"github.com/radynsade/faryengo/web/office/templates/pages"
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
	if err := writePage(writer, request, status, pageTitle, content); err != nil {
		slog.ErrorContext(request.Context(), "render an office page", "error", err)
		Fail(writer, request, http.StatusInternalServerError, "errors.unexpected")
	}
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

	if err != nil {
		slog.ErrorContext(request.Context(), "render an office form", "error", err)
		Fail(writer, request, http.StatusInternalServerError, "errors.unexpected")
	} else {
		send(writer, request, status, body)
	}
}

// Every failure is an error page that replaces the whole page region, so the
// visitor always has a way back into the office. A failed page load can be
// repeated; a failed submission cannot, because its address accepts only the
// form. Only when the error page itself cannot render is the error plain
// text.

func Fail(writer http.ResponseWriter, request *http.Request, status int, messageID string) {
	ctx := request.Context()
	props := pages.ErrorProps{
		Code:    strconv.Itoa(status),
		Title:   officei18n.T(ctx, errorTitleID(status)),
		Message: officei18n.T(ctx, messageID),
		HomeURL: OfficePath(request),
	}

	if status >= http.StatusInternalServerError && request.Method == http.MethodGet {
		props.RetryURL = request.URL.RequestURI()
	}

	if err := writePage(writer, request, status, props.Title, pages.Error(props)); err != nil {
		slog.ErrorContext(ctx, "render an office error page", "error", err)
		http.Error(writer, props.Message, status)
	}
}

//
// Helpers
//

func writePage(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	pageTitle string,
	content templ.Component,
) error {
	title := officei18n.T(request.Context(), "document.title", map[string]any{"Page": pageTitle})
	page := layouts.Page(title, content)
	state := &layouts.RenderState{ContentOnly: targetsContent(request)}

	// A failure while rendering a form region leaves its retargeting behind.
	writer.Header().Del("HX-Retarget")
	writer.Header().Del("HX-Reswap")
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

	if err == nil {
		if state.ContentOnly && !state.DashboardRendered {
			writer.Header().Set("HX-Retarget", "#"+layouts.PageRegionID)
			writer.Header().Set("HX-Reswap", "morph:outerHTML show:none")
		}

		send(writer, request, status, body)
	}

	return err
}

// Rendering completes before anything is written, so headers can depend on
// what was rendered and a failed render is reported with an error page
// instead of a truncated one.

func render(request *http.Request, content templ.Component) ([]byte, error) {
	var rendered, minified bytes.Buffer

	err := content.Render(request.Context(), &rendered)

	if err == nil {
		err = minifier.Minify("text/html", &minified, &rendered)
	}

	return minified.Bytes(), err
}

func send(writer http.ResponseWriter, request *http.Request, status int, body []byte) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(status)

	if _, err := writer.Write(body); err != nil {
		slog.DebugContext(request.Context(), "write an office page", "error", err)
	}
}

// The title names the kind of failure; the message says what failed.

func errorTitleID(status int) string {
	var id string

	switch {
	case status == http.StatusForbidden:
		id = "errors.title_forbidden"
	case status == http.StatusNotFound:
		id = "errors.title_not_found"
	case status == http.StatusServiceUnavailable:
		id = "errors.title_unavailable"
	case status >= http.StatusInternalServerError:
		id = "errors.title_server"
	default:
		id = "errors.title_bad_request"
	}

	return id
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
