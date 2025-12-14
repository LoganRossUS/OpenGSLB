// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"net/http"
	"testing"
)

func TestHandleDomains_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDomains, http.MethodGet, "/api/domains", "")
	assertStatus(t, rr, http.StatusOK)

	var resp DomainsResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(resp.Domains))
	}

	// Verify first domain
	found := false
	for _, d := range resp.Domains {
		if d.Name == "api.example.com" {
			found = true
			if d.RoutingAlgorithm != "latency" {
				t.Errorf("expected routing algorithm 'latency', got '%s'", d.RoutingAlgorithm)
			}
			if len(d.Regions) != 2 {
				t.Errorf("expected 2 regions, got %d", len(d.Regions))
			}
			if d.TTL != 60 {
				t.Errorf("expected TTL 60, got %d", d.TTL)
			}
		}
	}
	if !found {
		t.Error("expected to find domain 'api.example.com'")
	}
}

func TestHandleDomains_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"name": "new.example.com",
		"routingAlgorithm": "failover",
		"regions": ["us-east-1"],
		"ttl": 120
	}`

	rr := makeRequest(t, h.handleDomains, http.MethodPost, "/api/domains", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp DomainResponse
	decodeJSON(t, rr, &resp)

	if resp.Domain.Name != "new.example.com" {
		t.Errorf("expected name 'new.example.com', got '%s'", resp.Domain.Name)
	}
	if resp.Domain.RoutingAlgorithm != "failover" {
		t.Errorf("expected routing algorithm 'failover', got '%s'", resp.Domain.RoutingAlgorithm)
	}
}

func TestHandleDomains_POST_MissingName(t *testing.T) {
	h := testHandlers()

	body := `{
		"routingAlgorithm": "failover",
		"regions": ["us-east-1"]
	}`

	rr := makeRequest(t, h.handleDomains, http.MethodPost, "/api/domains", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if !resp.Error {
		t.Error("expected error response")
	}
	if resp.Code != "MISSING_FIELD" {
		t.Errorf("expected code 'MISSING_FIELD', got '%s'", resp.Code)
	}
}

func TestHandleDomains_POST_MissingRoutingAlgorithm(t *testing.T) {
	h := testHandlers()

	body := `{
		"name": "test.example.com",
		"regions": ["us-east-1"]
	}`

	rr := makeRequest(t, h.handleDomains, http.MethodPost, "/api/domains", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleDomains_POST_MissingRegions(t *testing.T) {
	h := testHandlers()

	body := `{
		"name": "test.example.com",
		"routingAlgorithm": "latency"
	}`

	rr := makeRequest(t, h.handleDomains, http.MethodPost, "/api/domains", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleDomains_POST_InvalidAlgorithm(t *testing.T) {
	h := testHandlers()

	body := `{
		"name": "test.example.com",
		"routingAlgorithm": "invalid",
		"regions": ["us-east-1"]
	}`

	rr := makeRequest(t, h.handleDomains, http.MethodPost, "/api/domains", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "INVALID_ALGORITHM" {
		t.Errorf("expected code 'INVALID_ALGORITHM', got '%s'", resp.Code)
	}
}

func TestHandleDomains_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDomains, http.MethodPost, "/api/domains", "invalid json")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleDomains_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDomains, http.MethodDelete, "/api/domains", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleDomainByName_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getDomain(w, r, "api.example.com")
	}, http.MethodGet, "/api/domains/api.example.com", "")

	assertStatus(t, rr, http.StatusOK)

	var resp DomainResponse
	decodeJSON(t, rr, &resp)

	if resp.Domain.Name != "api.example.com" {
		t.Errorf("expected name 'api.example.com', got '%s'", resp.Domain.Name)
	}
}

func TestHandleDomainByName_GET_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getDomain(w, r, "nonexistent.example.com")
	}, http.MethodGet, "/api/domains/nonexistent.example.com", "")

	assertStatus(t, rr, http.StatusNotFound)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "DOMAIN_NOT_FOUND" {
		t.Errorf("expected code 'DOMAIN_NOT_FOUND', got '%s'", resp.Code)
	}
}

func TestHandleDomainByName_PUT(t *testing.T) {
	h := testHandlers()

	body := `{
		"routingAlgorithm": "geolocation",
		"ttl": 180
	}`

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.updateDomain(w, r, "api.example.com")
	}, http.MethodPut, "/api/domains/api.example.com", body)

	assertStatus(t, rr, http.StatusOK)

	var resp DomainResponse
	decodeJSON(t, rr, &resp)

	if resp.Domain.RoutingAlgorithm != "geolocation" {
		t.Errorf("expected routing algorithm 'geolocation', got '%s'", resp.Domain.RoutingAlgorithm)
	}
	if resp.Domain.TTL != 180 {
		t.Errorf("expected TTL 180, got %d", resp.Domain.TTL)
	}
}

func TestHandleDomainByName_PUT_NotFound(t *testing.T) {
	h := testHandlers()

	body := `{"routingAlgorithm": "latency"}`

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.updateDomain(w, r, "nonexistent.example.com")
	}, http.MethodPut, "/api/domains/nonexistent.example.com", body)

	assertStatus(t, rr, http.StatusNotFound)
}

func TestHandleDomainByName_DELETE(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteDomain(w, r, "api.example.com")
	}, http.MethodDelete, "/api/domains/api.example.com", "")

	assertStatus(t, rr, http.StatusOK)

	var resp SuccessResponse
	decodeJSON(t, rr, &resp)

	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHandleDomainByName_DELETE_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteDomain(w, r, "nonexistent.example.com")
	}, http.MethodDelete, "/api/domains/nonexistent.example.com", "")

	assertStatus(t, rr, http.StatusNotFound)
}

func TestGetDomainBackends(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getDomainBackends(w, r, "api.example.com")
	}, http.MethodGet, "/api/domains/api.example.com/backends", "")

	assertStatus(t, rr, http.StatusOK)

	var resp ServersResponse
	decodeJSON(t, rr, &resp)

	// Should have backends from us-east-1 and eu-west-1
	if len(resp.Servers) != 3 {
		t.Errorf("expected 3 backends, got %d", len(resp.Servers))
	}
}

func TestGetDomainBackends_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getDomainBackends(w, r, "nonexistent.example.com")
	}, http.MethodGet, "/api/domains/nonexistent.example.com/backends", "")

	assertStatus(t, rr, http.StatusNotFound)
}

func TestGetDomainBackends_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getDomainBackends(w, r, "api.example.com")
	}, http.MethodPost, "/api/domains/api.example.com/backends", "")

	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestDomainMatchesService(t *testing.T) {
	tests := []struct {
		domain   string
		service  string
		expected bool
	}{
		{"api.example.com", "api", true},
		{"api.example.com", "api.example.com", true},
		{"api.example.com", "web", false},
		{"api-service.example.com", "api", true},
		{"web.example.com", "web", true},
	}

	for _, tt := range tests {
		result := domainMatchesService(tt.domain, tt.service)
		if result != tt.expected {
			t.Errorf("domainMatchesService(%s, %s) = %v, want %v",
				tt.domain, tt.service, result, tt.expected)
		}
	}
}
