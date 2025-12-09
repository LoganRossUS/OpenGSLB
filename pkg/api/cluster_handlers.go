// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/loganrossus/OpenGSLB/pkg/cluster"
)

// ClusterManager defines the interface for cluster operations needed by handlers.
// This allows for easy testing with mocks.
type ClusterManager interface {
	IsLeader() bool
	NodeID() string
	State() cluster.State
	Leader() (cluster.LeaderInfo, error)
	Nodes() []cluster.NodeInfo
	AddVoter(nodeID, address string) error
	RemoveServer(nodeID string) error
}

// ClusterHandlers provides HTTP handlers for cluster management APIs.
type ClusterHandlers struct {
	manager ClusterManager
	mode    string // "standalone" or "cluster"
	logger  *slog.Logger
}

// NewClusterHandlers creates a new ClusterHandlers instance.
// manager can be nil if running in standalone mode.
func NewClusterHandlers(manager ClusterManager, mode string, logger *slog.Logger) *ClusterHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &ClusterHandlers{
		manager: manager,
		mode:    mode,
		logger:  logger,
	}
}

// HandleJoin processes requests from nodes wanting to join the cluster.
// POST /api/v1/cluster/join
func (h *ClusterHandlers) HandleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.mode != "cluster" {
		h.writeJSON(w, http.StatusBadRequest, cluster.JoinResponse{
			Success: false,
			Message: "node is running in standalone mode",
		})
		return
	}

	if h.manager == nil {
		h.writeJSON(w, http.StatusServiceUnavailable, cluster.JoinResponse{
			Success: false,
			Message: "cluster manager not initialized",
		})
		return
	}

	var req cluster.JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSON(w, http.StatusBadRequest, cluster.JoinResponse{
			Success: false,
			Message: "invalid request body: " + err.Error(),
		})
		return
	}

	if req.NodeID == "" {
		h.writeJSON(w, http.StatusBadRequest, cluster.JoinResponse{
			Success: false,
			Message: "node_id is required",
		})
		return
	}
	if req.RaftAddress == "" {
		h.writeJSON(w, http.StatusBadRequest, cluster.JoinResponse{
			Success: false,
			Message: "raft_address is required",
		})
		return
	}

	if !h.manager.IsLeader() {
		leader, err := h.manager.Leader()
		if err != nil {
			h.writeJSON(w, http.StatusServiceUnavailable, cluster.JoinResponse{
				Success: false,
				Message: "no leader available: " + err.Error(),
			})
			return
		}
		h.writeJSON(w, http.StatusTemporaryRedirect, cluster.JoinResponse{
			Success:       false,
			Message:       "not the leader, redirect to leader",
			LeaderID:      leader.NodeID,
			LeaderAddress: leader.Address,
		})
		return
	}

	h.logger.Info("processing cluster join request",
		"joining_node_id", req.NodeID,
		"joining_raft_address", req.RaftAddress,
	)

	if err := h.manager.AddVoter(req.NodeID, req.RaftAddress); err != nil {
		h.logger.Error("failed to add voter",
			"node_id", req.NodeID,
			"error", err,
		)
		h.writeJSON(w, http.StatusInternalServerError, cluster.JoinResponse{
			Success: false,
			Message: "failed to add node: " + err.Error(),
		})
		return
	}

	h.logger.Info("node joined cluster successfully",
		"node_id", req.NodeID,
		"raft_address", req.RaftAddress,
	)

	h.writeJSON(w, http.StatusOK, cluster.JoinResponse{
		Success:  true,
		Message:  "node added to cluster successfully",
		LeaderID: h.manager.NodeID(),
	})
}

// ClusterStatusResponse provides information about the cluster state.
type ClusterStatusResponse struct {
	Mode          string             `json:"mode"`
	NodeID        string             `json:"node_id"`
	State         string             `json:"state"`
	IsLeader      bool               `json:"is_leader"`
	LeaderID      string             `json:"leader_id,omitempty"`
	LeaderAddress string             `json:"leader_address,omitempty"`
	Nodes         []cluster.NodeInfo `json:"nodes,omitempty"`
}

// HandleStatus returns the current cluster status.
// GET /api/v1/cluster/status
func (h *ClusterHandlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.mode != "cluster" || h.manager == nil {
		h.writeJSON(w, http.StatusOK, ClusterStatusResponse{
			Mode:     "standalone",
			IsLeader: true,
			State:    "standalone",
		})
		return
	}

	leader, _ := h.manager.Leader()

	status := ClusterStatusResponse{
		Mode:          "cluster",
		NodeID:        h.manager.NodeID(),
		State:         h.manager.State().String(),
		IsLeader:      h.manager.IsLeader(),
		LeaderID:      leader.NodeID,
		LeaderAddress: leader.Address,
		Nodes:         h.manager.Nodes(),
	}

	h.writeJSON(w, http.StatusOK, status)
}

// ClusterRemoveRequest is sent to remove a node from the cluster.
type ClusterRemoveRequest struct {
	NodeID string `json:"node_id"`
}

// ClusterRemoveResponse is returned after processing a remove request.
type ClusterRemoveResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// HandleRemove processes requests to remove a node from the cluster.
// POST /api/v1/cluster/remove
func (h *ClusterHandlers) HandleRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.mode != "cluster" {
		h.writeJSON(w, http.StatusBadRequest, ClusterRemoveResponse{
			Success: false,
			Message: "node is running in standalone mode",
		})
		return
	}

	if h.manager == nil {
		h.writeJSON(w, http.StatusServiceUnavailable, ClusterRemoveResponse{
			Success: false,
			Message: "cluster manager not initialized",
		})
		return
	}

	var req ClusterRemoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSON(w, http.StatusBadRequest, ClusterRemoveResponse{
			Success: false,
			Message: "invalid request body: " + err.Error(),
		})
		return
	}

	if req.NodeID == "" {
		h.writeJSON(w, http.StatusBadRequest, ClusterRemoveResponse{
			Success: false,
			Message: "node_id is required",
		})
		return
	}

	if !h.manager.IsLeader() {
		leader, _ := h.manager.Leader()
		h.writeJSON(w, http.StatusTemporaryRedirect, ClusterRemoveResponse{
			Success: false,
			Message: "not the leader, redirect to " + leader.Address,
		})
		return
	}

	h.logger.Info("processing cluster remove request", "node_id", req.NodeID)

	if err := h.manager.RemoveServer(req.NodeID); err != nil {
		h.logger.Error("failed to remove node", "node_id", req.NodeID, "error", err)
		h.writeJSON(w, http.StatusInternalServerError, ClusterRemoveResponse{
			Success: false,
			Message: "failed to remove node: " + err.Error(),
		})
		return
	}

	h.logger.Info("node removed from cluster", "node_id", req.NodeID)

	h.writeJSON(w, http.StatusOK, ClusterRemoveResponse{
		Success: true,
		Message: "node removed successfully",
	})
}

// writeJSON writes a JSON response with the given status code.
func (h *ClusterHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a simple JSON error response.
func (h *ClusterHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}
