package jwt

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/radynsade/faryengo/internal/security"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func testManager(t *testing.T) *Manager {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)

	if err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(Config{PrivateKey: key, KeyID: "current", Issuer: "faryen", AccessAudience: "admin", RefreshAudience: "refresh", AccessTTL: 5 * time.Minute})

	if err != nil {
		t.Fatal(err)
	}

	manager.now = func() time.Time { return time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC) }
	return manager
}

func TestJWTValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		use    security.TokenUse
		mutate func(*jwtlib.Token, *claims)
		raw    string
		valid  bool
	}{
		{name: "access", use: security.AccessToken, valid: true},
		{name: "refresh", use: security.RefreshToken, valid: true},
		{name: "wrong token use", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.Use = security.RefreshToken }},
		{name: "wrong issuer", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.Issuer = "attacker" }},
		{name: "wrong audience", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.Audience = jwtlib.ClaimStrings{"refresh"} }},
		{name: "extra audience", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.Audience = jwtlib.ClaimStrings{"admin", "other"} }},
		{name: "missing expiry", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.ExpiresAt = nil }},
		{name: "missing issued at", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.IssuedAt = nil }},
		{name: "missing not before", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.NotBefore = nil }},
		{name: "expired", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.ExpiresAt = jwtlib.NewNumericDate(c.IssuedAt.Add(-time.Minute)) }},
		{name: "future issued at", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) {
			c.IssuedAt = jwtlib.NewNumericDate(c.IssuedAt.Add(time.Minute))
			c.NotBefore = c.IssuedAt
		}},
		{name: "wrong type", use: security.AccessToken, mutate: func(t *jwtlib.Token, _ *claims) { t.Header["typ"] = "refresh+jwt" }},
		{name: "unknown key", use: security.AccessToken, mutate: func(t *jwtlib.Token, _ *claims) { t.Header["kid"] = "unknown" }},
		{name: "missing key", use: security.AccessToken, mutate: func(t *jwtlib.Token, _ *claims) { delete(t.Header, "kid") }},
		{name: "nil user", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.Subject = uuid.Nil.String() }},
		{name: "invalid session", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.SessionID = "bad" }},
		{name: "missing token ID", use: security.AccessToken, mutate: func(_ *jwtlib.Token, c *claims) { c.ID = "" }},
		{name: "none algorithm", use: security.AccessToken, mutate: func(t *jwtlib.Token, _ *claims) { t.Method = jwtlib.SigningMethodNone }},
		{name: "HMAC confusion", use: security.AccessToken, mutate: func(t *jwtlib.Token, _ *claims) { t.Method = jwtlib.SigningMethodHS256 }},
		{name: "malformed", use: security.AccessToken, raw: "bad"},
		{name: "oversized", use: security.AccessToken, raw: strings.Repeat("a", 4097)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager := testManager(t)
			userID, sessionID := security.UserID(uuid.New()), uuid.New()
			pair, err := manager.Issue(context.Background(), userID, sessionID, manager.now().Add(time.Hour))

			if err != nil {
				t.Fatal(err)
			}

			raw := pair.AccessToken

			if tt.use == security.RefreshToken {
				raw = pair.RefreshToken
			}

			if tt.raw != "" {
				raw = tt.raw
			}

			if tt.mutate != nil {
				c := &claims{}
				token, _, parseErr := new(jwtlib.Parser).ParseUnverified(raw, c)

				if parseErr != nil {
					t.Fatal(parseErr)
				}

				tt.mutate(token, c)
				var key any = manager.config.PrivateKey

				if token.Method.Alg() == "none" {
					key = jwtlib.UnsafeAllowNoneSignatureType
				}

				if token.Method.Alg() == "HS256" {
					key = []byte(strings.Repeat("a", 32))
				}

				raw, err = token.SignedString(key)

				if err != nil {
					t.Fatal(err)
				}
			}

			parsed, err := manager.Verify(context.Background(), raw, tt.use)

			if tt.valid {
				if err != nil || parsed.UserID != userID || parsed.SessionID != sessionID {
					t.Fatalf("claims = %v, error = %v", parsed, err)
				}
			} else if !errors.Is(err, security.ErrInvalidToken) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestJWTSeparationExpiryAndCancellation(t *testing.T) {
	manager := testManager(t)
	pair, err := manager.Issue(context.Background(), security.UserID(uuid.New()), uuid.New(), manager.now().Add(time.Hour))

	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, raw string
		use       security.TokenUse
	}{
		{"refresh as access", pair.RefreshToken, security.AccessToken},
		{"access as refresh", pair.AccessToken, security.RefreshToken},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := manager.Verify(context.Background(), tt.raw, tt.use); !errors.Is(err, security.ErrInvalidToken) {
				t.Fatal(err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := manager.Verify(ctx, pair.AccessToken, security.AccessToken); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	manager.now = func() time.Time { return pair.AccessExpiresAt.Add(31 * time.Second) }

	if _, err := manager.Verify(context.Background(), pair.AccessToken, security.AccessToken); !errors.Is(err, security.ErrInvalidToken) {
		t.Fatal(err)
	}
}

func TestJWTKeyRotation(t *testing.T) {
	old := testManager(t)
	pair, err := old.Issue(context.Background(), security.UserID(uuid.New()), uuid.New(), old.now().Add(time.Hour))

	if err != nil {
		t.Fatal(err)
	}

	next := testManager(t)
	config := next.config
	config.KeyID = "next"
	config.VerificationKeys = map[string]ed25519.PublicKey{"current": old.config.PrivateKey.Public().(ed25519.PublicKey)}
	rotated, err := NewManager(config)

	if err != nil {
		t.Fatal(err)
	}

	rotated.now = old.now
	clear(config.VerificationKeys)

	if _, err := rotated.Verify(context.Background(), pair.AccessToken, security.AccessToken); err != nil {
		t.Fatal(err)
	}
}
