// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"log/slog"
	"net"

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

// LeaderChecker checks if this node should serve DNS.
// ADR-015: Deprecated - all Overwatch nodes serve DNS independently.
// Kept for backward compatibility, always returns true.
type LeaderChecker interface {
	IsLeader() bool
}

// HandlerConfig contains configuration for the DNS handler.
type HandlerConfig struct {
	Registry       *Registry
	HealthProvider HealthProvider
	LeaderChecker  LeaderChecker // Deprecated: ignored in ADR-015
	DefaultTTL     uint32
	Logger         *slog.Logger
}
