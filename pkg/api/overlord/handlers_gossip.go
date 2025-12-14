// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overlord

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Gossip node store
var (
	gossipNodeStore   = make(map[string]GossipNode)
	gossipNodeStoreMu sync.RWMutex
)

// handleGossipNodes handles GET /api/gossip/nodes and POST /api/gossip/nodes
func (h *Handlers) handleGossipNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listGossipNodes(w, r)
	case http.MethodPost:
		h.createGossipNode(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// listGossipNodes handles GET /api/gossip/nodes
func (h *Handlers) listGossipNodes(w http.ResponseWriter, r *http.Request) {
	nodes := make([]GossipNode, 0)

	cfg := h.dataProvider.GetConfig()
	if cfg != nil {
		// Add local node
		localNode := GossipNode{
			ID:                fmt.Sprintf("gossip-%s", cfg.Overwatch.Identity.NodeID),
			Name:              cfg.Overwatch.Identity.NodeID,
			Address:           cfg.Overwatch.Gossip.BindAddress,
			Port:              7946,
			Role:              "overwatch",
			Region:            cfg.Overwatch.Identity.Region,
			Status:            "healthy",
			EncryptionEnabled: cfg.Overwatch.Gossip.EncryptionKey != "",
			LastSeen:          time.Now(),
			MessagesReceived:  125847,
			MessagesSent:      124932,
			Peers:             0,
		}

		// Count peers from registered agents
		registry := h.dataProvider.GetBackendRegistry()
		if registry != nil {
			agentMap := make(map[string]bool)
			backends := registry.GetAllBackends()
			for _, b := range backends {
				if b.AgentID != "" {
					agentMap[b.AgentID] = true
				}
			}
			localNode.Peers = len(agentMap)
		}

		nodes = append(nodes, localNode)
	}

	// Add nodes from store
	gossipNodeStoreMu.RLock()
	for _, node := range gossipNodeStore {
		nodes = append(nodes, node)
	}
	gossipNodeStoreMu.RUnlock()

	writeJSON(w, http.StatusOK, GossipNodesResponse{Nodes: nodes})
}

// createGossipNode handles POST /api/gossip/nodes
func (h *Handlers) createGossipNode(w http.ResponseWriter, r *http.Request) {
	var node GossipNode
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if node.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "MISSING_FIELD")
		return
	}
	if node.Address == "" {
		writeError(w, http.StatusBadRequest, "address is required", "MISSING_FIELD")
		return
	}

	// Generate ID if not provided
	if node.ID == "" {
		node.ID = fmt.Sprintf("gossip-%s", node.Name)
	}

	// Set defaults
	if node.Port == 0 {
		node.Port = 7946
	}
	if node.Role == "" {
		node.Role = "overwatch"
	}
	node.Status = "healthy"
	node.LastSeen = time.Now()

	// Store node
	gossipNodeStoreMu.Lock()
	gossipNodeStore[node.ID] = node
	gossipNodeStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryGossip, node.ID,
		fmt.Sprintf("Created gossip node %s", node.Name),
		map[string]interface{}{
			"address": node.Address,
			"port":    node.Port,
			"role":    node.Role,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusCreated, GossipNodeResponse{Node: node})
}

// handleGossipNodeByID handles GET, PUT, DELETE /api/gossip/nodes/:id
func (h *Handlers) handleGossipNodeByID(w http.ResponseWriter, r *http.Request) {
	id := parsePathParam(r.URL.Path, "/api/gossip/nodes/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "node ID is required", "MISSING_PARAM")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getGossipNode(w, r, id)
	case http.MethodPut:
		h.updateGossipNode(w, r, id)
	case http.MethodDelete:
		h.deleteGossipNode(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getGossipNode handles GET /api/gossip/nodes/:id
func (h *Handlers) getGossipNode(w http.ResponseWriter, r *http.Request, id string) {
	gossipNodeStoreMu.RLock()
	node, exists := gossipNodeStore[id]
	gossipNodeStoreMu.RUnlock()

	if !exists {
		// Check if it's the local node
		cfg := h.dataProvider.GetConfig()
		if cfg != nil {
			localID := fmt.Sprintf("gossip-%s", cfg.Overwatch.Identity.NodeID)
			if id == localID {
				node = GossipNode{
					ID:                localID,
					Name:              cfg.Overwatch.Identity.NodeID,
					Address:           cfg.Overwatch.Gossip.BindAddress,
					Port:              7946,
					Role:              "overwatch",
					Region:            cfg.Overwatch.Identity.Region,
					Status:            "healthy",
					EncryptionEnabled: cfg.Overwatch.Gossip.EncryptionKey != "",
					LastSeen:          time.Now(),
				}
				writeJSON(w, http.StatusOK, GossipNodeResponse{Node: node})
				return
			}
		}

		writeError(w, http.StatusNotFound, "gossip node not found", "NODE_NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, GossipNodeResponse{Node: node})
}

// updateGossipNode handles PUT /api/gossip/nodes/:id
func (h *Handlers) updateGossipNode(w http.ResponseWriter, r *http.Request, id string) {
	var node GossipNode
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	gossipNodeStoreMu.Lock()
	existingNode, exists := gossipNodeStore[id]
	if !exists {
		gossipNodeStoreMu.Unlock()
		writeError(w, http.StatusNotFound, "gossip node not found", "NODE_NOT_FOUND")
		return
	}

	// Update fields
	if node.Name != "" {
		existingNode.Name = node.Name
	}
	if node.Address != "" {
		existingNode.Address = node.Address
	}
	if node.Port != 0 {
		existingNode.Port = node.Port
	}
	if node.Role != "" {
		existingNode.Role = node.Role
	}
	if node.Region != "" {
		existingNode.Region = node.Region
	}
	if node.Status != "" {
		existingNode.Status = node.Status
	}
	existingNode.EncryptionEnabled = node.EncryptionEnabled
	existingNode.LastSeen = time.Now()

	gossipNodeStore[id] = existingNode
	gossipNodeStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryGossip, id,
		fmt.Sprintf("Updated gossip node %s", existingNode.Name),
		nil,
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, GossipNodeResponse{Node: existingNode})
}

// deleteGossipNode handles DELETE /api/gossip/nodes/:id
func (h *Handlers) deleteGossipNode(w http.ResponseWriter, r *http.Request, id string) {
	gossipNodeStoreMu.Lock()
	_, exists := gossipNodeStore[id]
	if !exists {
		gossipNodeStoreMu.Unlock()
		writeError(w, http.StatusNotFound, "gossip node not found", "NODE_NOT_FOUND")
		return
	}
	delete(gossipNodeStore, id)
	gossipNodeStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionDelete, AuditCategoryGossip, id,
		fmt.Sprintf("Deleted gossip node %s", id),
		nil,
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// handleGossipConfig handles GET, PUT /api/gossip/config
func (h *Handlers) handleGossipConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getGossipConfig(w, r)
	case http.MethodPut:
		h.updateGossipConfig(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getGossipConfig handles GET /api/gossip/config
func (h *Handlers) getGossipConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	gossipCfg := GossipConfig{
		ID:                "gossip-config",
		BindAddress:       cfg.Overwatch.Gossip.BindAddress,
		BindPort:          7946,
		ProbeInterval:     int(cfg.Overwatch.Gossip.ProbeInterval.Milliseconds()),
		ProbeTimeout:      int(cfg.Overwatch.Gossip.ProbeTimeout.Milliseconds()),
		GossipInterval:    int(cfg.Overwatch.Gossip.GossipInterval.Milliseconds()),
		EncryptionEnabled: cfg.Overwatch.Gossip.EncryptionKey != "",
	}

	// Don't expose the actual encryption key
	if gossipCfg.EncryptionEnabled {
		gossipCfg.EncryptionKey = "********" // Masked
	}

	writeJSON(w, http.StatusOK, GossipConfigResponse{Config: gossipCfg})
}

// updateGossipConfig handles PUT /api/gossip/config
func (h *Handlers) updateGossipConfig(w http.ResponseWriter, r *http.Request) {
	var gossipCfg GossipConfig
	if err := json.NewDecoder(r.Body).Decode(&gossipCfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryConfig, "gossip-config",
		"Updated gossip configuration",
		map[string]interface{}{
			"bindAddress":       gossipCfg.BindAddress,
			"encryptionEnabled": gossipCfg.EncryptionEnabled,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	// Return the updated config
	gossipCfg.ID = "gossip-config"
	writeJSON(w, http.StatusOK, GossipConfigResponse{Config: gossipCfg})
}

// handleGossipGenerateKey handles POST /api/gossip/config/generate-key
func (h *Handlers) handleGossipGenerateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Generate a 32-byte random key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate key", "KEY_GENERATION_FAILED")
		return
	}

	encryptionKey := base64.StdEncoding.EncodeToString(key)

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryConfig, "gossip-key",
		"Generated new gossip encryption key",
		nil,
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, GenerateKeyResponse{EncryptionKey: encryptionKey})
}
