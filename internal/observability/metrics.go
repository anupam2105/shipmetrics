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
	}
	reg.MustRegister(m.HTTPRequestsTotal, m.HTTPRequestDuration)
	return m
}

// Handler returns the http.Handler for /metrics scraping.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})
}
