// Package slo evaluates deployment success SLOs against observed events
// and exports burn-rate gauges consumable by Prometheus alerting rules.
package slo

import (
	"errors"
	"fmt"
	"time"
)

// Definition captures one SLO for a (service, environment) pair. A separate
// Definition applies to each window a caller wants to alert on — most sites
// pair a fast window (1h) with a slow window (6h).
type Definition struct {
	ServiceName string
	Environment string

	// Target success rate expressed as a fraction in [0, 1]. e.g., 0.99 = 99%.
	Target float64

	// Window over which the SLI (failure rate) is computed. Shorter windows
	// react faster; longer windows reduce noise.
	Window time.Duration
}

// Validate enforces that a Definition is well-formed before evaluation.
func (d Definition) Validate() error {
	var errs []error
	if d.ServiceName == "" {
		errs = append(errs, errors.New("service_name is required"))
	}
	if d.Environment == "" {
		errs = append(errs, errors.New("environment is required"))
	}
	if d.Target <= 0 || d.Target >= 1 {
		errs = append(errs, fmt.Errorf("target must be in (0, 1); got %v", d.Target))
	}
	if d.Window <= 0 {
		errs = append(errs, fmt.Errorf("window must be positive; got %v", d.Window))
	}
	return errors.Join(errs...)
}

// ErrorBudget is 1 - Target. 99% target → 1% error budget.
func (d Definition) ErrorBudget() float64 { return 1 - d.Target }

// WindowLabel formats the window so it can be used as a Prometheus label
// (e.g., "1h", "6h"). Keeping the format tight prevents label churn.
func (d Definition) WindowLabel() string { return d.Window.String() }

// Evaluation is the outcome of applying a Definition to one observation window.
type Evaluation struct {
	Definition   Definition
	SuccessCount int
	FailureCount int

	// FailureRate = failures / total. Zero when there is no data yet.
	FailureRate float64

	// BurnRate = FailureRate / ErrorBudget. Values above 1 mean the SLO
	// budget is being consumed faster than the target permits over its
	// nominal 30-day horizon; multi-window alerting uses much higher
	// thresholds (e.g., 14x for fast burn).
	BurnRate float64

	// Compliant is true when FailureRate <= ErrorBudget for the window.
	Compliant   bool
	EvaluatedAt time.Time
}

// Evaluate applies def to the observed counts, producing an Evaluation.
func Evaluate(def Definition, successes, failures int, now time.Time) Evaluation {
	total := successes + failures
	e := Evaluation{
		Definition:   def,
		SuccessCount: successes,
		FailureCount: failures,
		EvaluatedAt:  now,
	}
	if total == 0 {
		e.Compliant = true // no data ⇒ no violation; keeps gauges stable during cold-start
		return e
	}

	e.FailureRate = float64(failures) / float64(total)
	budget := def.ErrorBudget()
	if budget > 0 {
		e.BurnRate = e.FailureRate / budget
	}
	e.Compliant = e.FailureRate <= budget
	return e
}
