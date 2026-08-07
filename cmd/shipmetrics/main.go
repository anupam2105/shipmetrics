package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/anupam2105/shipmetrics/internal/config"
	"github.com/anupam2105/shipmetrics/internal/httpserver"
	"github.com/anupam2105/shipmetrics/internal/logging"
	"github.com/anupam2105/shipmetrics/internal/observability"
	"github.com/anupam2105/shipmetrics/internal/storage/postgres"
	"github.com/anupam2105/shipmetrics/internal/webhook"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := logging.New(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(logger)

	metrics := observability.NewMetrics()

	// Run migrations *before* opening the runtime pool so the app never sees
	// a half-migrated schema.
	if err := postgres.Migrate(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	store := postgres.New(pool)
	webhookHandler := webhook.NewHandler(store, logger, metrics)
	srv := httpserver.New(cfg.HTTPAddr, logger, metrics, webhookHandler)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting server",
			"addr", cfg.HTTPAddr,
			"version", buildVersion,
		)
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}

// buildVersion is populated at build time via -ldflags.
var buildVersion = "dev"
