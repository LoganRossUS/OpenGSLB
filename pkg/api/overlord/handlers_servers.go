// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overlord

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// handleServers handles GET /api/servers and POST /api/servers
func (h *Handlers) handleServers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listServers(w, r)
	case http.MethodPost:
		h.createServer(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// listServers handles GET /api/servers
func (h *Handlers) listServers(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for filtering
	region := r.URL.Query().Get("region")
	service := r.URL.Query().Get("service")
	status := r.URL.Query().Get("status")

	servers := make([]Backend, 0)

	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		backends := registry.GetAllBackends()
		for _, b := range backends {
			// Apply filters
			if region != "" && b.Region != region {
				continue
			}
			if service != "" && b.Service != service {
				continue
			}
			if status != "" && string(b.EffectiveStatus) != status {
				continue
			}

			server := backendToServer(b)
			servers = append(servers, server)
		}
	} else {
		// Fallback: use config-based servers
		cfg := h.dataProvider.GetConfig()
		if cfg != nil {
			for _, r := range cfg.Regions {
				if region != "" && r.Name != region {
					continue
				}
				for _, s := range r.Servers {
					server := Backend{
						ID:              fmt.Sprintf("%s:%d", s.Address, s.Port),
						Address:         s.Address,
						Port:            s.Port,
						Weight:          s.Weight,
						Region:          r.Name,
						EffectiveStatus: "healthy",
						AgentHealthy:    true,
						HealthCheck: HealthCheckConfig{
							Enabled:  true,
							Type:     r.HealthCheck.Type,
							Path:     r.HealthCheck.Path,
							Interval: int(r.HealthCheck.Interval.Seconds()),
							Timeout:  int(r.HealthCheck.Timeout.Seconds()),
						},
					}
					servers = append(servers, server)
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, ServersResponse{Servers: servers})
}

// createServer handles POST /api/servers
func (h *Handlers) createServer(w http.ResponseWriter, r *http.Request) {
	var req ServerCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Validate required fields
	if req.Service == "" {
		writeError(w, http.StatusBadRequest, "service is required", "MISSING_FIELD")
		return
	}
	if req.Address == "" {
		writeError(w, http.StatusBadRequest, "address is required", "MISSING_FIELD")
		return
	}
	if req.Port == 0 {
		writeError(w, http.StatusBadRequest, "port is required", "MISSING_FIELD")
		return
	}
	if req.Region == "" {
		writeError(w, http.StatusBadRequest, "region is required", "MISSING_FIELD")
		return
	}

	// Set default weight if not provided
	weight := req.Weight
	if weight == 0 {
		weight = 100
	}

	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		// Register the backend
		agentID := req.AgentID
		if agentID == "" {
			agentID = "api-created"
		}

		err := registry.Register(agentID, req.Region, req.Service, req.Address, req.Port, weight, true)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "REGISTER_FAILED")
			return
		}
	}

	now := time.Now()
	server := Backend{
		ID:              fmt.Sprintf("%s:%d", req.Address, req.Port),
		Service:         req.Service,
		Address:         req.Address,
		Port:            req.Port,
		Weight:          weight,
		Region:          req.Region,
		AgentID:         req.AgentID,
		EffectiveStatus: "healthy",
		AgentHealthy:    true,
		AgentLastSeen:   &now,
		HealthCheck:     req.HealthCheck,
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryServer, server.ID,
		fmt.Sprintf("Created server %s for service %s", server.ID, req.Service),
		map[string]interface{}{
			"service": req.Service,
			"address": req.Address,
			"port":    req.Port,
			"region":  req.Region,
			"weight":  weight,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusCreated, ServerResponse{Server: server})
}

// handleServerByID handles GET, PUT, DELETE /api/servers/:id
func (h *Handlers) handleServerByID(w http.ResponseWriter, r *http.Request) {
	// Parse server ID from path
	// ID format: address:port or service:address:port
	id, subPath := parseSubPath(r.URL.Path, "/api/servers/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "server ID is required", "MISSING_PARAM")
		return
	}

	// Handle sub-paths
	if subPath == "health-check" {
		h.handleServerHealthCheck(w, r, id)
		return
	}
	if subPath == "health-status" {
		h.handleServerHealthStatus(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getServer(w, r, id)
	case http.MethodPut:
		h.updateServer(w, r, id)
	case http.MethodDelete:
		h.deleteServer(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getServer handles GET /api/servers/:id
func (h *Handlers) getServer(w http.ResponseWriter, r *http.Request, id string) {
	registry := h.dataProvider.GetBackendRegistry()
	if registry == nil {
		writeError(w, http.StatusServiceUnavailable, "backend registry not available", "REGISTRY_UNAVAILABLE")
		return
	}

	// Parse ID (format: address:port or service:address:port)
	service, address, port := parseServerID(id)

	if service != "" {
		backend, found := registry.GetBackend(service, address, port)
		if found {
			writeJSON(w, http.StatusOK, ServerResponse{Server: backendToServer(backend)})
			return
		}
	} else {
		// Search by address:port across all services
		backends := registry.GetAllBackends()
		for _, b := range backends {
			if b.Address == address && b.Port == port {
				writeJSON(w, http.StatusOK, ServerResponse{Server: backendToServer(b)})
				return
			}
		}
	}

	writeError(w, http.StatusNotFound, "server not found", "SERVER_NOT_FOUND")
}

// updateServer handles PUT /api/servers/:id
func (h *Handlers) updateServer(w http.ResponseWriter, r *http.Request, id string) {
	var req ServerUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	registry := h.dataProvider.GetBackendRegistry()
	if registry == nil {
		writeError(w, http.StatusServiceUnavailable, "backend registry not available", "REGISTRY_UNAVAILABLE")
		return
	}

	// Parse ID
	service, address, port := parseServerID(id)

	// Find the backend
	var backend *overwatch.Backend
	if service != "" {
		b, found := registry.GetBackend(service, address, port)
		if found {
			backend = b
		}
	} else {
		backends := registry.GetAllBackends()
		for _, b := range backends {
			if b.Address == address && b.Port == port {
				backend = b
				service = b.Service
				break
			}
		}
	}

	if backend == nil {
		writeError(w, http.StatusNotFound, "server not found", "SERVER_NOT_FOUND")
		return
	}

	// Apply updates by re-registering
	weight := backend.Weight
	if req.Weight > 0 {
		weight = req.Weight
	}
	region := backend.Region
	if req.Region != "" {
		region = req.Region
	}
	agentID := backend.AgentID
	if req.AgentID != "" {
		agentID = req.AgentID
	}
	if req.Service != "" {
		service = req.Service
	}

	err := registry.Register(agentID, region, service, address, port, weight, backend.AgentHealthy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "UPDATE_FAILED")
		return
	}

	// Get updated backend
	updatedBackend, _ := registry.GetBackend(service, address, port)
	server := backendToServer(updatedBackend)

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryServer, id,
		fmt.Sprintf("Updated server %s", id),
		map[string]interface{}{
			"weight": weight,
			"region": region,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, ServerResponse{Server: server})
}

// deleteServer handles DELETE /api/servers/:id
func (h *Handlers) deleteServer(w http.ResponseWriter, r *http.Request, id string) {
	registry := h.dataProvider.GetBackendRegistry()
	if registry == nil {
		writeError(w, http.StatusServiceUnavailable, "backend registry not available", "REGISTRY_UNAVAILABLE")
		return
	}

	// Parse ID
	service, address, port := parseServerID(id)

	// Find and delete the backend
	var found bool
	if service != "" {
		_, found = registry.GetBackend(service, address, port)
		if found {
			_ = registry.Deregister(service, address, port)
		}
	} else {
		backends := registry.GetAllBackends()
		for _, b := range backends {
			if b.Address == address && b.Port == port {
				_ = registry.Deregister(b.Service, address, port)
				found = true
				break
			}
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "server not found", "SERVER_NOT_FOUND")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionDelete, AuditCategoryServer, id,
		fmt.Sprintf("Deleted server %s", id),
		nil,
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// handleServerHealthCheck handles GET/PUT /api/servers/:id/health-check
func (h *Handlers) handleServerHealthCheck(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		h.getServerHealthCheck(w, r, id)
	case http.MethodPut:
		h.updateServerHealthCheck(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getServerHealthCheck handles GET /api/servers/:id/health-check
func (h *Handlers) getServerHealthCheck(w http.ResponseWriter, r *http.Request, id string) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Parse ID
	_, address, port := parseServerID(id)

	// Find server in config to get health check settings
	for _, region := range cfg.Regions {
		for _, server := range region.Servers {
			if server.Address == address && server.Port == port {
				hc := HealthCheckConfig{
					Enabled:            true,
					Type:               region.HealthCheck.Type,
					Path:               region.HealthCheck.Path,
					Interval:           int(region.HealthCheck.Interval.Seconds()),
					Timeout:            int(region.HealthCheck.Timeout.Seconds()),
					HealthyThreshold:   region.HealthCheck.SuccessThreshold,
					UnhealthyThreshold: region.HealthCheck.FailureThreshold,
				}
				writeJSON(w, http.StatusOK, map[string]HealthCheckConfig{"healthCheck": hc})
				return
			}
		}
	}

	// Return default health check if not found in config
	hc := HealthCheckConfig{
		Enabled:            true,
		Type:               "http",
		Path:               "/health",
		Interval:           30,
		Timeout:            5,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
	}
	writeJSON(w, http.StatusOK, map[string]HealthCheckConfig{"healthCheck": hc})
}

// updateServerHealthCheck handles PUT /api/servers/:id/health-check
func (h *Handlers) updateServerHealthCheck(w http.ResponseWriter, r *http.Request, id string) {
	var hc HealthCheckConfig
	if err := json.NewDecoder(r.Body).Decode(&hc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryHealth, id,
		fmt.Sprintf("Updated health check for server %s", id),
		map[string]interface{}{
			"type":     hc.Type,
			"path":     hc.Path,
			"interval": hc.Interval,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, map[string]HealthCheckConfig{"healthCheck": hc})
}

// handleServerHealthStatus handles POST /api/servers/:id/health-status
func (h *Handlers) handleServerHealthStatus(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	var req HealthStatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	registry := h.dataProvider.GetBackendRegistry()
	if registry == nil {
		writeError(w, http.StatusServiceUnavailable, "backend registry not available", "REGISTRY_UNAVAILABLE")
		return
	}

	// Parse ID
	service, address, port := parseServerID(id)

	// Find the backend
	var backend *overwatch.Backend
	if service != "" {
		b, found := registry.GetBackend(service, address, port)
		if found {
			backend = b
		}
	} else {
		backends := registry.GetAllBackends()
		for _, b := range backends {
			if b.Address == address && b.Port == port {
				backend = b
				service = b.Service
				break
			}
		}
	}

	if backend == nil {
		writeError(w, http.StatusNotFound, "server not found", "SERVER_NOT_FOUND")
		return
	}

	// Update validation result based on status
	healthy := req.Status == "healthy"
	validationErr := ""
	if !healthy {
		validationErr = "manually set to " + req.Status
	}

	err := registry.UpdateValidation(service, address, port, healthy, validationErr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "UPDATE_FAILED")
		return
	}

	// Get updated backend
	updatedBackend, _ := registry.GetBackend(service, address, port)
	server := backendToServer(updatedBackend)

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryHealth, id,
		fmt.Sprintf("Updated health status for server %s to %s", id, req.Status),
		map[string]interface{}{"status": req.Status},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, ServerResponse{Server: server})
}

// ============================================================================
// Helper Functions
// ============================================================================

// backendToServer converts an overwatch.Backend to a Backend response.
func backendToServer(b *overwatch.Backend) Backend {
	server := Backend{
		ID:              fmt.Sprintf("%s:%d", b.Address, b.Port),
		Service:         b.Service,
		Address:         b.Address,
		Port:            b.Port,
		Weight:          b.Weight,
		Region:          b.Region,
		AgentID:         b.AgentID,
		EffectiveStatus: string(b.EffectiveStatus),
		AgentHealthy:    b.AgentHealthy,
		SmoothedLatency: int(b.SmoothedLatency.Milliseconds()),
		LatencySamples:  b.LatencySamples,
	}

	if !b.AgentLastSeen.IsZero() {
		server.AgentLastSeen = &b.AgentLastSeen
	}
	if b.ValidationHealthy != nil {
		server.ValidationHealthy = b.ValidationHealthy
		if !b.ValidationLastCheck.IsZero() {
			server.ValidationLastCheck = &b.ValidationLastCheck
		}
		server.ValidationError = b.ValidationError
	}

	return server
}

// parseServerID parses a server ID into service, address, and port.
// ID format can be:
// - "address:port" -> service="", address=address, port=port
// - "service:address:port" -> service=service, address=address, port=port
func parseServerID(id string) (service string, address string, port int) {
	parts := strings.Split(id, ":")
	if len(parts) == 2 {
		// address:port format
		address = parts[0]
		port, _ = strconv.Atoi(parts[1])
	} else if len(parts) == 3 {
		// service:address:port format
		service = parts[0]
		address = parts[1]
		port, _ = strconv.Atoi(parts[2])
	} else if len(parts) > 3 {
		// Handle IPv6 addresses or complex formats
		// Assume last part is port
		port, _ = strconv.Atoi(parts[len(parts)-1])
		address = strings.Join(parts[:len(parts)-1], ":")
	}
	return
}
