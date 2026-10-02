package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func clearAuthEnvironment(t *testing.T) {
	t.Helper()

	for _, key := range []string{"REDIS_URL", "HTTP_ADDRESS", "JWT_SIGNING_SEED", "JWT_KEY_ID", "JWT_ISSUER", "JWT_ACCESS_AUDIENCE", "JWT_REFRESH_AUDIENCE", "ACCESS_TOKEN_TTL", "REFRESH_TOKEN_TTL", "AUTH_COOKIE_SECURE"} {
		t.Setenv(key, "")
	}
}

func TestAuthenticationConfig(t *testing.T) {
	for _, tt := range []struct {
		name, key, value string
		wantErr          bool
	}{
		{name: "secure defaults"},
		{name: "local HTTP", key: "AUTH_COOKIE_SECURE", value: "false"},
		{name: "bad secure flag", key: "AUTH_COOKIE_SECURE", value: "invalid", wantErr: true},
		{name: "bad access lifetime", key: "ACCESS_TOKEN_TTL", value: "invalid", wantErr: true},
		{name: "bad refresh lifetime", key: "REFRESH_TOKEN_TTL", value: "invalid", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			clearAuthEnvironment(t)

			if tt.key != "" {
				t.Setenv(tt.key, tt.value)
			}

			config, err := Load()

			if tt.wantErr {
				if err == nil {
					t.Fatal("invalid configuration accepted")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}

				if config.AccessTokenTTL != 5*time.Minute || config.RefreshTokenTTL != 30*24*time.Hour {
					t.Fatal("incorrect token defaults")
				}

				if config.AuthCookieSecure != (tt.value != "false") {
					t.Fatal("incorrect cookie default")
				}

				if _, err := config.SigningKey(); err == nil {
					t.Fatal("missing signing key accepted")
				}
			}
		})
	}
}

func TestJWTSigningSeed(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, seed string
		valid      bool
	}{
		{name: "empty"}, {name: "malformed", seed: "not-base64"},
		{name: "short", seed: base64.StdEncoding.EncodeToString(make([]byte, 16))},
		{name: "long", seed: base64.StdEncoding.EncodeToString(make([]byte, 33))},
		{name: "valid", seed: base64.StdEncoding.EncodeToString(seed), valid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key, err := (Config{JWTSigningSeed: tt.seed}).SigningKey()

			if tt.valid {
				if err != nil || len(key) != ed25519.PrivateKeySize {
					t.Fatal("valid seed rejected")
				}
			} else if err == nil {
				t.Fatal("invalid seed accepted")
			}
		})
	}
}
