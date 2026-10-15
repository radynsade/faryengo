package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config contains application settings read from the environment.
type Config struct {
	DatabaseURL        string
	RedisURL           string
	HTTPAddress        string
	JWTSigningSeed     string
	JWTKeyID           string
	JWTIssuer          string
	JWTAccessAudience  string
	JWTRefreshAudience string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	SessionTTL         time.Duration
	AuthCookieSecure   bool
	TrustedProxies     []netip.Prefix
}

// Load adds variables from .env when present, then reads application settings.
// Existing environment variables take precedence over values in .env.
func Load() (Config, error) {
	var settings Config

	if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return settings, fmt.Errorf("load .env: %w", err)
	}

	settings.DatabaseURL = os.Getenv("DATABASE_URL")
	settings.RedisURL = envDefault("REDIS_URL", "redis://127.0.0.1:6379/0")
	settings.HTTPAddress = envDefault("HTTP_ADDRESS", ":8080")
	settings.JWTSigningSeed = os.Getenv("JWT_SIGNING_SEED")
	settings.JWTKeyID = envDefault("JWT_KEY_ID", "v1")
	settings.JWTIssuer = envDefault("JWT_ISSUER", "faryen")
	settings.JWTAccessAudience = envDefault("JWT_ACCESS_AUDIENCE", "faryen-admin")
	settings.JWTRefreshAudience = envDefault("JWT_REFRESH_AUDIENCE", "faryen-refresh")
	accessTTL, accessErr := time.ParseDuration(envDefault("ACCESS_TOKEN_TTL", "5m"))
	refreshTTL, refreshErr := time.ParseDuration(envDefault("REFRESH_TOKEN_TTL", "720h"))
	sessionTTL, sessionErr := time.ParseDuration(envDefault("SESSION_TTL", "168h"))
	secure, secureErr := strconv.ParseBool(envDefault("AUTH_COOKIE_SECURE", "true"))
	proxies, proxiesErr := parsePrefixes(os.Getenv("TRUSTED_PROXIES"))
	var err error

	if accessErr != nil {
		err = fmt.Errorf("parse ACCESS_TOKEN_TTL: %w", accessErr)
	} else if refreshErr != nil {
		err = fmt.Errorf("parse REFRESH_TOKEN_TTL: %w", refreshErr)
	} else if sessionErr != nil {
		err = fmt.Errorf("parse SESSION_TTL: %w", sessionErr)
	} else if sessionTTL < time.Second || sessionTTL > 90*24*time.Hour {
		err = errors.New("SESSION_TTL must be between 1s and 2160h")
	} else if secureErr != nil {
		err = fmt.Errorf("parse AUTH_COOKIE_SECURE: %w", secureErr)
	} else if proxiesErr != nil {
		err = fmt.Errorf("parse TRUSTED_PROXIES: %w", proxiesErr)
	} else {
		settings.AccessTokenTTL, settings.RefreshTokenTTL, settings.SessionTTL, settings.AuthCookieSecure = accessTTL, refreshTTL, sessionTTL, secure
		settings.TrustedProxies = proxies
	}

	return settings, err
}

func envDefault(key, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		value = fallback
	}

	return value
}

// parsePrefixes reads a comma-separated list of IP addresses and CIDR
// prefixes; a single address stands for a prefix holding only itself.
func parsePrefixes(value string) ([]netip.Prefix, error) {
	var (
		prefixes []netip.Prefix
		err      error
	)

	items := strings.Split(value, ",")

	for index := 0; err == nil && index < len(items); index++ {
		item := strings.TrimSpace(items[index])

		if strings.Contains(item, "/") {
			var prefix netip.Prefix

			prefix, err = netip.ParsePrefix(item)
			prefixes = append(prefixes, prefix.Masked())
		} else if item != "" {
			var addr netip.Addr

			addr, err = netip.ParseAddr(item)
			prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}

	if err != nil {
		prefixes = nil
	}

	return prefixes, err
}

// SigningKey deliberately has no generated or development fallback. Keeping a
// stable, secret random seed permits verification across process restarts.
func (c Config) SigningKey() (ed25519.PrivateKey, error) {
	var key ed25519.PrivateKey
	var err error
	seed, decodeErr := base64.StdEncoding.Strict().DecodeString(c.JWTSigningSeed)

	if decodeErr != nil || len(seed) != ed25519.SeedSize {
		err = errors.New("JWT_SIGNING_SEED must be a base64-encoded random 32-byte Ed25519 seed")
	} else {
		key = ed25519.NewKeyFromSeed(seed)
	}

	return key, err
}
