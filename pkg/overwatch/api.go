// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIHandlers provides HTTP handlers for Overwatch-specific endpoints.
type APIHandlers struct {
	registry  *Registry
	validator *Validator
}

// NewAPIHandlers creates new Overwatch API handlers.
func NewAPIHandlers(registry *Registry, validator *Validator) *APIHandlers {
	return &APIHandlers{
		registry:  registry,
		validator: validator,
	}
}

// BackendResponse represents a backend in API responses.
type BackendResponse struct {
	Service           string     `json:"service"`
	Address           string     `json:"address"`
	Port              int        `json:"port"`
	Weight            int        `json:"weight"`
	AgentID           string     `json:"agent_id"`
	Region            string     `json:"region"`
	EffectiveStatus   string     `json:"effective_status"`
	AgentHealthy      bool       `json:"agent_healthy"`
	AgentLastSeen     time.Time  `json:"agent_last_seen"`
	ValidationHealthy *bool      `json:"validation_healthy,omitempty"`
	ValidationCheck   *time.Time `json:"validation_last_check,omitempty"`
	ValidationError   string     `json:"validation_error,omitempty"`
	OverrideStatus    *bool      `json:"override_status,omitempty"`
	OverrideReason    string     `json:"override_reason,omitempty"`
	OverrideBy        string     `json:"override_by,omitempty"`
	OverrideAt        *time.Time `json:"override_at,omitempty"`
}

// BackendsResponse is the response for GET /api/v1/overwatch/backends.
type BackendsResponse struct {
	Backends    []BackendResponse `json:"backends"`
	GeneratedAt time.Time         `json:"generated_at"`
}

// OverrideRequest is the request body for POST /api/v1/overwatch/backends/{key}/override.
type OverrideRequest struct {
	Healthy bool   `json:"healthy"`
	Reason  string `json:"reason"`
}

// OverrideResponse is the response for override operations.
type OverrideResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// StatsResponse is the response for GET /api/v1/overwatch/stats.
type StatsResponse struct {
	TotalBackends       int       `json:"total_backends"`
	HealthyBackends     int       `json:"healthy_backends"`
	UnhealthyBackends   int       `json:"unhealthy_backends"`
	StaleBackends       int       `json:"stale_backends"`
	ActiveOverrides     int       `json:"active_overrides"`
	ValidationEnabled   bool      `json:"validation_enabled"`
	ValidatedBackends   int       `json:"validated_backends"`
	DisagreementCount   int       `json:"disagreement_count"`
	ActiveAgents        int       `json:"active_agents"`
	UniqueServices      int       `json:"unique_services"`
	BackendsByService   map[string]int `json:"backends_by_service"`
	BackendsByRegion    map[string]int `json:"backends_by_region"`
	GeneratedAt         time.Time `json:"generated_at"`
}

// ErrorResponse represents an API error.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// HandleBackends handles GET /api/v1/overwatch/backends
func (h *APIHandlers) HandleBackends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse query parameters for filtering
	service := r.URL.Query().Get("service")
	region := r.URL.Query().Get("region")
	status := r.URL.Query().Get("status")

	backends := h.registry.GetAllBackends()
	response := make([]BackendResponse, 0, len(backends))

	for _, b := range backends {
		// Apply filters
		if service != "" && b.Service != service {
			continue
		}
		if region != "" && b.Region != region {
			continue
		}
		if status != "" && string(b.EffectiveStatus) != status {
			continue
		}

		br := BackendResponse{
			Service:         b.Service,
			Address:         b.Address,
			Port:            b.Port,
			Weight:          b.Weight,
			AgentID:         b.AgentID,
			Region:          b.Region,
			EffectiveStatus: string(b.EffectiveStatus),
			AgentHealthy:    b.AgentHealthy,
			AgentLastSeen:   b.AgentLastSeen,
		}

		if b.ValidationHealthy != nil {
			br.ValidationHealthy = b.ValidationHealthy
			br.ValidationCheck = &b.ValidationLastCheck
			br.ValidationError = b.ValidationError
		}

		if b.OverrideStatus != nil {
			br.OverrideStatus = b.OverrideStatus
			br.OverrideReason = b.OverrideReason
			br.OverrideBy = b.OverrideBy
			br.OverrideAt = &b.OverrideAt
		}

		response = append(response, br)
	}

	writeJSON(w, http.StatusOK, BackendsResponse{
		Backends:    response,
		GeneratedAt: time.Now().UTC(),
	})
}

// HandleBackendOverride handles POST/DELETE /api/v1/overwatch/backends/{service}/{address}/{port}/override
func (h *APIHandlers) HandleBackendOverride(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/v1/overwatch/backends/{service}/{address}/{port}/override
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/overwatch/backends/"), "/")
	if len(parts) < 4 || parts[3] != "override" {
		writeError(w, http.StatusBadRequest, "invalid path: expected /api/v1/overwatch/backends/{service}/{address}/{port}/override")
		return
	}

	service := parts[0]
	address := parts[1]
	port, err := strconv.Atoi(parts[2])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid port")
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.setOverride(w, r, service, address, port)
	case http.MethodDelete:
		h.clearOverride(w, r, service, address, port)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// setOverride sets a manual override for a backend.
func (h *APIHandlers) setOverride(w http.ResponseWriter, r *http.Request, service, address string, port int) {
	var req OverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get the user from headers (for audit trail)
	user := r.Header.Get("X-User")
	if user == "" {
		user = "api"
	}

	if err := h.registry.SetOverride(service, address, port, req.Healthy, req.Reason, user); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	RecordOverrideOperation("set")

	status := "unhealthy"
	if req.Healthy {
		status = "healthy"
	}

	writeJSON(w, http.StatusOK, OverrideResponse{
		Success: true,
		Message: "Override set: backend marked as " + status,
	})
}

// clearOverride clears a manual override for a backend.
func (h *APIHandlers) clearOverride(w http.ResponseWriter, r *http.Request, service, address string, port int) {
	if err := h.registry.ClearOverride(service, address, port); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	RecordOverrideOperation("clear")

	writeJSON(w, http.StatusOK, OverrideResponse{
		Success: true,
		Message: "Override cleared",
	})
}

// HandleStats handles GET /api/v1/overwatch/stats
func (h *APIHandlers) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	backends := h.registry.GetAllBackends()

	stats := StatsResponse{
		TotalBackends:      len(backends),
		ValidationEnabled:  h.validator != nil && h.validator.IsRunning(),
		BackendsByService:  make(map[string]int),
		BackendsByRegion:   make(map[string]int),
		GeneratedAt:        time.Now().UTC(),
	}

	agentIDs := make(map[string]bool)
	services := make(map[string]bool)

	for _, b := range backends {
		// Count by status
		switch b.EffectiveStatus {
		case StatusHealthy:
			stats.HealthyBackends++
		case StatusUnhealthy:
			stats.UnhealthyBackends++
		case StatusStale:
			stats.StaleBackends++
		}

		// Count overrides
		if b.OverrideStatus != nil {
			stats.ActiveOverrides++
		}

		// Count validated
		if b.ValidationHealthy != nil {
			stats.ValidatedBackends++
			// Check for disagreement
			if b.AgentHealthy != *b.ValidationHealthy {
				stats.DisagreementCount++
			}
		}

		// Count unique agents and services
		agentIDs[b.AgentID] = true
		services[b.Service] = true

		// Count by service
		stats.BackendsByService[b.Service]++

		// Count by region
		stats.BackendsByRegion[b.Region]++
	}

	stats.ActiveAgents = len(agentIDs)
	stats.UniqueServices = len(services)

	writeJSON(w, http.StatusOK, stats)
}

// HandleValidate handles POST /api/v1/overwatch/validate
// Triggers immediate validation of all or specific backends.
func (h *APIHandlers) HandleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.validator == nil {
		writeError(w, http.StatusServiceUnavailable, "validator not available")
		return
	}

	// Check for specific backend in query params
	service := r.URL.Query().Get("service")
	address := r.URL.Query().Get("address")
	portStr := r.URL.Query().Get("port")

	if service != "" && address != "" && portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid port")
			return
		}

		if err := h.validator.ValidateBackend(service, address, port); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "Validation triggered for specific backend",
		})
		return
	}

	// Validate all backends
	h.validator.ValidateNow()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Validation triggered for all backends",
	})
}

// RegisterRoutes registers Overwatch API routes with an HTTP mux.
func (h *APIHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/overwatch/backends", h.HandleBackends)
	mux.HandleFunc("/api/v1/overwatch/backends/", h.HandleBackendOverride)
	mux.HandleFunc("/api/v1/overwatch/stats", h.HandleStats)
	mux.HandleFunc("/api/v1/overwatch/validate", h.HandleValidate)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		return
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
