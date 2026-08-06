// Package domain contains the core types shared across shipmetrics packages.
package domain

import (
	"errors"
	"fmt"
	"time"
)

// Source identifies the CI system that produced a deployment event.
type Source string

// Recognised CI sources. New sources must be added here and to the DB check
// constraint in migrations/000001_init.up.sql.
const (
	SourceJenkins       Source = "jenkins"
	SourceGitHubActions Source = "github-actions"
	SourceGitLabCI      Source = "gitlab-ci"
)

// Valid reports whether s is a known Source.
func (s Source) Valid() bool {
	switch s {
	case SourceJenkins, SourceGitHubActions, SourceGitLabCI:
		return true
	}
	return false
}

// Status describes the current state of a deployment.
type Status string

// Recognised statuses. Terminal statuses require FinishedAt to be set.
const (
	StatusInProgress Status = "in_progress"
	StatusSuccess    Status = "success"
	StatusFailure    Status = "failure"
	StatusCancelled  Status = "cancelled"
)

// Valid reports whether s is a known Status.
func (s Status) Valid() bool {
	switch s {
	case StatusInProgress, StatusSuccess, StatusFailure, StatusCancelled:
		return true
	}
	return false
}

// Terminal reports whether the status represents a completed deployment.
func (s Status) Terminal() bool {
	return s == StatusSuccess || s == StatusFailure || s == StatusCancelled
}

// DeploymentEvent is the canonical record of one deployment action captured
// from a CI system. It is the raw material for all downstream DORA and SLO
// calculations, so field semantics are strict and enforced by Validate.
type DeploymentEvent struct {
	Source          Source
	PipelineName    string
	PipelineID      string // provider-scoped unique id — used for dedup on upsert
	ServiceName     string
	Environment     string
	Status          Status
	StartedAt       time.Time
	FinishedAt      *time.Time // nil while in-progress
	CommitSHA       string
	CommitTimestamp *time.Time // when the commit was authored — used for lead-time
	Metadata        map[string]string
}

// Validate enforces the invariants required for storage and DORA maths.
// Multiple violations are aggregated via errors.Join.
func (e *DeploymentEvent) Validate() error {
	var errs []error
	if !e.Source.Valid() {
		errs = append(errs, fmt.Errorf("invalid source %q", e.Source))
	}
	if e.PipelineID == "" {
		errs = append(errs, errors.New("pipeline_id is required"))
	}
	if e.ServiceName == "" {
		errs = append(errs, errors.New("service_name is required"))
	}
	if e.Environment == "" {
		errs = append(errs, errors.New("environment is required"))
	}
	if !e.Status.Valid() {
		errs = append(errs, fmt.Errorf("invalid status %q", e.Status))
	}
	if e.StartedAt.IsZero() {
		errs = append(errs, errors.New("started_at is required"))
	}
	if e.Status.Terminal() {
		switch {
		case e.FinishedAt == nil:
			errs = append(errs, errors.New("finished_at is required for terminal status"))
		case e.FinishedAt.Before(e.StartedAt):
			errs = append(errs, errors.New("finished_at cannot be before started_at"))
		}
	}
	return errors.Join(errs...)
}

// Duration returns the elapsed deployment time when terminal, otherwise 0.
func (e *DeploymentEvent) Duration() time.Duration {
	if e.FinishedAt == nil {
		return 0
	}
	return e.FinishedAt.Sub(e.StartedAt)
}
