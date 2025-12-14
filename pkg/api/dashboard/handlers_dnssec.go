// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// DNSSEC key store
var (
	dnssecKeyStore   = make(map[string]DNSKey)
	dnssecKeyStoreMu sync.RWMutex
	dnssecLastSync   *time.Time
	dnssecSyncStatus = "idle"
)

// handleDNSSECStatus handles GET /api/dnssec/status
func (h *Handlers) handleDNSSECStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Get keys
	keys := make([]DNSKey, 0)
	dnssecKeyStoreMu.RLock()
	for _, key := range dnssecKeyStore {
		keys = append(keys, key)
	}
	dnssecKeyStoreMu.RUnlock()

	// Generate DS records from KSK keys
	dsRecords := make([]DSRecord, 0)
	for _, key := range keys {
		if key.KeyType == "KSK" {
			dsRecords = append(dsRecords, DSRecord{
				KeyTag:     key.KeyTag,
				Algorithm:  13, // ECDSAP256SHA256
				DigestType: 2,  // SHA-256
				Digest:     fmt.Sprintf("placeholder-digest-for-%s", key.ID),
			})
		}
	}

	status := DNSSECStatus{
		Enabled:    cfg.Overwatch.DNSSEC.Enabled,
		Keys:       keys,
		DSRecords:  dsRecords,
		SyncStatus: dnssecSyncStatus,
	}

	writeJSON(w, http.StatusOK, status)
}

// handleDNSSECKeys handles GET /api/dnssec/keys
func (h *Handlers) handleDNSSECKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	keys := make([]DNSKey, 0)
	dnssecKeyStoreMu.RLock()
	for _, key := range dnssecKeyStore {
		keys = append(keys, key)
	}
	dnssecKeyStoreMu.RUnlock()

	writeJSON(w, http.StatusOK, map[string][]DNSKey{"keys": keys})
}

// handleDNSSECKeysGenerate handles POST /api/dnssec/keys/generate
func (h *Handlers) handleDNSSECKeysGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	var req DNSKeyGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if req.Zone == "" {
		writeError(w, http.StatusBadRequest, "zone is required", "MISSING_FIELD")
		return
	}

	// Default algorithm
	if req.Algorithm == "" {
		req.Algorithm = "ECDSAP256SHA256"
	}

	// Validate algorithm
	validAlgorithms := map[string]bool{
		"RSASHA256":       true,
		"ECDSAP256SHA256": true,
		"ED25519":         true,
	}
	if !validAlgorithms[req.Algorithm] {
		writeError(w, http.StatusBadRequest, "invalid algorithm", "INVALID_ALGORITHM")
		return
	}

	// Generate key ID and key tag
	keyIDBytes := make([]byte, 4)
	_, _ = rand.Read(keyIDBytes)
	keyID := hex.EncodeToString(keyIDBytes)
	keyTag := uint16(keyIDBytes[0])<<8 | uint16(keyIDBytes[1])

	now := time.Now()
	key := DNSKey{
		ID:          keyID,
		Zone:        req.Zone,
		Algorithm:   req.Algorithm,
		KeyTag:      keyTag,
		PublicKey:   "placeholder-public-key",
		CreatedAt:   now,
		ActivatedAt: &now,
		KeyType:     "ZSK", // Zone Signing Key by default
	}

	// Store key
	dnssecKeyStoreMu.Lock()
	dnssecKeyStore[keyID] = key
	dnssecKeyStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryDNSSEC, keyID,
		fmt.Sprintf("Generated DNSSEC key for zone %s", req.Zone),
		map[string]interface{}{
			"zone":      req.Zone,
			"algorithm": req.Algorithm,
			"keyTag":    keyTag,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusCreated, DNSKeyResponse{Key: key})
}

// handleDNSSECKeyByID handles GET, PUT, DELETE /api/dnssec/keys/:id
func (h *Handlers) handleDNSSECKeyByID(w http.ResponseWriter, r *http.Request) {
	id := parsePathParam(r.URL.Path, "/api/dnssec/keys/")
	if id == "" || id == "generate" {
		// Handle generate endpoint
		if id == "generate" {
			h.handleDNSSECKeysGenerate(w, r)
			return
		}
		writeError(w, http.StatusBadRequest, "key ID is required", "MISSING_PARAM")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getDNSSECKey(w, r, id)
	case http.MethodPut:
		h.updateDNSSECKey(w, r, id)
	case http.MethodDelete:
		h.deleteDNSSECKey(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getDNSSECKey handles GET /api/dnssec/keys/:id
func (h *Handlers) getDNSSECKey(w http.ResponseWriter, r *http.Request, id string) {
	dnssecKeyStoreMu.RLock()
	key, exists := dnssecKeyStore[id]
	dnssecKeyStoreMu.RUnlock()

	if !exists {
		writeError(w, http.StatusNotFound, "DNSSEC key not found", "KEY_NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, DNSKeyResponse{Key: key})
}

// updateDNSSECKey handles PUT /api/dnssec/keys/:id
func (h *Handlers) updateDNSSECKey(w http.ResponseWriter, r *http.Request, id string) {
	var req DNSKeyGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	dnssecKeyStoreMu.Lock()
	key, exists := dnssecKeyStore[id]
	if !exists {
		dnssecKeyStoreMu.Unlock()
		writeError(w, http.StatusNotFound, "DNSSEC key not found", "KEY_NOT_FOUND")
		return
	}

	if req.Zone != "" {
		key.Zone = req.Zone
	}
	if req.Algorithm != "" {
		key.Algorithm = req.Algorithm
	}

	dnssecKeyStore[id] = key
	dnssecKeyStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryDNSSEC, id,
		fmt.Sprintf("Updated DNSSEC key %s", id),
		nil,
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, DNSKeyResponse{Key: key})
}

// deleteDNSSECKey handles DELETE /api/dnssec/keys/:id
func (h *Handlers) deleteDNSSECKey(w http.ResponseWriter, r *http.Request, id string) {
	dnssecKeyStoreMu.Lock()
	_, exists := dnssecKeyStore[id]
	if !exists {
		dnssecKeyStoreMu.Unlock()
		writeError(w, http.StatusNotFound, "DNSSEC key not found", "KEY_NOT_FOUND")
		return
	}
	delete(dnssecKeyStore, id)
	dnssecKeyStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionDelete, AuditCategoryDNSSEC, id,
		fmt.Sprintf("Deleted DNSSEC key %s", id),
		nil,
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// handleDNSSECSync handles POST /api/dnssec/sync
func (h *Handlers) handleDNSSECSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Simulate sync process
	dnssecSyncStatus = "syncing"
	now := time.Now()
	dnssecLastSync = &now

	// In a real implementation, this would sync keys with other Overwatch nodes
	go func() {
		time.Sleep(2 * time.Second)
		dnssecSyncStatus = "completed"
	}()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryDNSSEC, "sync",
		"Triggered DNSSEC key sync",
		nil,
		AuditSeverityInfo, r.RemoteAddr)

	writeJSON(w, http.StatusOK, DNSSECSyncResponse{
		Status:     "syncing",
		LastSync:   dnssecLastSync,
		SyncStatus: dnssecSyncStatus,
	})
}

// handleDNSSECSyncStatus handles GET /api/dnssec/sync/status
func (h *Handlers) handleDNSSECSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	writeJSON(w, http.StatusOK, DNSSECSyncResponse{
		Status:     dnssecSyncStatus,
		LastSync:   dnssecLastSync,
		SyncStatus: dnssecSyncStatus,
	})
}
