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

func TestHandleGeoMappings_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGeoMappings, http.MethodGet, "/api/geo-mappings", "")
	assertStatus(t, rr, http.StatusOK)

	var resp GeoMappingsResponse
	decodeJSON(t, rr, &resp)

	if resp.Mappings == nil {
		t.Error("expected non-nil mappings")
	}
}

func TestHandleGeoMappings_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"cidr": "192.168.0.0/16",
		"region": "us-east-1",
		"comment": "Private network"
	}`

	rr := makeRequest(t, h.handleGeoMappings, http.MethodPost, "/api/geo-mappings", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp GeoMappingResponse
	decodeJSON(t, rr, &resp)

	if resp.Mapping.ID == "" {
		t.Error("expected non-empty mapping ID")
	}
	if resp.Mapping.CIDR != "192.168.0.0/16" {
		t.Errorf("expected CIDR '192.168.0.0/16', got '%s'", resp.Mapping.CIDR)
	}
	if resp.Mapping.Region != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got '%s'", resp.Mapping.Region)
	}
}

func TestHandleGeoMappings_POST_MissingCIDR(t *testing.T) {
	h := testHandlers()

	body := `{
		"region": "us-east-1"
	}`

	rr := makeRequest(t, h.handleGeoMappings, http.MethodPost, "/api/geo-mappings", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "MISSING_FIELD" {
		t.Errorf("expected code 'MISSING_FIELD', got '%s'", resp.Code)
	}
}

func TestHandleGeoMappings_POST_MissingRegion(t *testing.T) {
	h := testHandlers()

	body := `{
		"cidr": "192.168.0.0/16"
	}`

	rr := makeRequest(t, h.handleGeoMappings, http.MethodPost, "/api/geo-mappings", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleGeoMappings_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGeoMappings, http.MethodPost, "/api/geo-mappings", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleGeoMappings_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGeoMappings, http.MethodDelete, "/api/geo-mappings", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleGeoMappingByID_GET_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getGeoMapping(w, r, "nonexistent")
	}, http.MethodGet, "/api/geo-mappings/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "MAPPING_NOT_FOUND" {
		t.Errorf("expected code 'MAPPING_NOT_FOUND', got '%s'", resp.Code)
	}
}

func TestHandleGeoMappingByID_PUT_NotFound(t *testing.T) {
	h := testHandlers()

	body := `{"region": "eu-west-1"}`

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.updateGeoMapping(w, r, "nonexistent")
	}, http.MethodPut, "/api/geo-mappings/nonexistent", body)

	assertStatus(t, rr, http.StatusNotFound)
}

func TestHandleGeoMappingByID_DELETE_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteGeoMapping(w, r, "nonexistent")
	}, http.MethodDelete, "/api/geo-mappings/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)
}

func TestHandleGeolocationConfig_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGeolocationConfig, http.MethodGet, "/api/geolocation/config", "")
	assertStatus(t, rr, http.StatusOK)

	var resp GeoConfigResponse
	decodeJSON(t, rr, &resp)

	if resp.Config.DefaultRegion != "us-east-1" {
		t.Errorf("expected default region 'us-east-1', got '%s'", resp.Config.DefaultRegion)
	}
	if !resp.Config.ECSEnabled {
		t.Error("expected ECS enabled")
	}
}

func TestHandleGeolocationConfig_PUT(t *testing.T) {
	h := testHandlers()

	body := `{
		"defaultRegion": "eu-west-1",
		"ecsEnabled": false
	}`

	rr := makeRequest(t, h.handleGeolocationConfig, http.MethodPut, "/api/geolocation/config", body)
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleGeolocationConfig_PUT_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGeolocationConfig, http.MethodPut, "/api/geolocation/config", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleGeolocationConfig_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGeolocationConfig, http.MethodDelete, "/api/geolocation/config", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleGeolocationLookup_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"ip": "8.8.8.8"
	}`

	rr := makeRequest(t, h.handleGeolocationLookup, http.MethodPost, "/api/geolocation/lookup", body)
	assertStatus(t, rr, http.StatusOK)

	var resp GeoLookupResponse
	decodeJSON(t, rr, &resp)

	if resp.Region == "" {
		t.Error("expected non-empty region")
	}
}

func TestHandleGeolocationLookup_POST_MissingIP(t *testing.T) {
	h := testHandlers()

	body := `{}`

	rr := makeRequest(t, h.handleGeolocationLookup, http.MethodPost, "/api/geolocation/lookup", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "MISSING_FIELD" {
		t.Errorf("expected code 'MISSING_FIELD', got '%s'", resp.Code)
	}
}

func TestHandleGeolocationLookup_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGeolocationLookup, http.MethodPost, "/api/geolocation/lookup", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleGeolocationLookup_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleGeolocationLookup, http.MethodGet, "/api/geolocation/lookup", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestGeoMappingCreateUpdateDelete(t *testing.T) {
	h := testHandlers()

	// Create mapping
	body := `{
		"cidr": "10.0.0.0/8",
		"region": "us-west-2",
		"comment": "test"
	}`

	rr := makeRequest(t, h.handleGeoMappings, http.MethodPost, "/api/geo-mappings", body)
	assertStatus(t, rr, http.StatusCreated)

	var createResp GeoMappingResponse
	decodeJSON(t, rr, &createResp)

	mappingID := createResp.Mapping.ID

	// Update mapping
	updateBody := `{"region": "us-east-1"}`
	rr = makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.updateGeoMapping(w, r, mappingID)
	}, http.MethodPut, "/api/geo-mappings/"+mappingID, updateBody)

	assertStatus(t, rr, http.StatusOK)

	var updateResp GeoMappingResponse
	decodeJSON(t, rr, &updateResp)

	if updateResp.Mapping.Region != "us-east-1" {
		t.Errorf("expected updated region 'us-east-1', got '%s'", updateResp.Mapping.Region)
	}

	// Delete mapping
	rr = makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteGeoMapping(w, r, mappingID)
	}, http.MethodDelete, "/api/geo-mappings/"+mappingID, "")

	assertStatus(t, rr, http.StatusOK)

	// Verify deleted
	rr = makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getGeoMapping(w, r, mappingID)
	}, http.MethodGet, "/api/geo-mappings/"+mappingID, "")

	assertStatus(t, rr, http.StatusNotFound)
}
