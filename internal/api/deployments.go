// Package api exposes read-only JSON endpoints over stored deployment events.
// Its first consumer is pageguard, which asks "what deployed recently to this
// service?" while investigating an alert.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
	"github.com/anupam2105/shipmetrics/internal/storage"
)

const (
	defaultLimit = 20
	maxLimit     = 200
)

// Handler serves the read API.
type Handler struct {
	store  storage.Store
	logger *slog.Logger
	token  string
}

// NewHandler constructs a Handler. When token is non-empty every request must
// carry "Authorization: Bearer <token>"; an empty token leaves the API open,
// which is only appropriate on a trusted network.
func NewHandler(store storage.Store, logger *slog.Logger, token string) *Handler {
	return &Handler{store: store, logger: logger, token: token}
}

// Deployment is the wire shape of one deployment event.
type Deployment struct {
	Source          string            `json:"source"`
	PipelineName    string            `json:"pipeline_name,omitempty"`
	PipelineID      string            `json:"pipeline_id"`
	Service         string            `json:"service"`
	Environment     string            `json:"environment"`
	Status          string            `json:"status"`
	StartedAt       time.Time         `json:"started_at"`
	FinishedAt      *time.Time        `json:"finished_at,omitempty"`
	CommitSHA       string            `json:"commit_sha,omitempty"`
	CommitTimestamp *time.Time        `json:"commit_timestamp,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// DeploymentList is the response envelope for GET /api/v1/deployments.
type DeploymentList struct {
	Deployments []Deployment `json:"deployments"`
}

// ListDeployments handles GET /api/v1/deployments.
//
// Query parameters:
//   - service, environment: optional exact-match filters
//   - since: optional RFC 3339 timestamp; only deploys finished at/after it
//   - limit: optional, 1..200 (default 20)
func (h *Handler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="shipmetrics"`)
		writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
		return
	}

	q := r.URL.Query()
	limit := defaultLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			writeError(w, http.StatusBadRequest, "limit must be an integer between 1 and 200")
			return
		}
		limit = n
	}

	var since time.Time
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be an RFC 3339 timestamp")
			return
		}
		since = t
	}

	events, err := h.store.ListRecentEvents(r.Context(), storage.EventFilter{
		ServiceName: q.Get("service"),
		Environment: q.Get("environment"),
	}, limit)
	if err != nil {
		h.logger.Error("list deployments", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := DeploymentList{Deployments: make([]Deployment, 0, len(events))}
	for _, e := range events {
		// Events are ordered newest-finished first, so everything after the
		// first too-old event is also too old.
		if !since.IsZero() && e.FinishedAt != nil && e.FinishedAt.Before(since) {
			break
		}
		out.Deployments = append(out.Deployments, toWire(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) authorized(r *http.Request) bool {
	if h.token == "" {
		return true
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.token)) == 1
}

func toWire(e *domain.DeploymentEvent) Deployment {
	return Deployment{
		Source:          string(e.Source),
		PipelineName:    e.PipelineName,
		PipelineID:      e.PipelineID,
		Service:         e.ServiceName,
		Environment:     e.Environment,
		Status:          string(e.Status),
		StartedAt:       e.StartedAt,
		FinishedAt:      e.FinishedAt,
		CommitSHA:       e.CommitSHA,
		CommitTimestamp: e.CommitTimestamp,
		Metadata:        e.Metadata,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
