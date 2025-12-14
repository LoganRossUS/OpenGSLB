// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overlord

import (
	"net/http"
	"testing"
)

func TestHandleGossipNodes_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGossipNodes, http.MethodGet, "/api/gossip/nodes", "")
	assertStatus(t, rr, http.StatusOK)

	var resp GossipNodesResponse
	decodeJSON(t, rr, &resp)

	if resp.Nodes == nil {
		t.Error("expected non-nil nodes")
	}
}

func TestHandleGossipNodes_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"name": "test-node",
		"address": "10.0.1.10",
		"port": 7946,
		"role": "overwatch",
		"region": "us-east-1"
	}`

	rr := makeRequest(t, h.handleGossipNodes, http.MethodPost, "/api/gossip/nodes", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp GossipNodeResponse
	decodeJSON(t, rr, &resp)

	if resp.Node.ID == "" {
		t.Error("expected non-empty node ID")
	}
	if resp.Node.Name != "test-node" {
		t.Errorf("expected name 'test-node', got '%s'", resp.Node.Name)
	}
}

func TestHandleGossipNodes_POST_MissingName(t *testing.T) {
	h := testHandlers()

	body := `{
		"address": "10.0.1.10",
		"port": 7946
	}`

	rr := makeRequest(t, h.handleGossipNodes, http.MethodPost, "/api/gossip/nodes", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleGossipNodes_POST_MissingAddress(t *testing.T) {
	h := testHandlers()

	body := `{
		"name": "test-node",
		"port": 7946
	}`

	rr := makeRequest(t, h.handleGossipNodes, http.MethodPost, "/api/gossip/nodes", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleGossipNodes_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGossipNodes, http.MethodPost, "/api/gossip/nodes", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleGossipNodes_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGossipNodes, http.MethodDelete, "/api/gossip/nodes", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleGossipNodeByID_GET_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getGossipNode(w, r, "nonexistent")
	}, http.MethodGet, "/api/gossip/nodes/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "NODE_NOT_FOUND" {
		t.Errorf("expected code 'NODE_NOT_FOUND', got '%s'", resp.Code)
	}
}

func TestHandleGossipNodeByID_DELETE_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteGossipNode(w, r, "nonexistent")
	}, http.MethodDelete, "/api/gossip/nodes/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)
}

func TestHandleGossipConfig_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGossipConfig, http.MethodGet, "/api/gossip/config", "")
	assertStatus(t, rr, http.StatusOK)

	var resp GossipConfigResponse
	decodeJSON(t, rr, &resp)

	if resp.Config.BindAddress == "" {
		t.Error("expected non-empty bind address")
	}
}

func TestHandleGossipConfig_PUT(t *testing.T) {
	h := testHandlers()

	body := `{
		"bindAddress": "0.0.0.0",
		"bindPort": 7947,
		"probeInterval": 2000,
		"probeTimeout": 1000,
		"gossipInterval": 500
	}`

	rr := makeRequest(t, h.handleGossipConfig, http.MethodPut, "/api/gossip/config", body)
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleGossipConfig_PUT_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGossipConfig, http.MethodPut, "/api/gossip/config", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleGossipConfig_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGossipConfig, http.MethodDelete, "/api/gossip/config", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleGossipGenerateKey_POST(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGossipGenerateKey, http.MethodPost, "/api/gossip/config/generate-key", "")
	assertStatus(t, rr, http.StatusOK)

	var resp GenerateKeyResponse
	decodeJSON(t, rr, &resp)

	if resp.EncryptionKey == "" {
		t.Error("expected non-empty encryption key")
	}
	// Key should be base64 encoded 32-byte key
	if len(resp.EncryptionKey) < 40 {
		t.Errorf("encryption key seems too short: %d chars", len(resp.EncryptionKey))
	}
}

func TestHandleGossipGenerateKey_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGossipGenerateKey, http.MethodGet, "/api/gossip/config/generate-key", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}
