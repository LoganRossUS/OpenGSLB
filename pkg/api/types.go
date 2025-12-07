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

// Package api provides the HTTP API for OpenGSLB health and status endpoints.
package api

import "time"

// ServerHealthResponse represents the health status of a single server.
type ServerHealthResponse struct {
	Address              string     `json:"address"`
	Port                 int        `json:"port"`
	Region               string     `json:"region,omitempty"`
	Healthy              bool       `json:"healthy"`
	Status               string     `json:"status"`
	LastCheck            *time.Time `json:"last_check,omitempty"`
	LastHealthy          *time.Time `json:"last_healthy,omitempty"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	ConsecutiveSuccesses int        `json:"consecutive_successes"`
	LastError            string     `json:"last_error,omitempty"`
}

// HealthResponse is the response for GET /api/v1/health/servers.
type HealthResponse struct {
	Servers     []ServerHealthResponse `json:"servers"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// RegionHealthResponse represents health summary for a region.
type RegionHealthResponse struct {
	Name           string  `json:"name"`
	TotalServers   int     `json:"total_servers"`
	HealthyCount   int     `json:"healthy_count"`
	UnhealthyCount int     `json:"unhealthy_count"`
	HealthPercent  float64 `json:"health_percent"`
}

// RegionsResponse is the response for GET /api/v1/health/regions.
type RegionsResponse struct {
	Regions     []RegionHealthResponse `json:"regions"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// ReadyResponse is the response for GET /api/v1/ready.
type ReadyResponse struct {
	Ready       bool   `json:"ready"`
	Message     string `json:"message,omitempty"`
	DNSReady    bool   `json:"dns_ready"`
	HealthReady bool   `json:"health_ready"`
}

// LiveResponse is the response for GET /api/v1/live.
type LiveResponse struct {
	Alive bool `json:"alive"`
}

// ErrorResponse is returned for API errors.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}
