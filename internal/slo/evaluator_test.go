package slo_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
	"github.com/anupam2105/shipmetrics/internal/observability"
	"github.com/anupam2105/shipmetrics/internal/slo"
	"github.com/anupam2105/shipmetrics/internal/storage"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeStore only implements the read surface the evaluator uses.
type fakeStore struct {
	targets      []storage.DORATarget
	observations map[string]storage.ObservationWindow // key = service|env|window
	observeCalls int
}

func newFakeStore() *fakeStore {
	return &fakeStore{observations: map[string]storage.ObservationWindow{}}
}

func obsKey(svc, env string, window time.Duration) string {
	return svc + "|" + env + "|" + window.String()
}

// Store interface fluff — unused surface returns zero values.
func (f *fakeStore) UpsertEvent(_ context.Context, _ *domain.DeploymentEvent) error { return nil }
func (f *fakeStore) GetEvent(_ context.Context, _ domain.Source, _ string) (*domain.DeploymentEvent, error) {
	return nil, storage.ErrNotFound
}
func (f *fakeStore) ListRecentEvents(_ context.Context, _ storage.EventFilter, _ int) ([]*domain.DeploymentEvent, error) {
	return nil, nil
}
func (f *fakeStore) DORAWindow(_ context.Context, t storage.DORATarget, _ time.Time) (storage.DORAWindowStats, error) {
	return storage.DORAWindowStats{Target: t}, nil
}
func (f *fakeStore) Ping(_ context.Context) error { return nil }

func (f *fakeStore) ListDORATargets(_ context.Context, _ time.Time) ([]storage.DORATarget, error) {
	return f.targets, nil
}

func (f *fakeStore) ObservationWindow(_ context.Context, svc, env string, since, until time.Time) (storage.ObservationWindow, error) {
	f.observeCalls++
	window := until.Sub(since)
	if obs, ok := f.observations[obsKey(svc, env, window)]; ok {
		return obs, nil
	}
	return storage.ObservationWindow{}, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- Evaluate() pure logic tests ---

func TestEvaluateCompliantWhenFailureRateBelowBudget(t *testing.T) {
	t.Parallel()
	def := slo.Definition{ServiceName: "svc", Environment: "prod", Target: 0.99, Window: time.Hour}
	e := slo.Evaluate(def, 100, 0, time.Now())
	if !e.Compliant {
		t.Errorf("expected compliant, got %+v", e)
	}
	if e.BurnRate != 0 {
		t.Errorf("BurnRate = %v, want 0 (no failures)", e.BurnRate)
	}
}

func TestEvaluateBurnRateForExcessFailures(t *testing.T) {
	t.Parallel()
	def := slo.Definition{ServiceName: "svc", Environment: "prod", Target: 0.99, Window: time.Hour}
	// 5 failures out of 100 → 5% failure rate, budget = 1%, burn = 5.
	e := slo.Evaluate(def, 95, 5, time.Now())
	if e.Compliant {
		t.Errorf("expected non-compliant, got compliant")
	}
	if e.BurnRate < 4.99 || e.BurnRate > 5.01 {
		t.Errorf("BurnRate = %v, want 5", e.BurnRate)
	}
}

func TestEvaluateEmptyWindowIsCompliant(t *testing.T) {
	t.Parallel()
	def := slo.Definition{ServiceName: "svc", Environment: "prod", Target: 0.99, Window: time.Hour}
	e := slo.Evaluate(def, 0, 0, time.Now())
	if !e.Compliant {
		t.Errorf("expected compliant for empty window; got %+v", e)
	}
	if e.FailureRate != 0 || e.BurnRate != 0 {
		t.Errorf("empty window emitted non-zero rate: %+v", e)
	}
}

func TestDefinitionValidate(t *testing.T) {
	t.Parallel()

	cases := map[string]slo.Definition{
		"missing service": {Environment: "prod", Target: 0.99, Window: time.Hour},
		"missing env":     {ServiceName: "s", Target: 0.99, Window: time.Hour},
		"target zero":     {ServiceName: "s", Environment: "prod", Target: 0, Window: time.Hour},
		"target one":      {ServiceName: "s", Environment: "prod", Target: 1, Window: time.Hour},
		"zero window":     {ServiceName: "s", Environment: "prod", Target: 0.99, Window: 0},
		"negative window": {ServiceName: "s", Environment: "prod", Target: 0.99, Window: -1 * time.Second},
	}
	for name, def := range cases {
		def := def
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := def.Validate(); err == nil {
				t.Errorf("%s: expected error, got nil", name)
			}
		})
	}

	valid := slo.Definition{ServiceName: "s", Environment: "prod", Target: 0.99, Window: time.Hour}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid def failed: %v", err)
	}
}

// --- Evaluator loop tests ---

func TestTickEvaluatesEachTargetAgainstEachWindow(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.targets = []storage.DORATarget{
		{ServiceName: "checkout-api", Environment: "prod"},
	}
	// 1h window: 2 failures / 20 total → 10% failure rate, prod budget = 1%, burn = 10x.
	store.observations[obsKey("checkout-api", "prod", time.Hour)] = storage.ObservationWindow{
		SuccessCount: 18, FailureCount: 2,
	}
	// 6h window: 5 failures / 100 total → 5% failure rate, burn = 5x.
	store.observations[obsKey("checkout-api", "prod", 6*time.Hour)] = storage.ObservationWindow{
		SuccessCount: 95, FailureCount: 5,
	}

	metrics := observability.NewMetrics()
	ev := slo.New(store, metrics, discardLogger(), slo.DefaultConfig())
	if err := ev.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if store.observeCalls != 2 {
		t.Errorf("observeCalls = %d, want 2 (one per window)", store.observeCalls)
	}

	burn1h := testutil.ToFloat64(metrics.SLOBurnRate.WithLabelValues("checkout-api", "prod", "1h0m0s"))
	if burn1h < 9.99 || burn1h > 10.01 {
		t.Errorf("1h burn rate = %v, want ~10", burn1h)
	}
	burn6h := testutil.ToFloat64(metrics.SLOBurnRate.WithLabelValues("checkout-api", "prod", "6h0m0s"))
	if burn6h < 4.99 || burn6h > 5.01 {
		t.Errorf("6h burn rate = %v, want ~5", burn6h)
	}

	compliant1h := testutil.ToFloat64(metrics.SLOCompliant.WithLabelValues("checkout-api", "prod", "1h0m0s"))
	if compliant1h != 0 {
		t.Errorf("1h compliant = %v, want 0 (violating)", compliant1h)
	}
}

func TestTickAppliesEnvironmentSpecificTargets(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.targets = []storage.DORATarget{
		{ServiceName: "svc", Environment: "prod"},
		{ServiceName: "svc", Environment: "stage"},
	}
	// Same failure rate (2%), but prod target 99% (budget 1%, burn 2x) vs
	// stage target 95% (budget 5%, burn 0.4x).
	for _, w := range []time.Duration{time.Hour, 6 * time.Hour} {
		store.observations[obsKey("svc", "prod", w)] = storage.ObservationWindow{SuccessCount: 98, FailureCount: 2}
		store.observations[obsKey("svc", "stage", w)] = storage.ObservationWindow{SuccessCount: 98, FailureCount: 2}
	}

	metrics := observability.NewMetrics()
	ev := slo.New(store, metrics, discardLogger(), slo.DefaultConfig())
	if err := ev.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	burnProd := testutil.ToFloat64(metrics.SLOBurnRate.WithLabelValues("svc", "prod", "1h0m0s"))
	burnStage := testutil.ToFloat64(metrics.SLOBurnRate.WithLabelValues("svc", "stage", "1h0m0s"))
	if burnProd < 1.99 || burnProd > 2.01 {
		t.Errorf("prod burn = %v, want ~2", burnProd)
	}
	if burnStage < 0.39 || burnStage > 0.41 {
		t.Errorf("stage burn = %v, want ~0.4", burnStage)
	}

	compliantProd := testutil.ToFloat64(metrics.SLOCompliant.WithLabelValues("svc", "prod", "1h0m0s"))
	compliantStage := testutil.ToFloat64(metrics.SLOCompliant.WithLabelValues("svc", "stage", "1h0m0s"))
	if compliantProd != 0 {
		t.Errorf("prod compliant = %v, want 0 (2%% > 1%% budget)", compliantProd)
	}
	if compliantStage != 1 {
		t.Errorf("stage compliant = %v, want 1 (2%% < 5%% budget)", compliantStage)
	}
}
