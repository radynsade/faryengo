package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Preload describes a stylesheet or font to fetch from the document head.
type Preload struct {
	URL  string
	As   string
	Type string
}

var preloadAssets []Preload

var cssURLPattern = regexp.MustCompile(`url\(\s*(?:"([^"]+)"|'([^']+)'|([^\s)]+))\s*\)`)

// Preloads returns the build's stylesheets and their WOFF2 font URLs.
// Font URLs retain CSS query strings so the preload and font requests match.
func Preloads() []Preload {
	return slices.Clone(preloadAssets)
}

func loadPreloads(ctx context.Context, built fs.FS) ([]Preload, error) {
	var result []Preload
	var data []byte
	err := ctx.Err()

	if err == nil {
		data, err = fs.ReadFile(built, ".vite/manifest.json")
	}

	if err == nil {
		var manifest map[string]struct {
			File string   `json:"file"`
			CSS  []string `json:"css"`
		}
		err = json.Unmarshal(data, &manifest)

		if err == nil {
			styles := make(map[string]struct{})
			fonts := make(map[string]struct{})

			for _, source := range slices.Sorted(maps.Keys(manifest)) {
				chunk := manifest[source]

				if strings.EqualFold(path.Ext(chunk.File), ".css") {
					styles[chunk.File] = struct{}{}
				}

				for _, css := range chunk.CSS {
					styles[css] = struct{}{}
				}
			}

			for _, css := range slices.Sorted(maps.Keys(styles)) {
				err = ctx.Err()

				if err == nil {
					data, err = fs.ReadFile(built, css)
				}

				if err != nil {
					err = fmt.Errorf("read stylesheet %q: %w", css, err)
					break
				}

				cssURL := url.URL{Path: URLPrefix + css}
				result = append(result, Preload{URL: cssURL.String(), As: "style", Type: "text/css"})

				for _, match := range cssURLPattern.FindAllSubmatch(data, -1) {
					reference := string(match[1]) + string(match[2]) + string(match[3])
					fontURL, parseErr := url.Parse(reference)

					if parseErr == nil && strings.EqualFold(path.Ext(fontURL.Path), ".woff2") {
						fonts[cssURL.ResolveReference(fontURL).String()] = struct{}{}
					}
				}
			}

			for _, fontURL := range slices.Sorted(maps.Keys(fonts)) {
				result = append(result, Preload{URL: fontURL, As: "font", Type: "font/woff2"})
			}
		}
	}

	if err == nil {
		err = ctx.Err()
	}

	if err != nil {
		result = nil
		err = fmt.Errorf("load admin asset preloads: %w", err)
	}

	return result, err
}
