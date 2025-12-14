// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// GossipProvider defines the interface for gossip protocol operations.
type GossipProvider interface {
	// ListGossipNodes returns all nodes in the gossip cluster.
	ListGossipNodes() []GossipNode
	// GetGossipNode returns a gossip node by ID.
	GetGossipNode(id string) (*GossipNode, error)
	// GetGossipConfig returns the gossip protocol configuration.
	GetGossipConfig() (*GossipConfig, error)
	// UpdateGossipConfig updates the gossip protocol configuration.
	UpdateGossipConfig(config GossipConfig) error
}

// GossipNode represents a node in the gossip cluster.
type GossipNode struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Address    string            `json:"address"`
	Port       int               `json:"port"`
	Status     string            `json:"status"` // alive, suspect, dead
	State      string            `json:"state"`  // leader, follower, candidate
	Region     string            `json:"region"`
	Datacenter string            `json:"datacenter,omitempty"`
	Version    string            `json:"version"`
	RTT        int64             `json:"rtt_ms"`
	LastSeen   time.Time         `json:"last_seen"`
	JoinedAt   time.Time         `json:"joined_at"`
	Tags       map[string]string `json:"tags,omitempty"`
}

// GossipConfig represents gossip protocol configuration.
type GossipConfig struct {
	Enabled                 bool     `json:"enabled"`
	BindAddress             string   `json:"bind_address"`
	BindPort                int      `json:"bind_port"`
	AdvertiseAddress        string   `json:"advertise_address,omitempty"`
	AdvertisePort           int      `json:"advertise_port,omitempty"`
	ClusterName             string   `json:"cluster_name"`
	EncryptionEnabled       bool     `json:"encryption_enabled"`
	EncryptionKey           string   `json:"encryption_key,omitempty"` // Hidden in responses
	RetransmitMult          int      `json:"retransmit_mult"`
	GossipInterval          int      `json:"gossip_interval_ms"`
	ProbeInterval           int      `json:"probe_interval_ms"`
	ProbeTimeout            int      `json:"probe_timeout_ms"`
	SuspicionMult           int      `json:"suspicion_mult"`
	SuspicionMaxTimeoutMult int      `json:"suspicion_max_timeout_mult"`
	PushPullInterval        int      `json:"push_pull_interval_ms"`
	Seeds                   []string `json:"seeds,omitempty"`
}

// GossipNodeListResponse is the response for GET /api/v1/gossip/nodes.
type GossipNodeListResponse struct {
	Nodes       []GossipNode `json:"nodes"`
	Total       int          `json:"total"`
	Healthy     int          `json:"healthy"`
	GeneratedAt time.Time    `json:"generated_at"`
}

// GossipNodeResponse is the response for single gossip node operations.
type GossipNodeResponse struct {
	Node GossipNode `json:"node"`
}

// GossipConfigResponse is the response for GET /api/v1/gossip/config.
type GossipConfigResponse struct {
	Config GossipConfig `json:"config"`
}

// GossipKeyResponse is the response for POST /api/v1/gossip/generate-key.
type GossipKeyResponse struct {
	Key       string `json:"key"`
	Algorithm string `json:"algorithm"`
	Bits      int    `json:"bits"`
}

// GossipHandlers provides HTTP handlers for gossip API endpoints.
type GossipHandlers struct {
	provider GossipProvider
	logger   *slog.Logger
}

// NewGossipHandlers creates a new GossipHandlers instance.
func NewGossipHandlers(provider GossipProvider, logger *slog.Logger) *GossipHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &GossipHandlers{
		provider: provider,
		logger:   logger,
	}
}

// HandleGossip routes /api/v1/gossip requests based on HTTP method and path.
func (h *GossipHandlers) HandleGossip(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/gossip")
	path = strings.TrimPrefix(path, "/")

	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		h.writeError(w, http.StatusNotFound, "specify endpoint: /api/v1/gossip/nodes, /api/v1/gossip/config, or /api/v1/gossip/generate-key")
		return
	}

	resource := parts[0]
	var subID string
	if len(parts) > 1 {
		subID = parts[1]
	}

	switch resource {
	case "nodes":
		h.handleNodes(w, r, subID)
	case "config":
		h.handleConfig(w, r)
	case "generate-key":
		h.handleGenerateKey(w, r)
	default:
		h.writeError(w, http.StatusNotFound, "unknown endpoint: "+resource)
	}
}

// handleNodes handles /api/v1/gossip/nodes requests.
func (h *GossipHandlers) handleNodes(w http.ResponseWriter, r *http.Request, nodeID string) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "gossip provider not configured")
		return
	}

	// List all gossip nodes
	if nodeID == "" {
		nodes := h.provider.ListGossipNodes()

		// Apply filters
		statusFilter := r.URL.Query().Get("status")
		regionFilter := r.URL.Query().Get("region")

		healthy := 0
		if statusFilter != "" || regionFilter != "" {
			filtered := make([]GossipNode, 0, len(nodes))
			for _, n := range nodes {
				if statusFilter != "" && n.Status != statusFilter {
					continue
				}
				if regionFilter != "" && n.Region != regionFilter {
					continue
				}
				if n.Status == "alive" {
					healthy++
				}
				filtered = append(filtered, n)
			}
			nodes = filtered
		} else {
			for _, n := range nodes {
				if n.Status == "alive" {
					healthy++
				}
			}
		}

		resp := GossipNodeListResponse{
			Nodes:       nodes,
			Total:       len(nodes),
			Healthy:     healthy,
			GeneratedAt: time.Now().UTC(),
		}
		h.writeJSON(w, http.StatusOK, resp)
		return
	}

	// Get single gossip node
	node, err := h.provider.GetGossipNode(nodeID)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "gossip node not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, GossipNodeResponse{Node: *node})
}

// handleConfig handles /api/v1/gossip/config requests.
func (h *GossipHandlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "gossip provider not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		config, err := h.provider.GetGossipConfig()
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "failed to get gossip config: "+err.Error())
			return
		}
		// Hide encryption key in response
		config.EncryptionKey = ""
		h.writeJSON(w, http.StatusOK, GossipConfigResponse{Config: *config})

	case http.MethodPut, http.MethodPatch:
		var config GossipConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		if err := h.provider.UpdateGossipConfig(config); err != nil {
			h.logger.Error("failed to update gossip config", "error", err)
			h.writeError(w, http.StatusInternalServerError, "failed to update gossip config: "+err.Error())
			return
		}

		h.logger.Info("gossip config updated")
		// Hide encryption key in response
		config.EncryptionKey = ""
		h.writeJSON(w, http.StatusOK, GossipConfigResponse{Config: config})

	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleGenerateKey handles POST /api/v1/gossip/generate-key requests.
func (h *GossipHandlers) handleGenerateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Generate a 32-byte (256-bit) random key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		h.logger.Error("failed to generate gossip key", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}

	resp := GossipKeyResponse{
		Key:       base64.StdEncoding.EncodeToString(key),
		Algorithm: "AES-256-GCM",
		Bits:      256,
	}

	h.logger.Info("gossip encryption key generated")
	h.writeJSON(w, http.StatusOK, resp)
}

// writeJSON writes a JSON response with the given status code.
func (h *GossipHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *GossipHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
