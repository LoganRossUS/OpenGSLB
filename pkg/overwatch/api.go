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

// ClusterStatusProvider provides information about the gossip cluster.
type ClusterStatusProvider interface {
	// NumMembers returns the number of cluster members.
	NumMembers() int
	// Members returns cluster member information.
	GetClusterMembers() []ClusterMember
	// GetLocalNodeID returns the local node's ID.
	GetLocalNodeID() string
	// GetUptimeSeconds returns how long the node has been running.
	GetUptimeSeconds() int64
}

// ClusterMember represents a node in the gossip cluster.
type ClusterMember struct {
	NodeID   string    `json:"node_id"`
	Role     string    `json:"role"`
	Region   string    `json:"region,omitempty"`
	Address  string    `json:"address"`
	LastSeen time.Time `json:"last_seen"`
}

// APIHandlers provides HTTP handlers for Overwatch-specific endpoints.
type APIHandlers struct {
	registry        *Registry
	validator       *Validator
	agentAuth       *AgentAuth
	latencyTable    *LearnedLatencyTable
	clusterProvider ClusterStatusProvider
}

// NewAPIHandlers creates new Overwatch API handlers.
func NewAPIHandlers(registry *Registry, validator *Validator) *APIHandlers {
	return &APIHandlers{
		registry:  registry,
		validator: validator,
	}
}

// SetAgentAuth sets the agent authenticator for certificate management endpoints.
func (h *APIHandlers) SetAgentAuth(auth *AgentAuth) {
	h.agentAuth = auth
}

// SetLatencyTable sets the learned latency table for latency API endpoints.
func (h *APIHandlers) SetLatencyTable(table *LearnedLatencyTable) {
	h.latencyTable = table
}

// SetClusterProvider sets the cluster status provider for cluster status endpoint.
func (h *APIHandlers) SetClusterProvider(provider ClusterStatusProvider) {
	h.clusterProvider = provider
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
	TotalBackends     int            `json:"total_backends"`
	HealthyBackends   int            `json:"healthy_backends"`
	UnhealthyBackends int            `json:"unhealthy_backends"`
	StaleBackends     int            `json:"stale_backends"`
	ActiveOverrides   int            `json:"active_overrides"`
	ValidationEnabled bool           `json:"validation_enabled"`
	ValidatedBackends int            `json:"validated_backends"`
	DisagreementCount int            `json:"disagreement_count"`
	ActiveAgents      int            `json:"active_agents"`
	UniqueServices    int            `json:"unique_services"`
	BackendsByService map[string]int `json:"backends_by_service"`
	BackendsByRegion  map[string]int `json:"backends_by_region"`
	GeneratedAt       time.Time      `json:"generated_at"`
}

// ErrorResponse represents an API error.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// ClusterStatusResponse is the response for GET /api/v1/cluster/status.
// This endpoint is used by deployment scripts to verify cluster health.
type ClusterStatusResponse struct {
	ClusterHealthy bool                  `json:"cluster_healthy"`
	Overwatch      ClusterNodeStatus     `json:"overwatch"`
	Agents         []ClusterAgentStatus  `json:"agents"`
	ExpectedAgents int                   `json:"expected_agents,omitempty"`
	HealthyAgents  int                   `json:"healthy_agents"`
	GossipMembers  int                   `json:"gossip_members"`
	BackendSummary ClusterBackendSummary `json:"backend_summary"`
	GeneratedAt    time.Time             `json:"generated_at"`
}

// ClusterNodeStatus represents the Overwatch node's status.
type ClusterNodeStatus struct {
	NodeID        string `json:"node_id"`
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// ClusterAgentStatus represents an agent's status in the cluster.
type ClusterAgentStatus struct {
	AgentID      string    `json:"agent_id"`
	Region       string    `json:"region"`
	Status       string    `json:"status"`
	LastSeen     time.Time `json:"last_seen"`
	BackendCount int       `json:"backend_count"`
}

// ClusterBackendSummary provides a summary of backend health across the cluster.
type ClusterBackendSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
	Stale     int `json:"stale"`
}

// AgentCertResponse represents an agent certificate in API responses.
type AgentCertResponse struct {
	AgentID        string    `json:"agent_id"`
	Fingerprint    string    `json:"fingerprint"`
	Region         string    `json:"region"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`
	NotAfter       time.Time `json:"not_after"`
	Revoked        bool      `json:"revoked"`
	RevokedAt      time.Time `json:"revoked_at,omitempty"`
	RevokedReason  string    `json:"revoked_reason,omitempty"`
	ExpiresInHours int       `json:"expires_in_hours"`
}

// AgentCertsResponse is the response for GET /api/v1/overwatch/agents.
type AgentCertsResponse struct {
	Agents      []AgentCertResponse `json:"agents"`
	Total       int                 `json:"total"`
	Revoked     int                 `json:"revoked"`
	Expiring    int                 `json:"expiring"`
	GeneratedAt time.Time           `json:"generated_at"`
}

// RevokeRequest is the request body for POST /api/v1/overwatch/agents/{id}/revoke.
type RevokeRequest struct {
	Reason string `json:"reason"`
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
		TotalBackends:     len(backends),
		ValidationEnabled: h.validator != nil && h.validator.IsRunning(),
		BackendsByService: make(map[string]int),
		BackendsByRegion:  make(map[string]int),
		GeneratedAt:       time.Now().UTC(),
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

// HandleAgents handles GET /api/v1/overwatch/agents
// Returns all pinned agent certificates.
func (h *APIHandlers) HandleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.agentAuth == nil {
		writeError(w, http.StatusServiceUnavailable, "agent auth not configured")
		return
	}

	certs, err := h.agentAuth.ListPinnedCertificates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := AgentCertsResponse{
		Agents:      make([]AgentCertResponse, 0, len(certs)),
		GeneratedAt: time.Now().UTC(),
	}

	now := time.Now()
	expiryThreshold := 30 * 24 * time.Hour // 30 days

	for _, cert := range certs {
		expiresIn := cert.NotAfter.Sub(now)
		agentResp := AgentCertResponse{
			AgentID:        cert.AgentID,
			Fingerprint:    cert.Fingerprint,
			Region:         cert.Region,
			FirstSeen:      cert.FirstSeen,
			LastSeen:       cert.LastSeen,
			NotAfter:       cert.NotAfter,
			Revoked:        cert.Revoked,
			RevokedAt:      cert.RevokedAt,
			RevokedReason:  cert.RevokedReason,
			ExpiresInHours: int(expiresIn.Hours()),
		}
		response.Agents = append(response.Agents, agentResp)
		response.Total++

		if cert.Revoked {
			response.Revoked++
		} else if expiresIn < expiryThreshold {
			response.Expiring++
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// HandleAgent handles GET/DELETE /api/v1/overwatch/agents/{agent_id}
func (h *APIHandlers) HandleAgent(w http.ResponseWriter, r *http.Request) {
	// Parse agent ID from path
	agentID := strings.TrimPrefix(r.URL.Path, "/api/v1/overwatch/agents/")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	// Remove any trailing path components (e.g., "/revoke")
	if idx := strings.Index(agentID, "/"); idx != -1 {
		agentID = agentID[:idx]
	}

	if h.agentAuth == nil {
		writeError(w, http.StatusServiceUnavailable, "agent auth not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getAgent(w, r, agentID)
	case http.MethodDelete:
		h.deleteAgent(w, r, agentID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// getAgent returns a specific agent's certificate info.
func (h *APIHandlers) getAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	cert, err := h.agentAuth.GetPinnedCertificate(r.Context(), agentID)
	if err != nil {
		if err == ErrAgentNotFound {
			writeError(w, http.StatusNotFound, "agent not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	now := time.Now()
	expiresIn := cert.NotAfter.Sub(now)

	writeJSON(w, http.StatusOK, AgentCertResponse{
		AgentID:        cert.AgentID,
		Fingerprint:    cert.Fingerprint,
		Region:         cert.Region,
		FirstSeen:      cert.FirstSeen,
		LastSeen:       cert.LastSeen,
		NotAfter:       cert.NotAfter,
		Revoked:        cert.Revoked,
		RevokedAt:      cert.RevokedAt,
		RevokedReason:  cert.RevokedReason,
		ExpiresInHours: int(expiresIn.Hours()),
	})
}

// deleteAgent deletes (unpins) an agent's certificate.
func (h *APIHandlers) deleteAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	if err := h.agentAuth.DeletePinnedCertificate(r.Context(), agentID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Agent certificate deleted",
	})
}

// HandleAgentRevoke handles POST /api/v1/overwatch/agents/{agent_id}/revoke
func (h *APIHandlers) HandleAgentRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse agent ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/overwatch/agents/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "revoke" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	agentID := parts[0]

	if h.agentAuth == nil {
		writeError(w, http.StatusServiceUnavailable, "agent auth not configured")
		return
	}

	var req RevokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Reason == "" {
		req.Reason = "revoked via API"
	}

	if err := h.agentAuth.RevokeCertificate(r.Context(), agentID, req.Reason); err != nil {
		if err == ErrAgentNotFound {
			writeError(w, http.StatusNotFound, "agent not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Agent certificate revoked",
	})
}

// HandleAgentsExpiring handles GET /api/v1/overwatch/agents/expiring
func (h *APIHandlers) HandleAgentsExpiring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.agentAuth == nil {
		writeError(w, http.StatusServiceUnavailable, "agent auth not configured")
		return
	}

	// Parse threshold from query parameter (default 30 days)
	thresholdStr := r.URL.Query().Get("threshold_days")
	thresholdDays := 30
	if thresholdStr != "" {
		if d, err := strconv.Atoi(thresholdStr); err == nil && d > 0 {
			thresholdDays = d
		}
	}

	threshold := time.Duration(thresholdDays) * 24 * time.Hour
	certs, err := h.agentAuth.GetExpiringCertificates(r.Context(), threshold)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]AgentCertResponse, 0, len(certs))
	now := time.Now()

	for _, cert := range certs {
		expiresIn := cert.NotAfter.Sub(now)
		response = append(response, AgentCertResponse{
			AgentID:        cert.AgentID,
			Fingerprint:    cert.Fingerprint,
			Region:         cert.Region,
			FirstSeen:      cert.FirstSeen,
			LastSeen:       cert.LastSeen,
			NotAfter:       cert.NotAfter,
			Revoked:        cert.Revoked,
			ExpiresInHours: int(expiresIn.Hours()),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"expiring":       response,
		"count":          len(response),
		"threshold_days": thresholdDays,
		"generated_at":   time.Now().UTC(),
	})
}

// HandleLatencyTable handles GET/POST /api/v1/overwatch/latency
// GET: Returns the learned latency data from agents (ADR-017).
// POST: Injects test latency data for integration testing.
func (h *APIHandlers) HandleLatencyTable(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleLatencyTableGet(w, r)
	case http.MethodPost:
		h.handleLatencyTablePost(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *APIHandlers) handleLatencyTableGet(w http.ResponseWriter, r *http.Request) {
	if h.latencyTable == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"entries":      []LatencyEntry{},
			"count":        0,
			"subnet_count": 0,
			"generated_at": time.Now().UTC(),
		})
		return
	}

	entries := h.latencyTable.GetAllEntries()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"entries":      entries,
		"count":        len(entries),
		"subnet_count": h.latencyTable.SubnetCount(),
		"generated_at": time.Now().UTC(),
	})
}

// latencyInjectRequest is the request body for POST /api/v1/overwatch/latency
type latencyInjectRequest struct {
	Subnet    string `json:"subnet"`     // e.g., "10.0.0.0/24"
	Backend   string `json:"backend"`    // e.g., "web.test.local"
	Region    string `json:"region"`     // e.g., "eu-west"
	LatencyMs int64  `json:"latency_ms"` // e.g., 80
	Samples   uint64 `json:"samples"`    // e.g., 10
}

func (h *APIHandlers) handleLatencyTablePost(w http.ResponseWriter, r *http.Request) {
	if h.latencyTable == nil {
		writeError(w, http.StatusServiceUnavailable, "latency table not initialized")
		return
	}

	var req latencyInjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate required fields
	if req.Subnet == "" || req.Backend == "" || req.Region == "" {
		writeError(w, http.StatusBadRequest, "subnet, backend, and region are required")
		return
	}

	if err := h.latencyTable.InjectTestData(req.Subnet, req.Backend, req.Region, req.LatencyMs, req.Samples); err != nil {
		writeError(w, http.StatusBadRequest, "invalid subnet: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "latency data injected",
		"subnet":  req.Subnet,
		"backend": req.Backend,
		"region":  req.Region,
	})
}

// HandleClusterStatus handles GET /api/v1/cluster/status
// Returns the overall cluster health status for deployment validation.
// Query parameter: ?expected_agents=N to specify expected agent count for health check.
func (h *APIHandlers) HandleClusterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse expected agents from query parameter
	expectedAgents := 0
	if exp := r.URL.Query().Get("expected_agents"); exp != "" {
		if n, err := strconv.Atoi(exp); err == nil && n > 0 {
			expectedAgents = n
		}
	}

	// Get backend information from registry
	backends := h.registry.GetAllBackends()

	// Build agent status from backends
	agentMap := make(map[string]*ClusterAgentStatus)
	backendSummary := ClusterBackendSummary{
		Total: len(backends),
	}

	for _, b := range backends {
		// Count backend status
		switch b.EffectiveStatus {
		case StatusHealthy:
			backendSummary.Healthy++
		case StatusUnhealthy:
			backendSummary.Unhealthy++
		case StatusStale:
			backendSummary.Stale++
		}

		// Aggregate agent info
		if _, exists := agentMap[b.AgentID]; !exists {
			status := "healthy"
			if !b.AgentHealthy {
				status = "unhealthy"
			}
			if time.Since(b.AgentLastSeen) > 30*time.Second {
				status = "stale"
			}
			agentMap[b.AgentID] = &ClusterAgentStatus{
				AgentID:      b.AgentID,
				Region:       b.Region,
				Status:       status,
				LastSeen:     b.AgentLastSeen,
				BackendCount: 0,
			}
		}
		agentMap[b.AgentID].BackendCount++
	}

	// Convert agent map to slice
	agents := make([]ClusterAgentStatus, 0, len(agentMap))
	healthyAgents := 0
	for _, agent := range agentMap {
		agents = append(agents, *agent)
		if agent.Status == "healthy" {
			healthyAgents++
		}
	}

	// Build overwatch status
	overwatchStatus := ClusterNodeStatus{
		Status: "healthy",
	}

	gossipMembers := 0
	if h.clusterProvider != nil {
		overwatchStatus.NodeID = h.clusterProvider.GetLocalNodeID()
		overwatchStatus.UptimeSeconds = h.clusterProvider.GetUptimeSeconds()
		gossipMembers = h.clusterProvider.NumMembers()
	}

	// Determine cluster health
	clusterHealthy := true
	if expectedAgents > 0 && healthyAgents < expectedAgents {
		clusterHealthy = false
	}
	if backendSummary.Total > 0 && backendSummary.Healthy == 0 {
		clusterHealthy = false
	}

	response := ClusterStatusResponse{
		ClusterHealthy: clusterHealthy,
		Overwatch:      overwatchStatus,
		Agents:         agents,
		ExpectedAgents: expectedAgents,
		HealthyAgents:  healthyAgents,
		GossipMembers:  gossipMembers,
		BackendSummary: backendSummary,
		GeneratedAt:    time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, response)
}

// RegisterRoutes registers Overwatch API routes with an HTTP mux.
func (h *APIHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/overwatch/backends", h.HandleBackends)
	mux.HandleFunc("/api/v1/overwatch/backends/", h.HandleBackendOverride)
	mux.HandleFunc("/api/v1/overwatch/stats", h.HandleStats)
	mux.HandleFunc("/api/v1/overwatch/validate", h.HandleValidate)

	// Agent certificate management endpoints
	mux.HandleFunc("/api/v1/overwatch/agents", h.HandleAgents)
	mux.HandleFunc("/api/v1/overwatch/agents/expiring", h.HandleAgentsExpiring)
	mux.HandleFunc("/api/v1/overwatch/agents/", h.HandleAgentRoute)

	// Latency learning endpoints (ADR-017)
	mux.HandleFunc("/api/v1/overwatch/latency", h.HandleLatencyTable)

	// Cluster status endpoint (for deployment validation)
	mux.HandleFunc("/api/v1/cluster/status", h.HandleClusterStatus)
}

// HandleAgentRoute routes agent requests based on path.
func (h *APIHandlers) HandleAgentRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/overwatch/agents/")

	// Check for /expiring which is handled separately
	if path == "expiring" {
		h.HandleAgentsExpiring(w, r)
		return
	}

	// Check for /{agent_id}/revoke
	if strings.HasSuffix(path, "/revoke") {
		h.HandleAgentRevoke(w, r)
		return
	}

	// Otherwise it's GET/DELETE /{agent_id}
	h.HandleAgent(w, r)
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
