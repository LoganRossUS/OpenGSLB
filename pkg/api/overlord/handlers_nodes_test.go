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

func TestHandleOverwatchNodes_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleOverwatchNodes, http.MethodGet, "/api/nodes/overwatch", "")
	assertStatus(t, rr, http.StatusOK)

	var resp OverwatchNodesResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Nodes) != 1 {
		t.Errorf("expected 1 overwatch node, got %d", len(resp.Nodes))
	}
	if resp.Nodes[0].NodeID != "test-node-1" {
		t.Errorf("expected node ID 'test-node-1', got '%s'", resp.Nodes[0].NodeID)
	}
	if resp.Nodes[0].Region != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got '%s'", resp.Nodes[0].Region)
	}
}

func TestHandleOverwatchNodes_POST(t *testing.T) {
	h := testHandlers()

	// POST is allowed for creating nodes but requires valid body
	body := `{"nodeId": "new-node", "region": "eu-west-1", "address": "10.0.2.1"}`
	rr := makeRequest(t, h.handleOverwatchNodes, http.MethodPost, "/api/nodes/overwatch", body)
	assertStatus(t, rr, http.StatusCreated)
}

func TestHandleOverwatchNodes_DELETE_NotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleOverwatchNodes, http.MethodDelete, "/api/nodes/overwatch", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleOverwatchNodeByID_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getOverwatchNode(w, r, "test-node-1")
	}, http.MethodGet, "/api/nodes/overwatch/test-node-1", "")

	assertStatus(t, rr, http.StatusOK)

	var resp OverwatchNodeResponse
	decodeJSON(t, rr, &resp)

	if resp.Node.NodeID != "test-node-1" {
		t.Errorf("expected node ID 'test-node-1', got '%s'", resp.Node.NodeID)
	}
}

func TestHandleOverwatchNodeByID_GET_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getOverwatchNode(w, r, "nonexistent-node")
	}, http.MethodGet, "/api/nodes/overwatch/nonexistent-node", "")

	assertStatus(t, rr, http.StatusNotFound)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "NODE_NOT_FOUND" {
		t.Errorf("expected code 'NODE_NOT_FOUND', got '%s'", resp.Code)
	}
}

func TestHandleAgentNodes_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleAgentNodes, http.MethodGet, "/api/nodes/agent", "")
	assertStatus(t, rr, http.StatusOK)

	var resp AgentNodesResponse
	decodeJSON(t, rr, &resp)

	// Without registry, should return empty agents
	if resp.Agents == nil {
		t.Error("expected non-nil agents")
	}
}

func TestHandleAgentNodes_POST(t *testing.T) {
	h := testHandlers()

	// POST is allowed for registering agents
	body := `{"agentId": "test-agent", "region": "us-east-1"}`
	rr := makeRequest(t, h.handleAgentNodes, http.MethodPost, "/api/nodes/agent", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp AgentNodeResponse
	decodeJSON(t, rr, &resp)

	if resp.Agent.AgentID != "test-agent" {
		t.Errorf("expected agent ID 'test-agent', got '%s'", resp.Agent.AgentID)
	}
}

func TestHandleAgentNodes_DELETE_NotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleAgentNodes, http.MethodDelete, "/api/nodes/agent", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleAgentNodeByID_RegistryUnavailable(t *testing.T) {
	h := testHandlers()

	// Without registry, should return 503
	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getAgentNode(w, r, "any-agent")
	}, http.MethodGet, "/api/nodes/agent/any-agent", "")

	assertStatus(t, rr, http.StatusServiceUnavailable)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "REGISTRY_UNAVAILABLE" {
		t.Errorf("expected code 'REGISTRY_UNAVAILABLE', got '%s'", resp.Code)
	}
}

func TestHandleAgentNodeByID_DELETE_RegistryUnavailable(t *testing.T) {
	h := testHandlers()

	// Without registry, should return 503
	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteAgentNode(w, r, "any-agent")
	}, http.MethodDelete, "/api/nodes/agent/any-agent", "")

	assertStatus(t, rr, http.StatusServiceUnavailable)
}

func TestHandleAgentCertificate_GET(t *testing.T) {
	h := testHandlers()

	// Certificate endpoint returns placeholder data regardless of agent existence
	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.handleAgentCertificate(w, r, "any-agent")
	}, http.MethodGet, "/api/nodes/agent/any-agent/certificate", "")

	assertStatus(t, rr, http.StatusOK)

	var resp CertificateResponse
	decodeJSON(t, rr, &resp)

	if resp.Certificate.Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
}

func TestHandleAgentCertificate_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.handleAgentCertificate(w, r, "any-agent")
	}, http.MethodDelete, "/api/nodes/agent/any-agent/certificate", "")

	assertStatus(t, rr, http.StatusMethodNotAllowed)
}
