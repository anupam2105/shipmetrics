// Package httpserver builds and manages the HTTP endpoint that receives
// deployment events and exposes health / metrics endpoints.
package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/anupam2105/shipmetrics/internal/observability"
)

// Server owns the http.Server lifecycle.
type Server struct {
	srv     *http.Server
	logger  *slog.Logger
	metrics *observability.Metrics
}

// New constructs a Server bound to addr with the given logger and metrics.
func New(addr string, logger *slog.Logger, metrics *observability.Metrics) *Server {
	s := &Server{
		logger:  logger,
		metrics: metrics,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.Handle("GET /metrics", metrics.Handler())

	s.srv = &http.Server{
		Addr:              addr,
		Handler:           s.instrument(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s
}

// Start begins serving. It blocks until the server stops.
func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server, draining in-flight requests until ctx is done.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// Handler exposes the fully instrumented http.Handler for testing.
func (s *Server) Handler() http.Handler {
	return s.srv.Handler
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, `{"status":"ok"}`)
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, `{"status":"ready"}`)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// instrument wraps the mux with metrics and structured logging.
func (s *Server) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		route := routeLabel(r.URL.Path)
		s.metrics.HTTPRequestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
		s.metrics.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(duration.Seconds())

		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", duration.Milliseconds(),
		)
	})
}

// routeLabel collapses request paths to a bounded label set so external
// scanners cannot inflate metric cardinality with arbitrary URLs.
func routeLabel(path string) string {
	switch path {
	case "/healthz", "/readyz", "/metrics":
		return path
	default:
		return "unknown"
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}
