// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/version"
)

// ============================================================================
// Overwatch Node Handlers
// ============================================================================

// handleOverwatchNodes handles GET /api/nodes/overwatch and POST /api/nodes/overwatch
func (h *Handlers) handleOverwatchNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listOverwatchNodes(w, r)
	case http.MethodPost:
		h.createOverwatchNode(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// listOverwatchNodes handles GET /api/nodes/overwatch
func (h *Handlers) listOverwatchNodes(w http.ResponseWriter, r *http.Request) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Get local node info
	nodes := make([]OverwatchNode, 0, 1)

	// Add this node
	backendsManaged := 0
	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		backendsManaged = registry.BackendCount()
	}

	localNode := OverwatchNode{
		NodeID:          cfg.Overwatch.Identity.NodeID,
		Region:          cfg.Overwatch.Identity.Region,
		Status:          "healthy",
		LastSeen:        time.Now(),
		Version:         version.Version,
		Address:         cfg.API.Address,
		BackendsManaged: backendsManaged,
		Uptime:          86400, // Placeholder
	}

	// Get gossip peers for DNS queries per second
	// This would come from actual metrics in a full implementation

	nodes = append(nodes, localNode)

	// In a multi-node setup, we would query other nodes via gossip
	// For now, just return the local node

	writeJSON(w, http.StatusOK, OverwatchNodesResponse{Nodes: nodes})
}

// createOverwatchNode handles POST /api/nodes/overwatch
func (h *Handlers) createOverwatchNode(w http.ResponseWriter, r *http.Request) {
	var node OverwatchNode
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if node.NodeID == "" {
		writeError(w, http.StatusBadRequest, "nodeId is required", "MISSING_FIELD")
		return
	}
	if node.Region == "" {
		writeError(w, http.StatusBadRequest, "region is required", "MISSING_FIELD")
		return
	}
	if node.Address == "" {
		writeError(w, http.StatusBadRequest, "address is required", "MISSING_FIELD")
		return
	}

	node.Status = "healthy"
	node.LastSeen = time.Now()
	node.Version = version.Version

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryConfig, node.NodeID,
		fmt.Sprintf("Created overwatch node %s", node.NodeID),
		map[string]interface{}{
			"region":  node.Region,
			"address": node.Address,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusCreated, OverwatchNodeResponse{Node: node})
}

// handleOverwatchNodeByID handles GET, PUT /api/nodes/overwatch/:id
func (h *Handlers) handleOverwatchNodeByID(w http.ResponseWriter, r *http.Request) {
	id := parsePathParam(r.URL.Path, "/api/nodes/overwatch/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "node ID is required", "MISSING_PARAM")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getOverwatchNode(w, r, id)
	case http.MethodPut:
		h.updateOverwatchNode(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getOverwatchNode handles GET /api/nodes/overwatch/:id
func (h *Handlers) getOverwatchNode(w http.ResponseWriter, r *http.Request, id string) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Check if this is the local node
	if cfg.Overwatch.Identity.NodeID == id {
		backendsManaged := 0
		registry := h.dataProvider.GetBackendRegistry()
		if registry != nil {
			backendsManaged = registry.BackendCount()
		}

		node := OverwatchNode{
			NodeID:          cfg.Overwatch.Identity.NodeID,
			Region:          cfg.Overwatch.Identity.Region,
			Status:          "healthy",
			LastSeen:        time.Now(),
			Version:         version.Version,
			Address:         cfg.API.Address,
			BackendsManaged: backendsManaged,
			Uptime:          86400,
		}

		writeJSON(w, http.StatusOK, OverwatchNodeResponse{Node: node})
		return
	}

	writeError(w, http.StatusNotFound, "overwatch node not found", "NODE_NOT_FOUND")
}

// updateOverwatchNode handles PUT /api/nodes/overwatch/:id
func (h *Handlers) updateOverwatchNode(w http.ResponseWriter, r *http.Request, id string) {
	var node OverwatchNode
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Check if this is the local node
	if cfg.Overwatch.Identity.NodeID != id {
		writeError(w, http.StatusNotFound, "overwatch node not found", "NODE_NOT_FOUND")
		return
	}

	// Update with current values
	node.NodeID = id
	if node.Region == "" {
		node.Region = cfg.Overwatch.Identity.Region
	}
	node.LastSeen = time.Now()
	node.Version = version.Version

	backendsManaged := 0
	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		backendsManaged = registry.BackendCount()
	}
	node.BackendsManaged = backendsManaged

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryConfig, id,
		fmt.Sprintf("Updated overwatch node %s", id),
		map[string]interface{}{
			"region": node.Region,
			"status": node.Status,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, OverwatchNodeResponse{Node: node})
}

// ============================================================================
// Agent Node Handlers
// ============================================================================

// handleAgentNodes handles GET /api/nodes/agent and POST /api/nodes/agent
func (h *Handlers) handleAgentNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listAgentNodes(w, r)
	case http.MethodPost:
		h.createAgentNode(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// listAgentNodes handles GET /api/nodes/agent
func (h *Handlers) listAgentNodes(w http.ResponseWriter, r *http.Request) {
	agents := make([]AgentNode, 0)

	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		// Build agent list from registered backends
		agentMap := make(map[string]*AgentNode)
		backends := registry.GetAllBackends()

		for _, b := range backends {
			if b.AgentID == "" {
				continue
			}

			agent, exists := agentMap[b.AgentID]
			if !exists {
				status := "healthy"
				switch b.EffectiveStatus {
				case "stale":
					status = "stale"
				case "unhealthy":
					status = "suspect"
				}

				agent = &AgentNode{
					AgentID:           b.AgentID,
					Region:            b.Region,
					Service:           b.Service,
					Status:            status,
					LastSeen:          b.AgentLastSeen,
					BackendsReporting: 0,
				}
				agentMap[b.AgentID] = agent
			}
			agent.BackendsReporting++
		}

		for _, agent := range agentMap {
			agents = append(agents, *agent)
		}
	}

	writeJSON(w, http.StatusOK, AgentNodesResponse{Agents: agents})
}

// createAgentNode handles POST /api/nodes/agent
func (h *Handlers) createAgentNode(w http.ResponseWriter, r *http.Request) {
	var agent AgentNode
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if agent.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agentId is required", "MISSING_FIELD")
		return
	}
	if agent.Region == "" {
		writeError(w, http.StatusBadRequest, "region is required", "MISSING_FIELD")
		return
	}

	agent.Status = "healthy"
	agent.LastSeen = time.Now()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryAgent, agent.AgentID,
		fmt.Sprintf("Created agent node %s", agent.AgentID),
		map[string]interface{}{
			"region":  agent.Region,
			"service": agent.Service,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusCreated, AgentNodeResponse{Agent: agent})
}

// handleAgentNodeByID handles GET, PUT, DELETE /api/nodes/agent/:id
func (h *Handlers) handleAgentNodeByID(w http.ResponseWriter, r *http.Request) {
	id, subPath := parseSubPath(r.URL.Path, "/api/nodes/agent/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "agent ID is required", "MISSING_PARAM")
		return
	}

	// Handle certificate sub-path
	if subPath == "certificate" {
		h.handleAgentCertificate(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getAgentNode(w, r, id)
	case http.MethodPut:
		h.updateAgentNode(w, r, id)
	case http.MethodDelete:
		h.deleteAgentNode(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getAgentNode handles GET /api/nodes/agent/:id
func (h *Handlers) getAgentNode(w http.ResponseWriter, r *http.Request, id string) {
	registry := h.dataProvider.GetBackendRegistry()
	if registry == nil {
		writeError(w, http.StatusServiceUnavailable, "backend registry not available", "REGISTRY_UNAVAILABLE")
		return
	}

	// Build agent info from registered backends
	backends := registry.GetAllBackends()
	var agent *AgentNode

	for _, b := range backends {
		if b.AgentID == id {
			if agent == nil {
				status := "healthy"
				switch b.EffectiveStatus {
				case "stale":
					status = "stale"
				case "unhealthy":
					status = "suspect"
				}

				agent = &AgentNode{
					AgentID:           b.AgentID,
					Region:            b.Region,
					Service:           b.Service,
					Status:            status,
					LastSeen:          b.AgentLastSeen,
					BackendsReporting: 0,
				}
			}
			agent.BackendsReporting++
		}
	}

	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found", "AGENT_NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, AgentNodeResponse{Agent: *agent})
}

// updateAgentNode handles PUT /api/nodes/agent/:id
func (h *Handlers) updateAgentNode(w http.ResponseWriter, r *http.Request, id string) {
	var agent AgentNode
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	registry := h.dataProvider.GetBackendRegistry()
	if registry == nil {
		writeError(w, http.StatusServiceUnavailable, "backend registry not available", "REGISTRY_UNAVAILABLE")
		return
	}

	// Check if agent exists
	backends := registry.GetAllBackends()
	found := false
	for _, b := range backends {
		if b.AgentID == id {
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "agent not found", "AGENT_NOT_FOUND")
		return
	}

	agent.AgentID = id
	agent.LastSeen = time.Now()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryAgent, id,
		fmt.Sprintf("Updated agent node %s", id),
		map[string]interface{}{
			"region":  agent.Region,
			"status":  agent.Status,
			"service": agent.Service,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, AgentNodeResponse{Agent: agent})
}

// deleteAgentNode handles DELETE /api/nodes/agent/:id
func (h *Handlers) deleteAgentNode(w http.ResponseWriter, r *http.Request, id string) {
	registry := h.dataProvider.GetBackendRegistry()
	if registry == nil {
		writeError(w, http.StatusServiceUnavailable, "backend registry not available", "REGISTRY_UNAVAILABLE")
		return
	}

	// Deregister all backends for this agent
	backends := registry.GetAllBackends()
	found := false
	for _, b := range backends {
		if b.AgentID == id {
			found = true
			_ = registry.Deregister(b.Service, b.Address, b.Port)
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "agent not found", "AGENT_NOT_FOUND")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionDelete, AuditCategoryAgent, id,
		fmt.Sprintf("Deleted agent node %s", id),
		nil,
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// handleAgentCertificate handles GET, PUT /api/nodes/agent/:id/certificate
func (h *Handlers) handleAgentCertificate(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		h.getAgentCertificate(w, r, id)
	case http.MethodPut:
		h.updateAgentCertificate(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getAgentCertificate handles GET /api/nodes/agent/:id/certificate
func (h *Handlers) getAgentCertificate(w http.ResponseWriter, r *http.Request, id string) {
	// In a full implementation, this would retrieve certificate info from agent auth
	// For now, return placeholder data

	now := time.Now()
	notAfter := now.Add(365 * 24 * time.Hour)

	cert := Certificate{
		Fingerprint:  "SHA256:placeholder",
		Subject:      fmt.Sprintf("CN=%s,O=OpenGSLB", id),
		Issuer:       "CN=OpenGSLB-CA,O=OpenGSLB",
		SerialNumber: "01:23:45:67:89",
		NotBefore:    &now,
		NotAfter:     &notAfter,
		Revoked:      false,
	}

	writeJSON(w, http.StatusOK, CertificateResponse{Certificate: cert})
}

// updateAgentCertificate handles PUT /api/nodes/agent/:id/certificate
func (h *Handlers) updateAgentCertificate(w http.ResponseWriter, r *http.Request, id string) {
	var req CertificateActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if req.Action != "rotate" && req.Action != "revoke" {
		writeError(w, http.StatusBadRequest, "action must be 'rotate' or 'revoke'", "INVALID_ACTION")
		return
	}

	now := time.Now()
	notAfter := now.Add(365 * 24 * time.Hour)

	cert := Certificate{
		Fingerprint:  "SHA256:new-placeholder",
		Subject:      fmt.Sprintf("CN=%s,O=OpenGSLB", id),
		Issuer:       "CN=OpenGSLB-CA,O=OpenGSLB",
		SerialNumber: "01:23:45:67:AB",
		NotBefore:    &now,
		NotAfter:     &notAfter,
		Revoked:      req.Action == "revoke",
	}

	if req.Action == "revoke" {
		cert.RevokedAt = &now
		cert.RevokedReason = req.Reason
		if cert.RevokedReason == "" {
			cert.RevokedReason = "revoked via API"
		}
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryAuth, id,
		fmt.Sprintf("Certificate %s for agent %s", req.Action, id),
		map[string]interface{}{
			"action": req.Action,
			"reason": req.Reason,
		},
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, CertificateResponse{Certificate: cert})
}
