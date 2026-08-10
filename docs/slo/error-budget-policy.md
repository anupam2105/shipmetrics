# Error Budget Policy — deploy success

## What we measure

**SLI (Service Level Indicator):** the ratio of successful deployments to total terminal deployments in a rolling window.

```
SLI = successful_deploys / (successful_deploys + failed_deploys)
```

Non-terminal deploys (`in_progress`) are excluded so half-finished runs never distort the number.

## Targets (SLOs)

| Environment | Target | Error budget | Rationale |
|---|---|---|---|
| `prod` / `production` | 99.0% | 1.0% | Customer-facing risk. A rolling 30-day window of 1000 deploys tolerates 10 failures. |
| `stage` / `staging` | 95.0% | 5.0% | Pre-prod validation should catch issues; slightly higher failure tolerance is expected. |
| `dev` / `development` | 90.0% | 10.0% | Experimentation environment. Failures are cheap and expected. |

Environments not in the table fall back to a 95% default target.

## Windows

We evaluate two windows and require **both to breach before paging**:

- **1h (fast):** reacts within one refresh cycle. Sensitive to spikes.
- **6h (slow):** confirms the burn is sustained, not a single-request flake.

Multi-window alerting is the SRE canonical pattern from the Google SRE Workbook, chapter 5.

## Burn-rate math

Burn rate is *how many times faster than the target rate* you're consuming error budget:

```
burn_rate = observed_failure_rate / error_budget
```

Interpretation, assuming a 30-day SLO horizon:

| Burn rate | Time to exhaust monthly budget |
|---|---|
| 1x | ~30 days (steady-state) |
| 3x | ~10 days |
| 6x | ~5 days |
| 14.4x | ~48 hours |
| 36x | ~20 hours |

## Alert thresholds

| Alert | Trigger | Severity | Action |
|---|---|---|---|
| `DeploySLOFastBurn` | 1h burn > 14.4x **and** 6h burn > 6x, for 2m | `page` | Wake someone up. Freeze non-critical deploys. Escalate to release engineering. |
| `DeploySLOSlowBurn` | 6h burn > 3x **and** 1h burn > 1x, for 15m | `ticket` | Open a ticket. Investigate flaky pipelines / stage drift. |
| `DeploySLOViolating` | 1h compliant == 0, for 30m | `info` | Status-page signal. No paging. |

## Response policy when the SLO is breached

1. **Budget frozen** — non-critical deploys pause until burn rate returns below 1x for 1h.
2. **Owning team creates a postmortem** if a `page`-severity alert fired.
3. **Add a preventive test or gate** for the specific failure class in the following sprint.

## Reviewing the policy

Reviewed quarterly. Change proposals should include:

- Data justifying the new target (e.g., "actual failure rate has been 0.4% for the last 90 days; we can tighten to 99.5%").
- The corresponding change to `internal/slo` defaults.
