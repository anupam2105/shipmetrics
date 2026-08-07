// Package dora computes the four DORA metrics from the deployment_events store
// and exports them as Prometheus gauges. Metrics are recomputed on a fixed
// interval so gauge values reflect a sliding lookback window.
package dora

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/anupam2105/shipmetrics/internal/observability"
	"github.com/anupam2105/shipmetrics/internal/storage"
)

// Config controls the analyzer loop.
type Config struct {
	// Interval between refreshes. Small enough to feel responsive on a demo,
	// large enough to keep DB load negligible.
	Interval time.Duration
	// Lookback window used for every metric. 7 days matches DORA's default
	// "recent" horizon and gives dashboards enough data on realistic services.
	Lookback time.Duration
}

// DefaultConfig returns sensible defaults used by main.go when the operator
// does not override them.
func DefaultConfig() Config {
	return Config{
		Interval: 30 * time.Second,
		Lookback: 7 * 24 * time.Hour,
	}
}

// Analyzer periodically refreshes DORA gauges from the store.
type Analyzer struct {
	store   storage.Store
	metrics *observability.Metrics
	logger  *slog.Logger
	cfg     Config
	now     func() time.Time
}

// New builds an Analyzer. Callers own store and metrics.
func New(store storage.Store, metrics *observability.Metrics, logger *slog.Logger, cfg Config) *Analyzer {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.Lookback <= 0 {
		cfg.Lookback = 7 * 24 * time.Hour
	}
	return &Analyzer{
		store:   store,
		metrics: metrics,
		logger:  logger,
		cfg:     cfg,
		now:     time.Now,
	}
}

// Run blocks until ctx is cancelled, refreshing metrics at each Interval.
// One immediate refresh runs on entry so dashboards populate without waiting.
func (a *Analyzer) Run(ctx context.Context) {
	if err := a.Tick(ctx); err != nil {
		a.logger.Warn("dora initial tick failed", "error", err)
	}

	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.Tick(ctx); err != nil {
				a.logger.Warn("dora tick failed", "error", err)
			}
		}
	}
}

// Tick runs one refresh: discover targets, compute stats for each, publish
// gauges. Exposed for tests.
func (a *Analyzer) Tick(ctx context.Context) error {
	since := a.now().Add(-a.cfg.Lookback)

	targets, err := a.store.ListDORATargets(ctx, since)
	if err != nil {
		return fmt.Errorf("list targets: %w", err)
	}

	for _, t := range targets {
		stats, err := a.store.DORAWindow(ctx, t, since)
		if err != nil {
			a.logger.Warn("dora window failed",
				"service", t.ServiceName,
				"environment", t.Environment,
				"error", err,
			)
			continue
		}
		a.publish(stats)
	}
	return nil
}

// publish maps one target's stats onto the labelled Prometheus gauges.
func (a *Analyzer) publish(s storage.DORAWindowStats) {
	labels := []string{s.Target.ServiceName, s.Target.Environment}

	a.metrics.DORADeployments.WithLabelValues(labels...).Set(float64(s.SuccessCount))
	a.metrics.DORAFailures.WithLabelValues(labels...).Set(float64(s.FailureCount))
	a.metrics.DORAChangeFailureRate.WithLabelValues(labels...).Set(s.ChangeFailureRate())
	a.metrics.DORALeadTimeSeconds.WithLabelValues(s.Target.ServiceName, s.Target.Environment, "0.5").Set(s.LeadTimeP50Sec)
	a.metrics.DORALeadTimeSeconds.WithLabelValues(s.Target.ServiceName, s.Target.Environment, "0.95").Set(s.LeadTimeP95Sec)
	a.metrics.DORAMTTRSeconds.WithLabelValues(labels...).Set(s.MTTRAverageSec)
}
