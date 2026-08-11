# Security

## Reporting a vulnerability

If you believe you've found a security issue in shipmetrics, please **do not open a public GitHub issue**. Instead, open a private security advisory via GitHub's Security tab, or email the maintainer directly.

Expected turnaround: acknowledgement within 3 business days.

## Demo credentials are demo credentials

The `just demo` stack ships with intentionally weak, well-known credentials:

- Postgres: `shipmetrics / shipmetrics`
- Grafana admin: `admin / admin`

These are only exposed on `localhost` inside the `docker-compose` demo profile. They exist so that cloning the repo and running `just demo` "just works" without a config step. **They must not be used in any deployment reachable from the internet or a shared network.**

## Production configuration

For real deployments:

- Provide `SHIPMETRICS_DATABASE_URL` from a secret manager (Vault, AWS Secrets Manager, Kubernetes Secrets).
- Change Grafana admin credentials via `GF_SECURITY_ADMIN_PASSWORD` and disable anonymous access (`GF_AUTH_ANONYMOUS_ENABLED=false`).
- Terminate TLS at an ingress in front of shipmetrics (the binary speaks plain HTTP by design).
- Consider adding webhook signature verification (HMAC on Jenkins / GitHub payloads) — currently on the roadmap, not implemented.

## What's already in place

- All SQL queries are parameterised via `pgx` — no string concatenation.
- HTTP request bodies are capped at 1 MiB (`http.MaxBytesReader`) to prevent memory-exhaustion via giant payloads.
- Prometheus metric labels are drawn from a bounded whitelist to prevent cardinality-attack from unknown URLs.
- Container image uses `gcr.io/distroless/static-debian12:nonroot` — no shell, no package manager, non-root user.
- CI runs `govulncheck` on stdlib and dependencies on every push.
- CI runs `gitleaks` on every push to catch accidentally committed secrets.
- Dependencies are pinned; upgrades reviewed via `go mod tidy` diffs.

## Known gaps (roadmap)

- Webhook payload signature verification (HMAC-SHA256).
- Rate limiting per source.
- Authentication on read APIs (once they exist).
- Multi-tenancy / per-team data isolation.
