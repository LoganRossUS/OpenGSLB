// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    → https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

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

// Latency routing metrics (Sprint 6)
var (
	// RoutingLatencySelectedMs records the smoothed latency of selected server.
	RoutingLatencySelectedMs = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "routing_latency_selected_ms",
			Help:      "Smoothed latency in milliseconds of the selected server for latency-based routing",
		},
		[]string{"domain", "server"},
	)

	// RoutingLatencyRejectedTotal counts servers rejected due to latency threshold.
	RoutingLatencyRejectedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "routing_latency_rejected_total",
			Help:      "Total number of servers rejected due to latency threshold or insufficient data",
		},
		[]string{"domain", "server", "reason"},
	)

	// RoutingLatencyFallbackTotal counts fallbacks to round-robin.
	RoutingLatencyFallbackTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "routing_latency_fallback_total",
			Help:      "Total number of fallbacks to round-robin when latency data unavailable",
		},
		[]string{"domain", "reason"},
	)

	// BackendSmoothedLatencyMs records smoothed latency for each backend.
	BackendSmoothedLatencyMs = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "backend_smoothed_latency_ms",
			Help:      "Current smoothed (EMA) latency in milliseconds for each backend",
		},
		[]string{"service", "address"},
	)

	// BackendLatencySamples records number of latency samples for each backend.
	BackendLatencySamples = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "backend_latency_samples",
			Help:      "Number of latency samples collected for each backend",
		},
		[]string{"service", "address"},
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

// Reload metrics
var (
	// ConfigReloadsTotal counts configuration reload attempts.
	ConfigReloadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "config_reloads_total",
			Help:      "Total number of configuration reload attempts",
		},
		[]string{"result"}, // "success" or "failure"
	)

	// ConfigReloadTimestamp tracks when config was last reloaded.
	ConfigReloadTimestamp = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "config_reload_timestamp_seconds",
			Help:      "Timestamp of the last successful configuration reload",
		},
	)
)

// Additional DNSSEC metrics (Stories 7 & 8)
// Note: Core DNSSEC metrics (DNSSECEnabled, DNSSECSigningLatency, DNSSECKeyAgeSeconds)
// are defined in cluster.go
var (
	// DNSSECSigningTotal counts DNSSEC signing operations by zone and result.
	DNSSECSigningTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dnssec_signing_operations_total",
			Help:      "Total number of DNSSEC signing operations",
		},
		[]string{"zone", "result"},
	)

	// DNSSECKeysImported counts keys imported from peers.
	DNSSECKeysImported = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dnssec_keys_imported_total",
			Help:      "Total number of DNSSEC keys imported from peers",
		},
		[]string{"peer"},
	)

	// DNSSECManagedZones tracks number of zones with DNSSEC keys.
	DNSSECManagedZones = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "dnssec_managed_zones",
			Help:      "Number of zones with DNSSEC keys",
		},
	)
)

// Predictive Health metrics
var (
	// PredictiveCPUPercent tracks current CPU utilization.
	PredictiveCPUPercent = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "predictive_cpu_percent",
			Help:      "Current CPU utilization percentage for predictive health",
		},
	)

	// PredictiveMemoryPercent tracks current memory utilization.
	PredictiveMemoryPercent = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "predictive_memory_percent",
			Help:      "Current memory utilization percentage for predictive health",
		},
	)

	// PredictiveErrorRate tracks current health check error rate.
	PredictiveErrorRate = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "predictive_error_rate",
			Help:      "Current health check error rate per minute",
		},
	)

	// PredictiveBleeding tracks if the node is currently signaling bleed.
	PredictiveBleeding = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "predictive_bleeding",
			Help:      "1 if node is signaling bleed, 0 otherwise",
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

// RecordLatencyRoutingDecision records a latency-based routing decision.
func RecordLatencyRoutingDecision(domain, server string, latencyMs float64) {
	RoutingLatencySelectedMs.WithLabelValues(domain, server).Set(latencyMs)
}

// RecordLatencyRejection records a server rejected due to latency.
func RecordLatencyRejection(domain, server, reason string) {
	RoutingLatencyRejectedTotal.WithLabelValues(domain, server, reason).Inc()
}

// RecordLatencyFallback records a fallback to round-robin.
func RecordLatencyFallback(domain, reason string) {
	RoutingLatencyFallbackTotal.WithLabelValues(domain, reason).Inc()
}

// SetBackendLatency sets the current latency metrics for a backend.
func SetBackendLatency(service, address string, smoothedMs float64, samples int) {
	BackendSmoothedLatencyMs.WithLabelValues(service, address).Set(smoothedMs)
	BackendLatencySamples.WithLabelValues(service, address).Set(float64(samples))
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

// SetPredictiveMetrics sets predictive health metrics.
func SetPredictiveMetrics(cpu, memory, errorRate float64, bleeding bool) {
	PredictiveCPUPercent.Set(cpu)
	PredictiveMemoryPercent.Set(memory)
	PredictiveErrorRate.Set(errorRate)
	if bleeding {
		PredictiveBleeding.Set(1)
	} else {
		PredictiveBleeding.Set(0)
	}
}

// RecordDNSSECSigning records a DNSSEC signing operation.
// Uses DNSSECSigningTotal from this file and DNSSECSigningLatency from cluster.go.
func RecordDNSSECSigning(zone string, durationSeconds float64, success bool) {
	result := "success"
	if !success {
		result = "failure"
	}
	DNSSECSigningTotal.WithLabelValues(zone, result).Inc()
	if success {
		ObserveDNSSECSigningLatency(durationSeconds)
	}
}

// RecordDNSSECKeySync records a key sync operation.
// Uses metrics from both this file and cluster.go.
func RecordDNSSECKeySync(peer string, success bool, keysImported int) {
	if success {
		RecordDNSSECKeySyncSuccess()
	} else {
		RecordDNSSECKeySyncFailure(peer)
	}
	if keysImported > 0 {
		DNSSECKeysImported.WithLabelValues(peer).Add(float64(keysImported))
	}
}

// SetDNSSECManagedZones sets the number of zones with DNSSEC keys.
func SetDNSSECManagedZones(count int) {
	DNSSECManagedZones.Set(float64(count))
}
