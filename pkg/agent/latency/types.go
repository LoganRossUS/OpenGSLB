// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package latency implements passive latency learning using OS-native TCP statistics.
// ADR-017: Agents learn actual client-to-backend latency by reading TCP RTT data
// that the operating system already tracks for congestion control.
//
// Key principles:
// - Stay out of the application's way: No SDK, no eBPF, no packet capture
// - Just read what the kernel already knows via standard APIs
// - Linux: netlink INET_DIAG (same as `ss -ti`)
// - Windows: GetPerTcpConnectionEStats API
package latency

import (
	"net/netip"
	"time"
)

// Observation represents a single TCP connection's RTT at a point in time.
// This is the raw data collected from the operating system.
type Observation struct {
	// RemoteAddr is the client's IP address.
	RemoteAddr netip.Addr
	// LocalPort is the local (backend) port of the connection.
	LocalPort uint16
	// RTT is the round-trip time measured by TCP.
	RTT time.Duration
	// Timestamp is when this observation was collected.
	Timestamp time.Time
}

// SubnetStats contains aggregated latency statistics for a subnet.
type SubnetStats struct {
	// Subnet is the aggregated subnet prefix (e.g., 203.0.113.0/24).
	Subnet netip.Prefix
	// EWMA is the exponentially weighted moving average RTT.
	EWMA time.Duration
	// SampleCount is the total number of samples contributing to this stat.
	SampleCount uint64
	// LastUpdated is when this subnet was last updated.
	LastUpdated time.Time
	// MinRTT is the minimum observed RTT.
	MinRTT time.Duration
	// MaxRTT is the maximum observed RTT.
	MaxRTT time.Duration
}

// SubnetLatency is used for gossip reporting to Overwatch.
type SubnetLatency struct {
	// Subnet is the client subnet in CIDR notation (e.g., "203.0.113.0/24").
	Subnet string `json:"subnet"`
	// EWMA is the smoothed RTT in nanoseconds.
	EWMA int64 `json:"ewma_ns"`
	// SampleCount is the number of samples.
	SampleCount uint64 `json:"sample_count"`
	// LastSeen is when the subnet was last seen.
	LastSeen time.Time `json:"last_seen"`
}

// LatencyReport is gossiped from agent to overwatch.
type LatencyReport struct {
	// AgentID is the agent that collected this data.
	AgentID string `json:"agent_id"`
	// Backend is the backend service name.
	Backend string `json:"backend"`
	// Region is the agent's region.
	Region string `json:"region"`
	// Timestamp is when this report was generated.
	Timestamp time.Time `json:"timestamp"`
	// Subnets contains the aggregated latency data.
	Subnets []SubnetLatency `json:"subnets"`
}

// CollectorConfig configures the TCP RTT collector.
type CollectorConfig struct {
	// Ports to monitor (only collect RTT for connections to these local ports).
	Ports []uint16
	// PollInterval is how often to poll the OS for connection data.
	// Default: 10s
	PollInterval time.Duration
	// MinConnectionAge is the minimum connection age before collecting RTT.
	// Newly established connections have unstable RTT.
	// Default: 5s
	MinConnectionAge time.Duration
}

// AggregatorConfig configures the subnet aggregator.
type AggregatorConfig struct {
	// IPv4Prefix is the prefix length for IPv4 subnet aggregation.
	// Default: 24 (e.g., /24)
	IPv4Prefix int
	// IPv6Prefix is the prefix length for IPv6 subnet aggregation.
	// Default: 48 (e.g., /48)
	IPv6Prefix int
	// EWMAAlpha is the smoothing factor (0-1).
	// Higher values give more weight to recent samples.
	// Default: 0.3
	EWMAAlpha float64
	// MaxSubnets is the maximum number of subnets to track.
	// Prevents unbounded memory growth.
	// Default: 100000
	MaxSubnets int
	// SubnetTTL is how long to keep subnet entries without updates.
	// Default: 168h (7 days)
	SubnetTTL time.Duration
	// MinSamples is the minimum samples before reporting a subnet.
	// Default: 5
	MinSamples int
}

// DefaultCollectorConfig returns sensible defaults for the collector.
func DefaultCollectorConfig() CollectorConfig {
	return CollectorConfig{
		PollInterval:     10 * time.Second,
		MinConnectionAge: 5 * time.Second,
	}
}

// DefaultAggregatorConfig returns sensible defaults for the aggregator.
func DefaultAggregatorConfig() AggregatorConfig {
	return AggregatorConfig{
		IPv4Prefix: 24,
		IPv6Prefix: 48,
		EWMAAlpha:  0.3,
		MaxSubnets: 100000,
		SubnetTTL:  168 * time.Hour, // 7 days
		MinSamples: 5,
	}
}
