// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/cluster"
)

// mockClusterManager implements ClusterManager for testing.
type mockClusterManager struct {
	isLeader       bool
	nodeID         string
	state          cluster.State
	leaderInfo     cluster.LeaderInfo
	leaderErr      error
	nodes          []cluster.NodeInfo
	addVoterErr    error
	removeErr      error
	addVoterCalled bool
	removeCalled   bool
	addedNodeID    string
	addedAddress   string
	removedNodeID  string
}

func (m *mockClusterManager) IsLeader() bool       { return m.isLeader }
func (m *mockClusterManager) NodeID() string       { return m.nodeID }
func (m *mockClusterManager) State() cluster.State { return m.state }
func (m *mockClusterManager) Leader() (cluster.LeaderInfo, error) {
	return m.leaderInfo, m.leaderErr
}
func (m *mockClusterManager) Nodes() []cluster.NodeInfo { return m.nodes }

func (m *mockClusterManager) AddVoter(nodeID, address string) error {
	m.addVoterCalled = true
	m.addedNodeID = nodeID
	m.addedAddress = address
	return m.addVoterErr
}

func (m *mockClusterManager) RemoveServer(nodeID string) error {
	m.removeCalled = true
	m.removedNodeID = nodeID
	return m.removeErr
}

func TestHandleJoin_StandaloneMode(t *testing.T) {
	h := NewClusterHandlers(nil, "standalone", nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", nil)
	w := httptest.NewRecorder()

	h.HandleJoin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp cluster.JoinResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Success {
		t.Error("expected Success=false for standalone mode")
	}
}

func TestHandleJoin_NotLeader(t *testing.T) {
	mgr := &mockClusterManager{
		isLeader: false,
		nodeID:   "node1",
		leaderInfo: cluster.LeaderInfo{
			NodeID:  "leader1",
			Address: "10.0.0.1:7946",
		},
	}
	h := NewClusterHandlers(mgr, "cluster", nil)

	body := `{"node_id": "newnode", "raft_address": "10.0.0.2:7946"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleJoin(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}

	var resp cluster.JoinResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.LeaderAddress != "10.0.0.1:7946" {
		t.Errorf("expected leader address '10.0.0.1:7946', got '%s'", resp.LeaderAddress)
	}
}

func TestHandleJoin_NoLeaderAvailable(t *testing.T) {
	mgr := &mockClusterManager{
		isLeader:  false,
		nodeID:    "node1",
		leaderErr: errors.New("no leader elected"),
	}
	h := NewClusterHandlers(mgr, "cluster", nil)

	body := `{"node_id": "newnode", "raft_address": "10.0.0.2:7946"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleJoin(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHandleJoin_Success(t *testing.T) {
	mgr := &mockClusterManager{
		isLeader: true,
		nodeID:   "leader1",
	}
	h := NewClusterHandlers(mgr, "cluster", nil)

	body := `{"node_id": "newnode", "raft_address": "10.0.0.2:7946"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleJoin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp cluster.JoinResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Success {
		t.Errorf("expected Success=true, got false: %s", resp.Message)
	}

	if !mgr.addVoterCalled {
		t.Error("expected AddVoter to be called")
	}
	if mgr.addedNodeID != "newnode" {
		t.Errorf("expected node_id 'newnode', got '%s'", mgr.addedNodeID)
	}
	if mgr.addedAddress != "10.0.0.2:7946" {
		t.Errorf("expected address '10.0.0.2:7946', got '%s'", mgr.addedAddress)
	}
}

func TestHandleJoin_AddVoterError(t *testing.T) {
	mgr := &mockClusterManager{
		isLeader:    true,
		nodeID:      "leader1",
		addVoterErr: errors.New("raft error"),
	}
	h := NewClusterHandlers(mgr, "cluster", nil)

	body := `{"node_id": "newnode", "raft_address": "10.0.0.2:7946"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleJoin(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestHandleJoin_MissingFields(t *testing.T) {
	mgr := &mockClusterManager{isLeader: true, nodeID: "leader1"}
	h := NewClusterHandlers(mgr, "cluster", nil)

	tests := []struct {
		name string
		body string
	}{
		{"missing node_id", `{"raft_address": "10.0.0.2:7946"}`},
		{"missing raft_address", `{"node_id": "node1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.HandleJoin(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
			}
		})
	}
}

func TestHandleJoin_InvalidJSON(t *testing.T) {
	mgr := &mockClusterManager{isLeader: true, nodeID: "leader1"}
	h := NewClusterHandlers(mgr, "cluster", nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleJoin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleJoin_ManagerNotInitialized(t *testing.T) {
	h := NewClusterHandlers(nil, "cluster", nil)

	body := `{"node_id": "newnode", "raft_address": "10.0.0.2:7946"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleJoin(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHandleStatus_Standalone(t *testing.T) {
	h := NewClusterHandlers(nil, "standalone", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/status", nil)
	w := httptest.NewRecorder()

	h.HandleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ClusterStatusResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Mode != "standalone" {
		t.Errorf("expected mode 'standalone', got '%s'", resp.Mode)
	}
	if !resp.IsLeader {
		t.Error("standalone should always be leader")
	}
}

func TestHandleStatus_Cluster(t *testing.T) {
	mgr := &mockClusterManager{
		isLeader: true,
		nodeID:   "node1",
		state:    cluster.StateLeader,
		leaderInfo: cluster.LeaderInfo{
			NodeID:  "node1",
			Address: "10.0.0.1:7946",
		},
		nodes: []cluster.NodeInfo{
			{ID: "node1", Address: "10.0.0.1:7946", State: cluster.StateLeader, IsVoter: true},
			{ID: "node2", Address: "10.0.0.2:7946", State: cluster.StateFollower, IsVoter: true},
		},
	}
	h := NewClusterHandlers(mgr, "cluster", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/status", nil)
	w := httptest.NewRecorder()

	h.HandleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ClusterStatusResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Mode != "cluster" {
		t.Errorf("expected mode 'cluster', got '%s'", resp.Mode)
	}
	if !resp.IsLeader {
		t.Error("expected IsLeader=true")
	}
	if len(resp.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(resp.Nodes))
	}
}

func TestHandleStatus_WrongMethod(t *testing.T) {
	h := NewClusterHandlers(nil, "cluster", nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/status", nil)
	w := httptest.NewRecorder()

	h.HandleStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleRemove_Success(t *testing.T) {
	mgr := &mockClusterManager{isLeader: true, nodeID: "leader1"}
	h := NewClusterHandlers(mgr, "cluster", nil)

	body := `{"node_id": "node2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/remove", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleRemove(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if !mgr.removeCalled {
		t.Error("expected RemoveServer to be called")
	}
	if mgr.removedNodeID != "node2" {
		t.Errorf("expected node_id 'node2', got '%s'", mgr.removedNodeID)
	}
}

func TestHandleRemove_NotLeader(t *testing.T) {
	mgr := &mockClusterManager{
		isLeader: false,
		nodeID:   "node1",
		leaderInfo: cluster.LeaderInfo{
			NodeID:  "leader1",
			Address: "10.0.0.1:7946",
		},
	}
	h := NewClusterHandlers(mgr, "cluster", nil)

	body := `{"node_id": "node2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/remove", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleRemove(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
}

func TestHandleRemove_Error(t *testing.T) {
	mgr := &mockClusterManager{
		isLeader:  true,
		nodeID:    "leader1",
		removeErr: errors.New("raft error"),
	}
	h := NewClusterHandlers(mgr, "cluster", nil)

	body := `{"node_id": "node2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/remove", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleRemove(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestHandleJoin_WrongMethod(t *testing.T) {
	h := NewClusterHandlers(nil, "cluster", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/join", nil)
	w := httptest.NewRecorder()

	h.HandleJoin(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleRemove_WrongMethod(t *testing.T) {
	h := NewClusterHandlers(nil, "cluster", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/remove", nil)
	w := httptest.NewRecorder()

	h.HandleRemove(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}
