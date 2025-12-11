// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"log/slog"
	"net"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/routing"
)

// ServerInfo contains information about a backend server.
type ServerInfo struct {
	Address net.IP
	Port    int
	Weight  int
	Region  string
}

// DomainEntry contains configuration for a single domain.
type DomainEntry struct {
	Name             string
	TTL              uint32
	RoutingAlgorithm string
	Router           routing.Router
	Servers          []ServerInfo
}

// HealthProvider checks if a server is healthy.
type HealthProvider interface {
	IsHealthy(address string, port int) bool
}

// LatencyInfo contains latency information for a backend server.
type LatencyInfo struct {
	// SmoothedLatency is the EMA of validation latency measurements.
	SmoothedLatency time.Duration
	// Samples is the number of latency samples collected.
	Samples int
	// LastLatency is the most recent raw latency measurement.
	LastLatency time.Duration
	// HasData indicates whether latency data is available.
	HasData bool
}

// LatencyProvider provides latency data for servers (used by latency-based routing).
type LatencyProvider interface {
	GetLatency(address string, port int) LatencyInfo
}

// LeaderChecker checks if this node should serve DNS.
// ADR-015: Deprecated - all Overwatch nodes serve DNS independently.
// Kept for backward compatibility, always returns true.
type LeaderChecker interface {
	IsLeader() bool
}

// DNSSECSigner signs DNS responses with DNSSEC.
// Implemented by dnssec.Signer.
type DNSSECSigner interface {
	// SignResponse signs a DNS message and returns the signed message.
	// If signing fails or is not possible, returns the original message unchanged.
	SignResponse(msg interface{}) (interface{}, error)
}

// HandlerConfig contains configuration for the DNS handler.
type HandlerConfig struct {
	Registry       *Registry
	HealthProvider HealthProvider
	LeaderChecker  LeaderChecker // Deprecated: ignored in ADR-015
	DNSSECSigner   DNSSECSigner  // Optional: signs responses if provided
	DNSSECEnabled  bool          // Whether DNSSEC is enabled
	ECSEnabled     bool          // Whether to use EDNS Client Subnet for geolocation
	DefaultTTL     uint32
	Logger         *slog.Logger
}
