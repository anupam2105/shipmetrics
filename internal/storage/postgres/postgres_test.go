package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
	"github.com/anupam2105/shipmetrics/internal/storage"
	"github.com/anupam2105/shipmetrics/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool returns a pool connected to TEST_DATABASE_URL, or skips the test
// when the env var is unset. Callers use this so unit test runs stay fast
// and don't require Docker; CI and local `just db-up` runs enable it.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping Postgres integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	if err := postgres.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	// Reset table between tests so ordering assertions stay deterministic.
	if _, err := pool.Exec(ctx, "TRUNCATE deployment_events"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}

func sampleEvent(pipelineID string, started time.Time, status domain.Status) *domain.DeploymentEvent {
	finished := started.Add(3 * time.Minute)
	e := &domain.DeploymentEvent{
		Source:       domain.SourceJenkins,
		PipelineName: "checkout-api-deploy",
		PipelineID:   pipelineID,
		ServiceName:  "checkout-api",
		Environment:  "prod",
		Status:       status,
		StartedAt:    started,
		CommitSHA:    "deadbeef",
		Metadata:     map[string]string{"triggered_by": "webhook"},
	}
	if status.Terminal() {
		e.FinishedAt = &finished
	}
	return e
}

func TestUpsertAndGetRoundTrip(t *testing.T) {
	pool := testPool(t)
	s := postgres.New(pool)
	ctx := context.Background()

	want := sampleEvent("run-1", time.Now().UTC().Add(-10*time.Minute), domain.StatusSuccess)
	if err := s.UpsertEvent(ctx, want); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}

	got, err := s.GetEvent(ctx, domain.SourceJenkins, "run-1")
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.PipelineID != want.PipelineID ||
		got.ServiceName != want.ServiceName ||
		got.Environment != want.Environment ||
		got.Status != want.Status ||
		got.CommitSHA != want.CommitSHA {
		t.Errorf("event mismatch:\ngot  %+v\nwant %+v", got, want)
	}
	if got.Metadata["triggered_by"] != "webhook" {
		t.Errorf("metadata lost: got %v", got.Metadata)
	}
}

func TestUpsertIsIdempotent(t *testing.T) {
	pool := testPool(t)
	s := postgres.New(pool)
	ctx := context.Background()

	first := sampleEvent("run-2", time.Now().UTC().Add(-10*time.Minute), domain.StatusInProgress)
	if err := s.UpsertEvent(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	updated := sampleEvent("run-2", first.StartedAt, domain.StatusSuccess)
	if err := s.UpsertEvent(ctx, updated); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := s.GetEvent(ctx, domain.SourceJenkins, "run-2")
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.Status != domain.StatusSuccess {
		t.Errorf("status after second upsert = %q, want success", got.Status)
	}
	if got.FinishedAt == nil {
		t.Error("finished_at not populated after status transition to success")
	}
}

func TestGetEventNotFoundReturnsSentinel(t *testing.T) {
	pool := testPool(t)
	s := postgres.New(pool)
	ctx := context.Background()

	_, err := s.GetEvent(ctx, domain.SourceJenkins, "does-not-exist")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestListRecentEventsOrdersByFinishedAtDesc(t *testing.T) {
	pool := testPool(t)
	s := postgres.New(pool)
	ctx := context.Background()

	base := time.Now().UTC().Add(-1 * time.Hour)
	for i, off := range []time.Duration{0, 10 * time.Minute, 20 * time.Minute} {
		e := sampleEvent(fmt.Sprintf("run-list-%d", i), base.Add(off), domain.StatusSuccess)
		if err := s.UpsertEvent(ctx, e); err != nil {
			t.Fatalf("upsert %d: %v", i, err)
		}
	}

	events, err := s.ListRecentEvents(ctx, storage.EventFilter{}, 10)
	if err != nil {
		t.Fatalf("ListRecentEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].FinishedAt.After(*events[i-1].FinishedAt) {
			t.Errorf("results not ordered desc by finished_at at index %d", i)
		}
	}
}

func TestListRecentEventsRespectsFilter(t *testing.T) {
	pool := testPool(t)
	s := postgres.New(pool)
	ctx := context.Background()

	base := time.Now().UTC().Add(-1 * time.Hour)

	prod := sampleEvent("run-prod", base, domain.StatusSuccess)
	prod.Environment = "prod"
	stage := sampleEvent("run-stage", base.Add(5*time.Minute), domain.StatusSuccess)
	stage.Environment = "stage"

	for _, e := range []*domain.DeploymentEvent{prod, stage} {
		if err := s.UpsertEvent(ctx, e); err != nil {
			t.Fatalf("upsert %s: %v", e.PipelineID, err)
		}
	}

	events, err := s.ListRecentEvents(ctx, storage.EventFilter{Environment: "prod"}, 10)
	if err != nil {
		t.Fatalf("ListRecentEvents: %v", err)
	}
	if len(events) != 1 || events[0].PipelineID != "run-prod" {
		t.Errorf("filter did not narrow correctly: %+v", events)
	}
}
