// Package postgres provides a PostgreSQL-backed implementation of storage.Store,
// using pgx/v5 with a connection pool. All queries are parameterised; there is
// no string concatenation of user input into SQL anywhere in this package.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/anupam2105/shipmetrics/internal/domain"
	"github.com/anupam2105/shipmetrics/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the PostgreSQL implementation of storage.Store.
type Store struct {
	pool *pgxpool.Pool
}

// New builds a Store from an existing connection pool. Ownership of the pool
// stays with the caller; Store does not close it.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Connect opens a pgxpool with production-safe defaults and pings the database.
// The caller owns the returned pool and must Close it.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 2
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Ping verifies the store is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

const upsertEventQuery = `
INSERT INTO deployment_events (
    source, pipeline_name, pipeline_id, service_name, environment,
    status, started_at, finished_at, commit_sha, commit_timestamp, metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (source, pipeline_id) DO UPDATE SET
    pipeline_name    = EXCLUDED.pipeline_name,
    service_name     = EXCLUDED.service_name,
    environment      = EXCLUDED.environment,
    status           = EXCLUDED.status,
    started_at       = EXCLUDED.started_at,
    finished_at      = EXCLUDED.finished_at,
    commit_sha       = EXCLUDED.commit_sha,
    commit_timestamp = EXCLUDED.commit_timestamp,
    metadata         = EXCLUDED.metadata
`

// UpsertEvent inserts or updates the event, using (source, pipeline_id) as the
// idempotency key. Domain validation runs before any database work.
func (s *Store) UpsertEvent(ctx context.Context, e *domain.DeploymentEvent) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	metadataJSON, err := marshalMetadata(e.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = s.pool.Exec(ctx, upsertEventQuery,
		string(e.Source),
		e.PipelineName,
		e.PipelineID,
		e.ServiceName,
		e.Environment,
		string(e.Status),
		e.StartedAt,
		e.FinishedAt,
		e.CommitSHA,
		e.CommitTimestamp,
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert deployment_event: %w", err)
	}
	return nil
}

const getEventQuery = `
SELECT source, pipeline_name, pipeline_id, service_name, environment,
       status, started_at, finished_at, commit_sha, commit_timestamp, metadata
FROM deployment_events
WHERE source = $1 AND pipeline_id = $2
`

// GetEvent returns the deployment event identified by (source, pipelineID).
func (s *Store) GetEvent(ctx context.Context, source domain.Source, pipelineID string) (*domain.DeploymentEvent, error) {
	row := s.pool.QueryRow(ctx, getEventQuery, string(source), pipelineID)
	e, err := scanEvent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("get deployment_event: %w", err)
	}
	return e, nil
}

const listRecentQuery = `
SELECT source, pipeline_name, pipeline_id, service_name, environment,
       status, started_at, finished_at, commit_sha, commit_timestamp, metadata
FROM deployment_events
WHERE ($1 = '' OR service_name = $1)
  AND ($2 = '' OR environment  = $2)
  AND finished_at IS NOT NULL
ORDER BY finished_at DESC
LIMIT $3
`

// ListRecentEvents returns up to limit terminal events matching the filter,
// most recently finished first. Non-terminal events are excluded so DORA maths
// don't accidentally count in-progress runs.
func (s *Store) ListRecentEvents(ctx context.Context, filter storage.EventFilter, limit int) ([]*domain.DeploymentEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, listRecentQuery, filter.ServiceName, filter.Environment, limit)
	if err != nil {
		return nil, fmt.Errorf("query deployment_events: %w", err)
	}
	defer rows.Close()

	events := make([]*domain.DeploymentEvent, 0, limit)
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return events, nil
}

// scanner is the minimal subset of *pgx.Rows / pgx.Row used by scanEvent,
// letting us share row-decoding across single-row and multi-row queries.
type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(r scanner) (*domain.DeploymentEvent, error) {
	var (
		e         domain.DeploymentEvent
		source    string
		status    string
		finished  *time.Time
		committed *time.Time
		metaBytes []byte
	)
	if err := r.Scan(
		&source, &e.PipelineName, &e.PipelineID, &e.ServiceName, &e.Environment,
		&status, &e.StartedAt, &finished, &e.CommitSHA, &committed, &metaBytes,
	); err != nil {
		return nil, err
	}
	e.Source = domain.Source(source)
	e.Status = domain.Status(status)
	e.FinishedAt = finished
	e.CommitTimestamp = committed
	if len(metaBytes) > 0 {
		if err := json.Unmarshal(metaBytes, &e.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return &e, nil
}

func marshalMetadata(m map[string]string) ([]byte, error) {
	if len(m) == 0 {
		return []byte(`{}`), nil
	}
	return json.Marshal(m)
}

const listDORATargetsQuery = `
SELECT service_name, environment
FROM deployment_events
WHERE finished_at IS NOT NULL AND finished_at >= $1
GROUP BY service_name, environment
ORDER BY service_name, environment
`

// ListDORATargets returns distinct (service, environment) pairs that had at
// least one terminal event since the given time. The analyzer uses this to
// auto-discover which pairs to compute DORA metrics for.
func (s *Store) ListDORATargets(ctx context.Context, since time.Time) ([]storage.DORATarget, error) {
	rows, err := s.pool.Query(ctx, listDORATargetsQuery, since)
	if err != nil {
		return nil, fmt.Errorf("query dora targets: %w", err)
	}
	defer rows.Close()

	var targets []storage.DORATarget
	for rows.Next() {
		var t storage.DORATarget
		if err := rows.Scan(&t.ServiceName, &t.Environment); err != nil {
			return nil, fmt.Errorf("scan dora target: %w", err)
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return targets, nil
}

// doraWindowQuery aggregates four DORA metrics in a single round trip.
// Lead-time percentiles only consider successful deploys with a known commit
// timestamp; MTTR pairs each failure with the next chronological success in
// the same (service, environment) scope.
const doraWindowQuery = `
WITH windowed AS (
    SELECT status, finished_at, commit_timestamp
    FROM deployment_events
    WHERE service_name = $1
      AND environment  = $2
      AND finished_at IS NOT NULL
      AND finished_at >= $3
),
counts AS (
    SELECT
        COALESCE(COUNT(*) FILTER (WHERE status = 'success'), 0) AS success_count,
        COALESCE(COUNT(*) FILTER (WHERE status = 'failure'), 0) AS failure_count
    FROM windowed
),
lead_time AS (
    SELECT
        COALESCE(PERCENTILE_CONT(0.5)  WITHIN GROUP (
            ORDER BY EXTRACT(EPOCH FROM (finished_at - commit_timestamp))
        ), 0) AS p50,
        COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (
            ORDER BY EXTRACT(EPOCH FROM (finished_at - commit_timestamp))
        ), 0) AS p95
    FROM windowed
    WHERE status = 'success' AND commit_timestamp IS NOT NULL
),
mttr AS (
    SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (next_success - failure_time))), 0) AS avg_seconds
    FROM (
        SELECT
            e.finished_at AS failure_time,
            (
                SELECT MIN(e2.finished_at)
                FROM windowed e2
                WHERE e2.status = 'success' AND e2.finished_at > e.finished_at
            ) AS next_success
        FROM windowed e
        WHERE e.status = 'failure'
    ) t
    WHERE t.next_success IS NOT NULL
)
SELECT counts.success_count, counts.failure_count,
       lead_time.p50, lead_time.p95,
       mttr.avg_seconds
FROM counts, lead_time, mttr
`

// DORAWindow computes the four DORA metrics for one target over the window
// starting at since. All fields default to zero when the underlying data is
// insufficient, so gauges remain stable during cold-start.
func (s *Store) DORAWindow(ctx context.Context, t storage.DORATarget, since time.Time) (storage.DORAWindowStats, error) {
	row := s.pool.QueryRow(ctx, doraWindowQuery, t.ServiceName, t.Environment, since)

	stats := storage.DORAWindowStats{Target: t}
	if err := row.Scan(
		&stats.SuccessCount,
		&stats.FailureCount,
		&stats.LeadTimeP50Sec,
		&stats.LeadTimeP95Sec,
		&stats.MTTRAverageSec,
	); err != nil {
		return storage.DORAWindowStats{}, fmt.Errorf("scan dora window: %w", err)
	}
	return stats, nil
}
