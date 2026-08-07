package dora_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
	"github.com/anupam2105/shipmetrics/internal/dora"
	"github.com/anupam2105/shipmetrics/internal/observability"
	"github.com/anupam2105/shipmetrics/internal/storage"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeStore is a tiny in-memory implementation of storage.Store scoped to the
// analyzer's read surface. Unrelated methods return zero values.
type fakeStore struct {
	targets    []storage.DORATarget
	stats      map[string]storage.DORAWindowStats // key = service|env
	statsErr   error
	targetsErr error

	windowCalls int
}

func newFakeStore(targets []storage.DORATarget, stats map[string]storage.DORAWindowStats) *fakeStore {
	return &fakeStore{targets: targets, stats: stats}
}

func key(t storage.DORATarget) string { return t.ServiceName + "|" + t.Environment }

func (f *fakeStore) UpsertEvent(_ context.Context, _ *domain.DeploymentEvent) error { return nil }
func (f *fakeStore) GetEvent(_ context.Context, _ domain.Source, _ string) (*domain.DeploymentEvent, error) {
	return nil, storage.ErrNotFound
}
func (f *fakeStore) ListRecentEvents(_ context.Context, _ storage.EventFilter, _ int) ([]*domain.DeploymentEvent, error) {
	return nil, nil
}
func (f *fakeStore) Ping(_ context.Context) error { return nil }

func (f *fakeStore) ListDORATargets(_ context.Context, _ time.Time) ([]storage.DORATarget, error) {
	if f.targetsErr != nil {
		return nil, f.targetsErr
	}
	return f.targets, nil
}

func (f *fakeStore) DORAWindow(_ context.Context, t storage.DORATarget, _ time.Time) (storage.DORAWindowStats, error) {
	f.windowCalls++
	if f.statsErr != nil {
		return storage.DORAWindowStats{}, f.statsErr
	}
	s, ok := f.stats[key(t)]
	if !ok {
		return storage.DORAWindowStats{Target: t}, nil
	}
	return s, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestTickPopulatesGaugesForEachTarget(t *testing.T) {
	t.Parallel()

	targets := []storage.DORATarget{
		{ServiceName: "checkout-api", Environment: "prod"},
		{ServiceName: "cart", Environment: "stage"},
	}
	stats := map[string]storage.DORAWindowStats{
		"checkout-api|prod": {
			Target:         targets[0],
			SuccessCount:   40,
			FailureCount:   4,
			LeadTimeP50Sec: 3600,
			LeadTimeP95Sec: 7200,
			MTTRAverageSec: 1800,
		},
		"cart|stage": {
			Target:         targets[1],
			SuccessCount:   10,
			FailureCount:   1,
			LeadTimeP50Sec: 900,
			LeadTimeP95Sec: 1800,
			MTTRAverageSec: 600,
		},
	}

	store := newFakeStore(targets, stats)
	metrics := observability.NewMetrics()
	a := dora.New(store, metrics, discardLogger(), dora.Config{Interval: time.Hour, Lookback: 24 * time.Hour})

	if err := a.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if store.windowCalls != 2 {
		t.Errorf("windowCalls = %d, want 2", store.windowCalls)
	}

	if got := testutil.ToFloat64(metrics.DORADeployments.WithLabelValues("checkout-api", "prod")); got != 40 {
		t.Errorf("checkout-api prod deployments = %v, want 40", got)
	}
	if got := testutil.ToFloat64(metrics.DORAFailures.WithLabelValues("checkout-api", "prod")); got != 4 {
		t.Errorf("checkout-api prod failures = %v, want 4", got)
	}
	if got := testutil.ToFloat64(metrics.DORADeployments.WithLabelValues("cart", "stage")); got != 10 {
		t.Errorf("cart stage deployments = %v, want 10", got)
	}

	wantCFR := 4.0 / 44.0
	if got := testutil.ToFloat64(metrics.DORAChangeFailureRate.WithLabelValues("checkout-api", "prod")); got < wantCFR-1e-6 || got > wantCFR+1e-6 {
		t.Errorf("cfr(prod, checkout-api) = %f, want ~%f", got, wantCFR)
	}

	if got := testutil.ToFloat64(metrics.DORALeadTimeSeconds.WithLabelValues("checkout-api", "prod", "0.5")); got != 3600 {
		t.Errorf("lead-time p50 = %v, want 3600", got)
	}
	if got := testutil.ToFloat64(metrics.DORALeadTimeSeconds.WithLabelValues("checkout-api", "prod", "0.95")); got != 7200 {
		t.Errorf("lead-time p95 = %v, want 7200", got)
	}
	if got := testutil.ToFloat64(metrics.DORAMTTRSeconds.WithLabelValues("checkout-api", "prod")); got != 1800 {
		t.Errorf("mttr = %v, want 1800", got)
	}
}

func TestTickSurfacesTargetsError(t *testing.T) {
	t.Parallel()
	store := newFakeStore(nil, nil)
	store.targetsErr = errors.New("db offline")

	a := dora.New(store, observability.NewMetrics(), discardLogger(), dora.DefaultConfig())
	if err := a.Tick(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTickContinuesWhenSingleWindowFails(t *testing.T) {
	t.Parallel()

	targets := []storage.DORATarget{
		{ServiceName: "s1", Environment: "prod"},
		{ServiceName: "s2", Environment: "prod"},
	}
	store := newFakeStore(targets, map[string]storage.DORAWindowStats{
		"s2|prod": {Target: targets[1], SuccessCount: 5},
	})
	store.statsErr = errors.New("transient")

	a := dora.New(store, observability.NewMetrics(), discardLogger(), dora.DefaultConfig())
	err := a.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if store.windowCalls != 2 {
		t.Errorf("windowCalls = %d, want 2 (should attempt every target)", store.windowCalls)
	}
}

func TestChangeFailureRateHandlesEmptyWindow(t *testing.T) {
	t.Parallel()
	s := storage.DORAWindowStats{}
	if got := s.ChangeFailureRate(); got != 0 {
		t.Errorf("empty window CFR = %v, want 0", got)
	}
}
