package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anupam2105/shipmetrics/internal/httpserver"
	"github.com/anupam2105/shipmetrics/internal/observability"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := observability.NewMetrics()
	s := httpserver.New(":0", logger, metrics, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// doGet issues a GET and returns (status, body). The response body is fully
// read and closed inside this helper so callers cannot leak connections and
// linters see close discipline at the point of the HTTP call.
func doGet(t *testing.T, url string) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, body
}

func TestHealthzReturns200(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	status, _ := doGet(t, ts.URL+"/healthz")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
}

func TestReadyzReturns200(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	status, _ := doGet(t, ts.URL+"/readyz")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
}

func TestUnknownRouteIs404(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	status, _ := doGet(t, ts.URL+"/does-not-exist")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestMetricsEndpointExposesExpectedSeries(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	// Prime one request so the http_requests counter has data.
	_, _ = doGet(t, ts.URL+"/healthz")

	status, body := doGet(t, ts.URL+"/metrics")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	want := []string{
		"go_goroutines",
		"shipmetrics_http_requests_total",
		"shipmetrics_http_request_duration_seconds",
	}
	for _, series := range want {
		if !strings.Contains(string(body), series) {
			t.Errorf("metrics body missing %q", series)
		}
	}
}
