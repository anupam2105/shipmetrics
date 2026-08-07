// Package storage defines the Store abstraction that hides persistence
// implementation from the rest of the codebase.
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
)

// Sentinel errors returned by all Store implementations.
var (
	ErrNotFound = errors.New("event not found")
)

// EventFilter narrows list queries by service and environment. Zero values
// mean "no filter on this dimension".
type EventFilter struct {
	ServiceName string
	Environment string
}

// DORATarget identifies a (service, environment) pair the analyzer will
// compute metrics for.
type DORATarget struct {
	ServiceName string
	Environment string
}

// DORAWindowStats bundles the four DORA metrics for one (service, environment)
// over a fixed lookback window. All duration fields are seconds. Zero values
// indicate the metric is not computable (e.g. no failures → no MTTR).
type DORAWindowStats struct {
	Target         DORATarget
	SuccessCount   int
	FailureCount   int
	LeadTimeP50Sec float64
	LeadTimeP95Sec float64
	MTTRAverageSec float64
}

// ChangeFailureRate is failures / (successes + failures). Returns 0 when
// the denominator is zero to keep gauges stable during warm-up.
func (s DORAWindowStats) ChangeFailureRate() float64 {
	total := s.SuccessCount + s.FailureCount
	if total == 0 {
		return 0
	}
	return float64(s.FailureCount) / float64(total)
}

// Store persists DeploymentEvents and answers read queries used by the
// DORA metric evaluators and API surface.
type Store interface {
	// UpsertEvent inserts or updates a deployment event, keyed by
	// (Source, PipelineID). Idempotent — safe to call for repeated webhooks.
	UpsertEvent(ctx context.Context, e *domain.DeploymentEvent) error

	// GetEvent returns the event uniquely identified by (Source, PipelineID).
	// Returns ErrNotFound if no such event exists.
	GetEvent(ctx context.Context, source domain.Source, pipelineID string) (*domain.DeploymentEvent, error)

	// ListRecentEvents returns up to limit terminal events, most recently finished first.
	ListRecentEvents(ctx context.Context, filter EventFilter, limit int) ([]*domain.DeploymentEvent, error)

	// ListDORATargets returns distinct (service, environment) pairs with at
	// least one terminal event in the window ending at now.
	ListDORATargets(ctx context.Context, since time.Time) ([]DORATarget, error)

	// DORAWindow computes the four DORA metrics for one target over events
	// finished at or after since.
	DORAWindow(ctx context.Context, t DORATarget, since time.Time) (DORAWindowStats, error)

	// Ping verifies the store is reachable.
	Ping(ctx context.Context) error
}
