// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Runtime mode metrics
var (
	// RuntimeMode indicates the current runtime mode.
	// Values: 0=agent, 1=overwatch
	RuntimeMode = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "runtime_mode",
			Help:      "Runtime mode (0=agent, 1=overwatch)",
		},
	)

	// RuntimeModeInfo provides mode identification as labels.
	RuntimeModeInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "runtime_mode_info",
			Help:      "Runtime mode information",
		},
		[]string{"mode", "node_id", "region"},
	)
)

// Agent metrics (when running in agent mode)
var (
	// AgentBackendsRegistered tracks the number of backends registered by this agent.
	AgentBackendsRegistered = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_backends_registered",
			Help:      "Number of backends registered by this agent",
		},
	)

	// AgentBackendHealthChecksTotal counts health checks performed by the agent.
	AgentBackendHealthChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "agent_backend_health_checks_total",
			Help:      "Total health checks performed by agent",
		},
		[]string{"service", "result"},
	)

	// AgentHeartbeatsSentTotal counts heartbeats sent to Overwatches.
	AgentHeartbeatsSentTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "agent_heartbeats_sent_total",
			Help:      "Total heartbeats sent to Overwatch nodes",
		},
	)

	// AgentHeartbeatFailuresTotal counts failed heartbeat sends.
	AgentHeartbeatFailuresTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "agent_heartbeat_failures_total",
			Help:      "Total failed heartbeat sends",
		},
	)

	// AgentOverwatchConnectionsActive tracks active connections to Overwatch nodes.
	AgentOverwatchConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_overwatch_connections_active",
			Help:      "Number of active connections to Overwatch nodes",
		},
	)

	// AgentPredictiveStateActive indicates if predictive signals are active.
	AgentPredictiveStateActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_predictive_state_active",
			Help:      "Predictive health state (1=bleeding, 0=normal)",
		},
		[]string{"service", "signal_type"},
	)
)

// Overwatch metrics (when running in overwatch mode)
var (
	// OverwatchAgentsRegistered tracks the number of agents registered with this Overwatch.
	OverwatchAgentsRegistered = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overwatch_agents_registered",
			Help:      "Number of agents registered with this Overwatch",
		},
	)

	// OverwatchBackendsTotal tracks total backends known to this Overwatch.
	OverwatchBackendsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overwatch_backends_total",
			Help:      "Total backends known to this Overwatch",
		},
	)

	// OverwatchBackendsHealthy tracks healthy backends.
	OverwatchBackendsHealthy = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overwatch_backends_healthy",
			Help:      "Number of healthy backends",
		},
	)

	// OverwatchBackendsByAuthority tracks backends by health authority level.
	OverwatchBackendsByAuthority = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overwatch_backends_by_authority",
			Help:      "Backends grouped by health authority source",
		},
		[]string{"authority"},
	)

	// OverwatchValidationChecksTotal counts external validation checks.
	OverwatchValidationChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "overwatch_validation_checks_total",
			Help:      "Total external validation checks performed",
		},
		[]string{"service", "result"},
	)

	// OverwatchValidationLatency tracks external validation check duration.
	OverwatchValidationLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "overwatch_validation_latency_seconds",
			Help:      "External validation check latency in seconds",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		},
		[]string{"service"},
	)

	// OverwatchVetoesTotal counts veto decisions by reason.
	OverwatchVetoesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "overwatch_vetoes_total",
			Help:      "Total number of overwatch vetoes applied",
		},
		[]string{"service", "reason"},
	)

	// OverwatchOverridesActive tracks active health overrides.
	OverwatchOverridesActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overwatch_overrides_active",
			Help:      "Number of active health overrides",
		},
	)

	// OverwatchOverridesByAuthority tracks overrides by authority level.
	OverwatchOverridesByAuthority = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overwatch_overrides_by_authority",
			Help:      "Active overrides grouped by authority source",
		},
		[]string{"authority"},
	)

	// OverwatchAgentHeartbeatAge tracks time since last heartbeat per agent.
	OverwatchAgentHeartbeatAge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overwatch_agent_heartbeat_age_seconds",
			Help:      "Seconds since last heartbeat from agent",
		},
		[]string{"agent_id", "region"},
	)

	// OverwatchStaleAgentsTotal tracks agents marked as stale.
	OverwatchStaleAgentsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overwatch_stale_agents_total",
			Help:      "Number of agents marked as stale (missed heartbeats)",
		},
	)
)

// Agent identity/TOFU metrics
var (
	// OverwatchAgentCertsPinned tracks the number of pinned agent certificates.
	OverwatchAgentCertsPinned = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overwatch_agent_certs_pinned",
			Help:      "Number of pinned agent certificates (TOFU)",
		},
	)

	// OverwatchAgentAuthSuccessTotal counts successful agent authentications.
	OverwatchAgentAuthSuccessTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "overwatch_agent_auth_success_total",
			Help:      "Total successful agent authentications",
		},
	)

	// OverwatchAgentAuthFailuresTotal counts failed agent authentications.
	OverwatchAgentAuthFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "overwatch_agent_auth_failures_total",
			Help:      "Total failed agent authentications",
		},
		[]string{"reason"},
	)
)

// DNS metrics for mode-specific behavior
var (
	// DNSRefusedTotal counts DNS queries refused.
	// In agent mode: all queries refused (agents don't serve DNS)
	// In overwatch mode: should be 0 (all Overwatches serve DNS)
	// Kept for backward compatibility during migration.
	DNSRefusedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dns_refused_total",
			Help:      "Total DNS queries refused (agent mode or misconfiguration)",
		},
	)
)

// Gossip protocol metrics
var (
	// GossipMembersTotal tracks the number of gossip cluster members.
	GossipMembersTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "gossip_members_total",
			Help:      "Total number of gossip cluster members",
		},
	)

	// GossipHealthyMembers tracks the number of healthy (alive) gossip members.
	GossipHealthyMembers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "gossip_healthy_members",
			Help:      "Number of healthy gossip cluster members",
		},
	)

	// GossipMessagesReceivedTotal counts received gossip messages by type.
	GossipMessagesReceivedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_messages_received_total",
			Help:      "Total gossip messages received by type",
		},
		[]string{"type"},
	)

	// GossipMessagesSentTotal counts sent gossip messages by type.
	GossipMessagesSentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_messages_sent_total",
			Help:      "Total gossip messages sent by type",
		},
		[]string{"type"},
	)

	// GossipMessageSendFailures counts failed message sends.
	GossipMessageSendFailures = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_message_send_failures_total",
			Help:      "Total failed gossip message sends",
		},
	)

	// GossipNodeJoinsTotal counts node join events.
	GossipNodeJoinsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_node_joins_total",
			Help:      "Total gossip node join events",
		},
	)

	// GossipNodeLeavesTotal counts node leave events.
	GossipNodeLeavesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_node_leaves_total",
			Help:      "Total gossip node leave events",
		},
	)

	// GossipHealthUpdatesReceivedTotal counts health updates received from agents.
	GossipHealthUpdatesReceivedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_health_updates_received_total",
			Help:      "Total health update messages received via gossip",
		},
	)

	// GossipHealthUpdatesBroadcastTotal counts health updates broadcast.
	GossipHealthUpdatesBroadcastTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_health_updates_broadcast_total",
			Help:      "Total health update messages broadcast via gossip",
		},
	)

	// GossipPredictiveSignalsTotal counts predictive signals by signal type.
	GossipPredictiveSignalsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_predictive_signals_total",
			Help:      "Total predictive signals received via gossip",
		},
		[]string{"signal"},
	)

	// GossipOverridesTotal counts override commands by action.
	GossipOverridesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_overrides_total",
			Help:      "Total override commands received via gossip",
		},
		[]string{"action"},
	)

	// GossipPropagationLatency tracks the latency of message propagation.
	GossipPropagationLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "gossip_propagation_latency_seconds",
			Help:      "Latency of gossip message propagation in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
	)

	// GossipQueueDepth tracks the depth of the gossip message queue.
	GossipQueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "gossip_queue_depth",
			Help:      "Current depth of the gossip message queue",
		},
	)

	// GossipEncryptionEnabled indicates if gossip encryption is enabled (should always be 1).
	GossipEncryptionEnabled = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "gossip_encryption_enabled",
			Help:      "Gossip encryption status (always 1, encryption is mandatory)",
		},
	)

	// GossipDecryptionFailuresTotal counts gossip message decryption failures.
	GossipDecryptionFailuresTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "gossip_decryption_failures_total",
			Help:      "Total gossip message decryption failures",
		},
	)
)

// Per-agent connectivity metrics (Sprint 6)
var (
	// AgentConnectedPerAgent tracks connection status per agent (1=connected, 0=disconnected).
	AgentConnectedPerAgent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_connected",
			Help:      "Agent connection status (1=connected, 0=disconnected)",
		},
		[]string{"agent_id", "region"},
	)

	// AgentBackendsRegisteredPerAgent tracks backends registered per agent.
	AgentBackendsRegisteredPerAgent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_backends_registered_per_agent",
			Help:      "Number of backends registered by each agent",
		},
		[]string{"agent_id"},
	)

	// AgentStaleEventsPerAgent counts stale events per agent.
	AgentStaleEventsPerAgent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "agent_stale_events_total",
			Help:      "Total stale events per agent",
		},
		[]string{"agent_id"},
	)

	// AgentHeartbeatAgePerAgent tracks heartbeat age per agent in seconds.
	AgentHeartbeatAgePerAgent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_heartbeat_age_seconds",
			Help:      "Seconds since last heartbeat per agent",
		},
		[]string{"agent_id"},
	)
)

// Override metrics with service granularity (Sprint 6)
var (
	// OverridesActiveByService tracks active overrides per service.
	OverridesActiveByService = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "overrides_active",
			Help:      "Number of active overrides per service",
		},
		[]string{"service"},
	)

	// OverrideChangesTotal counts override changes by service and action.
	OverrideChangesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "overrides_changes_total",
			Help:      "Total override changes by service and action",
		},
		[]string{"service", "action"},
	)
)

// DNSSEC metrics
var (
	// DNSSECEnabled indicates if DNSSEC signing is enabled.
	DNSSECEnabled = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "dnssec_enabled",
			Help:      "DNSSEC signing status (1=enabled, 0=disabled)",
		},
	)

	// DNSSECSigningLatency tracks DNSSEC signing latency.
	DNSSECSigningLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "dnssec_signing_latency_seconds",
			Help:      "DNSSEC response signing latency in seconds",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025},
		},
	)

	// DNSSECKeyAgeSeconds tracks the age of the current DNSSEC key.
	DNSSECKeyAgeSeconds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "dnssec_key_age_seconds",
			Help:      "Age of the current DNSSEC signing key in seconds",
		},
	)

	// DNSSECKeyAgeByZone tracks DNSSEC key age per zone and key tag.
	DNSSECKeyAgeByZone = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "dnssec_key_age_by_zone_seconds",
			Help:      "Age of DNSSEC signing keys in seconds, per zone and key tag",
		},
		[]string{"zone", "key_tag"},
	)

	// DNSSECSignaturesTotal counts DNSSEC signatures generated per zone.
	DNSSECSignaturesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dnssec_signatures_total",
			Help:      "Total DNSSEC signatures generated per zone",
		},
		[]string{"zone"},
	)

	// DNSSECKeySyncSuccessTotal counts successful key syncs from peers.
	DNSSECKeySyncSuccessTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dnssec_key_sync_success_total",
			Help:      "Total successful DNSSEC key syncs from peers",
		},
	)

	// DNSSECKeySyncFailuresTotal counts failed key syncs.
	DNSSECKeySyncFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dnssec_key_sync_failures_total",
			Help:      "Total failed DNSSEC key syncs",
		},
		[]string{"peer"},
	)
)

// Runtime mode helper functions

// SetRuntimeMode sets the runtime mode metric.
func SetRuntimeMode(mode string) {
	switch mode {
	case "agent":
		RuntimeMode.Set(0)
	case "overwatch":
		RuntimeMode.Set(1)
	default:
		RuntimeMode.Set(-1)
	}
}

// SetRuntimeModeInfo sets the runtime mode info with labels.
func SetRuntimeModeInfo(mode, nodeID, region string) {
	RuntimeModeInfo.WithLabelValues(mode, nodeID, region).Set(1)
}

// Agent metric helper functions

// SetAgentBackendsRegistered sets the number of backends registered by this agent.
func SetAgentBackendsRegistered(count int) {
	AgentBackendsRegistered.Set(float64(count))
}

// RecordAgentHealthCheck records a health check result.
func RecordAgentHealthCheck(service string, healthy bool) {
	result := "healthy"
	if !healthy {
		result = "unhealthy"
	}
	AgentBackendHealthChecksTotal.WithLabelValues(service, result).Inc()
}

// RecordAgentHeartbeatSent increments the heartbeat sent counter.
func RecordAgentHeartbeatSent() {
	AgentHeartbeatsSentTotal.Inc()
}

// RecordAgentHeartbeatFailure increments the heartbeat failure counter.
func RecordAgentHeartbeatFailure() {
	AgentHeartbeatFailuresTotal.Inc()
}

// SetAgentOverwatchConnections sets the number of active Overwatch connections.
func SetAgentOverwatchConnections(count int) {
	AgentOverwatchConnectionsActive.Set(float64(count))
}

// SetAgentPredictiveState sets the predictive state for a service/signal.
func SetAgentPredictiveState(service, signalType string, active bool) {
	value := 0.0
	if active {
		value = 1.0
	}
	AgentPredictiveStateActive.WithLabelValues(service, signalType).Set(value)
}

// Overwatch metric helper functions

// SetOverwatchAgentsRegistered sets the number of registered agents.
func SetOverwatchAgentsRegistered(count int) {
	OverwatchAgentsRegistered.Set(float64(count))
}

// SetOverwatchBackends sets the backend count metrics.
func SetOverwatchBackends(total, healthy int) {
	OverwatchBackendsTotal.Set(float64(total))
	OverwatchBackendsHealthy.Set(float64(healthy))
}

// SetOverwatchBackendsByAuthority sets backend counts per authority level.
func SetOverwatchBackendsByAuthority(authority string, count int) {
	OverwatchBackendsByAuthority.WithLabelValues(authority).Set(float64(count))
}

// RecordOverwatchValidation records an external validation check result.
func RecordOverwatchValidation(service string, healthy bool, latencySeconds float64) {
	result := "healthy"
	if !healthy {
		result = "unhealthy"
	}
	OverwatchValidationChecksTotal.WithLabelValues(service, result).Inc()
	OverwatchValidationLatency.WithLabelValues(service).Observe(latencySeconds)
}

// RecordOverwatchVeto increments the veto counter.
func RecordOverwatchVeto(service, reason string) {
	OverwatchVetoesTotal.WithLabelValues(service, reason).Inc()
}

// SetOverwatchOverridesActive sets the number of active overrides.
func SetOverwatchOverridesActive(count int) {
	OverwatchOverridesActive.Set(float64(count))
}

// SetOverwatchOverridesByAuthority sets override counts per authority level.
func SetOverwatchOverridesByAuthority(authority string, count int) {
	OverwatchOverridesByAuthority.WithLabelValues(authority).Set(float64(count))
}

// SetOverwatchAgentHeartbeatAge sets the heartbeat age for an agent.
func SetOverwatchAgentHeartbeatAge(agentID, region string, ageSeconds float64) {
	OverwatchAgentHeartbeatAge.WithLabelValues(agentID, region).Set(ageSeconds)
}

// SetOverwatchStaleAgents sets the count of stale agents.
func SetOverwatchStaleAgents(count int) {
	OverwatchStaleAgentsTotal.Set(float64(count))
}

// Agent identity helper functions

// SetOverwatchAgentCertsPinned sets the number of pinned certificates.
func SetOverwatchAgentCertsPinned(count int) {
	OverwatchAgentCertsPinned.Set(float64(count))
}

// RecordOverwatchAgentAuthSuccess increments the auth success counter.
func RecordOverwatchAgentAuthSuccess() {
	OverwatchAgentAuthSuccessTotal.Inc()
}

// RecordOverwatchAgentAuthFailure increments the auth failure counter.
func RecordOverwatchAgentAuthFailure(reason string) {
	OverwatchAgentAuthFailuresTotal.WithLabelValues(reason).Inc()
}

// DNS helper functions

// RecordDNSRefused increments the DNS refused counter.
// In the agent-overwatch architecture, this is called when:
// - An agent receives a DNS query (agents don't serve DNS)
// - A misconfigured node refuses queries
// All Overwatches serve DNS, so this should be 0 in overwatch mode.
func RecordDNSRefused() {
	DNSRefusedTotal.Inc()
}

// Gossip metric helper functions

// SetGossipMembers updates the gossip member count metrics.
func SetGossipMembers(total, healthy int) {
	GossipMembersTotal.Set(float64(total))
	GossipHealthyMembers.Set(float64(healthy))
}

// RecordGossipMessageReceived increments the received message counter.
func RecordGossipMessageReceived(msgType string) {
	GossipMessagesReceivedTotal.WithLabelValues(msgType).Inc()
}

// RecordGossipMessageSent increments the sent message counter.
func RecordGossipMessageSent(msgType string) {
	GossipMessagesSentTotal.WithLabelValues(msgType).Inc()
}

// RecordGossipSendFailure increments the send failure counter.
func RecordGossipSendFailure() {
	GossipMessageSendFailures.Inc()
}

// RecordGossipNodeJoin increments the node join counter.
func RecordGossipNodeJoin() {
	GossipNodeJoinsTotal.Inc()
}

// RecordGossipNodeLeave increments the node leave counter.
func RecordGossipNodeLeave() {
	GossipNodeLeavesTotal.Inc()
}

// RecordGossipHealthUpdateReceived increments the health update received counter.
func RecordGossipHealthUpdateReceived() {
	GossipHealthUpdatesReceivedTotal.Inc()
}

// RecordGossipHealthUpdateBroadcast increments the health update broadcast counter.
func RecordGossipHealthUpdateBroadcast() {
	GossipHealthUpdatesBroadcastTotal.Inc()
}

// RecordGossipPredictiveSignal increments the predictive signal counter.
func RecordGossipPredictiveSignal(signal string) {
	GossipPredictiveSignalsTotal.WithLabelValues(signal).Inc()
}

// RecordGossipOverride increments the override command counter.
func RecordGossipOverride(action string) {
	GossipOverridesTotal.WithLabelValues(action).Inc()
}

// ObserveGossipPropagationLatency records a message propagation latency observation.
func ObserveGossipPropagationLatency(seconds float64) {
	GossipPropagationLatency.Observe(seconds)
}

// SetGossipQueueDepth sets the current gossip queue depth.
func SetGossipQueueDepth(depth int) {
	GossipQueueDepth.Set(float64(depth))
}

// SetGossipEncryptionEnabled sets the encryption status (should always be 1).
func SetGossipEncryptionEnabled() {
	GossipEncryptionEnabled.Set(1)
}

// DNSSEC metric helper functions

// SetDNSSECEnabled sets the DNSSEC enabled status.
func SetDNSSECEnabled(enabled bool) {
	if enabled {
		DNSSECEnabled.Set(1)
	} else {
		DNSSECEnabled.Set(0)
	}
}

// ObserveDNSSECSigningLatency records a DNSSEC signing latency observation.
func ObserveDNSSECSigningLatency(seconds float64) {
	DNSSECSigningLatency.Observe(seconds)
}

// SetDNSSECKeyAge sets the age of the current DNSSEC key.
func SetDNSSECKeyAge(ageSeconds float64) {
	DNSSECKeyAgeSeconds.Set(ageSeconds)
}

// RecordDNSSECKeySyncSuccess increments the key sync success counter.
func RecordDNSSECKeySyncSuccess() {
	DNSSECKeySyncSuccessTotal.Inc()
}

// RecordDNSSECKeySyncFailure increments the key sync failure counter.
func RecordDNSSECKeySyncFailure(peer string) {
	DNSSECKeySyncFailuresTotal.WithLabelValues(peer).Inc()
}

// SetDNSSECKeyAgeByZone sets the key age for a specific zone and key tag.
func SetDNSSECKeyAgeByZone(zone, keyTag string, ageSeconds float64) {
	DNSSECKeyAgeByZone.WithLabelValues(zone, keyTag).Set(ageSeconds)
}

// RecordDNSSECSignature increments the signature counter for a zone.
func RecordDNSSECSignature(zone string) {
	DNSSECSignaturesTotal.WithLabelValues(zone).Inc()
}

// Gossip decryption helper functions

// RecordGossipDecryptionFailure increments the gossip decryption failure counter.
func RecordGossipDecryptionFailure() {
	GossipDecryptionFailuresTotal.Inc()
}

// Per-agent connectivity helper functions

// SetAgentConnected sets the connection status for an agent.
func SetAgentConnected(agentID, region string, connected bool) {
	value := 0.0
	if connected {
		value = 1.0
	}
	AgentConnectedPerAgent.WithLabelValues(agentID, region).Set(value)
}

// SetAgentBackendsRegisteredPerAgent sets the backend count for an agent.
func SetAgentBackendsRegisteredPerAgent(agentID string, count int) {
	AgentBackendsRegisteredPerAgent.WithLabelValues(agentID).Set(float64(count))
}

// RecordAgentStaleEvent increments the stale event counter for an agent.
func RecordAgentStaleEvent(agentID string) {
	AgentStaleEventsPerAgent.WithLabelValues(agentID).Inc()
}

// SetAgentHeartbeatAgePerAgent sets the heartbeat age for an agent.
func SetAgentHeartbeatAgePerAgent(agentID string, ageSeconds float64) {
	AgentHeartbeatAgePerAgent.WithLabelValues(agentID).Set(ageSeconds)
}

// Override helper functions

// SetOverridesActiveByService sets the active override count for a service.
func SetOverridesActiveByService(service string, count int) {
	OverridesActiveByService.WithLabelValues(service).Set(float64(count))
}

// RecordOverrideChange records an override change action for a service.
func RecordOverrideChange(service, action string) {
	OverrideChangesTotal.WithLabelValues(service, action).Inc()
}
