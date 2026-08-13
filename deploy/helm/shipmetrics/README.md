# shipmetrics Helm chart

Deploys [shipmetrics](https://github.com/anupam2105/shipmetrics) — an open-source DORA metrics and SLO platform for CI/CD pipelines — onto Kubernetes.

## Prerequisites

- Kubernetes 1.26+
- Helm 3.10+
- A reachable PostgreSQL 15+ instance (this chart does **not** bundle Postgres)

## Install

```bash
helm install shipmetrics ./deploy/helm/shipmetrics \
  --namespace shipmetrics --create-namespace \
  --set database.url='postgres://user:pass@postgres.default.svc:5432/shipmetrics?sslmode=disable'
```

For production, provide the DSN via a Kubernetes Secret:

```bash
kubectl -n shipmetrics create secret generic shipmetrics-db \
  --from-literal=database-url='postgres://user:pass@host:5432/shipmetrics?sslmode=require'

helm install shipmetrics ./deploy/helm/shipmetrics \
  --namespace shipmetrics --create-namespace \
  --set database.existingSecret=shipmetrics-db
```

## Key values

| Key | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/anupam2105/shipmetrics` | Container image |
| `image.tag` | `""` (falls back to `Chart.appVersion`) | Image tag |
| `replicaCount` | `1` | Pod replicas |
| `database.url` | `""` | Plain-text DSN — dev only |
| `database.existingSecret` | `""` | Name of an existing Secret holding the DSN |
| `database.existingSecretKey` | `"database-url"` | Key inside the Secret |
| `config.logLevel` | `info` | `debug`, `info`, `warn`, `error` |
| `config.logFormat` | `json` | `json` or `text` |
| `service.type` | `ClusterIP` | Service type |
| `service.port` | `8080` | Service port |
| `ingress.enabled` | `false` | Create an Ingress |
| `serviceMonitor.enabled` | `false` | Create a Prometheus Operator ServiceMonitor |
| `resources` | 50m CPU / 64Mi req, 500m / 256Mi limit | Pod resource shape |
| `podDisruptionBudget.enabled` | `false` | Enable PDB |

Full reference: [`values.yaml`](values.yaml).

## Security hardening (already applied)

- Runs as non-root uid `65532`
- Read-only root filesystem
- All capabilities dropped
- Seccomp `RuntimeDefault`
- Service account token not auto-mounted
- Distroless base image

## Uninstall

```bash
helm uninstall shipmetrics -n shipmetrics
```
