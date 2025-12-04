// Package metrics provides Prometheus metrics for OpenGSLB observability.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Namespace for all OpenGSLB metrics.
const namespace = "opengslb"

// DNS metrics
var (
	// DNSQueriesTotal counts total DNS queries received.
	DNSQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dns_queries_total",
			Help:      "Total number of DNS queries received",
		},
		[]string{"domain", "type", "status"},
	)

	// DNSQueryDuration measures DNS query processing time.
	DNSQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "dns_query_duration_seconds",
			Help:      "DNS query processing duration in seconds",
			Buckets:   []float64{.0001, .0005, .001, .005, .01, .025, .05, .1},
		},
		[]string{"domain", "status"},
	)
)

// Health check metrics
var (
	// HealthCheckResultsTotal counts health check results by outcome.
	HealthCheckResultsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "health_check_results_total",
			Help:      "Total number of health check results by server and outcome",
		},
		[]string{"region", "server", "result"},
	)

	// HealthCheckDuration measures health check latency.
	HealthCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "health_check_duration_seconds",
			Help:      "Health check duration in seconds",
			Buckets:   []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"region", "server"},
	)

	// HealthyServersGauge tracks current number of healthy servers per region.
	HealthyServersGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "healthy_servers",
			Help:      "Current number of healthy servers per region",
		},
		[]string{"region"},
	)
)

// Routing metrics
var (
	// RoutingDecisionsTotal counts routing decisions made.
	RoutingDecisionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "routing_decisions_total",
			Help:      "Total number of routing decisions by domain and selected server",
		},
		[]string{"domain", "algorithm", "server"},
	)
)

// Application metrics
var (
	// AppInfo provides build information as labels.
	AppInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "app_info",
			Help:      "OpenGSLB application information",
		},
		[]string{"version"},
	)

	// ConfigLoadTimestamp tracks when config was last loaded.
	ConfigLoadTimestamp = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "config_load_timestamp_seconds",
			Help:      "Unix timestamp of last configuration load",
		},
	)

	// ConfiguredDomains tracks number of configured domains.
	ConfiguredDomains = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "configured_domains",
			Help:      "Number of configured domains",
		},
	)

	// ConfiguredServers tracks total number of configured servers.
	ConfiguredServers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "configured_servers",
			Help:      "Total number of configured servers across all regions",
		},
	)
)

// RecordDNSQuery records a DNS query metric.
func RecordDNSQuery(domain, queryType, status string) {
	DNSQueriesTotal.WithLabelValues(domain, queryType, status).Inc()
}

// RecordDNSQueryDuration records DNS query processing time.
func RecordDNSQueryDuration(domain, status string, durationSeconds float64) {
	DNSQueryDuration.WithLabelValues(domain, status).Observe(durationSeconds)
}

// RecordHealthCheckResult records a health check result.
func RecordHealthCheckResult(region, server, result string) {
	HealthCheckResultsTotal.WithLabelValues(region, server, result).Inc()
}

// RecordHealthCheckDuration records health check latency.
func RecordHealthCheckDuration(region, server string, durationSeconds float64) {
	HealthCheckDuration.WithLabelValues(region, server).Observe(durationSeconds)
}

// SetHealthyServers sets the current count of healthy servers for a region.
func SetHealthyServers(region string, count int) {
	HealthyServersGauge.WithLabelValues(region).Set(float64(count))
}

// RecordRoutingDecision records a routing decision.
func RecordRoutingDecision(domain, algorithm, server string) {
	RoutingDecisionsTotal.WithLabelValues(domain, algorithm, server).Inc()
}

// SetAppInfo sets the application info metric.
func SetAppInfo(version string) {
	AppInfo.WithLabelValues(version).Set(1)
}

// SetConfigMetrics sets configuration-related metrics.
func SetConfigMetrics(domains, servers int, loadTime float64) {
	ConfiguredDomains.Set(float64(domains))
	ConfiguredServers.Set(float64(servers))
	ConfigLoadTimestamp.Set(loadTime)
}

// Reload metrics
var (
	ConfigReloadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "config_reloads_total",
			Help:      "Total number of configuration reload attempts",
		},
		[]string{"result"}, // "success" or "failure"
	)

	ConfigReloadTimestamp = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "config_reload_timestamp_seconds",
			Help:      "Timestamp of the last successful configuration reload",
		},
	)
)

// RecordReload records a configuration reload attempt.
func RecordReload(success bool) {
	result := "success"
	if !success {
		result = "failure"
	}
	ConfigReloadsTotal.WithLabelValues(result).Inc()
	if success {
		ConfigReloadTimestamp.SetToCurrentTime()
	}
}
