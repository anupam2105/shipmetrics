// Package webhook translates CI-provider webhook payloads into
// domain.DeploymentEvent and persists them via storage.Store. Handlers are
// wired individually so a broken source cannot poison unrelated intake paths.
package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/anupam2105/shipmetrics/internal/domain"
	"github.com/anupam2105/shipmetrics/internal/observability"
	"github.com/anupam2105/shipmetrics/internal/storage"
)

// maxPayloadBytes bounds a single webhook body. Real Jenkins/GitHub payloads
// are well under this; the cap prevents accidental DoS from misconfigured
// senders shipping megabytes of logs into the JSON body.
const maxPayloadBytes = 1 << 20 // 1 MiB

// Handler groups the webhook receivers and their dependencies.
type Handler struct {
	store   storage.Store
	logger  *slog.Logger
	metrics *observability.Metrics
}

// NewHandler constructs a Handler backed by the given store.
func NewHandler(store storage.Store, logger *slog.Logger, metrics *observability.Metrics) *Handler {
	return &Handler{store: store, logger: logger, metrics: metrics}
}

// decodeJSON reads at most maxPayloadBytes and strictly decodes into v.
// Unknown fields are permitted so providers can extend their payloads
// without breaking us.
func decodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxPayloadBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

// writeJSON is a tiny helper for consistent JSON responses.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

// errorBody is the canonical error envelope returned by handlers.
type errorBody struct {
	Error string `json:"error"`
}

// writeError is a small helper so every 4xx/5xx path returns the same shape.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorBody{Error: err.Error()})
}

// classifyValidationError converts a domain validation error into a caller-facing
// message. We intentionally do not leak internal errors verbatim in future
// updates; today domain.Validate produces caller-safe messages so we return them.
func classifyValidationError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New("invalid deployment event: " + err.Error())
}

// process runs the shared post-parse flow: validate, store, record metrics.
// Every provider handler funnels through this so behavior stays consistent.
func (h *Handler) process(r *http.Request, source domain.Source, event *domain.DeploymentEvent) (int, error) {
	if event == nil {
		h.metrics.WebhookErrors.WithLabelValues(string(source), "empty_event").Inc()
		return http.StatusBadRequest, errors.New("empty deployment event")
	}
	event.Source = source

	if err := event.Validate(); err != nil {
		h.metrics.WebhookErrors.WithLabelValues(string(source), "validation").Inc()
		return http.StatusBadRequest, classifyValidationError(err)
	}

	if err := h.store.UpsertEvent(r.Context(), event); err != nil {
		h.metrics.WebhookErrors.WithLabelValues(string(source), "storage").Inc()
		h.logger.Error("webhook storage error",
			"source", source,
			"pipeline_id", event.PipelineID,
			"error", err,
		)
		return http.StatusInternalServerError, errors.New("internal error")
	}

	h.metrics.WebhookEvents.WithLabelValues(string(source), string(event.Status)).Inc()
	h.logger.Info("webhook accepted",
		"source", source,
		"pipeline_id", event.PipelineID,
		"service", event.ServiceName,
		"environment", event.Environment,
		"status", event.Status,
	)
	return http.StatusAccepted, nil
}
