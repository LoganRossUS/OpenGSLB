// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func setupTestHandlers() (*APIHandlers, *Registry) {
	registry := NewRegistry(RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}, nil)

	handlers := NewAPIHandlers(registry, nil)
	return handlers, registry
}

func TestAPIHandlers_HandleBackends(t *testing.T) {
	handlers, registry := setupTestHandlers()

	// Register some backends
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	_ = registry.Register("agent-2", "us-west", "web", "192.168.1.2", 80, 100, false)
	_ = registry.Register("agent-3", "us-east", "api", "192.168.2.1", 8080, 50, true)

	// Test GET /api/v1/overwatch/backends
	req := httptest.NewRequest("GET", "/api/v1/overwatch/backends", nil)
	w := httptest.NewRecorder()

	handlers.HandleBackends(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response BackendsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(response.Backends) != 3 {
		t.Errorf("expected 3 backends, got %d", len(response.Backends))
	}
}

func TestAPIHandlers_HandleBackends_Filtering(t *testing.T) {
	handlers, registry := setupTestHandlers()

	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	_ = registry.Register("agent-2", "us-west", "web", "192.168.1.2", 80, 100, true)
	_ = registry.Register("agent-3", "us-east", "api", "192.168.2.1", 8080, 50, true)

	// Filter by service
	req := httptest.NewRequest("GET", "/api/v1/overwatch/backends?service=web", nil)
	w := httptest.NewRecorder()

	handlers.HandleBackends(w, req)

	var response BackendsResponse
	_ = json.Unmarshal(w.Body.Bytes(), &response)

	if len(response.Backends) != 2 {
		t.Errorf("expected 2 web backends, got %d", len(response.Backends))
	}

	// Filter by region
	req = httptest.NewRequest("GET", "/api/v1/overwatch/backends?region=us-east", nil)
	w = httptest.NewRecorder()

	handlers.HandleBackends(w, req)

	_ = json.Unmarshal(w.Body.Bytes(), &response)

	if len(response.Backends) != 2 {
		t.Errorf("expected 2 us-east backends, got %d", len(response.Backends))
	}
}

func TestAPIHandlers_HandleBackendOverride_Set(t *testing.T) {
	handlers, registry := setupTestHandlers()

	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, false)

	// Set override
	body := bytes.NewBufferString(`{"healthy": true, "reason": "maintenance bypass"}`)
	req := httptest.NewRequest("POST", "/api/v1/overwatch/backends/web/192.168.1.1/80/override", body)
	req.Header.Set("X-User", "admin")
	w := httptest.NewRecorder()

	handlers.HandleBackendOverride(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify override was set
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.OverrideStatus == nil || !*backend.OverrideStatus {
		t.Error("override should be set to true")
	}
	if backend.OverrideReason != "maintenance bypass" {
		t.Errorf("expected reason 'maintenance bypass', got %q", backend.OverrideReason)
	}
	if backend.OverrideBy != "admin" {
		t.Errorf("expected override by 'admin', got %q", backend.OverrideBy)
	}
}

func TestAPIHandlers_HandleBackendOverride_Clear(t *testing.T) {
	handlers, registry := setupTestHandlers()

	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, false)
	_ = registry.SetOverride("web", "192.168.1.1", 80, true, "test", "admin")

	// Clear override
	req := httptest.NewRequest("DELETE", "/api/v1/overwatch/backends/web/192.168.1.1/80/override", nil)
	w := httptest.NewRecorder()

	handlers.HandleBackendOverride(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify override was cleared
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.OverrideStatus != nil {
		t.Error("override should be cleared")
	}
}

func TestAPIHandlers_HandleBackendOverride_NotFound(t *testing.T) {
	handlers, _ := setupTestHandlers()

	body := bytes.NewBufferString(`{"healthy": true, "reason": "test"}`)
	req := httptest.NewRequest("POST", "/api/v1/overwatch/backends/nonexistent/192.168.1.1/80/override", body)
	w := httptest.NewRecorder()

	handlers.HandleBackendOverride(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestAPIHandlers_HandleStats(t *testing.T) {
	handlers, registry := setupTestHandlers()

	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	_ = registry.Register("agent-2", "us-west", "web", "192.168.1.2", 80, 100, false)
	_ = registry.Register("agent-3", "us-east", "api", "192.168.2.1", 8080, 50, true)
	_ = registry.SetOverride("web", "192.168.1.2", 80, true, "bypass", "admin")

	req := httptest.NewRequest("GET", "/api/v1/overwatch/stats", nil)
	w := httptest.NewRecorder()

	handlers.HandleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var stats StatsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if stats.TotalBackends != 3 {
		t.Errorf("expected 3 total backends, got %d", stats.TotalBackends)
	}
	if stats.HealthyBackends != 3 { // 2 agent healthy + 1 override
		t.Errorf("expected 3 healthy backends, got %d", stats.HealthyBackends)
	}
	if stats.ActiveOverrides != 1 {
		t.Errorf("expected 1 active override, got %d", stats.ActiveOverrides)
	}
	if stats.ActiveAgents != 3 {
		t.Errorf("expected 3 active agents, got %d", stats.ActiveAgents)
	}
	if stats.UniqueServices != 2 {
		t.Errorf("expected 2 unique services, got %d", stats.UniqueServices)
	}
	if stats.BackendsByService["web"] != 2 {
		t.Errorf("expected 2 web backends, got %d", stats.BackendsByService["web"])
	}
	if stats.BackendsByRegion["us-east"] != 2 {
		t.Errorf("expected 2 us-east backends, got %d", stats.BackendsByRegion["us-east"])
	}
}

func TestAPIHandlers_MethodNotAllowed(t *testing.T) {
	handlers, _ := setupTestHandlers()

	// Test POST on GET-only endpoint
	req := httptest.NewRequest("POST", "/api/v1/overwatch/backends", nil)
	w := httptest.NewRecorder()

	handlers.HandleBackends(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}

	// Test GET on POST-only endpoint
	req = httptest.NewRequest("GET", "/api/v1/overwatch/stats", nil)
	w = httptest.NewRecorder()

	handlers.HandleStats(w, req)

	// GET is allowed for stats
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
