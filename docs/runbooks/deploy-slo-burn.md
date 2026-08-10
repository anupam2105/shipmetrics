# Runbook — Deploy SLO burn

## When this fires

You've been paged (or ticketed) because deployment success rate for a specific `(service, environment)` is consuming error budget faster than the target permits. The alert label tells you which SLO is affected:

```
labels:
  service: checkout-api
  environment: prod
  slo: deploy-success
```

## First 5 minutes

1. **Check the Grafana DORA dashboard.** Filter by the labelled service + environment. Confirm:
   - `Change failure rate` is elevated.
   - `Successful deployments over time` shows a step change or a run of failures.

2. **Query the raw events:**
   ```sql
   SELECT pipeline_id, status, finished_at, commit_sha, metadata
   FROM deployment_events
   WHERE service_name = 'checkout-api'
     AND environment  = 'prod'
     AND finished_at >= NOW() - INTERVAL '2 hours'
   ORDER BY finished_at DESC;
   ```

3. **Correlate with commits.** Look at the `commit_sha` column of the failed rows. If they all cluster around the same commit or PR, that PR is the likely cause.

## Common causes and mitigations

| Cause | Signal | Mitigation |
|---|---|---|
| Flaky test causing false failures | Failures cluster on one test class | Quarantine the test, re-run failing pipelines |
| Bad release rolling out | Failures follow one specific commit range | Roll back the release; investigate offline |
| Infrastructure regression (bad AMI, expired cred, DNS) | Failures across multiple services / envs | Check dependency dashboards; roll back infra change |
| Webhook sender misconfigured | Only *one* pipeline_id repeating, or wrong status field | Fix the CI job; delete synthetic rows if needed |
| Cardinality attack / bad data | `unknown` labels spike | Reject at handler; verify `routeLabel` bounds |

## Budget freeze

If the alert severity is `page`:

1. Announce in the deploy-ops channel that non-critical deploys are paused.
2. Reason posted: **"burn rate X.Yx on service/env, budget frozen until 1h burn < 1 for 60 min consecutive."**
3. Emergency deploys are allowed with pre-approval from an on-call SRE.

## After resolution

- Confirm `shipmetrics_slo_compliant{service, environment, window="1h0m0s"}` has returned to 1 for at least 60 minutes.
- File a **short-form postmortem** if this was a `page`-severity fire. Template: `docs/postmortems/TEMPLATE.md` (add one when needed).
- Add or tune a preventive control (a test, a preflight, a canary gate).

## Escalation

- **First line:** owning team's release engineer.
- **Second line:** platform / SRE on-call.
- **Third line:** engineering leadership; consider slowing all deploys.
