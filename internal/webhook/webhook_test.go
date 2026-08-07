package webhook_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
	"github.com/anupam2105/shipmetrics/internal/observability"
	"github.com/anupam2105/shipmetrics/internal/storage"
	"github.com/anupam2105/shipmetrics/internal/webhook"
)

// fakeStore is a tiny in-memory storage.Store for handler unit tests.
// It is deliberately minimal — the real Postgres store has its own
// integration tests and we don't want to re-test SQL semantics here.
type fakeStore struct {
	mu         sync.Mutex
	events     map[string]*domain.DeploymentEvent // key: source|pipeline_id
	upsertErr  error
	upsertCall int
}

func newFakeStore() *fakeStore {
	return &fakeStore{events: map[string]*domain.DeploymentEvent{}}
}

func fakeKey(s domain.Source, pid string) string { return string(s) + "|" + pid }

func (f *fakeStore) UpsertEvent(_ context.Context, e *domain.DeploymentEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upsertCall++
	if f.upsertErr != nil {
		return f.upsertErr
	}
	// Deep-copy the event so later mutations by tests don't affect stored state.
	stored := *e
	f.events[fakeKey(e.Source, e.PipelineID)] = &stored
	return nil
}

func (f *fakeStore) GetEvent(_ context.Context, s domain.Source, pid string) (*domain.DeploymentEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.events[fakeKey(s, pid)]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return e, nil
}

func (f *fakeStore) ListRecentEvents(_ context.Context, _ storage.EventFilter, _ int) ([]*domain.DeploymentEvent, error) {
	return nil, nil
}

func (f *fakeStore) ListDORATargets(_ context.Context, _ time.Time) ([]storage.DORATarget, error) {
	return nil, nil
}

func (f *fakeStore) DORAWindow(_ context.Context, t storage.DORATarget, _ time.Time) (storage.DORAWindowStats, error) {
	return storage.DORAWindowStats{Target: t}, nil
}

func (f *fakeStore) Ping(_ context.Context) error { return nil }

func newTestHandler(t *testing.T) (*webhook.Handler, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := observability.NewMetrics()
	return webhook.NewHandler(store, logger, metrics), store
}

// postJSON issues a JSON POST to the given handler function and returns the
// response. The response body is fully read and closed so callers can't leak.
func postJSON(t *testing.T, handler http.HandlerFunc, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)
	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(respBody)
}

// --- Jenkins handler ---

func TestJenkinsAcceptsValidPayload(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(t)

	body := `{
		"pipeline_name": "checkout-deploy",
		"pipeline_id": "42",
		"service_name": "checkout-api",
		"environment": "prod",
		"status": "success",
		"started_at": "2026-08-05T10:00:00Z",
		"finished_at": "2026-08-05T10:03:00Z",
		"commit_sha": "abc123"
	}`
	status, _ := postJSON(t, h.Jenkins, "/webhooks/jenkins", body)
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	got, err := store.GetEvent(context.Background(), domain.SourceJenkins, "42")
	if err != nil {
		t.Fatalf("event not persisted: %v", err)
	}
	if got.ServiceName != "checkout-api" || got.Status != domain.StatusSuccess {
		t.Errorf("event fields wrong: %+v", got)
	}
}

func TestJenkinsRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler(t)
	status, body := postJSON(t, h.Jenkins, "/webhooks/jenkins", `{ not-json`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if !strings.Contains(body, "error") {
		t.Errorf("expected error envelope, got %s", body)
	}
}

func TestJenkinsRejectsValidationError(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler(t)
	// Missing service_name.
	body := `{
		"pipeline_id": "42",
		"environment": "prod",
		"status": "success",
		"started_at": "2026-08-05T10:00:00Z",
		"finished_at": "2026-08-05T10:03:00Z"
	}`
	status, respBody := postJSON(t, h.Jenkins, "/webhooks/jenkins", body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if !strings.Contains(respBody, "service_name") {
		t.Errorf("expected mention of service_name in error, got %s", respBody)
	}
}

func TestJenkinsSurfacesStorageErrorAs500(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(t)
	store.upsertErr = errors.New("db offline")

	body := `{
		"pipeline_id": "42",
		"service_name": "svc",
		"environment": "prod",
		"status": "success",
		"started_at": "2026-08-05T10:00:00Z",
		"finished_at": "2026-08-05T10:03:00Z"
	}`
	status, _ := postJSON(t, h.Jenkins, "/webhooks/jenkins", body)
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
}

// --- GitHub handler ---

func TestGitHubAcceptsCompletedWorkflowRun(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(t)

	body := `{
		"action": "completed",
		"workflow_run": {
			"id": 12345,
			"name": "Deploy",
			"status": "completed",
			"conclusion": "success",
			"created_at": "2026-08-05T10:00:00Z",
			"updated_at": "2026-08-05T10:05:00Z",
			"head_sha": "sha-abc",
			"head_commit": {"timestamp": "2026-08-05T09:55:00Z"}
		},
		"repository": {"name": "web", "full_name": "acme/web"}
	}`

	status, _ := postJSON(t, h.GitHub, "/webhooks/github?service=web&environment=prod", body)
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	got, err := store.GetEvent(context.Background(), domain.SourceGitHubActions, "12345")
	if err != nil {
		t.Fatalf("event not persisted: %v", err)
	}
	if got.Status != domain.StatusSuccess {
		t.Errorf("status = %q, want success", got.Status)
	}
	if got.ServiceName != "web" || got.Environment != "prod" {
		t.Errorf("service/env mapped wrong: service=%q env=%q", got.ServiceName, got.Environment)
	}
	if got.CommitSHA != "sha-abc" || got.CommitTimestamp == nil {
		t.Errorf("commit metadata dropped: %+v", got)
	}
}

func TestGitHubMapsFailureConclusion(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(t)

	body := `{
		"action": "completed",
		"workflow_run": {
			"id": 12346,
			"name": "Deploy",
			"status": "completed",
			"conclusion": "failure",
			"created_at": "2026-08-05T10:00:00Z",
			"updated_at": "2026-08-05T10:02:00Z",
			"head_sha": "sha-def"
		},
		"repository": {"name": "web", "full_name": "acme/web"}
	}`

	status, _ := postJSON(t, h.GitHub, "/webhooks/github?service=web&environment=prod", body)
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	got, _ := store.GetEvent(context.Background(), domain.SourceGitHubActions, "12346")
	if got.Status != domain.StatusFailure {
		t.Errorf("status = %q, want failure", got.Status)
	}
}

func TestGitHubMapsInProgressStatus(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(t)

	body := `{
		"action": "in_progress",
		"workflow_run": {
			"id": 12347,
			"name": "Deploy",
			"status": "in_progress",
			"created_at": "2026-08-05T10:00:00Z",
			"updated_at": "2026-08-05T10:00:30Z",
			"head_sha": "sha-ghi"
		},
		"repository": {"name": "web", "full_name": "acme/web"}
	}`

	status, _ := postJSON(t, h.GitHub, "/webhooks/github?service=web&environment=prod", body)
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	got, _ := store.GetEvent(context.Background(), domain.SourceGitHubActions, "12347")
	if got.Status != domain.StatusInProgress {
		t.Errorf("status = %q, want in_progress", got.Status)
	}
	if got.FinishedAt != nil {
		t.Errorf("finished_at should be nil for in_progress, got %v", got.FinishedAt)
	}
}

func TestGitHubRejectsMissingWorkflowRun(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler(t)
	body := `{"action":"completed","workflow_run":{},"repository":{"name":"x","full_name":"a/x"}}`
	status, respBody := postJSON(t, h.GitHub, "/webhooks/github?service=x&environment=prod", body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if !strings.Contains(respBody, "workflow_run.id") {
		t.Errorf("expected workflow_run.id error, got %s", respBody)
	}
}

func TestGitHubFallsBackToRepoNameForService(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(t)

	body := `{
		"action": "completed",
		"workflow_run": {
			"id": 99,
			"name": "Deploy",
			"status": "completed",
			"conclusion": "success",
			"created_at": "2026-08-05T10:00:00Z",
			"updated_at": "2026-08-05T10:02:00Z",
			"head_sha": "sha-xxx"
		},
		"repository": {"name": "fallback-repo", "full_name": "acme/fallback-repo"}
	}`

	status, _ := postJSON(t, h.GitHub, "/webhooks/github?environment=prod", body)
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	got, _ := store.GetEvent(context.Background(), domain.SourceGitHubActions, "99")
	if got.ServiceName != "fallback-repo" {
		t.Errorf("service fallback failed: got %q", got.ServiceName)
	}
}

func TestGitHubIsIdempotent(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler(t)

	body := `{
		"action": "completed",
		"workflow_run": {
			"id": 77,
			"name": "Deploy",
			"status": "completed",
			"conclusion": "success",
			"created_at": "2026-08-05T10:00:00Z",
			"updated_at": "2026-08-05T10:02:00Z",
			"head_sha": "sha-77"
		},
		"repository": {"name": "web", "full_name": "acme/web"}
	}`

	for i := 0; i < 3; i++ {
		status, _ := postJSON(t, h.GitHub, "/webhooks/github?service=web&environment=prod", body)
		if status != http.StatusAccepted {
			t.Fatalf("attempt %d: status = %d, want 202", i, status)
		}
	}
	if store.upsertCall != 3 {
		t.Errorf("upsertCall = %d, want 3 (idempotent handler must still call store each time)", store.upsertCall)
	}
	// Only one distinct event should exist in the store.
	if len(store.events) != 1 {
		t.Errorf("events map size = %d, want 1", len(store.events))
	}
}
