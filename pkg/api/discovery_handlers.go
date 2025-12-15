// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"net/http"
	"time"
)

// APIEndpoint represents a discoverable API endpoint.
type APIEndpoint struct {
	Path        string   `json:"path"`
	Description string   `json:"description"`
	Methods     []string `json:"methods,omitempty"`
}

// APIDiscoveryResponse is the response for API discovery endpoints.
type APIDiscoveryResponse struct {
	Endpoints   []APIEndpoint `json:"endpoints"`
	GeneratedAt time.Time     `json:"generated_at"`
}

// APIVersionResponse is the response for /api showing available versions.
type APIVersionResponse struct {
	Versions    []APIEndpoint `json:"versions"`
	GeneratedAt time.Time     `json:"generated_at"`
}

// DiscoveryHandlers handles API discovery endpoints.
type DiscoveryHandlers struct {
	// Track which handler groups are registered
	hasOverwatch   bool
	hasOverrides   bool
	hasDNSSEC      bool
	hasGeo         bool
	hasDomains     bool
	hasServers     bool
	hasRegions     bool
	hasNodes       bool
	hasGossip      bool
	hasAuditLogs   bool
	hasMetrics     bool
	hasConfig      bool
	hasRouting     bool
	hasHealth      bool
	hasSimpleHealth bool
}

// NewDiscoveryHandlers creates a new DiscoveryHandlers instance.
func NewDiscoveryHandlers() *DiscoveryHandlers {
	return &DiscoveryHandlers{}
}

// SetHasOverwatch marks that overwatch endpoints are available.
func (h *DiscoveryHandlers) SetHasOverwatch(has bool) { h.hasOverwatch = has }

// SetHasOverrides marks that override endpoints are available.
func (h *DiscoveryHandlers) SetHasOverrides(has bool) { h.hasOverrides = has }

// SetHasDNSSEC marks that DNSSEC endpoints are available.
func (h *DiscoveryHandlers) SetHasDNSSEC(has bool) { h.hasDNSSEC = has }

// SetHasGeo marks that geolocation endpoints are available.
func (h *DiscoveryHandlers) SetHasGeo(has bool) { h.hasGeo = has }

// SetHasDomains marks that domain endpoints are available.
func (h *DiscoveryHandlers) SetHasDomains(has bool) { h.hasDomains = has }

// SetHasServers marks that server endpoints are available.
func (h *DiscoveryHandlers) SetHasServers(has bool) { h.hasServers = has }

// SetHasRegions marks that region endpoints are available.
func (h *DiscoveryHandlers) SetHasRegions(has bool) { h.hasRegions = has }

// SetHasNodes marks that node endpoints are available.
func (h *DiscoveryHandlers) SetHasNodes(has bool) { h.hasNodes = has }

// SetHasGossip marks that gossip endpoints are available.
func (h *DiscoveryHandlers) SetHasGossip(has bool) { h.hasGossip = has }

// SetHasAuditLogs marks that audit log endpoints are available.
func (h *DiscoveryHandlers) SetHasAuditLogs(has bool) { h.hasAuditLogs = has }

// SetHasMetrics marks that metrics endpoints are available.
func (h *DiscoveryHandlers) SetHasMetrics(has bool) { h.hasMetrics = has }

// SetHasConfig marks that config endpoints are available.
func (h *DiscoveryHandlers) SetHasConfig(has bool) { h.hasConfig = has }

// SetHasRouting marks that routing endpoints are available.
func (h *DiscoveryHandlers) SetHasRouting(has bool) { h.hasRouting = has }

// SetHasHealth marks that health endpoints are available.
func (h *DiscoveryHandlers) SetHasHealth(has bool) { h.hasHealth = has }

// SetHasSimpleHealth marks that simple health endpoint is available.
func (h *DiscoveryHandlers) SetHasSimpleHealth(has bool) { h.hasSimpleHealth = has }

// HandleAPIRoot handles GET /api - returns available API versions.
func (h *DiscoveryHandlers) HandleAPIRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Only show /api, not /api/ with trailing paths
	if r.URL.Path != "/api" && r.URL.Path != "/api/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	versions := []APIEndpoint{
		{
			Path:        "/api/v1",
			Description: "OpenGSLB API version 1",
		},
	}

	// Include /api/health if simple health is registered
	if h.hasSimpleHealth {
		versions = append(versions, APIEndpoint{
			Path:        "/api/health",
			Description: "Simple health check endpoint",
			Methods:     []string{"GET"},
		})
	}

	resp := APIVersionResponse{
		Versions:    versions,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleV1Root handles GET /api/v1 - returns available v1 endpoints.
func (h *DiscoveryHandlers) HandleV1Root(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Only show /api/v1, not /api/v1/ with trailing paths
	if r.URL.Path != "/api/v1" && r.URL.Path != "/api/v1/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	endpoints := []APIEndpoint{}

	// Health endpoints are always available
	if h.hasHealth {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/health",
			Description: "Health status endpoints",
		})
	}

	// Liveness and readiness probes
	endpoints = append(endpoints,
		APIEndpoint{
			Path:        "/api/v1/ready",
			Description: "Readiness probe",
			Methods:     []string{"GET"},
		},
		APIEndpoint{
			Path:        "/api/v1/live",
			Description: "Liveness probe",
			Methods:     []string{"GET"},
		},
	)

	// Conditional endpoints based on what handlers are registered
	if h.hasDomains {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/domains",
			Description: "Domain management",
			Methods:     []string{"GET", "POST"},
		})
	}

	if h.hasServers {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/servers",
			Description: "Server management",
			Methods:     []string{"GET", "POST"},
		})
	}

	if h.hasRegions {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/regions",
			Description: "Region management",
			Methods:     []string{"GET", "POST"},
		})
	}

	if h.hasNodes {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/nodes",
			Description: "Node management (Overwatch and Agent nodes)",
			Methods:     []string{"GET"},
		})
	}

	if h.hasGossip {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/gossip",
			Description: "Gossip protocol management",
			Methods:     []string{"GET"},
		})
	}

	if h.hasAuditLogs {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/audit-logs",
			Description: "Audit log retrieval",
			Methods:     []string{"GET"},
		})
	}

	if h.hasMetrics {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/metrics",
			Description: "System metrics",
			Methods:     []string{"GET"},
		})
	}

	if h.hasConfig {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/config",
			Description: "Configuration management",
			Methods:     []string{"GET", "PUT"},
		})
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/preferences",
			Description: "User preferences",
			Methods:     []string{"GET", "PUT"},
		})
	}

	if h.hasRouting {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/routing",
			Description: "Routing algorithms and decisions",
			Methods:     []string{"GET", "POST"},
		})
	}

	if h.hasOverrides {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/overrides",
			Description: "Health check overrides",
			Methods:     []string{"GET", "PUT", "DELETE"},
		})
	}

	if h.hasDNSSEC {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/dnssec",
			Description: "DNSSEC management",
			Methods:     []string{"GET", "POST"},
		})
	}

	if h.hasGeo {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/geo",
			Description: "Geolocation management",
		})
	}

	if h.hasOverwatch {
		endpoints = append(endpoints, APIEndpoint{
			Path:        "/api/v1/overwatch",
			Description: "Overwatch-specific endpoints",
		})
	}

	resp := APIDiscoveryResponse{
		Endpoints:   endpoints,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleHealthRoot handles GET /api/v1/health - returns available health endpoints.
func (h *DiscoveryHandlers) HandleHealthRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Only show /api/v1/health, not sub-paths
	if r.URL.Path != "/api/v1/health" && r.URL.Path != "/api/v1/health/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	endpoints := []APIEndpoint{
		{
			Path:        "/api/v1/health/servers",
			Description: "Health status of all servers",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/health/regions",
			Description: "Health status aggregated by region",
			Methods:     []string{"GET"},
		},
	}

	resp := APIDiscoveryResponse{
		Endpoints:   endpoints,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleGeoRoot handles GET /api/v1/geo - returns available geo endpoints.
func (h *DiscoveryHandlers) HandleGeoRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Only show /api/v1/geo, not sub-paths that should go to other handlers
	if r.URL.Path != "/api/v1/geo" && r.URL.Path != "/api/v1/geo/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	endpoints := []APIEndpoint{
		{
			Path:        "/api/v1/geo/mappings",
			Description: "Custom IP-to-region mappings",
			Methods:     []string{"GET", "PUT", "DELETE"},
		},
		{
			Path:        "/api/v1/geo/test",
			Description: "Test geolocation resolution for an IP",
			Methods:     []string{"GET"},
		},
	}

	resp := APIDiscoveryResponse{
		Endpoints:   endpoints,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleOverwatchRoot handles GET /api/v1/overwatch - returns available overwatch endpoints.
func (h *DiscoveryHandlers) HandleOverwatchRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Only show /api/v1/overwatch, not sub-paths
	if r.URL.Path != "/api/v1/overwatch" && r.URL.Path != "/api/v1/overwatch/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	endpoints := []APIEndpoint{
		{
			Path:        "/api/v1/overwatch/backends",
			Description: "Backend status for agents",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/overwatch/stats",
			Description: "Statistics aggregation",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/overwatch/validate",
			Description: "Configuration validation",
			Methods:     []string{"POST"},
		},
	}

	resp := APIDiscoveryResponse{
		Endpoints:   endpoints,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleDNSSECRoot handles GET /api/v1/dnssec - returns available DNSSEC endpoints.
// Note: This only handles the exact /api/v1/dnssec path for discovery.
// The actual DNSSEC operations are handled by DNSSECHandlers.HandleDNSSEC.
func (h *DiscoveryHandlers) HandleDNSSECRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Only show /api/v1/dnssec, not sub-paths
	if r.URL.Path != "/api/v1/dnssec" && r.URL.Path != "/api/v1/dnssec/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	endpoints := []APIEndpoint{
		{
			Path:        "/api/v1/dnssec/status",
			Description: "DNSSEC enablement status",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/dnssec/keys",
			Description: "DNSSEC key management",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/dnssec/ds",
			Description: "DS record retrieval",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/dnssec/sync",
			Description: "Key synchronization",
			Methods:     []string{"POST"},
		},
	}

	resp := APIDiscoveryResponse{
		Endpoints:   endpoints,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}
