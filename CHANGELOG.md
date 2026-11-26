# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Read API: `GET /api/v1/deployments` lists recent terminal deployments with optional `service`, `environment`, `since` and `limit` filters. Consumed by pageguard's `get_recent_deploys` tool.
- Optional bearer-token auth for the read API via `SHIPMETRICS_API_TOKEN` (Helm: `api.existingSecret`).

## [0.1.0-alpha] — 2026-08-11

Initial public alpha. Feature-complete for the "deploy webhook → DORA dashboard → SLO burn-rate alert" path. API surface is expected to change before 1.0.

### Added
- HTTP service with `/healthz`, `/readyz`, `/metrics`, and provider-specific webhook endpoints (`/webhooks/jenkins`, `/webhooks/github`).
- PostgreSQL storage with `pgx/v5` connection pool, idempotent upsert (dedup on `(source, pipeline_id)`), and embedded `golang-migrate` migrations.
- DORA analyzer package computing all four canonical metrics (deployment frequency, lead time p50/p95, change failure rate, MTTR) via a single PostgreSQL CTE. Auto-discovers `(service, environment)` targets — no config required.
- SLO engine with multi-window burn-rate evaluation (1h + 6h) and environment-tiered targets (prod 99%, stage 95%, dev 90%).
- Prometheus alerting rules — `DeploySLOFastBurn` (page), `DeploySLOSlowBurn` (ticket), `DeploySLOViolating` (info).
- Grafana provisioning with a pre-loaded DORA dashboard, filterable by service and environment.
- One-command demo stack (`just demo`) — shipmetrics + Postgres + Prometheus + Grafana.
- Dockerfile using `gcr.io/distroless/static-debian12:nonroot`.
- CI pipeline: golangci-lint, race-detector tests, build + curl-through-binary smoke test against a real Postgres, `govulncheck`, `gitleaks`.
- Docs: architecture decision record, error-budget policy, SLO-burn runbook.
- 41 tests: unit + Postgres integration + Go race detector.

### Security
- SQL queries parameterised throughout — no string concatenation.
- HTTP payload cap (`http.MaxBytesReader`) at 1 MiB to bound memory exposure.
- Prometheus label cardinality bounded via a route whitelist.
- Distroless non-root runtime container.
- Demo credentials documented as demo-only in `SECURITY.md`.

### Known limitations
- No webhook signature verification (HMAC) yet — planned for v0.2.
- No authentication on read paths — read paths do not exist yet.
- Single-tenant only.
- No published Docker image or Helm chart — planned for v0.1.0 (non-alpha).

[Unreleased]: https://github.com/anupam2105/shipmetrics/compare/v0.1.0-alpha...HEAD
[0.1.0-alpha]: https://github.com/anupam2105/shipmetrics/releases/tag/v0.1.0-alpha
