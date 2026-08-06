// Package storage defines the Store abstraction that hides persistence
// implementation from the rest of the codebase.
package storage

import (
	"context"
	"errors"

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

	// Ping verifies the store is reachable.
	Ping(ctx context.Context) error
}
