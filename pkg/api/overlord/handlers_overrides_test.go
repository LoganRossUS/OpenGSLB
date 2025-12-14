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

func TestHandleOverrides_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleOverrides, http.MethodGet, "/api/overrides", "")
	assertStatus(t, rr, http.StatusOK)

	var resp OverridesResponse
	decodeJSON(t, rr, &resp)

	if resp.Overrides == nil {
		t.Error("expected non-nil overrides")
	}
}

func TestHandleOverrides_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"service": "web",
		"address": "10.0.1.10:80",
		"healthy": false,
		"reason": "maintenance",
		"source": "api",
		"authority": "admin@example.com"
	}`

	rr := makeRequest(t, h.handleOverrides, http.MethodPost, "/api/overrides", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp OverrideResponse
	decodeJSON(t, rr, &resp)

	if resp.Override.ID == "" {
		t.Error("expected non-empty override ID")
	}
	if resp.Override.Service != "web" {
		t.Errorf("expected service 'web', got '%s'", resp.Override.Service)
	}
	if resp.Override.Healthy {
		t.Error("expected healthy=false")
	}
}

func TestHandleOverrides_POST_MissingService(t *testing.T) {
	h := testHandlers()

	body := `{
		"address": "10.0.1.10:80",
		"healthy": false
	}`

	rr := makeRequest(t, h.handleOverrides, http.MethodPost, "/api/overrides", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "MISSING_FIELD" {
		t.Errorf("expected code 'MISSING_FIELD', got '%s'", resp.Code)
	}
}

func TestHandleOverrides_POST_MissingAddress(t *testing.T) {
	h := testHandlers()

	body := `{
		"service": "web",
		"healthy": false
	}`

	rr := makeRequest(t, h.handleOverrides, http.MethodPost, "/api/overrides", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleOverrides_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleOverrides, http.MethodPost, "/api/overrides", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleOverrides_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleOverrides, http.MethodDelete, "/api/overrides", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleOverrideByID_DELETE_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteOverride(w, r, "nonexistent")
	}, http.MethodDelete, "/api/overrides/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)
}

func TestOverrideCreateAndDelete(t *testing.T) {
	h := testHandlers()

	// Create override
	body := `{
		"service": "web",
		"address": "10.0.1.10:80",
		"healthy": false,
		"reason": "test"
	}`

	rr := makeRequest(t, h.handleOverrides, http.MethodPost, "/api/overrides", body)
	assertStatus(t, rr, http.StatusCreated)

	var createResp OverrideResponse
	decodeJSON(t, rr, &createResp)

	overrideID := createResp.Override.ID

	// Verify override is in the list
	rr = makeRequest(t, h.handleOverrides, http.MethodGet, "/api/overrides", "")
	assertStatus(t, rr, http.StatusOK)

	var listResp OverridesResponse
	decodeJSON(t, rr, &listResp)

	found := false
	for _, o := range listResp.Overrides {
		if o.ID == overrideID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("override %s not found in list", overrideID)
	}

	// Delete override
	rr = makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteOverride(w, r, overrideID)
	}, http.MethodDelete, "/api/overrides/"+overrideID, "")

	assertStatus(t, rr, http.StatusOK)

	// Verify deleted (not in list)
	rr = makeRequest(t, h.handleOverrides, http.MethodGet, "/api/overrides", "")
	assertStatus(t, rr, http.StatusOK)

	decodeJSON(t, rr, &listResp)

	for _, o := range listResp.Overrides {
		if o.ID == overrideID {
			t.Errorf("override %s should have been deleted", overrideID)
		}
	}
}
