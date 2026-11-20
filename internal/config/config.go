// Package config reads the service settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config holds the settings read from the environment.
type Config struct {
	Env         string // development, test or production
	Port        int
	DatabaseURL string
}

// Load reads the configuration from the environment, applying defaults.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	cfg := Config{Env: "development", Port: 3000, DatabaseURL: getenv("DATABASE_URL")}
	var errs []error

	if v := getenv("APP_ENV"); v != "" {
		switch v {
		case "development", "test", "production":
			cfg.Env = v
		default:
			errs = append(errs, fmt.Errorf("APP_ENV: must be development, test or production, got %q", v))
		}
	}
	if v := getenv("PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil || port <= 0 || port > 65535 {
			errs = append(errs, fmt.Errorf("PORT: must be a TCP port, got %q", v))
		}
		cfg.Port = port
	}
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL: required"))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("invalid environment: %w", err)
	}
	return cfg, nil
}
