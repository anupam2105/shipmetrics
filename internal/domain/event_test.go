package domain_test

import (
	"testing"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
)

func timePtr(t time.Time) *time.Time { return &t }

func validEvent() domain.DeploymentEvent {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	finish := start.Add(5 * time.Minute)
	return domain.DeploymentEvent{
		Source:       domain.SourceJenkins,
		PipelineName: "checkout-api-deploy",
		PipelineID:   "jenkins-42",
		ServiceName:  "checkout-api",
		Environment:  "prod",
		Status:       domain.StatusSuccess,
		StartedAt:    start,
		FinishedAt:   &finish,
		CommitSHA:    "abc123",
	}
}

func TestValidateAcceptsValidEvent(t *testing.T) {
	t.Parallel()
	e := validEvent()
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: unexpected error: %v", err)
	}
}

func TestValidateRejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		mutate   func(*domain.DeploymentEvent)
		wantSubs []string
	}{
		{
			name:     "invalid source",
			mutate:   func(e *domain.DeploymentEvent) { e.Source = "bamboo" },
			wantSubs: []string{"invalid source"},
		},
		{
			name:     "empty pipeline id",
			mutate:   func(e *domain.DeploymentEvent) { e.PipelineID = "" },
			wantSubs: []string{"pipeline_id"},
		},
		{
			name:     "empty service",
			mutate:   func(e *domain.DeploymentEvent) { e.ServiceName = "" },
			wantSubs: []string{"service_name"},
		},
		{
			name:     "empty environment",
			mutate:   func(e *domain.DeploymentEvent) { e.Environment = "" },
			wantSubs: []string{"environment"},
		},
		{
			name:     "invalid status",
			mutate:   func(e *domain.DeploymentEvent) { e.Status = "unknown" },
			wantSubs: []string{"invalid status"},
		},
		{
			name:     "zero started_at",
			mutate:   func(e *domain.DeploymentEvent) { e.StartedAt = time.Time{} },
			wantSubs: []string{"started_at"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := validEvent()
			tc.mutate(&e)
			err := e.Validate()
			if err == nil {
				t.Fatalf("Validate: expected error, got nil")
			}
			msg := err.Error()
			for _, sub := range tc.wantSubs {
				if !contains(msg, sub) {
					t.Errorf("error message %q missing substring %q", msg, sub)
				}
			}
		})
	}
}

func TestValidateTerminalStatusRequiresFinishedAt(t *testing.T) {
	t.Parallel()
	e := validEvent()
	e.FinishedAt = nil
	err := e.Validate()
	if err == nil || !contains(err.Error(), "finished_at is required") {
		t.Fatalf("Validate: expected finished_at error, got %v", err)
	}
}

func TestValidateRejectsFinishedBeforeStarted(t *testing.T) {
	t.Parallel()
	e := validEvent()
	e.FinishedAt = timePtr(e.StartedAt.Add(-1 * time.Minute))
	err := e.Validate()
	if err == nil || !contains(err.Error(), "finished_at cannot be before") {
		t.Fatalf("Validate: expected ordering error, got %v", err)
	}
}

func TestValidateInProgressAllowsNilFinishedAt(t *testing.T) {
	t.Parallel()
	e := validEvent()
	e.Status = domain.StatusInProgress
	e.FinishedAt = nil
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: in-progress without finished_at should be valid, got %v", err)
	}
}

func TestDurationReturnsZeroWhenInProgress(t *testing.T) {
	t.Parallel()
	e := validEvent()
	e.FinishedAt = nil
	if got := e.Duration(); got != 0 {
		t.Errorf("Duration = %v, want 0", got)
	}
}

func TestDurationReturnsElapsedWhenFinished(t *testing.T) {
	t.Parallel()
	e := validEvent()
	want := 5 * time.Minute
	if got := e.Duration(); got != want {
		t.Errorf("Duration = %v, want %v", got, want)
	}
}

func TestStatusTerminal(t *testing.T) {
	t.Parallel()
	cases := map[domain.Status]bool{
		domain.StatusInProgress: false,
		domain.StatusSuccess:    true,
		domain.StatusFailure:    true,
		domain.StatusCancelled:  true,
	}
	for s, want := range cases {
		if got := s.Terminal(); got != want {
			t.Errorf("Status(%q).Terminal() = %v, want %v", s, got, want)
		}
	}
}

// contains is a tiny substring helper used to keep test assertions readable
// without pulling strings into the test file's imports.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
