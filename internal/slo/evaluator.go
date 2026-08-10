package slo

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/anupam2105/shipmetrics/internal/observability"
	"github.com/anupam2105/shipmetrics/internal/storage"
)

// Config controls the evaluator loop.
type Config struct {
	// Interval between refreshes. Small enough for demo responsiveness,
	// large enough that DB cost stays negligible even with dozens of targets.
	Interval time.Duration

	// Windows to evaluate for every discovered target. The canonical pair is
	// 1h (fast burn) and 6h (slow burn) — see docs/slo/error-budget-policy.md.
	Windows []time.Duration

	// DefaultTargets maps environment → target success rate. Any (service,
	// environment) pair discovered from ListDORATargets is evaluated against
	// the default for its environment, falling back to defaultTarget.
	DefaultTargets map[string]float64
	DefaultTarget  float64

	// Lookback constrains ListDORATargets — only pairs that have deployed
	// within this window are evaluated. Long enough that idle services still
	// show up during a demo.
	Lookback time.Duration
}

// DefaultConfig returns production-shaped defaults: 30s tick, 1h + 6h windows,
// 99% prod SLO, 95% for stage/dev.
func DefaultConfig() Config {
	return Config{
		Interval: 30 * time.Second,
		Windows:  []time.Duration{time.Hour, 6 * time.Hour},
		DefaultTargets: map[string]float64{
			"prod":        0.99,
			"production":  0.99,
			"stage":       0.95,
			"staging":     0.95,
			"dev":         0.90,
			"development": 0.90,
		},
		DefaultTarget: 0.95,
		Lookback:      7 * 24 * time.Hour,
	}
}

// Evaluator periodically applies SLO Definitions to observed events and
// publishes burn-rate and compliance gauges.
type Evaluator struct {
	store   storage.Store
	metrics *observability.Metrics
	logger  *slog.Logger
	cfg     Config
	now     func() time.Time
}

// New builds an Evaluator using cfg, filling missing fields from DefaultConfig.
func New(store storage.Store, metrics *observability.Metrics, logger *slog.Logger, cfg Config) *Evaluator {
	defaults := DefaultConfig()
	if cfg.Interval <= 0 {
		cfg.Interval = defaults.Interval
	}
	if len(cfg.Windows) == 0 {
		cfg.Windows = defaults.Windows
	}
	if cfg.DefaultTargets == nil {
		cfg.DefaultTargets = defaults.DefaultTargets
	}
	if cfg.DefaultTarget == 0 {
		cfg.DefaultTarget = defaults.DefaultTarget
	}
	if cfg.Lookback <= 0 {
		cfg.Lookback = defaults.Lookback
	}
	return &Evaluator{
		store:   store,
		metrics: metrics,
		logger:  logger,
		cfg:     cfg,
		now:     time.Now,
	}
}

// Run blocks until ctx is cancelled, refreshing at each Interval. An immediate
// refresh runs on entry so dashboards populate without waiting.
func (e *Evaluator) Run(ctx context.Context) {
	if err := e.Tick(ctx); err != nil {
		e.logger.Warn("slo initial tick failed", "error", err)
	}

	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.Tick(ctx); err != nil {
				e.logger.Warn("slo tick failed", "error", err)
			}
		}
	}
}

// Tick performs one refresh: discover targets, evaluate each (target, window)
// pair, publish gauges. Exposed for tests.
func (e *Evaluator) Tick(ctx context.Context) error {
	now := e.now()
	lookback := now.Add(-e.cfg.Lookback)

	targets, err := e.store.ListDORATargets(ctx, lookback)
	if err != nil {
		return fmt.Errorf("list targets: %w", err)
	}

	for _, t := range targets {
		target := e.targetFor(t.Environment)
		for _, window := range e.cfg.Windows {
			since := now.Add(-window)
			obs, err := e.store.ObservationWindow(ctx, t.ServiceName, t.Environment, since, now)
			if err != nil {
				e.logger.Warn("observation window failed",
					"service", t.ServiceName, "environment", t.Environment,
					"window", window, "error", err)
				continue
			}
			def := Definition{
				ServiceName: t.ServiceName,
				Environment: t.Environment,
				Target:      target,
				Window:      window,
			}
			eval := Evaluate(def, obs.SuccessCount, obs.FailureCount, now)
			e.publish(&eval)
		}
	}
	return nil
}

// targetFor returns the configured SLO target for an environment, falling
// back to the global default when the environment is not explicitly set.
func (e *Evaluator) targetFor(env string) float64 {
	if t, ok := e.cfg.DefaultTargets[env]; ok {
		return t
	}
	return e.cfg.DefaultTarget
}

// publish maps one Evaluation onto the labelled Prometheus gauges.
// Takes a pointer so the 112-byte struct isn't copied every tick.
func (e *Evaluator) publish(v *Evaluation) {
	labels := []string{v.Definition.ServiceName, v.Definition.Environment, v.Definition.WindowLabel()}

	e.metrics.SLOBurnRate.WithLabelValues(labels...).Set(v.BurnRate)
	e.metrics.SLOFailureRate.WithLabelValues(labels...).Set(v.FailureRate)
	e.metrics.SLOTarget.WithLabelValues(labels...).Set(v.Definition.Target)

	compliant := 0.0
	if v.Compliant {
		compliant = 1.0
	}
	e.metrics.SLOCompliant.WithLabelValues(labels...).Set(compliant)
}
