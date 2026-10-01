package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config contains application settings read from the environment.
type Config struct {
	DatabaseURL string
}

// Load adds variables from .env when present, then reads application settings.
// Existing environment variables take precedence over values in .env.
func Load() (Config, error) {
	var settings Config

	if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return settings, fmt.Errorf("load .env: %w", err)
	}

	settings.DatabaseURL = os.Getenv("DATABASE_URL")
	return settings, nil
}
