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

func TestHandleHealthValidate_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"scope": "all"
	}`

	rr := makeRequest(t, h.handleHealthValidate, http.MethodPost, "/api/health/validate", body)
	assertStatus(t, rr, http.StatusOK)

	var resp ValidationStartResponse
	decodeJSON(t, rr, &resp)

	if resp.ValidationID == "" {
		t.Error("expected non-empty validation ID")
	}
	if resp.Status != "in_progress" {
		t.Errorf("expected status 'in_progress', got '%s'", resp.Status)
	}
}

func TestHandleHealthValidate_POST_DefaultScope(t *testing.T) {
	h := testHandlers()

	body := `{}`

	rr := makeRequest(t, h.handleHealthValidate, http.MethodPost, "/api/health/validate", body)
	assertStatus(t, rr, http.StatusOK)

	var resp ValidationStartResponse
	decodeJSON(t, rr, &resp)

	if resp.ValidationID == "" {
		t.Error("expected non-empty validation ID")
	}
}

func TestHandleHealthValidate_POST_UnhealthyScope(t *testing.T) {
	h := testHandlers()

	body := `{
		"scope": "unhealthy"
	}`

	rr := makeRequest(t, h.handleHealthValidate, http.MethodPost, "/api/health/validate", body)
	assertStatus(t, rr, http.StatusOK)

	var resp ValidationStartResponse
	decodeJSON(t, rr, &resp)

	if resp.ValidationID == "" {
		t.Error("expected non-empty validation ID")
	}
}

func TestHandleHealthValidate_POST_SelectedScope(t *testing.T) {
	h := testHandlers()

	body := `{
		"scope": "selected",
		"backends": ["10.0.1.10:80", "10.0.1.11:80"]
	}`

	rr := makeRequest(t, h.handleHealthValidate, http.MethodPost, "/api/health/validate", body)
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleHealthValidate_POST_InvalidScope(t *testing.T) {
	h := testHandlers()

	body := `{
		"scope": "invalid"
	}`

	rr := makeRequest(t, h.handleHealthValidate, http.MethodPost, "/api/health/validate", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "INVALID_SCOPE" {
		t.Errorf("expected code 'INVALID_SCOPE', got '%s'", resp.Code)
	}
}

func TestHandleHealthValidate_POST_WithServiceFilter(t *testing.T) {
	h := testHandlers()

	body := `{
		"scope": "all",
		"service": "web"
	}`

	rr := makeRequest(t, h.handleHealthValidate, http.MethodPost, "/api/health/validate", body)
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleHealthValidate_POST_WithRegionFilter(t *testing.T) {
	h := testHandlers()

	body := `{
		"scope": "all",
		"region": "us-east-1"
	}`

	rr := makeRequest(t, h.handleHealthValidate, http.MethodPost, "/api/health/validate", body)
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleHealthValidate_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleHealthValidate, http.MethodPost, "/api/health/validate", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleHealthValidate_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleHealthValidate, http.MethodGet, "/api/health/validate", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleValidationStatus_GET_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		// Manually set path to simulate route parsing
		r.URL.Path = "/api/health/validation/nonexistent"
		h.handleValidationStatus(w, r)
	}, http.MethodGet, "/api/health/validation/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "VALIDATION_NOT_FOUND" {
		t.Errorf("expected code 'VALIDATION_NOT_FOUND', got '%s'", resp.Code)
	}
}

func TestHandleValidationStatus_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/health/validation/test-id"
		h.handleValidationStatus(w, r)
	}, http.MethodPost, "/api/health/validation/test-id", "")

	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleHealthStatus_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleHealthStatus, http.MethodGet, "/api/health/status", "")
	assertStatus(t, rr, http.StatusOK)

	var resp HealthStatusResponse
	decodeJSON(t, rr, &resp)

	// Without registry, should return empty backends
	if resp.Backends == nil {
		t.Error("expected non-nil backends")
	}
}

func TestHandleHealthStatus_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleHealthStatus, http.MethodPost, "/api/health/status", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}
