package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anupam2105/shipmetrics/internal/api"
	"github.com/anupam2105/shipmetrics/internal/domain"
	"github.com/anupam2105/shipmetrics/internal/storage"
)

type listStore struct {
	storage.Store // unused methods panic via nil embedding
	events        []*domain.DeploymentEvent
	gotFilter     storage.EventFilter
	gotLimit      int
}

func (s *listStore) ListRecentEvents(_ context.Context, f storage.EventFilter, limit int) ([]*domain.DeploymentEvent, error) {
	s.gotFilter, s.gotLimit = f, limit
	return s.events, nil
}

func ptr(t time.Time) *time.Time { return &t }

func newStore() *listStore {
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	return &listStore{events: []*domain.DeploymentEvent{
		{Source: domain.SourceJenkins, PipelineID: "3", ServiceName: "checkout-api", Environment: "prod",
			Status: domain.StatusSuccess, StartedAt: now.Add(-11 * time.Minute), FinishedAt: ptr(now.Add(-10 * time.Minute)), CommitSHA: "abc123"},
		{Source: domain.SourceJenkins, PipelineID: "2", ServiceName: "checkout-api", Environment: "prod",
			Status: domain.StatusSuccess, StartedAt: now.Add(-3 * time.Hour), FinishedAt: ptr(now.Add(-3 * time.Hour))},
	}}
}

func do(t *testing.T, h *api.Handler, target, auth string) (int, api.DeploymentList) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ListDeployments(rec, req)
	var out api.DeploymentList
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec.Code, out
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestListDeploymentsPassesFiltersAndAppliesSince(t *testing.T) {
	t.Parallel()
	store := newStore()
	h := api.NewHandler(store, discard(), "")

	code, out := do(t, h, "/api/v1/deployments?service=checkout-api&environment=prod&limit=50&since=2026-11-26T09:00:00Z", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if store.gotFilter.ServiceName != "checkout-api" || store.gotFilter.Environment != "prod" || store.gotLimit != 50 {
		t.Errorf("filter/limit not forwarded: %+v limit=%d", store.gotFilter, store.gotLimit)
	}
	if len(out.Deployments) != 1 || out.Deployments[0].PipelineID != "3" || out.Deployments[0].CommitSHA != "abc123" {
		t.Errorf("since filter wrong: %+v", out.Deployments)
	}
}

func TestListDeploymentsRejectsBadParams(t *testing.T) {
	t.Parallel()
	h := api.NewHandler(newStore(), discard(), "")
	for _, q := range []string{"?limit=0", "?limit=201", "?limit=x", "?since=yesterday"} {
		if code, _ := do(t, h, "/api/v1/deployments"+q, ""); code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, code)
		}
	}
}

func TestListDeploymentsEnforcesBearerToken(t *testing.T) {
	t.Parallel()
	h := api.NewHandler(newStore(), discard(), "s3cret")
	cases := map[string]int{
		"":              http.StatusUnauthorized,
		"Bearer wrong":  http.StatusUnauthorized,
		"Basic s3cret":  http.StatusUnauthorized,
		"Bearer s3cret": http.StatusOK,
	}
	for auth, want := range cases {
		if code, _ := do(t, h, "/api/v1/deployments", auth); code != want {
			t.Errorf("auth %q: status = %d, want %d", auth, code, want)
		}
	}
}
