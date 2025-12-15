// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoveryHandlers_HandleAPIRoot(t *testing.T) {
	h := NewDiscoveryHandlers()
	h.SetHasSimpleHealth(true)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"GET /api", http.MethodGet, "/api", http.StatusOK},
		{"GET /api/", http.MethodGet, "/api/", http.StatusOK},
		{"POST /api", http.MethodPost, "/api", http.StatusMethodNotAllowed},
		{"GET /api/unknown", http.MethodGet, "/api/unknown", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			h.HandleAPIRoot(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestDiscoveryHandlers_HandleAPIRoot_Response(t *testing.T) {
	h := NewDiscoveryHandlers()
	h.SetHasSimpleHealth(true)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rr := httptest.NewRecorder()
	h.HandleAPIRoot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp APIVersionResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Versions) < 1 {
		t.Errorf("expected at least 1 version, got %d", len(resp.Versions))
	}

	// Check that v1 is present
	foundV1 := false
	foundHealth := false
	for _, v := range resp.Versions {
		if v.Path == "/api/v1" {
			foundV1 = true
		}
		if v.Path == "/api/health" {
			foundHealth = true
		}
	}

	if !foundV1 {
		t.Error("expected /api/v1 in versions list")
	}
	if !foundHealth {
		t.Error("expected /api/health in versions list when simple health is enabled")
	}

	if resp.GeneratedAt.IsZero() {
		t.Error("expected generated_at to be set")
	}
}

func TestDiscoveryHandlers_HandleV1Root(t *testing.T) {
	h := NewDiscoveryHandlers()
	h.SetHasHealth(true)
	h.SetHasDomains(true)
	h.SetHasServers(true)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"GET /api/v1", http.MethodGet, "/api/v1", http.StatusOK},
		{"GET /api/v1/", http.MethodGet, "/api/v1/", http.StatusOK},
		{"POST /api/v1", http.MethodPost, "/api/v1", http.StatusMethodNotAllowed},
		{"GET /api/v1/unknown", http.MethodGet, "/api/v1/unknown", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			h.HandleV1Root(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestDiscoveryHandlers_HandleV1Root_Response(t *testing.T) {
	h := NewDiscoveryHandlers()
	h.SetHasHealth(true)
	h.SetHasDomains(true)
	h.SetHasServers(true)
	h.SetHasRegions(true)
	h.SetHasOverrides(true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	rr := httptest.NewRecorder()
	h.HandleV1Root(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp APIDiscoveryResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check for expected endpoints
	expectedPaths := map[string]bool{
		"/api/v1/health":    false,
		"/api/v1/ready":     false,
		"/api/v1/live":      false,
		"/api/v1/domains":   false,
		"/api/v1/servers":   false,
		"/api/v1/regions":   false,
		"/api/v1/overrides": false,
	}

	for _, ep := range resp.Endpoints {
		if _, exists := expectedPaths[ep.Path]; exists {
			expectedPaths[ep.Path] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("expected %s in endpoints list", path)
		}
	}

	if resp.GeneratedAt.IsZero() {
		t.Error("expected generated_at to be set")
	}
}

func TestDiscoveryHandlers_HandleV1Root_ConditionalEndpoints(t *testing.T) {
	// Test with no handlers registered
	h := NewDiscoveryHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	rr := httptest.NewRecorder()
	h.HandleV1Root(rr, req)

	var resp APIDiscoveryResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should still have ready and live endpoints
	foundReady := false
	foundLive := false
	foundDomains := false

	for _, ep := range resp.Endpoints {
		switch ep.Path {
		case "/api/v1/ready":
			foundReady = true
		case "/api/v1/live":
			foundLive = true
		case "/api/v1/domains":
			foundDomains = true
		}
	}

	if !foundReady {
		t.Error("ready endpoint should always be present")
	}
	if !foundLive {
		t.Error("live endpoint should always be present")
	}
	if foundDomains {
		t.Error("domains should not be present when hasDomains is false")
	}
}

func TestDiscoveryHandlers_HandleHealthRoot(t *testing.T) {
	h := NewDiscoveryHandlers()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"GET /api/v1/health", http.MethodGet, "/api/v1/health", http.StatusOK},
		{"GET /api/v1/health/", http.MethodGet, "/api/v1/health/", http.StatusOK},
		{"POST /api/v1/health", http.MethodPost, "/api/v1/health", http.StatusMethodNotAllowed},
		{"GET /api/v1/health/unknown", http.MethodGet, "/api/v1/health/unknown", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			h.HandleHealthRoot(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestDiscoveryHandlers_HandleHealthRoot_Response(t *testing.T) {
	h := NewDiscoveryHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	h.HandleHealthRoot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp APIDiscoveryResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	expectedPaths := map[string]bool{
		"/api/v1/health/servers": false,
		"/api/v1/health/regions": false,
	}

	for _, ep := range resp.Endpoints {
		if _, exists := expectedPaths[ep.Path]; exists {
			expectedPaths[ep.Path] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("expected %s in endpoints list", path)
		}
	}
}

func TestDiscoveryHandlers_HandleGeoRoot(t *testing.T) {
	h := NewDiscoveryHandlers()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"GET /api/v1/geo", http.MethodGet, "/api/v1/geo", http.StatusOK},
		{"GET /api/v1/geo/", http.MethodGet, "/api/v1/geo/", http.StatusOK},
		{"POST /api/v1/geo", http.MethodPost, "/api/v1/geo", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			h.HandleGeoRoot(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestDiscoveryHandlers_HandleGeoRoot_Response(t *testing.T) {
	h := NewDiscoveryHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo", nil)
	rr := httptest.NewRecorder()
	h.HandleGeoRoot(rr, req)

	var resp APIDiscoveryResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	expectedPaths := map[string]bool{
		"/api/v1/geo/mappings": false,
		"/api/v1/geo/test":     false,
	}

	for _, ep := range resp.Endpoints {
		if _, exists := expectedPaths[ep.Path]; exists {
			expectedPaths[ep.Path] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("expected %s in endpoints list", path)
		}
	}
}

func TestDiscoveryHandlers_HandleOverwatchRoot(t *testing.T) {
	h := NewDiscoveryHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overwatch", nil)
	rr := httptest.NewRecorder()
	h.HandleOverwatchRoot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp APIDiscoveryResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	expectedPaths := map[string]bool{
		"/api/v1/overwatch/backends": false,
		"/api/v1/overwatch/stats":    false,
		"/api/v1/overwatch/validate": false,
	}

	for _, ep := range resp.Endpoints {
		if _, exists := expectedPaths[ep.Path]; exists {
			expectedPaths[ep.Path] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("expected %s in endpoints list", path)
		}
	}
}

func TestDiscoveryHandlers_HandleDNSSECRoot(t *testing.T) {
	h := NewDiscoveryHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dnssec", nil)
	rr := httptest.NewRecorder()
	h.HandleDNSSECRoot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp APIDiscoveryResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	expectedPaths := map[string]bool{
		"/api/v1/dnssec/status": false,
		"/api/v1/dnssec/keys":   false,
		"/api/v1/dnssec/ds":     false,
		"/api/v1/dnssec/sync":   false,
	}

	for _, ep := range resp.Endpoints {
		if _, exists := expectedPaths[ep.Path]; exists {
			expectedPaths[ep.Path] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("expected %s in endpoints list", path)
		}
	}
}

func TestDiscoveryHandlers_EndpointMethods(t *testing.T) {
	h := NewDiscoveryHandlers()
	h.SetHasDomains(true)
	h.SetHasServers(true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	rr := httptest.NewRecorder()
	h.HandleV1Root(rr, req)

	var resp APIDiscoveryResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check that endpoints have methods defined
	for _, ep := range resp.Endpoints {
		if ep.Path == "/api/v1/domains" || ep.Path == "/api/v1/servers" {
			if len(ep.Methods) == 0 {
				t.Errorf("expected methods for %s", ep.Path)
			}
			// Should have GET and POST
			hasGet := false
			hasPost := false
			for _, m := range ep.Methods {
				if m == "GET" {
					hasGet = true
				}
				if m == "POST" {
					hasPost = true
				}
			}
			if !hasGet || !hasPost {
				t.Errorf("expected GET and POST methods for %s, got %v", ep.Path, ep.Methods)
			}
		}
	}
}
