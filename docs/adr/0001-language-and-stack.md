# ADR-0001: Language and stack choice

## Status
Accepted — 2026-08-05

## Context

`shipmetrics` is a long-lived service that ingests deployment events, persists them, evaluates SLOs, and exposes Prometheus metrics. It must be deployable as a container and as a standalone binary, run in air-gapped environments, and be trivially adoptable by SRE teams already using the cloud-native stack.

## Decision

- **Language: Go 1.22+.** Standard library covers HTTP, structured logging (`log/slog`), and testing. Rich Prometheus / OpenTelemetry ecosystem. Single static binary distribution. Idiomatic for Kubernetes-adjacent tooling.
- **HTTP: standard library `net/http`.** With Go 1.22's route-pattern syntax we no longer need a router library at this scope. Avoids dependency churn.
- **Metrics: `prometheus/client_golang`.** De-facto standard.
- **Logging: `log/slog` (standard library).** Structured JSON output by default, text for local dev. No third-party logger.
- **Config: environment variables** loaded with typed validation. No `viper`. Twelve-factor.
- **Persistence (next milestone): PostgreSQL** via `pgx/v5`. Time-series data with rich query needs; a purpose-built TSDB would be over-engineering for the write volume expected.

## Consequences

- Zero third-party HTTP router / logger reduces surface area for CVEs and version churn.
- Standard library commitment forces us to stay on Go 1.22+ (route patterns feature).
- PostgreSQL-first means we defer horizontal-scaling questions until real load demands them.

## Alternatives considered

| Option | Rejected because |
|---|---|
| Rust + Actix | Team velocity higher in Go; K8s/Prometheus ecosystem more mature in Go |
| Node.js / TypeScript | Deployment surface heavier (runtime, `node_modules`); less idiomatic in SRE stack |
| Chi / Gin router | Standard library sufficient at this scope |
| ClickHouse for storage | Operational overhead not justified at expected volume |
