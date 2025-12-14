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

func TestHandlePreferences_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handlePreferences, http.MethodGet, "/api/preferences", "")
	assertStatus(t, rr, http.StatusOK)

	var resp PreferencesResponse
	decodeJSON(t, rr, &resp)

	if resp.Preferences.Theme == "" {
		t.Error("expected non-empty theme")
	}
}

func TestHandlePreferences_PUT(t *testing.T) {
	h := testHandlers()

	body := `{
		"theme": "dark",
		"language": "en",
		"defaultTTL": 120,
		"autoRefresh": true
	}`

	rr := makeRequest(t, h.handlePreferences, http.MethodPut, "/api/preferences", body)
	assertStatus(t, rr, http.StatusOK)

	var resp PreferencesResponse
	decodeJSON(t, rr, &resp)

	if resp.Preferences.Theme != "dark" {
		t.Errorf("expected theme 'dark', got '%s'", resp.Preferences.Theme)
	}
}

func TestHandlePreferences_PUT_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handlePreferences, http.MethodPut, "/api/preferences", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandlePreferences_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handlePreferences, http.MethodDelete, "/api/preferences", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleAPISettings_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleAPISettings, http.MethodGet, "/api/config/api-settings", "")
	assertStatus(t, rr, http.StatusOK)

	var resp APISettingsResponse
	decodeJSON(t, rr, &resp)

	if !resp.Config.Enabled {
		t.Error("expected enabled=true")
	}
	if resp.Config.Address != ":8080" {
		t.Errorf("expected address ':8080', got '%s'", resp.Config.Address)
	}
}

func TestHandleAPISettings_PUT(t *testing.T) {
	h := testHandlers()

	body := `{
		"enabled": true,
		"address": ":9090",
		"trustProxyHeaders": true
	}`

	rr := makeRequest(t, h.handleAPISettings, http.MethodPut, "/api/config/api-settings", body)
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleAPISettings_PUT_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleAPISettings, http.MethodPut, "/api/config/api-settings", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleAPISettings_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleAPISettings, http.MethodDelete, "/api/config/api-settings", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleValidationConfig_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleValidationConfig, http.MethodGet, "/api/config/validation", "")
	assertStatus(t, rr, http.StatusOK)

	var resp ValidationConfigResponse
	decodeJSON(t, rr, &resp)

	if !resp.Validation.Enabled {
		t.Error("expected enabled=true")
	}
	if resp.Validation.CheckInterval != 30 {
		t.Errorf("expected check interval 30, got %d", resp.Validation.CheckInterval)
	}
}

func TestHandleValidationConfig_PUT(t *testing.T) {
	h := testHandlers()

	body := `{
		"enabled": true,
		"checkInterval": 60,
		"checkTimeout": 10
	}`

	rr := makeRequest(t, h.handleValidationConfig, http.MethodPut, "/api/config/validation", body)
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleValidationConfig_PUT_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleValidationConfig, http.MethodPut, "/api/config/validation", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleValidationConfig_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleValidationConfig, http.MethodDelete, "/api/config/validation", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleStaleConfig_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleStaleConfig, http.MethodGet, "/api/config/stale-handling", "")
	assertStatus(t, rr, http.StatusOK)

	var resp StaleConfigResponse
	decodeJSON(t, rr, &resp)

	if resp.StaleConfig.Threshold != 30 {
		t.Errorf("expected threshold 30, got %d", resp.StaleConfig.Threshold)
	}
}

func TestHandleStaleConfig_PUT(t *testing.T) {
	h := testHandlers()

	body := `{
		"threshold": 60,
		"removeAfter": 600
	}`

	rr := makeRequest(t, h.handleStaleConfig, http.MethodPut, "/api/config/stale-handling", body)
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleStaleConfig_PUT_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleStaleConfig, http.MethodPut, "/api/config/stale-handling", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleStaleConfig_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleStaleConfig, http.MethodDelete, "/api/config/stale-handling", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleRoutingAlgorithms_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleRoutingAlgorithms, http.MethodGet, "/api/routing/algorithms", "")
	assertStatus(t, rr, http.StatusOK)

	var resp RoutingAlgorithmsResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Algorithms) != 5 {
		t.Errorf("expected 5 algorithms, got %d", len(resp.Algorithms))
	}

	// Verify expected algorithms
	algos := make(map[string]bool)
	for _, a := range resp.Algorithms {
		algos[a.Value] = true
	}

	expectedAlgos := []string{"latency", "geolocation", "failover", "weighted", "round-robin"}
	for _, expected := range expectedAlgos {
		if !algos[expected] {
			t.Errorf("expected algorithm '%s' to be present", expected)
		}
	}
}

func TestHandleRoutingAlgorithms_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleRoutingAlgorithms, http.MethodPost, "/api/routing/algorithms", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleRoutingTest_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"domain": "api.example.com",
		"clientIp": "192.168.1.100",
		"sourceRegion": "us-east-1"
	}`

	rr := makeRequest(t, h.handleRoutingTest, http.MethodPost, "/api/routing/test", body)
	assertStatus(t, rr, http.StatusOK)

	var resp RoutingTestResponse
	decodeJSON(t, rr, &resp)

	if resp.SelectedBackend == "" {
		t.Error("expected non-empty selected backend")
	}
	if resp.Algorithm == "" {
		t.Error("expected non-empty algorithm")
	}
}

func TestHandleRoutingTest_POST_MissingDomain(t *testing.T) {
	h := testHandlers()

	body := `{
		"clientIp": "192.168.1.100"
	}`

	rr := makeRequest(t, h.handleRoutingTest, http.MethodPost, "/api/routing/test", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "MISSING_FIELD" {
		t.Errorf("expected code 'MISSING_FIELD', got '%s'", resp.Code)
	}
}

func TestHandleRoutingTest_POST_DomainNotFound(t *testing.T) {
	h := testHandlers()

	body := `{
		"domain": "nonexistent.example.com"
	}`

	rr := makeRequest(t, h.handleRoutingTest, http.MethodPost, "/api/routing/test", body)
	assertStatus(t, rr, http.StatusNotFound)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "DOMAIN_NOT_FOUND" {
		t.Errorf("expected code 'DOMAIN_NOT_FOUND', got '%s'", resp.Code)
	}
}

func TestHandleRoutingTest_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleRoutingTest, http.MethodPost, "/api/routing/test", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleRoutingTest_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleRoutingTest, http.MethodGet, "/api/routing/test", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleHealth_GET_Config(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleHealth, http.MethodGet, "/api/health", "")
	assertStatus(t, rr, http.StatusOK)

	var resp HealthCheckResponse
	decodeJSON(t, rr, &resp)

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp.Status)
	}
}

func TestHandleHealth_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleHealth, http.MethodPost, "/api/health", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}
