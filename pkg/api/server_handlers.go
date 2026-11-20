// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// BackendServerProvider defines the interface for server management operations.
//
// A server is one backend of one service (domain). The same objects are the
// backends of GET /api/v1/domains/{name}/backends: there is one model, kept
// in the overwatch backend registry, whatever endpoint created the object.
type BackendServerProvider interface {
	// ListServers returns all servers: from the configuration file, from
	// agents and created through the API.
	ListServers() []BackendServer
	// GetServer returns a server by ID.
	GetServer(id string) (*BackendServer, error)
	// CreateServer creates a new server and returns it as stored.
	CreateServer(spec ServerSpec) (*BackendServer, error)
	// UpdateServer applies the set fields to a server created through the API.
	UpdateServer(id string, update ServerUpdate) (*BackendServer, error)
	// DeleteServer deletes a server by ID.
	DeleteServer(id string) error
	// GetServerHealthCheck returns the health check in effect for a server.
	GetServerHealthCheck(id string) (*ServerHealthCheck, error)
	// UpdateServerHealthCheck changes the health check of a server created
	// through the API. With replace, fields that are not set go back to the
	// node-wide setting; without it they keep their value.
	UpdateServerHealthCheck(id string, check ServerHealthCheckRequest, replace bool) (*ServerHealthCheck, error)
}

// BackendServer is a backend server as the API returns it, both under
// /api/v1/servers and under /api/v1/domains/{name}/backends.
type BackendServer struct {
	// ID is "{service}:{address}:{port}".
	ID string `json:"id"`
	// Service is the name of the domain this server answers for. It is the
	// link between a server and its domain.
	Service string `json:"service"`
	// Name is a deprecated alias of Service.
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	// Protocol is deprecated and always "tcp"; see HealthCheck.Type.
	Protocol string `json:"protocol"`
	Weight   int    `json:"weight"`
	// Priority is stored and returned; no routing algorithm reads it.
	Priority int    `json:"priority"`
	Region   string `json:"region"`
	// Enabled is the administrative state. A disabled server is not answered in DNS.
	Enabled bool `json:"enabled"`
	// Healthy is true when the effective status of the server is healthy.
	Healthy bool `json:"healthy"`
	// Source says who owns the server: static (configuration file), agent or api.
	Source string `json:"source"`
	// Metadata carries status details as strings. Deprecated in favor of the
	// typed fields; kept for existing clients.
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	// LastCheck is the time of the last health check (Go zero time: never).
	LastCheck time.Time `json:"last_check"`
	// HealthCheck is the health check in effect for the server.
	HealthCheck ServerHealthCheck `json:"health_check"`
}

// ServerHealthCheck is the health check in effect for a server, as the API
// returns it. Every field holds the value actually used: the server's own
// setting where one was made through the API, the node-wide setting otherwise.
type ServerHealthCheck struct {
	// Enabled is false when the overwatch does not probe the server.
	Enabled bool `json:"enabled"`
	// Type is http, https or tcp.
	Type string `json:"type"`
	// Path is the request path of an http or https check.
	Path string `json:"path,omitempty"`
	// Host is the Host header and TLS server name of an http or https check.
	Host string `json:"host,omitempty"`
	// Interval is the time between two checks, for example "30s".
	Interval Duration `json:"interval"`
	// Timeout is the time one check may take, for example "5s".
	Timeout            Duration `json:"timeout"`
	HealthyThreshold   int      `json:"healthy_threshold"`
	UnhealthyThreshold int      `json:"unhealthy_threshold"`
}

// ServerHealthCheckRequest is a health check as a client sends it. Every field
// is optional. Fields that the overwatch cannot apply are refused, not ignored.
type ServerHealthCheckRequest struct {
	Enabled            *bool     `json:"enabled,omitempty"`
	Type               *string   `json:"type,omitempty"`
	Path               *string   `json:"path,omitempty"`
	Host               *string   `json:"host,omitempty"`
	Interval           *Duration `json:"interval,omitempty"`
	Timeout            *Duration `json:"timeout,omitempty"`
	HealthyThreshold   *int      `json:"healthy_threshold,omitempty"`
	UnhealthyThreshold *int      `json:"unhealthy_threshold,omitempty"`
}

// UnmarshalJSON decodes a health check strictly. A field that is not part of
// the contract (for example expected_status) is an error: a health check that
// silently does less than what was asked for is worse than a refused request.
func (r *ServerHealthCheckRequest) UnmarshalJSON(data []byte) error {
	type plain ServerHealthCheckRequest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var p plain
	if err := dec.Decode(&p); err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	*r = ServerHealthCheckRequest(p)
	return nil
}

// ServerSpec describes a server to create.
type ServerSpec struct {
	Service     string
	Address     string
	Port        int
	Weight      int
	Region      string
	Priority    int
	Enabled     bool
	HealthCheck *ServerHealthCheckRequest
}

// ServerUpdate holds the changes to a server. A nil field is left as it is.
type ServerUpdate struct {
	Weight      *int
	Region      *string
	Priority    *int
	Enabled     *bool
	HealthCheck *ServerHealthCheckRequest
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
	// Service is the domain the server belongs to.
	Service string `json:"service"`
	// Name is a deprecated alias of Service, used when Service is empty.
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight"`
	// Priority is stored and returned; no routing algorithm reads it.
	Priority int    `json:"priority"`
	Region   string `json:"region"`
	// Enabled defaults to true when it is left out.
	Enabled *bool `json:"enabled,omitempty"`
	// Metadata: only the deprecated key "service" is read, as an alias of Service.
	Metadata    map[string]string         `json:"metadata,omitempty"`
	HealthCheck *ServerHealthCheckRequest `json:"health_check,omitempty"`
}

// service returns the domain the request names, in order of preference:
// service, metadata.service (deprecated), name (deprecated).
func (r *ServerCreateRequest) service() string {
	if r.Service != "" {
		return r.Service
	}
	if svc := r.Metadata["service"]; svc != "" {
		return svc
	}
	return r.Name
}

// ServerUpdateRequest is the request body for updating a server. Service,
// address and port identify the server: they are accepted only when they
// repeat the current value.
type ServerUpdateRequest struct {
	Service     *string                   `json:"service,omitempty"`
	Address     *string                   `json:"address,omitempty"`
	Port        *int                      `json:"port,omitempty"`
	Weight      *int                      `json:"weight,omitempty"`
	Priority    *int                      `json:"priority,omitempty"`
	Region      *string                   `json:"region,omitempty"`
	Enabled     *bool                     `json:"enabled,omitempty"`
	HealthCheck *ServerHealthCheckRequest `json:"health_check,omitempty"`
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
		h.writeProviderError(w, err, "get server")
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
	service := req.service()
	if service == "" {
		h.writeError(w, http.StatusBadRequest, "service is required: the name of the domain this server belongs to")
		return
	}

	server, err := h.provider.CreateServer(ServerSpec{
		Service:     service,
		Address:     req.Address,
		Port:        req.Port,
		Weight:      req.Weight,
		Region:      req.Region,
		Priority:    req.Priority,
		Enabled:     req.Enabled == nil || *req.Enabled,
		HealthCheck: req.HealthCheck,
	})
	if err != nil {
		h.writeProviderError(w, err, "create server")
		return
	}

	h.logger.Info("server created", "id", server.ID)
	h.writeJSON(w, http.StatusCreated, ServerResponse{Server: *server})
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
		h.writeProviderError(w, err, "update server")
		return
	}

	var req ServerUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// The identity of a server cannot be changed.
	if (req.Service != nil && *req.Service != existing.Service) ||
		(req.Address != nil && *req.Address != existing.Address) ||
		(req.Port != nil && *req.Port != existing.Port) {
		h.writeError(w, http.StatusBadRequest,
			"service, address and port identify a server and cannot be changed; delete the server and create a new one")
		return
	}

	server, err := h.provider.UpdateServer(id, ServerUpdate{
		Weight:      req.Weight,
		Region:      req.Region,
		Priority:    req.Priority,
		Enabled:     req.Enabled,
		HealthCheck: req.HealthCheck,
	})
	if err != nil {
		h.writeProviderError(w, err, "update server")
		return
	}

	h.logger.Info("server updated", "id", id)
	h.writeJSON(w, http.StatusOK, ServerResponse{Server: *server})
}

// deleteServer handles DELETE /api/v1/servers/{id}.
func (h *ServerHandlers) deleteServer(w http.ResponseWriter, r *http.Request, id string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "server provider not configured")
		return
	}

	if err := h.provider.DeleteServer(id); err != nil {
		h.writeProviderError(w, err, "delete server")
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
		check, err := h.provider.GetServerHealthCheck(serverID)
		if err != nil {
			h.writeProviderError(w, err, "get health check")
			return
		}
		h.writeJSON(w, http.StatusOK, ServerHealthCheckResponse{
			ServerID:    serverID,
			HealthCheck: *check,
		})

	case http.MethodPut, http.MethodPatch:
		var req ServerHealthCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		// PUT replaces the server's health check, PATCH changes the given fields.
		check, err := h.provider.UpdateServerHealthCheck(serverID, req, r.Method == http.MethodPut)
		if err != nil {
			h.writeProviderError(w, err, "update health check")
			return
		}

		h.logger.Info("server health check updated", "server_id", serverID)
		h.writeJSON(w, http.StatusOK, ServerHealthCheckResponse{
			ServerID:    serverID,
			HealthCheck: *check,
		})

	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// writeProviderError answers with the status a provider error maps to.
func (h *ServerHandlers) writeProviderError(w http.ResponseWriter, err error, action string) {
	status, message := errorStatus(err, action)
	if status >= http.StatusInternalServerError {
		h.logger.Error("server API request failed", "action", action, "status", status, "error", err)
	}
	h.writeError(w, status, message)
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
