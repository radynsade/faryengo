package assets

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbeddedAssets(t *testing.T) {
	data, err := embeddedFiles.ReadFile("dist/.vite/manifest.json")
	if err != nil {
		t.Fatal(err)
	}

	var manifest map[string]struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	if err := RegisterHandlers(mux); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name   string
		source string
		lookup func(string) (string, error)
	}{
		{name: "script", source: "main.ts", lookup: BuiltAsset},
		{name: "CSS", source: "style.scss", lookup: BuiltCSS},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := manifest["src/"+tt.source].File
			if file == "" {
				t.Fatalf("manifest is missing %q", tt.source)
			}

			url, err := tt.lookup(tt.source)
			if err != nil || url != URLPrefix+file {
				t.Fatalf("lookup(%q) = (%q, %v), want %q", tt.source, url, err, URLPrefix+file)
			}

			expected, err := fs.ReadFile(embeddedFiles, "dist/"+file)
			if err != nil {
				t.Fatal(err)
			}

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url, nil))
			if response.Code != http.StatusOK || response.Body.String() != string(expected) {
				t.Fatalf("embedded asset response = %d %q", response.Code, response.Body.String())
			}
		})
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, URLPrefix+".vite/manifest.json", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("manifest response = %d, want 404", response.Code)
	}
}

func TestRegisterHandlersRejectsNilMux(t *testing.T) {
	if err := RegisterHandlers(nil); !errors.Is(err, ErrNilServeMux) {
		t.Fatalf("RegisterHandlers(nil) error = %v, want ErrNilServeMux", err)
	}
}
