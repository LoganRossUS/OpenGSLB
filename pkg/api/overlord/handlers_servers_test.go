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

func TestHandleServers_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleServers, http.MethodGet, "/api/servers", "")
	assertStatus(t, rr, http.StatusOK)

	var resp ServersResponse
	decodeJSON(t, rr, &resp)

	// Without registry, should fall back to config-based servers
	if len(resp.Servers) != 3 {
		t.Errorf("expected 3 servers, got %d", len(resp.Servers))
	}
}

func TestHandleServers_GET_WithRegionFilter(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleServers, http.MethodGet, "/api/servers?region=us-east-1", "")
	assertStatus(t, rr, http.StatusOK)

	var resp ServersResponse
	decodeJSON(t, rr, &resp)

	// Should only return us-east-1 servers (2 servers)
	if len(resp.Servers) != 2 {
		t.Errorf("expected 2 servers from us-east-1, got %d", len(resp.Servers))
	}
	for _, s := range resp.Servers {
		if s.Region != "us-east-1" {
			t.Errorf("expected region 'us-east-1', got '%s'", s.Region)
		}
	}
}

func TestHandleServers_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"service": "web",
		"address": "10.0.3.10",
		"port": 8080,
		"region": "us-east-1",
		"weight": 50
	}`

	rr := makeRequest(t, h.handleServers, http.MethodPost, "/api/servers", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp ServerResponse
	decodeJSON(t, rr, &resp)

	if resp.Server.Address != "10.0.3.10" {
		t.Errorf("expected address '10.0.3.10', got '%s'", resp.Server.Address)
	}
	if resp.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", resp.Server.Port)
	}
	if resp.Server.Weight != 50 {
		t.Errorf("expected weight 50, got %d", resp.Server.Weight)
	}
}

func TestHandleServers_POST_DefaultWeight(t *testing.T) {
	h := testHandlers()

	body := `{
		"service": "web",
		"address": "10.0.3.10",
		"port": 8080,
		"region": "us-east-1"
	}`

	rr := makeRequest(t, h.handleServers, http.MethodPost, "/api/servers", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp ServerResponse
	decodeJSON(t, rr, &resp)

	if resp.Server.Weight != 100 {
		t.Errorf("expected default weight 100, got %d", resp.Server.Weight)
	}
}

func TestHandleServers_POST_MissingService(t *testing.T) {
	h := testHandlers()

	body := `{
		"address": "10.0.3.10",
		"port": 8080,
		"region": "us-east-1"
	}`

	rr := makeRequest(t, h.handleServers, http.MethodPost, "/api/servers", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "MISSING_FIELD" {
		t.Errorf("expected code 'MISSING_FIELD', got '%s'", resp.Code)
	}
}

func TestHandleServers_POST_MissingAddress(t *testing.T) {
	h := testHandlers()

	body := `{
		"service": "web",
		"port": 8080,
		"region": "us-east-1"
	}`

	rr := makeRequest(t, h.handleServers, http.MethodPost, "/api/servers", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleServers_POST_MissingPort(t *testing.T) {
	h := testHandlers()

	body := `{
		"service": "web",
		"address": "10.0.3.10",
		"region": "us-east-1"
	}`

	rr := makeRequest(t, h.handleServers, http.MethodPost, "/api/servers", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleServers_POST_MissingRegion(t *testing.T) {
	h := testHandlers()

	body := `{
		"service": "web",
		"address": "10.0.3.10",
		"port": 8080
	}`

	rr := makeRequest(t, h.handleServers, http.MethodPost, "/api/servers", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleServers_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleServers, http.MethodPost, "/api/servers", "invalid json")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleServers_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleServers, http.MethodDelete, "/api/servers", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestParseServerID(t *testing.T) {
	tests := []struct {
		id          string
		expService  string
		expAddress  string
		expPort     int
	}{
		{"10.0.1.10:80", "", "10.0.1.10", 80},
		{"web:10.0.1.10:80", "web", "10.0.1.10", 80},
		{"api:192.168.1.1:8080", "api", "192.168.1.1", 8080},
		{"invalid", "", "", 0},
	}

	for _, tt := range tests {
		service, address, port := parseServerID(tt.id)
		if service != tt.expService || address != tt.expAddress || port != tt.expPort {
			t.Errorf("parseServerID(%s) = (%s, %s, %d), want (%s, %s, %d)",
				tt.id, service, address, port, tt.expService, tt.expAddress, tt.expPort)
		}
	}
}

func TestHandleServerHealthCheck_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getServerHealthCheck(w, r, "10.0.1.10:80")
	}, http.MethodGet, "/api/servers/10.0.1.10:80/health-check", "")

	assertStatus(t, rr, http.StatusOK)
	assertJSONField(t, rr, "healthCheck")
}

func TestHandleServerHealthCheck_PUT(t *testing.T) {
	h := testHandlers()

	body := `{
		"enabled": true,
		"type": "http",
		"path": "/healthz",
		"interval": 60,
		"timeout": 10
	}`

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.updateServerHealthCheck(w, r, "10.0.1.10:80")
	}, http.MethodPut, "/api/servers/10.0.1.10:80/health-check", body)

	assertStatus(t, rr, http.StatusOK)
}

func TestHandleServerHealthCheck_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.updateServerHealthCheck(w, r, "10.0.1.10:80")
	}, http.MethodPut, "/api/servers/10.0.1.10:80/health-check", "invalid")

	assertStatus(t, rr, http.StatusBadRequest)
}
