// Package observability exposes Prometheus metrics used across the process.
package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the registered Prometheus collectors and the private registry
// they belong to. Using a private registry (rather than the default global)
// keeps tests isolated and prevents importers from polluting metrics.
type Metrics struct {
	registry *prometheus.Registry

	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	WebhookEvents *prometheus.CounterVec // one per accepted event, labelled by source + status
	WebhookErrors *prometheus.CounterVec // one per rejected event, labelled by source + error_type

	// DORA gauges, refreshed by internal/dora.Analyzer. All are labelled by
	// (service, environment).
	DORADeployments       *prometheus.GaugeVec // count of successful deploys in window
	DORAFailures          *prometheus.GaugeVec // count of failed deploys in window
	DORAChangeFailureRate *prometheus.GaugeVec // 0..1 ratio
	DORALeadTimeSeconds   *prometheus.GaugeVec // labelled by quantile (0.5, 0.95)
	DORAMTTRSeconds       *prometheus.GaugeVec // average MTTR

	// SLO gauges refreshed by internal/slo.Evaluator, labelled by
	// (service, environment, window).
	SLOBurnRate    *prometheus.GaugeVec // failure_rate / error_budget
	SLOFailureRate *prometheus.GaugeVec // observed failure ratio in window
	SLOTarget      *prometheus.GaugeVec // configured target success rate
	SLOCompliant   *prometheus.GaugeVec // 1.0 if compliant else 0.0
}

// NewMetrics constructs the registry with Go runtime + process collectors and
// the HTTP request instruments.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		registry: reg,
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "shipmetrics",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total HTTP requests, labelled by method, route and status.",
			},
			[]string{"method", "route", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "shipmetrics",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request latency distribution.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "route"},
		),
		WebhookEvents: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "shipmetrics",
				Subsystem: "webhook",
				Name:      "events_total",
				Help:      "Accepted deployment events, labelled by source and status.",
			},
			[]string{"source", "status"},
		),
		WebhookErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "shipmetrics",
				Subsystem: "webhook",
				Name:      "errors_total",
				Help:      "Rejected webhook requests, labelled by source and error type.",
			},
			[]string{"source", "error_type"},
		),
		DORADeployments: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "shipmetrics",
				Subsystem: "dora",
				Name:      "successful_deployments",
				Help:      "Number of successful terminal deployments in the analyzer window.",
			},
			[]string{"service", "environment"},
		),
		DORAFailures: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "shipmetrics",
				Subsystem: "dora",
				Name:      "failed_deployments",
				Help:      "Number of failed terminal deployments in the analyzer window.",
			},
			[]string{"service", "environment"},
		),
		DORAChangeFailureRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "shipmetrics",
				Subsystem: "dora",
				Name:      "change_failure_rate",
				Help:      "Change failure rate in the analyzer window (0..1).",
			},
			[]string{"service", "environment"},
		),
		DORALeadTimeSeconds: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "shipmetrics",
				Subsystem: "dora",
				Name:      "lead_time_seconds",
				Help:      "Lead time for changes in seconds — commit to deploy — at the given quantile.",
			},
			[]string{"service", "environment", "quantile"},
		),
		DORAMTTRSeconds: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "shipmetrics",
				Subsystem: "dora",
				Name:      "mttr_seconds",
				Help:      "Mean time to recovery in seconds — failed deploy to next successful deploy.",
			},
			[]string{"service", "environment"},
		),
		SLOBurnRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "shipmetrics",
				Subsystem: "slo",
				Name:      "burn_rate",
				Help:      "Observed failure rate divided by the SLO error budget over the window.",
			},
			[]string{"service", "environment", "window"},
		),
		SLOFailureRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "shipmetrics",
				Subsystem: "slo",
				Name:      "failure_rate",
				Help:      "Observed deploy failure rate over the window (0..1).",
			},
			[]string{"service", "environment", "window"},
		),
		SLOTarget: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "shipmetrics",
				Subsystem: "slo",
				Name:      "target",
				Help:      "Configured deploy-success target for the (service, environment, window).",
			},
			[]string{"service", "environment", "window"},
		),
		SLOCompliant: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "shipmetrics",
				Subsystem: "slo",
				Name:      "compliant",
				Help:      "1 if the window is meeting its SLO, 0 otherwise.",
			},
			[]string{"service", "environment", "window"},
		),
	}
	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.WebhookEvents,
		m.WebhookErrors,
		m.DORADeployments,
		m.DORAFailures,
		m.DORAChangeFailureRate,
		m.DORALeadTimeSeconds,
		m.DORAMTTRSeconds,
		m.SLOBurnRate,
		m.SLOFailureRate,
		m.SLOTarget,
		m.SLOCompliant,
	)
	return m
}

// Handler returns the http.Handler for /metrics scraping.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})
}
