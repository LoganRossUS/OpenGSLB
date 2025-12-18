// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"net/http"
	"runtime"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/version"
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

// VersionResponse is the response for GET /api/v1/version.
type VersionResponse struct {
	Version     string    `json:"version"`
	GoVersion   string    `json:"go_version"`
	Platform    string    `json:"platform"`
	GeneratedAt time.Time `json:"generated_at"`
}

// DiscoveryHandlers handles API discovery endpoints.
type DiscoveryHandlers struct{}

// NewDiscoveryHandlers creates a new DiscoveryHandlers instance.
func NewDiscoveryHandlers() *DiscoveryHandlers {
	return &DiscoveryHandlers{}
}

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
		{
			Path:        "/api/health",
			Description: "Simple health check endpoint",
			Methods:     []string{"GET"},
		},
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

	// List all available API endpoints for discoverability
	endpoints := []APIEndpoint{
		{
			Path:        "/api/v1/version",
			Description: "OpenGSLB version information",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/health",
			Description: "Health status endpoints",
		},
		{
			Path:        "/api/v1/ready",
			Description: "Readiness probe",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/live",
			Description: "Liveness probe",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/domains",
			Description: "Domain management",
			Methods:     []string{"GET", "POST"},
		},
		{
			Path:        "/api/v1/servers",
			Description: "Server management",
			Methods:     []string{"GET", "POST"},
		},
		{
			Path:        "/api/v1/regions",
			Description: "Region management",
			Methods:     []string{"GET", "POST"},
		},
		{
			Path:        "/api/v1/nodes",
			Description: "Node management (Overwatch and Agent nodes)",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/gossip",
			Description: "Gossip protocol management",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/audit-logs",
			Description: "Audit log retrieval",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/metrics",
			Description: "System metrics",
			Methods:     []string{"GET"},
		},
		{
			Path:        "/api/v1/config",
			Description: "Configuration management",
			Methods:     []string{"GET", "PUT"},
		},
		{
			Path:        "/api/v1/preferences",
			Description: "User preferences",
			Methods:     []string{"GET", "PUT"},
		},
		{
			Path:        "/api/v1/routing",
			Description: "Routing algorithms and decisions",
			Methods:     []string{"GET", "POST"},
		},
		{
			Path:        "/api/v1/overrides",
			Description: "Health check overrides",
			Methods:     []string{"GET", "PUT", "DELETE"},
		},
		{
			Path:        "/api/v1/dnssec",
			Description: "DNSSEC management",
		},
		{
			Path:        "/api/v1/geo",
			Description: "Geolocation management",
		},
		{
			Path:        "/api/v1/overwatch",
			Description: "Overwatch-specific endpoints",
		},
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

// HandleVersion handles GET /api/v1/version - returns OpenGSLB version information.
func (h *DiscoveryHandlers) HandleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	resp := VersionResponse{
		Version:     version.GetVersion(),
		GoVersion:   runtime.Version(),
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}
