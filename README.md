# shipmetrics

[![CI](https://github.com/anupam2105/shipmetrics/actions/workflows/ci.yml/badge.svg)](https://github.com/anupam2105/shipmetrics/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/anupam2105/shipmetrics)](https://goreportcard.com/report/github.com/anupam2105/shipmetrics)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

**DORA metrics & SLO platform for CI/CD pipelines.** Turn Jenkins / GitHub Actions / GitLab CI deployment data into SRE-grade reliability dashboards and burn-rate alerts.

> Status: **early development** — pre-alpha, API subject to change.

## What it does

`shipmetrics` receives deployment events from your CI/CD system (via webhook), stores them, and exposes:

- The four [DORA metrics](https://cloud.google.com/blog/products/devops-sre/using-the-four-keys-to-measure-your-devops-performance): Deployment Frequency, Lead Time for Changes, Change Failure Rate, MTTR
- SLO evaluations with multi-window burn-rate alerts
- Grafana-ready dashboards
- Prometheus-scrapable metrics endpoint

Think of it as an open-source alternative to Datadog CI Visibility / LinearB, focused on the SRE angle.

## Why

Every dev team has a CI/CD pipeline. Almost nobody has good visibility into pipeline reliability — deployment success rate as an SLO, change failure rate, MTTR. The DORA report has been the industry standard since 2018; open-source tooling to actually measure it is thin.

`shipmetrics` fills that gap.

## One-command demo (full stack)

Prerequisites: Docker (or OrbStack) and `just`.

```bash
just demo
```

Brings up shipmetrics, Postgres, Prometheus, and Grafana with a pre-provisioned DORA dashboard.

- **shipmetrics API:** <http://localhost:8080>
- **Grafana:** <http://localhost:3000> → *shipmetrics* folder → *DORA metrics*
- **Prometheus:** <http://localhost:9090>

Seed some deployment events (a few `curl` calls) and the dashboard populates within one scrape interval.

Tear down with `just demo-down`.

## Local development (without demo containers)

Prerequisites: Go 1.22+, `just`, `golangci-lint`, Docker (for the Postgres dependency).

```bash
just dev          # boots Postgres via docker-compose, then runs the app
```

In another terminal:

```bash
curl -X POST http://localhost:8080/webhooks/jenkins \
  -H 'Content-Type: application/json' \
  -d '{
    "pipeline_name":"checkout-api-deploy",
    "pipeline_id":"jenkins-42",
    "service_name":"checkout-api",
    "environment":"prod",
    "status":"success",
    "started_at":"2026-08-05T10:00:00Z",
    "finished_at":"2026-08-05T10:03:00Z",
    "commit_sha":"abc123"
  }'
```

Verify persistence:

```bash
just db-psql
# then in psql:
SELECT source, service_name, environment, status, finished_at
FROM deployment_events ORDER BY finished_at DESC LIMIT 5;
```

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/healthz` | Liveness probe |
| GET | `/readyz` | Readiness probe |
| GET | `/metrics` | Prometheus scrape target |
| POST | `/webhooks/jenkins` | Canonical shipmetrics JSON payload |
| POST | `/webhooks/github` | GitHub Actions `workflow_run` payload (add `?service=X&environment=Y`) |

### Jenkins payload contract

Add a `sh` step at the end of your pipeline that posts:

```json
{
  "pipeline_name": "checkout-api-deploy",
  "pipeline_id":   "42",
  "service_name":  "checkout-api",
  "environment":   "prod",
  "status":        "success",
  "started_at":    "2026-08-05T10:00:00Z",
  "finished_at":   "2026-08-05T10:03:00Z",
  "commit_sha":    "abc123",
  "commit_timestamp": "2026-08-05T09:55:00Z",
  "metadata": {"triggered_by": "webhook"}
}
```

`status` must be one of: `in_progress`, `success`, `failure`, `cancelled`.
`finished_at` is required for terminal statuses.

### GitHub Actions

Point your workflow_run webhook at `POST /webhooks/github?service=<name>&environment=<env>`. If `service` is omitted the repository name is used; if `environment` is omitted, `unknown` is used.

## Development

```bash
just check           # golangci-lint + go test (unit only)
just db-up           # start Postgres
just test-integration # unit + Postgres integration tests
just build           # produce ./bin/shipmetrics
```

## Configuration

Environment variables:

| Variable | Default | Description |
|---|---|---|
| `SHIPMETRICS_HTTP_ADDR` | `:8080` | HTTP listen address |
| `SHIPMETRICS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SHIPMETRICS_LOG_FORMAT` | `json` | `json` or `text` |
| `SHIPMETRICS_DATABASE_URL` | *required* | Postgres DSN (e.g. `postgres://user:pass@host:5432/db?sslmode=disable`) |

## Architecture

See [docs/adr/](docs/adr/) for architecture decision records.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
