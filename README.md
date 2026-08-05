# shipmetrics

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

## Development

Prerequisites: Go 1.22+, `just`, `golangci-lint`.

```bash
just tidy   # download modules
just check  # lint + tests
just run    # start server on :8080
```

Endpoints:

- `GET /healthz` — liveness
- `GET /readyz` — readiness
- `GET /metrics` — Prometheus scrape target

## Configuration

Environment variables:

| Variable | Default | Description |
|---|---|---|
| `SHIPMETRICS_HTTP_ADDR` | `:8080` | HTTP listen address |
| `SHIPMETRICS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SHIPMETRICS_LOG_FORMAT` | `json` | `json` or `text` |

## Architecture

See [docs/adr/](docs/adr/) for architecture decision records.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
