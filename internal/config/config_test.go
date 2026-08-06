package config_test

import (
	"testing"

	"github.com/anupam2105/shipmetrics/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SHIPMETRICS_HTTP_ADDR", "")
	t.Setenv("SHIPMETRICS_LOG_LEVEL", "")
	t.Setenv("SHIPMETRICS_LOG_FORMAT", "")
	t.Setenv("SHIPMETRICS_DATABASE_URL", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json", cfg.LogFormat)
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
}

func TestLoadPicksUpDatabaseURL(t *testing.T) {
	// Not a real credential — synthetic DSN used only to assert env plumbing.
	const fakeDSN = "postgres://user:pass@localhost:5432/shipmetrics" //nolint:gosec // G101 false positive on fake test DSN
	t.Setenv("SHIPMETRICS_DATABASE_URL", fakeDSN)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != fakeDSN {
		t.Errorf("DatabaseURL not picked up: got %q", cfg.DatabaseURL)
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("SHIPMETRICS_LOG_LEVEL", "trace")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
}

func TestLoadRejectsInvalidLogFormat(t *testing.T) {
	t.Setenv("SHIPMETRICS_LOG_FORMAT", "xml")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid log format, got nil")
	}
}

func TestLoadHonoursEnvOverrides(t *testing.T) {
	t.Setenv("SHIPMETRICS_HTTP_ADDR", ":9090")
	t.Setenv("SHIPMETRICS_LOG_LEVEL", "DEBUG")
	t.Setenv("SHIPMETRICS_LOG_FORMAT", "TEXT")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug (case-insensitive)", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want text (case-insensitive)", cfg.LogFormat)
	}
}
