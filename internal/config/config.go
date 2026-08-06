// Package config loads and validates process configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all process configuration derived from the environment.
type Config struct {
	HTTPAddr        string
	LogLevel        string
	LogFormat       string
	DatabaseURL     string // empty until storage layer is wired in
	ShutdownTimeout time.Duration
}

// Load reads configuration from environment variables and validates it.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        getEnv("SHIPMETRICS_HTTP_ADDR", ":8080"),
		LogLevel:        strings.ToLower(getEnv("SHIPMETRICS_LOG_LEVEL", "info")),
		LogFormat:       strings.ToLower(getEnv("SHIPMETRICS_LOG_FORMAT", "json")),
		DatabaseURL:     getEnv("SHIPMETRICS_DATABASE_URL", ""),
		ShutdownTimeout: 15 * time.Second,
	}

	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("invalid SHIPMETRICS_LOG_LEVEL: %q (want debug|info|warn|error)", cfg.LogLevel)
	}

	switch cfg.LogFormat {
	case "json", "text":
	default:
		return Config{}, fmt.Errorf("invalid SHIPMETRICS_LOG_FORMAT: %q (want json|text)", cfg.LogFormat)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
