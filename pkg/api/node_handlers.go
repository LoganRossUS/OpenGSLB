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

// NodeProvider defines the interface for node management operations.
type NodeProvider interface {
	// ListOverwatchNodes returns all Overwatch nodes.
	ListOverwatchNodes() []OverwatchNode
	// GetOverwatchNode returns an Overwatch node by ID.
	GetOverwatchNode(id string) (*OverwatchNode, error)

	// ListAgentNodes returns all Agent nodes.
	ListAgentNodes() []AgentNode
	// GetAgentNode returns an Agent node by ID.
	GetAgentNode(id string) (*AgentNode, error)
	// RegisterAgentNode registers a new Agent node.
	RegisterAgentNode(node AgentNode) error
	// DeregisterAgentNode removes an Agent node.
	DeregisterAgentNode(id string) error

	// GetAgentCertificate retrieves the certificate for an Agent.
	GetAgentCertificate(id string) (*AgentCertificate, error)
	// RevokeAgentCertificate revokes an Agent's certificate.
	RevokeAgentCertificate(id string) error
	// ReissueAgentCertificate issues a new certificate for an Agent.
	ReissueAgentCertificate(id string) (*AgentCertificate, error)
}

// OverwatchNode represents an Overwatch DNS server node.
type OverwatchNode struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Address       string            `json:"address"`
	Port          int               `json:"port"`
	APIPort       int               `json:"api_port"`
	Region        string            `json:"region"`
	Status        string            `json:"status"` // online, offline, degraded
	Version       string            `json:"version"`
	Uptime        int64             `json:"uptime_seconds"`
	LastSeen      time.Time         `json:"last_seen"`
	QueriesTotal  int64             `json:"queries_total"`
	QueriesPerSec float64           `json:"queries_per_sec"`
	DNSSECEnabled bool              `json:"dnssec_enabled"`
	Features      []string          `json:"features,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// AgentNode represents an Agent health-check node.
type AgentNode struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Address           string            `json:"address"`
	Port              int               `json:"port"`
	Region            string            `json:"region"`
	Status            string            `json:"status"` // online, offline, pending
	Version           string            `json:"version"`
	Uptime            int64             `json:"uptime_seconds"`
	LastSeen          time.Time         `json:"last_seen"`
	ChecksTotal       int64             `json:"checks_total"`
	ChecksPerSec      float64           `json:"checks_per_sec"`
	ActiveChecks      int               `json:"active_checks"`
	TargetCount       int               `json:"target_count"`
	CertificateExpiry *time.Time        `json:"certificate_expiry,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	RegisteredAt      time.Time         `json:"registered_at"`
}

// AgentCertificate represents certificate information for an Agent.
type AgentCertificate struct {
	AgentID     string     `json:"agent_id"`
	Serial      string     `json:"serial"`
	NotBefore   time.Time  `json:"not_before"`
	NotAfter    time.Time  `json:"not_after"`
	Fingerprint string     `json:"fingerprint"`
	Status      string     `json:"status"` // valid, expiring, expired, revoked
	IssuedAt    time.Time  `json:"issued_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// OverwatchNodeListResponse is the response for GET /api/v1/nodes/overwatch.
type OverwatchNodeListResponse struct {
	Nodes       []OverwatchNode `json:"nodes"`
	Total       int             `json:"total"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// OverwatchNodeResponse is the response for single Overwatch node operations.
type OverwatchNodeResponse struct {
	Node OverwatchNode `json:"node"`
}

// AgentNodeListResponse is the response for GET /api/v1/nodes/agent.
type AgentNodeListResponse struct {
	Nodes       []AgentNode `json:"nodes"`
	Total       int         `json:"total"`
	GeneratedAt time.Time   `json:"generated_at"`
}

// AgentNodeResponse is the response for single Agent node operations.
type AgentNodeResponse struct {
	Node AgentNode `json:"node"`
}

// AgentCertificateResponse is the response for certificate operations.
type AgentCertificateResponse struct {
	Certificate AgentCertificate `json:"certificate"`
}

// AgentRegisterRequest is the request body for registering an Agent.
type AgentRegisterRequest struct {
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Port     int               `json:"port"`
	Region   string            `json:"region"`
	Version  string            `json:"version"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NodeHandlers provides HTTP handlers for node API endpoints.
type NodeHandlers struct {
	provider NodeProvider
	logger   *slog.Logger
}

// NewNodeHandlers creates a new NodeHandlers instance.
func NewNodeHandlers(provider NodeProvider, logger *slog.Logger) *NodeHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &NodeHandlers{
		provider: provider,
		logger:   logger,
	}
}

// HandleNodes routes /api/v1/nodes requests based on HTTP method and path.
func (h *NodeHandlers) HandleNodes(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes")
	path = strings.TrimPrefix(path, "/")

	parts := strings.SplitN(path, "/", 3)
	if len(parts) == 0 || parts[0] == "" {
		h.writeError(w, http.StatusNotFound, "specify node type: /api/v1/nodes/overwatch or /api/v1/nodes/agent")
		return
	}

	nodeType := parts[0]
	var nodeID string
	var subResource string
	if len(parts) > 1 {
		nodeID = parts[1]
	}
	if len(parts) > 2 {
		subResource = parts[2]
	}

	switch nodeType {
	case "overwatch":
		h.handleOverwatchNodes(w, r, nodeID)
	case "agent":
		h.handleAgentNodes(w, r, nodeID, subResource)
	default:
		h.writeError(w, http.StatusNotFound, "unknown node type: "+nodeType)
	}
}

// handleOverwatchNodes handles /api/v1/nodes/overwatch requests.
func (h *NodeHandlers) handleOverwatchNodes(w http.ResponseWriter, r *http.Request, nodeID string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "node provider not configured")
		return
	}

	// List all Overwatch nodes
	if nodeID == "" {
		if r.Method != http.MethodGet {
			h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		nodes := h.provider.ListOverwatchNodes()

		// Apply filters
		statusFilter := r.URL.Query().Get("status")
		regionFilter := r.URL.Query().Get("region")

		if statusFilter != "" || regionFilter != "" {
			filtered := make([]OverwatchNode, 0, len(nodes))
			for _, n := range nodes {
				if statusFilter != "" && n.Status != statusFilter {
					continue
				}
				if regionFilter != "" && n.Region != regionFilter {
					continue
				}
				filtered = append(filtered, n)
			}
			nodes = filtered
		}

		resp := OverwatchNodeListResponse{
			Nodes:       nodes,
			Total:       len(nodes),
			GeneratedAt: time.Now().UTC(),
		}
		h.writeJSON(w, http.StatusOK, resp)
		return
	}

	// Get single Overwatch node
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	node, err := h.provider.GetOverwatchNode(nodeID)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "overwatch node not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, OverwatchNodeResponse{Node: *node})
}

// handleAgentNodes handles /api/v1/nodes/agent requests.
func (h *NodeHandlers) handleAgentNodes(w http.ResponseWriter, r *http.Request, nodeID, subResource string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "node provider not configured")
		return
	}

	// List all Agent nodes or register new Agent
	if nodeID == "" {
		switch r.Method {
		case http.MethodGet:
			h.listAgentNodes(w, r)
		case http.MethodPost:
			h.registerAgentNode(w, r)
		default:
			h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Handle sub-resources
	if subResource != "" {
		switch subResource {
		case "certificate":
			h.handleAgentCertificate(w, r, nodeID)
		default:
			h.writeError(w, http.StatusNotFound, "unknown sub-resource: "+subResource)
		}
		return
	}

	// Single Agent node operations
	switch r.Method {
	case http.MethodGet:
		h.getAgentNode(w, r, nodeID)
	case http.MethodDelete:
		h.deregisterAgentNode(w, r, nodeID)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listAgentNodes handles GET /api/v1/nodes/agent.
func (h *NodeHandlers) listAgentNodes(w http.ResponseWriter, r *http.Request) {
	nodes := h.provider.ListAgentNodes()

	// Apply filters
	statusFilter := r.URL.Query().Get("status")
	regionFilter := r.URL.Query().Get("region")

	if statusFilter != "" || regionFilter != "" {
		filtered := make([]AgentNode, 0, len(nodes))
		for _, n := range nodes {
			if statusFilter != "" && n.Status != statusFilter {
				continue
			}
			if regionFilter != "" && n.Region != regionFilter {
				continue
			}
			filtered = append(filtered, n)
		}
		nodes = filtered
	}

	resp := AgentNodeListResponse{
		Nodes:       nodes,
		Total:       len(nodes),
		GeneratedAt: time.Now().UTC(),
	}
	h.writeJSON(w, http.StatusOK, resp)
}

// getAgentNode handles GET /api/v1/nodes/agent/{id}.
func (h *NodeHandlers) getAgentNode(w http.ResponseWriter, r *http.Request, id string) {
	node, err := h.provider.GetAgentNode(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "agent node not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, AgentNodeResponse{Node: *node})
}

// registerAgentNode handles POST /api/v1/nodes/agent.
func (h *NodeHandlers) registerAgentNode(w http.ResponseWriter, r *http.Request) {
	var req AgentRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.Name == "" {
		h.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Address == "" {
		h.writeError(w, http.StatusBadRequest, "address is required")
		return
	}

	node := AgentNode{
		Name:         req.Name,
		Address:      req.Address,
		Port:         req.Port,
		Region:       req.Region,
		Version:      req.Version,
		Status:       "pending",
		Metadata:     req.Metadata,
		RegisteredAt: time.Now().UTC(),
		LastSeen:     time.Now().UTC(),
	}

	// Set defaults
	if node.Port == 0 {
		node.Port = 8443
	}

	if err := h.provider.RegisterAgentNode(node); err != nil {
		h.logger.Error("failed to register agent node", "name", req.Name, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to register agent: "+err.Error())
		return
	}

	h.logger.Info("agent node registered", "name", req.Name, "address", req.Address)
	h.writeJSON(w, http.StatusCreated, AgentNodeResponse{Node: node})
}

// deregisterAgentNode handles DELETE /api/v1/nodes/agent/{id}.
func (h *NodeHandlers) deregisterAgentNode(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.provider.DeregisterAgentNode(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, http.StatusNotFound, "agent node not found")
			return
		}
		h.logger.Error("failed to deregister agent node", "id", id, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to deregister agent: "+err.Error())
		return
	}

	h.logger.Info("agent node deregistered", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// handleAgentCertificate handles /api/v1/nodes/agent/{id}/certificate requests.
func (h *NodeHandlers) handleAgentCertificate(w http.ResponseWriter, r *http.Request, agentID string) {
	switch r.Method {
	case http.MethodGet:
		cert, err := h.provider.GetAgentCertificate(agentID)
		if err != nil {
			h.writeError(w, http.StatusNotFound, "certificate not found: "+err.Error())
			return
		}
		h.writeJSON(w, http.StatusOK, AgentCertificateResponse{Certificate: *cert})

	case http.MethodDelete:
		// Revoke certificate
		if err := h.provider.RevokeAgentCertificate(agentID); err != nil {
			h.logger.Error("failed to revoke certificate", "agent_id", agentID, "error", err)
			h.writeError(w, http.StatusInternalServerError, "failed to revoke certificate: "+err.Error())
			return
		}
		h.logger.Info("agent certificate revoked", "agent_id", agentID)
		w.WriteHeader(http.StatusNoContent)

	case http.MethodPost:
		// Reissue certificate
		cert, err := h.provider.ReissueAgentCertificate(agentID)
		if err != nil {
			h.logger.Error("failed to reissue certificate", "agent_id", agentID, "error", err)
			h.writeError(w, http.StatusInternalServerError, "failed to reissue certificate: "+err.Error())
			return
		}
		h.logger.Info("agent certificate reissued", "agent_id", agentID)
		h.writeJSON(w, http.StatusCreated, AgentCertificateResponse{Certificate: *cert})

	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// writeJSON writes a JSON response with the given status code.
func (h *NodeHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *NodeHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
