package config_test

import (
	"testing"

	"github.com/anupam2105/shipmetrics/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SHIPMETRICS_HTTP_ADDR", "")
	t.Setenv("SHIPMETRICS_LOG_LEVEL", "")
	t.Setenv("SHIPMETRICS_LOG_FORMAT", "")

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
