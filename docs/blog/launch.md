# shipmetrics — an open-source DORA metrics and SLO platform for CI/CD pipelines

*Published: 2026-08-13. Repository: <https://github.com/anupam2105/shipmetrics>.*

## The gap I kept hitting

Every team I've worked with has a CI/CD pipeline. Almost nobody has a *structured* answer to four boring questions:

- How often do we deploy to production?
- How long does a commit take to reach production?
- What fraction of our deploys fail?
- How long does recovery take when they do?

These are the **four DORA metrics** — the industry standard since Google's *Accelerate* research made them the differentiator between elite and low-performing engineering orgs. Datadog CI Visibility measures them. LinearB measures them. Sleuth measures them. All three are SaaS, all three are expensive, and all three lock the data in their platforms.

The open-source landscape here is thin. Grafana dashboards exist for hand-rolled DORA queries. There's the occasional Python script buried in a Medium post. Nothing that ships as a service you can point a webhook at.

I wanted the equivalent of `helm install shipmetrics` and, within minutes, a Grafana dashboard showing my team's DORA numbers. So I built it.

## What `shipmetrics` does

The data path is deliberately simple:

```
Jenkins / GitHub Actions → webhook → shipmetrics → PostgreSQL
                                            ↓
                                     DORA analyzer (in-process, 30s tick)
                                            ↓
                                     Prometheus /metrics endpoint
                                            ↓
                                     Prometheus + Grafana
```

Every deployment event lands in one Postgres row (idempotent upsert keyed on `(source, pipeline_id)` — safe for at-least-once webhook delivery). A background analyzer runs a single PostgreSQL CTE every 30 seconds:

- `COUNT(*) FILTER (WHERE status = 'success')` — deployment frequency
- `PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY finished_at - commit_timestamp)` — lead time
- Failure rate — change failure rate
- A correlated subquery pairing each failure with its next success — MTTR

Results are exported as labelled Prometheus gauges. Grafana is pre-provisioned with a DORA dashboard that colour-codes each metric against the SRE-canonical thresholds (green under 15% CFR, red over 30%, and so on).

Screenshot after seeding ~15 realistic events:

*(insert docs/screenshots/dora-dashboard.png)*

The dashboard reports 12 successful deploys, 17.5% change failure rate, 35.4 minute p50 lead time, and 29.2 minute MTTR — all filterable by service and environment.

## The SLO layer

Raw DORA metrics aren't enough. The point of SRE is answering: *given a target, are we meeting it?*

`shipmetrics` includes a small SLO engine. Default targets:

- `prod` — 99% deploy success (error budget 1%)
- `stage` — 95% deploy success (error budget 5%)
- `dev` — 90% deploy success (error budget 10%)

Two windows are evaluated per target — 1h (fast) and 6h (slow). This is the canonical multi-window burn-rate pattern from the Google SRE Workbook. Prometheus alerting rules ship with the repo:

- `DeploySLOFastBurn` — page (1h > 14.4x AND 6h > 6x)
- `DeploySLOSlowBurn` — ticket (6h > 3x AND 1h > 1x)
- `DeploySLOViolating` — status-page signal (1h compliant == 0 for 30m)

An error-budget policy document and a runbook for the on-call responder are checked into the repo alongside the alerts. Because that's how real SRE works — alerts without runbooks are just noise generators.

## Try it in 60 seconds

```bash
git clone https://github.com/anupam2105/shipmetrics.git
cd shipmetrics
just demo
```

Opens up Grafana at <http://localhost:3000>, Prometheus at <http://localhost:9090>, and the shipmetrics API at <http://localhost:8080>. Fire a few webhook `curl` calls (there's a snippet in the README) and the dashboard populates within one scrape interval.

For Kubernetes:

```bash
helm install shipmetrics ./deploy/helm/shipmetrics \
  --namespace shipmetrics --create-namespace \
  --set database.existingSecret=my-db-secret
```

The chart is production-shaped: non-root uid 65532, read-only root filesystem, dropped capabilities, seccomp `RuntimeDefault`, opt-in `ServiceMonitor` for prometheus-operator, opt-in `PodDisruptionBudget`, opt-in Ingress.

## The stack

- **Go 1.26** with the standard library HTTP router (`net/http` with Go 1.22 route patterns)
- **PostgreSQL 16** via `pgx/v5` with a bounded connection pool
- **`golang-migrate/v4`** with `//go:embed` — migrations ship in the binary
- **Prometheus** with a bounded metric-cardinality whitelist (external URLs cannot inflate labels)
- **Grafana** with dashboards + datasource provisioning from filesystem
- **Distroless nonroot** container base
- CI: golangci-lint, race-detector tests, `govulncheck`, `gitleaks`, real Postgres integration + curl-through-binary smoke test

41 tests, 5 CI quality gates, one binary, one Helm chart, zero secrets in the repo.

## Roadmap

- Webhook HMAC signature verification (Jenkins + GitHub Actions)
- Read APIs (`GET /api/dora?service=X&range=7d`)
- Native GitLab CI support
- Cosign-signed images and SLSA provenance
- Multi-tenancy

## Feedback welcome

The repo is at <https://github.com/anupam2105/shipmetrics>. Open an issue if this solves a problem for you — I want to know what to build next.

If you found this useful, a star on the repo is the loudest signal you can send.
