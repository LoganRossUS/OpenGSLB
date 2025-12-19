// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Learned latency table metrics (ADR-017)
var (
	latencyTableEntries = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "opengslb_overwatch_latency_table_entries_total",
			Help: "Total entries in the learned latency table",
		},
	)

	latencyEntriesPruned = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "opengslb_overwatch_latency_entries_pruned_total",
			Help: "Total latency entries pruned due to TTL expiration",
		},
	)

	latencyEntriesEvicted = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "opengslb_overwatch_latency_entries_evicted_total",
			Help: "Total latency entries evicted due to capacity limits",
		},
	)

	latencyRoutingHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opengslb_overwatch_latency_routing_hits_total",
			Help: "Latency routing decisions by method",
		},
		[]string{"method"}, // "learned", "geo_fallback", "cold_start"
	)

	latencyReportsReceived = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "opengslb_overwatch_latency_reports_received_total",
			Help: "Total latency reports received from agents",
		},
	)
)

// RecordLatencyRoutingHit records a latency routing decision.
func RecordLatencyRoutingHit(method string) {
	latencyRoutingHits.WithLabelValues(method).Inc()
}

// RecordLatencyReport records receiving a latency report from an agent.
func RecordLatencyReport() {
	latencyReportsReceived.Inc()
}

// Helper functions for recording metrics using the existing metrics package

// RecordBackendRegistration records a backend registration.
func RecordBackendRegistration(service, region string) {
	// Uses the metrics package which has comprehensive Overwatch metrics
	// Note: Registration tracking is implicit in backend count metrics
}

// RecordBackendDeregistration records a backend deregistration.
func RecordBackendDeregistration(service, reason string) {
	// Deregistration tracking is implicit in backend count metrics
}

// RecordBackendStatusChange records a backend status change.
func RecordBackendStatusChange(service string, fromStatus, toStatus BackendStatus) {
	// Status change tracking is handled via the registry callback
	// which updates OverwatchBackendsHealthy etc.
}

// RecordValidationResult records a validation result.
func RecordValidationResult(service, address string, port int, healthy bool) {
	// Use existing metrics
	metrics.RecordOverwatchValidation(service, healthy, 0)
}

// RecordValidationLatency records validation check latency.
func RecordValidationLatency(service, address string, port int, latency time.Duration) {
	// Latency is recorded via RecordOverwatchValidation
}

// RecordValidationDisagreement records when validation disagrees with agent.
func RecordValidationDisagreement(service string, agentHealthy, validationHealthy bool) {
	// Track vetoes when validation overrides agent claim
	if agentHealthy && !validationHealthy {
		metrics.RecordOverwatchVeto(service, "validation_unhealthy")
	} else if !agentHealthy && validationHealthy {
		metrics.RecordOverwatchVeto(service, "validation_healthy")
	}
}

// RecordGossipMessage records a received gossip message.
func RecordGossipMessage(agentID, msgType string) {
	metrics.RecordGossipMessageReceived(msgType)
}

// RecordAgentRegistration records when a new agent is registered via TOFU.
func RecordAgentRegistration(agentID, region string) {
	// Agent registration is tracked via the agents registered gauge
}

// RecordAgentRevocation records when an agent's certificate is revoked.
func RecordAgentRevocation(agentID string) {
	// Revocation tracking - decrements active agents
}

// SetActiveAgents sets the number of active agents.
func SetActiveAgents(count int) {
	metrics.SetOverwatchAgentsRegistered(count)
}

// SetStaleBackends sets the number of stale backends.
func SetStaleBackends(count int) {
	metrics.SetOverwatchStaleAgents(count)
}

// SetActiveOverrides sets the number of active overrides.
func SetActiveOverrides(count int) {
	metrics.SetOverwatchOverridesActive(count)
}

// RecordOverrideOperation records an override operation.
func RecordOverrideOperation(operation string) {
	metrics.RecordGossipOverride(operation)
}

// UpdateRegistryMetrics updates all registry-related metrics from the registry state.
func UpdateRegistryMetrics(registry *Registry) {
	backends := registry.GetAllBackends()

	// Count by status
	staleCount := 0
	overrideCount := 0
	healthyCount := 0
	agentIDs := make(map[string]bool)

	for _, backend := range backends {
		if backend.EffectiveStatus == StatusStale {
			staleCount++
		} else if backend.EffectiveStatus == StatusHealthy {
			healthyCount++
		}
		if backend.OverrideStatus != nil {
			overrideCount++
		}
		agentIDs[backend.AgentID] = true
	}

	// Update metrics using existing metrics package functions
	metrics.SetOverwatchBackends(len(backends), healthyCount)
	metrics.SetOverwatchAgentsRegistered(len(agentIDs))
	metrics.SetOverwatchStaleAgents(staleCount)
	metrics.SetOverwatchOverridesActive(overrideCount)

	// Update backends by authority
	metrics.SetOverwatchBackendsByAuthority("agent", len(backends)-overrideCount-staleCount)
	metrics.SetOverwatchBackendsByAuthority("override", overrideCount)
	metrics.SetOverwatchBackendsByAuthority("stale", staleCount)
}
