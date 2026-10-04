package assets

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
)

const staticURLPrefix = URLPrefix + "static/"

var ErrStaticAssetNotFound = errors.New("static asset not found")

var staticHandler = http.StripPrefix(URLPrefix, http.FileServerFS(embeddedFiles))

// StaticAsset resolves a path relative to static/ to its embedded asset URL.
func StaticAsset(source string) (string, error) {
	var result string
	var err error
	valid := source != "." && fs.ValidPath(source) && !strings.Contains(source, "\\")

	if valid {
		for segment := range strings.SplitSeq(source, "/") {
			if strings.HasPrefix(segment, ".") {
				valid = false
				break
			}
		}
	}

	if !valid {
		err = ErrStaticAssetNotFound
	} else {
		info, statErr := fs.Stat(embeddedFiles, "static/"+source)

		if statErr != nil {
			err = fmt.Errorf("%w: %w", ErrStaticAssetNotFound, statErr)
		} else if !info.Mode().IsRegular() {
			err = ErrStaticAssetNotFound
		} else {
			assetURL := url.URL{Path: staticURLPrefix + source}
			result = assetURL.String()
		}
	}

	if err != nil {
		err = fmt.Errorf("resolve static asset %q: %w", source, err)
	}

	return result, err
}

func serveStaticAssets(writer http.ResponseWriter, request *http.Request) {
	source, matches := strings.CutPrefix(request.URL.Path, staticURLPrefix)

	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
	} else if !matches {
		http.NotFound(writer, request)
	} else if _, err := StaticAsset(source); err != nil {
		http.NotFound(writer, request)
	} else {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		staticHandler.ServeHTTP(writer, request)
	}
}
