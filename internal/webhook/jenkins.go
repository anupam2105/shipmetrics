package webhook

import (
	"net/http"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
)

// jenkinsPayload is the canonical shipmetrics JSON body that a Jenkins
// pipeline posts at the end of a deployment stage. It is not a Jenkins-native
// format — we prefer a fixed contract that any CI can produce with `curl`
// so users do not depend on a Jenkins plugin.
type jenkinsPayload struct {
	PipelineName    string            `json:"pipeline_name"`
	PipelineID      string            `json:"pipeline_id"`
	ServiceName     string            `json:"service_name"`
	Environment     string            `json:"environment"`
	Status          string            `json:"status"`
	StartedAt       time.Time         `json:"started_at"`
	FinishedAt      *time.Time        `json:"finished_at,omitempty"`
	CommitSHA       string            `json:"commit_sha,omitempty"`
	CommitTimestamp *time.Time        `json:"commit_timestamp,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// toDomain converts the wire payload into a DeploymentEvent. Source is set by
// the caller (process) so a single validation path handles both providers.
func (p *jenkinsPayload) toDomain() *domain.DeploymentEvent {
	return &domain.DeploymentEvent{
		PipelineName:    p.PipelineName,
		PipelineID:      p.PipelineID,
		ServiceName:     p.ServiceName,
		Environment:     p.Environment,
		Status:          domain.Status(p.Status),
		StartedAt:       p.StartedAt,
		FinishedAt:      p.FinishedAt,
		CommitSHA:       p.CommitSHA,
		CommitTimestamp: p.CommitTimestamp,
		Metadata:        p.Metadata,
	}
}

// Jenkins is the http.HandlerFunc for POST /webhooks/jenkins.
func (h *Handler) Jenkins(w http.ResponseWriter, r *http.Request) {
	var payload jenkinsPayload
	if err := decodeJSON(r, &payload); err != nil {
		h.metrics.WebhookErrors.WithLabelValues(string(domain.SourceJenkins), "decode").Inc()
		writeError(w, http.StatusBadRequest, err)
		return
	}

	status, err := h.process(r, domain.SourceJenkins, payload.toDomain())
	if err != nil {
		writeError(w, status, err)
		return
	}
	writeJSON(w, status, map[string]string{"result": "accepted"})
}
