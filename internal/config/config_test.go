package config_test

import (
	"strings"
	"testing"

	"github.com/anupam2105/shipmetrics/internal/config"
)

// Not a real credential — synthetic DSN used only to satisfy the required-field
// check in tests that don't care about the DB.
const fakeDSN = "postgres://user:pass@localhost:5432/shipmetrics" //nolint:gosec // G101 false positive on fake test DSN

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SHIPMETRICS_HTTP_ADDR", "")
	t.Setenv("SHIPMETRICS_LOG_LEVEL", "")
	t.Setenv("SHIPMETRICS_LOG_FORMAT", "")
	t.Setenv("SHIPMETRICS_DATABASE_URL", fakeDSN)

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
	if cfg.DatabaseURL != fakeDSN {
		t.Errorf("DatabaseURL not picked up: got %q", cfg.DatabaseURL)
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	t.Setenv("SHIPMETRICS_DATABASE_URL", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL, got nil")
	}
	if !strings.Contains(err.Error(), "SHIPMETRICS_DATABASE_URL") {
		t.Errorf("error should mention env var name; got %q", err.Error())
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("SHIPMETRICS_LOG_LEVEL", "trace")
	t.Setenv("SHIPMETRICS_DATABASE_URL", fakeDSN)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
}

func TestLoadRejectsInvalidLogFormat(t *testing.T) {
	t.Setenv("SHIPMETRICS_LOG_FORMAT", "xml")
	t.Setenv("SHIPMETRICS_DATABASE_URL", fakeDSN)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid log format, got nil")
	}
}

func TestLoadHonoursEnvOverrides(t *testing.T) {
	t.Setenv("SHIPMETRICS_HTTP_ADDR", ":9090")
	t.Setenv("SHIPMETRICS_LOG_LEVEL", "DEBUG")
	t.Setenv("SHIPMETRICS_LOG_FORMAT", "TEXT")
	t.Setenv("SHIPMETRICS_DATABASE_URL", fakeDSN)

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
