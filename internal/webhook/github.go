package webhook

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
)

// githubHeadCommit is a nested struct in the workflow_run payload; extracted
// so gofmt's alignment rules stay clean.
type githubHeadCommit struct {
	Timestamp *time.Time `json:"timestamp"`
}

// githubWorkflowRun mirrors the fields we consume from workflow_run.
type githubWorkflowRun struct {
	ID         int64             `json:"id"`
	Name       string            `json:"name"`
	Status     string            `json:"status"`
	Conclusion *string           `json:"conclusion"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	HeadSHA    string            `json:"head_sha"`
	HeadCommit *githubHeadCommit `json:"head_commit"`
}

// githubRepository holds the minimal repo identity we need for service mapping.
type githubRepository struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
}

// githubPayload is the subset of GitHub's workflow_run webhook payload we care
// about. See: https://docs.github.com/webhooks/webhook-events-and-payloads#workflow_run
type githubPayload struct {
	Action      string            `json:"action"`
	WorkflowRun githubWorkflowRun `json:"workflow_run"`
	Repository  githubRepository  `json:"repository"`
}

// mapGitHubStatus converts GitHub Actions' status + conclusion pair to our
// canonical Status. Unknown conclusions collapse to Cancelled so DORA maths
// never counts an ambiguous run as success.
func mapGitHubStatus(status string, conclusion *string) domain.Status {
	switch status {
	case "queued", "in_progress", "waiting":
		return domain.StatusInProgress
	case "completed":
		if conclusion == nil {
			return domain.StatusCancelled
		}
		switch *conclusion {
		case "success":
			return domain.StatusSuccess
		case "failure", "timed_out", "startup_failure":
			return domain.StatusFailure
		default: // cancelled, skipped, action_required, neutral, stale, ...
			return domain.StatusCancelled
		}
	}
	return domain.StatusInProgress
}

// GitHub is the http.HandlerFunc for POST /webhooks/github. Service name and
// environment are not carried in the native GitHub payload, so we accept them
// as query parameters (`?service=X&environment=Y`), falling back to the repo
// name for service and "unknown" for environment.
func (h *Handler) GitHub(w http.ResponseWriter, r *http.Request) {
	var payload githubPayload
	if err := decodeJSON(r, &payload); err != nil {
		h.metrics.WebhookErrors.WithLabelValues(string(domain.SourceGitHubActions), "decode").Inc()
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if payload.WorkflowRun.ID == 0 {
		h.metrics.WebhookErrors.WithLabelValues(string(domain.SourceGitHubActions), "missing_workflow_run").Inc()
		writeError(w, http.StatusBadRequest, errors.New("workflow_run.id is required"))
		return
	}

	service := r.URL.Query().Get("service")
	if service == "" {
		service = payload.Repository.Name
	}
	environment := r.URL.Query().Get("environment")
	if environment == "" {
		environment = "unknown"
	}

	status := mapGitHubStatus(payload.WorkflowRun.Status, payload.WorkflowRun.Conclusion)

	event := &domain.DeploymentEvent{
		PipelineName: payload.WorkflowRun.Name,
		PipelineID:   strconv.FormatInt(payload.WorkflowRun.ID, 10),
		ServiceName:  service,
		Environment:  environment,
		Status:       status,
		StartedAt:    payload.WorkflowRun.CreatedAt,
		CommitSHA:    payload.WorkflowRun.HeadSHA,
		Metadata: map[string]string{
			"repository": payload.Repository.FullName,
			"action":     payload.Action,
		},
	}

	if status.Terminal() {
		finished := payload.WorkflowRun.UpdatedAt
		event.FinishedAt = &finished
	}

	if payload.WorkflowRun.HeadCommit != nil && payload.WorkflowRun.HeadCommit.Timestamp != nil {
		event.CommitTimestamp = payload.WorkflowRun.HeadCommit.Timestamp
	}

	statusCode, err := h.process(r, domain.SourceGitHubActions, event)
	if err != nil {
		writeError(w, statusCode, err)
		return
	}
	writeJSON(w, statusCode, map[string]string{"result": "accepted"})
}
