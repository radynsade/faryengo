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
			clearAuthEnvironment(t)
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

func TestTrustedProxies(t *testing.T) {
	for _, tt := range []struct {
		name    string
		value   string
		want    []string
		wantErr bool
	}{
		{name: "none by default"},
		{name: "addresses and prefixes", value: "10.0.0.1, 192.168.0.0/16,::1", want: []string{"10.0.0.1/32", "192.168.0.0/16", "::1/128"}},
		{name: "prefix is masked", value: "10.1.2.3/8", want: []string{"10.0.0.0/8"}},
		{name: "empty items are skipped", value: " ,10.0.0.1,", want: []string{"10.0.0.1/32"}},
		{name: "invalid address", value: "10.0.0.1,proxy", wantErr: true},
		{name: "invalid prefix", value: "10.0.0.0/33", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			clearAuthEnvironment(t)
			t.Setenv("TRUSTED_PROXIES", tt.value)

			settings, err := Load()

			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
					t.Fatalf("Load() error = %v, want a TRUSTED_PROXIES error", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			got := make([]string, len(settings.TrustedProxies))

			for index, prefix := range settings.TrustedProxies {
				got[index] = prefix.String()
			}

			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("TrustedProxies = %v, want %v", got, tt.want)
			}
		})
	}
}
