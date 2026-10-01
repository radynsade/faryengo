package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	for _, tt := range []struct {
		name            string
		file            string
		writeFile       bool
		processURL      string
		wantDatabaseURL string
		wantExtra       string
		wantErr         string
	}{
		{
			name:            "loads dot env",
			file:            "DATABASE_URL=postgres://file\nFARYEN_CONFIG_TEST_EXTRA=loaded\n",
			writeFile:       true,
			wantDatabaseURL: "postgres://file",
			wantExtra:       "loaded",
		},
		{
			name:            "existing environment wins",
			file:            "DATABASE_URL=postgres://file\nFARYEN_CONFIG_TEST_EXTRA=loaded\n",
			writeFile:       true,
			processURL:      "postgres://process",
			wantDatabaseURL: "postgres://process",
			wantExtra:       "loaded",
		},
		{
			name:            "missing dot env uses environment",
			processURL:      "postgres://process",
			wantDatabaseURL: "postgres://process",
		},
		{
			name:      "invalid dot env",
			file:      "DATABASE_URL=\"unterminated\n",
			writeFile: true,
			wantErr:   "load .env",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()
			t.Chdir(directory)
			unsetEnv(t, "DATABASE_URL")
			unsetEnv(t, "FARYEN_CONFIG_TEST_EXTRA")

			if tt.processURL != "" {
				t.Setenv("DATABASE_URL", tt.processURL)
			}

			if tt.writeFile {
				if err := os.WriteFile(filepath.Join(directory, ".env"), []byte(tt.file), 0o600); err != nil {
					t.Fatalf("write .env: %v", err)
				}
			}

			settings, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if settings.DatabaseURL != tt.wantDatabaseURL {
				t.Fatalf("Load().DatabaseURL = %q, want %q", settings.DatabaseURL, tt.wantDatabaseURL)
			}

			if got := os.Getenv("FARYEN_CONFIG_TEST_EXTRA"); got != tt.wantExtra {
				t.Fatalf("FARYEN_CONFIG_TEST_EXTRA = %q, want %q", got, tt.wantExtra)
			}
		})
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	original, exists := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}

	t.Cleanup(func() {
		var err error
		if exists {
			err = os.Setenv(key, original)
		} else {
			err = os.Unsetenv(key)
		}

		if err != nil {
			t.Errorf("restore %s: %v", key, err)
		}
	})
}
