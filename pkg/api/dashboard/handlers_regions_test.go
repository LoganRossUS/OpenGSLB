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

func TestHandleRegions_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleRegions, http.MethodGet, "/api/regions", "")
	assertStatus(t, rr, http.StatusOK)

	var resp RegionsResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Regions) != 2 {
		t.Errorf("expected 2 regions, got %d", len(resp.Regions))
	}

	// Verify us-east-1 region
	found := false
	for _, r := range resp.Regions {
		if r.Name == "us-east-1" {
			found = true
			if len(r.Servers) != 2 {
				t.Errorf("expected 2 servers in us-east-1, got %d", len(r.Servers))
			}
		}
	}
	if !found {
		t.Error("expected to find region 'us-east-1'")
	}
}

func TestHandleRegions_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"name": "ap-southeast-1",
		"healthCheck": {
			"type": "http",
			"path": "/health"
		}
	}`

	rr := makeRequest(t, h.handleRegions, http.MethodPost, "/api/regions", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp RegionResponse
	decodeJSON(t, rr, &resp)

	if resp.Region.Name != "ap-southeast-1" {
		t.Errorf("expected name 'ap-southeast-1', got '%s'", resp.Region.Name)
	}
}

func TestHandleRegions_POST_MissingName(t *testing.T) {
	h := testHandlers()

	body := `{
		"healthCheck": {
			"type": "http"
		}
	}`

	rr := makeRequest(t, h.handleRegions, http.MethodPost, "/api/regions", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "MISSING_FIELD" {
		t.Errorf("expected code 'MISSING_FIELD', got '%s'", resp.Code)
	}
}

func TestHandleRegions_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleRegions, http.MethodPost, "/api/regions", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleRegions_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleRegions, http.MethodDelete, "/api/regions", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleRegionByName_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getRegion(w, r, "us-east-1")
	}, http.MethodGet, "/api/regions/us-east-1", "")

	assertStatus(t, rr, http.StatusOK)

	var resp RegionResponse
	decodeJSON(t, rr, &resp)

	if resp.Region.Name != "us-east-1" {
		t.Errorf("expected name 'us-east-1', got '%s'", resp.Region.Name)
	}
}

func TestHandleRegionByName_GET_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getRegion(w, r, "nonexistent")
	}, http.MethodGet, "/api/regions/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "REGION_NOT_FOUND" {
		t.Errorf("expected code 'REGION_NOT_FOUND', got '%s'", resp.Code)
	}
}

func TestHandleRegionByName_PUT(t *testing.T) {
	h := testHandlers()

	body := `{
		"healthCheck": {
			"type": "tcp",
			"timeout": 10
		}
	}`

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.updateRegion(w, r, "us-east-1")
	}, http.MethodPut, "/api/regions/us-east-1", body)

	assertStatus(t, rr, http.StatusOK)

	var resp RegionResponse
	decodeJSON(t, rr, &resp)

	if resp.Region.Name != "us-east-1" {
		t.Errorf("expected name 'us-east-1', got '%s'", resp.Region.Name)
	}
}

func TestHandleRegionByName_PUT_NotFound(t *testing.T) {
	h := testHandlers()

	body := `{"healthCheck": {"type": "tcp"}}`

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.updateRegion(w, r, "nonexistent")
	}, http.MethodPut, "/api/regions/nonexistent", body)

	assertStatus(t, rr, http.StatusNotFound)
}

func TestHandleRegionByName_DELETE(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteRegion(w, r, "us-east-1")
	}, http.MethodDelete, "/api/regions/us-east-1", "")

	assertStatus(t, rr, http.StatusOK)

	var resp SuccessResponse
	decodeJSON(t, rr, &resp)

	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHandleRegionByName_DELETE_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteRegion(w, r, "nonexistent")
	}, http.MethodDelete, "/api/regions/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)
}

func TestGetRegionHealth(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getRegionHealth(w, r, "us-east-1")
	}, http.MethodGet, "/api/regions/us-east-1/health", "")

	assertStatus(t, rr, http.StatusOK)

	var resp RegionHealthResponse
	decodeJSON(t, rr, &resp)

	if resp.HealthSummary.Region != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got '%s'", resp.HealthSummary.Region)
	}
	if resp.HealthSummary.TotalBackends != 2 {
		t.Errorf("expected 2 total backends, got %d", resp.HealthSummary.TotalBackends)
	}
}

func TestGetRegionHealth_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getRegionHealth(w, r, "nonexistent")
	}, http.MethodGet, "/api/regions/nonexistent/health", "")

	assertStatus(t, rr, http.StatusNotFound)
}

func TestGetRegionHealth_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getRegionHealth(w, r, "us-east-1")
	}, http.MethodPost, "/api/regions/us-east-1/health", "")

	assertStatus(t, rr, http.StatusMethodNotAllowed)
}
