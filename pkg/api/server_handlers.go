// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// BackendServerProvider defines the interface for server management operations.
type BackendServerProvider interface {
	// ListServers returns all configured backend servers.
	ListServers() []BackendServer
	// GetServer returns a server by ID.
	GetServer(id string) (*BackendServer, error)
	// CreateServer creates a new server.
	CreateServer(server BackendServer) error
	// UpdateServer updates an existing server.
	UpdateServer(id string, server BackendServer) error
	// DeleteServer deletes a server by ID.
	DeleteServer(id string) error
	// GetServerHealthCheck returns the health check configuration for a server.
	GetServerHealthCheck(id string) (*ServerHealthCheck, error)
	// UpdateServerHealthCheck updates the health check configuration for a server.
	UpdateServerHealthCheck(id string, config ServerHealthCheck) error
}

// BackendServer represents a backend server configuration.
type BackendServer struct {
	ID          string             `json:"id,omitempty"`
	Name        string             `json:"name"`
	Address     string             `json:"address"`
	Port        int                `json:"port"`
	Protocol    string             `json:"protocol"`
	Weight      int                `json:"weight"`
	Priority    int                `json:"priority"`
	Region      string             `json:"region"`
	Enabled     bool               `json:"enabled"`
	Healthy     bool               `json:"healthy"`
	Description string             `json:"description,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Metadata    map[string]string  `json:"metadata,omitempty"`
	CreatedAt   time.Time          `json:"created_at,omitempty"`
	UpdatedAt   time.Time          `json:"updated_at,omitempty"`
	HealthCheck *ServerHealthCheck `json:"health_check,omitempty"`
	Status      *ServerStatus      `json:"status,omitempty"`
}

// ServerHealthCheck represents health check configuration for a server.
type ServerHealthCheck struct {
	Enabled            bool              `json:"enabled"`
	Type               string            `json:"type"` // tcp, http, https, dns
	Path               string            `json:"path,omitempty"`
	Interval           time.Duration     `json:"interval"`
	Timeout            time.Duration     `json:"timeout"`
	HealthyThreshold   int               `json:"healthy_threshold"`
	UnhealthyThreshold int               `json:"unhealthy_threshold"`
	ExpectedStatus     int               `json:"expected_status,omitempty"`
	ExpectedBody       string            `json:"expected_body,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
}

// ServerStatus represents the current status of a server.
type ServerStatus struct {
	Healthy              bool       `json:"healthy"`
	LastCheck            *time.Time `json:"last_check,omitempty"`
	LastHealthy          *time.Time `json:"last_healthy,omitempty"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	ConsecutiveSuccesses int        `json:"consecutive_successes"`
	LastError            string     `json:"last_error,omitempty"`
	ResponseTime         int64      `json:"response_time_ms,omitempty"`
}

// ServerListResponse is the response for GET /api/v1/servers.
type ServerListResponse struct {
	Servers     []BackendServer `json:"servers"`
	Total       int             `json:"total"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// ServerResponse is the response for single server operations.
type ServerResponse struct {
	Server BackendServer `json:"server"`
}

// ServerHealthCheckResponse is the response for GET /api/v1/servers/{id}/health-check.
type ServerHealthCheckResponse struct {
	ServerID    string            `json:"server_id"`
	HealthCheck ServerHealthCheck `json:"health_check"`
}

// ServerCreateRequest is the request body for creating a server.
type ServerCreateRequest struct {
	Name        string             `json:"name"`
	Address     string             `json:"address"`
	Port        int                `json:"port"`
	Protocol    string             `json:"protocol"`
	Weight      int                `json:"weight"`
	Priority    int                `json:"priority"`
	Region      string             `json:"region"`
	Enabled     bool               `json:"enabled"`
	Description string             `json:"description,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Metadata    map[string]string  `json:"metadata,omitempty"`
	HealthCheck *ServerHealthCheck `json:"health_check,omitempty"`
}

// ServerUpdateRequest is the request body for updating a server.
type ServerUpdateRequest struct {
	Name        *string            `json:"name,omitempty"`
	Address     *string            `json:"address,omitempty"`
	Port        *int               `json:"port,omitempty"`
	Protocol    *string            `json:"protocol,omitempty"`
	Weight      *int               `json:"weight,omitempty"`
	Priority    *int               `json:"priority,omitempty"`
	Region      *string            `json:"region,omitempty"`
	Enabled     *bool              `json:"enabled,omitempty"`
	Description *string            `json:"description,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Metadata    map[string]string  `json:"metadata,omitempty"`
	HealthCheck *ServerHealthCheck `json:"health_check,omitempty"`
}

// ServerHandlers provides HTTP handlers for server API endpoints.
type ServerHandlers struct {
	provider BackendServerProvider
	logger   *slog.Logger
}

// NewServerHandlers creates a new ServerHandlers instance.
func NewServerHandlers(provider BackendServerProvider, logger *slog.Logger) *ServerHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &ServerHandlers{
		provider: provider,
		logger:   logger,
	}
}

// HandleServers routes /api/v1/servers requests based on HTTP method and path.
func (h *ServerHandlers) HandleServers(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/servers")
	path = strings.TrimPrefix(path, "/")

	// If path is empty, it's a list/create request
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			h.listServers(w, r)
		case http.MethodPost:
			h.createServer(w, r)
		default:
			h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Parse server ID and optional sub-resource
	parts := strings.SplitN(path, "/", 2)
	serverID := parts[0]

	// Check for sub-resources
	if len(parts) == 2 {
		subResource := parts[1]
		switch subResource {
		case "health-check":
			h.handleServerHealthCheck(w, r, serverID)
		default:
			h.writeError(w, http.StatusNotFound, "endpoint not found")
		}
		return
	}

	// Single server operations
	switch r.Method {
	case http.MethodGet:
		h.getServer(w, r, serverID)
	case http.MethodPut, http.MethodPatch:
		h.updateServer(w, r, serverID)
	case http.MethodDelete:
		h.deleteServer(w, r, serverID)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listServers handles GET /api/v1/servers.
func (h *ServerHandlers) listServers(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "server provider not configured")
		return
	}

	servers := h.provider.ListServers()

	// Apply filters from query parameters
	regionFilter := r.URL.Query().Get("region")
	enabledFilter := r.URL.Query().Get("enabled")
	healthyFilter := r.URL.Query().Get("healthy")

	if regionFilter != "" || enabledFilter != "" || healthyFilter != "" {
		filtered := make([]BackendServer, 0, len(servers))
		for _, s := range servers {
			if regionFilter != "" && s.Region != regionFilter {
				continue
			}
			if enabledFilter == "true" && !s.Enabled {
				continue
			}
			if enabledFilter == "false" && s.Enabled {
				continue
			}
			if healthyFilter == "true" && !s.Healthy {
				continue
			}
			if healthyFilter == "false" && s.Healthy {
				continue
			}
			filtered = append(filtered, s)
		}
		servers = filtered
	}

	resp := ServerListResponse{
		Servers:     servers,
		Total:       len(servers),
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// getServer handles GET /api/v1/servers/{id}.
func (h *ServerHandlers) getServer(w http.ResponseWriter, r *http.Request, id string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "server provider not configured")
		return
	}

	server, err := h.provider.GetServer(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "server not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, ServerResponse{Server: *server})
}

// createServer handles POST /api/v1/servers.
func (h *ServerHandlers) createServer(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "server provider not configured")
		return
	}

	var req ServerCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.Address == "" {
		h.writeError(w, http.StatusBadRequest, "address is required")
		return
	}
	if req.Port == 0 {
		h.writeError(w, http.StatusBadRequest, "port is required")
		return
	}

	server := BackendServer{
		Name:        req.Name,
		Address:     req.Address,
		Port:        req.Port,
		Protocol:    req.Protocol,
		Weight:      req.Weight,
		Priority:    req.Priority,
		Region:      req.Region,
		Enabled:     req.Enabled,
		Description: req.Description,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		HealthCheck: req.HealthCheck,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Set defaults
	if server.Protocol == "" {
		server.Protocol = "tcp"
	}
	if server.Weight == 0 {
		server.Weight = 1
	}

	if err := h.provider.CreateServer(server); err != nil {
		h.logger.Error("failed to create server", "address", req.Address, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to create server: "+err.Error())
		return
	}

	h.logger.Info("server created", "address", req.Address, "port", req.Port)
	h.writeJSON(w, http.StatusCreated, ServerResponse{Server: server})
}

// updateServer handles PUT/PATCH /api/v1/servers/{id}.
func (h *ServerHandlers) updateServer(w http.ResponseWriter, r *http.Request, id string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "server provider not configured")
		return
	}

	// Get existing server
	existing, err := h.provider.GetServer(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "server not found: "+err.Error())
		return
	}

	var req ServerUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Apply updates
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Address != nil {
		existing.Address = *req.Address
	}
	if req.Port != nil {
		existing.Port = *req.Port
	}
	if req.Protocol != nil {
		existing.Protocol = *req.Protocol
	}
	if req.Weight != nil {
		existing.Weight = *req.Weight
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.Region != nil {
		existing.Region = *req.Region
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Tags != nil {
		existing.Tags = req.Tags
	}
	if req.Metadata != nil {
		existing.Metadata = req.Metadata
	}
	if req.HealthCheck != nil {
		existing.HealthCheck = req.HealthCheck
	}
	existing.UpdatedAt = time.Now().UTC()

	if err := h.provider.UpdateServer(id, *existing); err != nil {
		h.logger.Error("failed to update server", "id", id, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to update server: "+err.Error())
		return
	}

	h.logger.Info("server updated", "id", id)
	h.writeJSON(w, http.StatusOK, ServerResponse{Server: *existing})
}

// deleteServer handles DELETE /api/v1/servers/{id}.
func (h *ServerHandlers) deleteServer(w http.ResponseWriter, r *http.Request, id string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "server provider not configured")
		return
	}

	if err := h.provider.DeleteServer(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, http.StatusNotFound, "server not found")
			return
		}
		h.logger.Error("failed to delete server", "id", id, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to delete server: "+err.Error())
		return
	}

	h.logger.Info("server deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// handleServerHealthCheck handles /api/v1/servers/{id}/health-check.
func (h *ServerHandlers) handleServerHealthCheck(w http.ResponseWriter, r *http.Request, serverID string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "server provider not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		config, err := h.provider.GetServerHealthCheck(serverID)
		if err != nil {
			h.writeError(w, http.StatusNotFound, "server not found: "+err.Error())
			return
		}
		h.writeJSON(w, http.StatusOK, ServerHealthCheckResponse{
			ServerID:    serverID,
			HealthCheck: *config,
		})

	case http.MethodPut, http.MethodPatch:
		var config ServerHealthCheck
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		if err := h.provider.UpdateServerHealthCheck(serverID, config); err != nil {
			h.logger.Error("failed to update health check", "server_id", serverID, "error", err)
			h.writeError(w, http.StatusInternalServerError, "failed to update health check: "+err.Error())
			return
		}

		h.logger.Info("server health check updated", "server_id", serverID)
		h.writeJSON(w, http.StatusOK, ServerHealthCheckResponse{
			ServerID:    serverID,
			HealthCheck: config,
		})

	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// writeJSON writes a JSON response with the given status code.
func (h *ServerHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *ServerHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
