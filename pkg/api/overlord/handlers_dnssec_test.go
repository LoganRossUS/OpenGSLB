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

func TestHandleDNSSECStatus_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDNSSECStatus, http.MethodGet, "/api/dnssec/status", "")
	assertStatus(t, rr, http.StatusOK)

	var resp DNSSECStatus
	decodeJSON(t, rr, &resp)

	if resp.Keys == nil {
		t.Error("expected non-nil keys")
	}
	if resp.DSRecords == nil {
		t.Error("expected non-nil DS records")
	}
}

func TestHandleDNSSECStatus_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDNSSECStatus, http.MethodPost, "/api/dnssec/status", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleDNSSECKeys_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDNSSECKeys, http.MethodGet, "/api/dnssec/keys", "")
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleDNSSECKeysGenerate_POST(t *testing.T) {
	h := testHandlers()

	body := `{
		"zone": "example.com",
		"algorithm": "RSASHA256"
	}`

	rr := makeRequest(t, h.handleDNSSECKeysGenerate, http.MethodPost, "/api/dnssec/keys/generate", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp DNSKeyResponse
	decodeJSON(t, rr, &resp)

	if resp.Key.ID == "" {
		t.Error("expected non-empty key ID")
	}
	if resp.Key.Zone != "example.com" {
		t.Errorf("expected zone 'example.com', got '%s'", resp.Key.Zone)
	}
	if resp.Key.Algorithm != "RSASHA256" {
		t.Errorf("expected algorithm 'RSASHA256', got '%s'", resp.Key.Algorithm)
	}
}

func TestHandleDNSSECKeysGenerate_POST_MissingZone(t *testing.T) {
	h := testHandlers()

	body := `{
		"algorithm": "RSASHA256"
	}`

	rr := makeRequest(t, h.handleDNSSECKeysGenerate, http.MethodPost, "/api/dnssec/keys/generate", body)
	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "MISSING_FIELD" {
		t.Errorf("expected code 'MISSING_FIELD', got '%s'", resp.Code)
	}
}

func TestHandleDNSSECKeys_POST_DefaultAlgorithm(t *testing.T) {
	h := testHandlers()

	body := `{
		"zone": "example.com"
	}`

	// When algorithm is not provided, it defaults to ECDSAP256SHA256
	rr := makeRequest(t, h.handleDNSSECKeysGenerate, http.MethodPost, "/api/dnssec/keys/generate", body)
	assertStatus(t, rr, http.StatusCreated)

	var resp DNSKeyResponse
	decodeJSON(t, rr, &resp)

	if resp.Key.Algorithm != "ECDSAP256SHA256" {
		t.Errorf("expected default algorithm 'ECDSAP256SHA256', got '%s'", resp.Key.Algorithm)
	}
}

func TestHandleDNSSECKeys_POST_InvalidJSON(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDNSSECKeysGenerate, http.MethodPost, "/api/dnssec/keys/generate", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestHandleDNSSECKeys_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDNSSECKeys, http.MethodDelete, "/api/dnssec/keys", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleDNSSECKeyByID_GET_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getDNSSECKey(w, r, "nonexistent")
	}, http.MethodGet, "/api/dnssec/keys/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "KEY_NOT_FOUND" {
		t.Errorf("expected code 'KEY_NOT_FOUND', got '%s'", resp.Code)
	}
}

func TestHandleDNSSECKeyByID_DELETE_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteDNSSECKey(w, r, "nonexistent")
	}, http.MethodDelete, "/api/dnssec/keys/nonexistent", "")

	assertStatus(t, rr, http.StatusNotFound)
}

func TestHandleDNSSECKeyByID_PUT_NotFound(t *testing.T) {
	h := testHandlers()

	body := `{"zone": "updated.example.com"}`
	rr := makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.updateDNSSECKey(w, r, "nonexistent")
	}, http.MethodPut, "/api/dnssec/keys/nonexistent", body)

	assertStatus(t, rr, http.StatusNotFound)
}

func TestHandleDNSSECSync_POST(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDNSSECSync, http.MethodPost, "/api/dnssec/sync", "")
	assertStatus(t, rr, http.StatusOK)

	var resp DNSSECSyncResponse
	decodeJSON(t, rr, &resp)

	if resp.Status == "" {
		t.Error("expected non-empty status")
	}
}

func TestHandleDNSSECSync_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleDNSSECSync, http.MethodGet, "/api/dnssec/sync", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestDNSSECKeyCreateAndDelete(t *testing.T) {
	h := testHandlers()

	// Create key
	body := `{
		"zone": "test.example.com",
		"algorithm": "ECDSAP256SHA256"
	}`

	rr := makeRequest(t, h.handleDNSSECKeysGenerate, http.MethodPost, "/api/dnssec/keys/generate", body)
	assertStatus(t, rr, http.StatusCreated)

	var createResp DNSKeyResponse
	decodeJSON(t, rr, &createResp)

	keyID := createResp.Key.ID

	// Get key
	rr = makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getDNSSECKey(w, r, keyID)
	}, http.MethodGet, "/api/dnssec/keys/"+keyID, "")

	assertStatus(t, rr, http.StatusOK)

	// Delete key
	rr = makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.deleteDNSSECKey(w, r, keyID)
	}, http.MethodDelete, "/api/dnssec/keys/"+keyID, "")

	assertStatus(t, rr, http.StatusOK)

	// Verify deleted
	rr = makeRequest(t, func(w http.ResponseWriter, r *http.Request) {
		h.getDNSSECKey(w, r, keyID)
	}, http.MethodGet, "/api/dnssec/keys/"+keyID, "")

	assertStatus(t, rr, http.StatusNotFound)
}
