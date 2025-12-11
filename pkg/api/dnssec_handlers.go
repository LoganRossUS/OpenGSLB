// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/dnssec"
)

// DNSSECHandlers provides HTTP handlers for DNSSEC API endpoints.
type DNSSECHandlers struct {
	keyStore   *dnssec.KeyStore
	keyManager *dnssec.KeyManager
	keySyncer  *dnssec.KeySyncer
	signer     *dnssec.Signer
	logger     *slog.Logger
	enabled    bool
}

// DNSSECHandlersConfig configures the DNSSEC API handlers.
type DNSSECHandlersConfig struct {
	KeyStore   *dnssec.KeyStore
	KeyManager *dnssec.KeyManager
	KeySyncer  *dnssec.KeySyncer
	Signer     *dnssec.Signer
	Logger     *slog.Logger
	Enabled    bool
}

// NewDNSSECHandlers creates new DNSSEC API handlers.
func NewDNSSECHandlers(config DNSSECHandlersConfig) *DNSSECHandlers {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &DNSSECHandlers{
		keyStore:   config.KeyStore,
		keyManager: config.KeyManager,
		keySyncer:  config.KeySyncer,
		signer:     config.Signer,
		logger:     logger,
		enabled:    config.Enabled,
	}
}

// HandleDS handles GET /api/v1/dnssec/ds requests.
// Returns DS records for all managed zones.
func (h *DNSSECHandlers) HandleDS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !h.enabled {
		h.writeJSON(w, http.StatusOK, DSResponse{
			Enabled: false,
			Message: "DNSSEC is disabled",
		})
		return
	}

	// Check for zone filter in query params
	zoneFilter := r.URL.Query().Get("zone")

	keys := h.keyManager.GetAllKeys()
	records := make([]DSRecord, 0, len(keys))

	for _, key := range keys {
		// Apply zone filter if specified
		if zoneFilter != "" && key.Zone != zoneFilter && key.Zone != zoneFilter+"." {
			continue
		}

		ds := key.DSRecord()
		if ds == nil {
			continue
		}

		records = append(records, DSRecord{
			Zone:         key.Zone,
			KeyTag:       ds.KeyTag,
			Algorithm:    ds.Algorithm,
			DigestType:   ds.DigestType,
			Digest:       ds.Digest,
			DSRecordText: ds.String(),
			CreatedAt:    key.CreatedAt.Format(time.RFC3339),
		})
	}

	resp := DSResponse{
		Enabled:   true,
		DSRecords: records,
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// HandleKeys handles GET /api/v1/dnssec/keys requests.
// Returns key information for key synchronization between Overwatches.
func (h *DNSSECHandlers) HandleKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !h.enabled {
		h.writeJSON(w, http.StatusOK, KeysResponse{
			Enabled: false,
			Keys:    nil,
		})
		return
	}

	keys := h.keyManager.GetAllKeys()

	resp := KeysResponse{
		Enabled: true,
		Keys:    keys,
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// HandleStatus handles GET /api/v1/dnssec/status requests.
// Returns the current DNSSEC status including key info and sync status.
func (h *DNSSECHandlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	resp := StatusResponse{
		Enabled: h.enabled,
	}

	if h.enabled && h.keyStore != nil {
		resp.Keys = h.keyStore.GetAllKeyInfo()
	}

	if h.keySyncer != nil {
		resp.Sync = h.keySyncer.GetStatus()
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// HandleSync handles POST /api/v1/dnssec/sync requests.
// Triggers an immediate key sync from all peers.
func (h *DNSSECHandlers) HandleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !h.enabled {
		h.writeError(w, http.StatusBadRequest, "DNSSEC is disabled")
		return
	}

	if h.keySyncer == nil {
		h.writeError(w, http.StatusBadRequest, "key sync is not configured")
		return
	}

	// Trigger sync
	h.keySyncer.SyncNow(r.Context())

	h.logger.Info("triggered manual DNSSEC key sync")

	h.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "key sync triggered",
	})
}

// HandleDNSSEC routes DNSSEC API requests.
func (h *DNSSECHandlers) HandleDNSSEC(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	// Expected paths:
	//   GET  /api/v1/dnssec/ds         -> DS records
	//   GET  /api/v1/dnssec/keys       -> Keys (for sync)
	//   GET  /api/v1/dnssec/status     -> Status
	//   POST /api/v1/dnssec/sync       -> Trigger sync

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/dnssec")
	path = strings.TrimPrefix(path, "/")

	switch {
	case path == "" || path == "status":
		h.HandleStatus(w, r)
	case path == "ds":
		h.HandleDS(w, r)
	case path == "keys":
		h.HandleKeys(w, r)
	case path == "sync":
		h.HandleSync(w, r)
	default:
		h.writeError(w, http.StatusNotFound, "endpoint not found")
	}
}

// writeJSON writes a JSON response with the given status code.
func (h *DNSSECHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *DNSSECHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}

// DSRecord represents a DS record in API responses.
type DSRecord struct {
	Zone         string `json:"zone"`
	KeyTag       uint16 `json:"key_tag"`
	Algorithm    uint8  `json:"algorithm"`
	DigestType   uint8  `json:"digest_type"`
	Digest       string `json:"digest"`
	DSRecordText string `json:"ds_record"`
	CreatedAt    string `json:"created_at"`
}

// DSResponse is the response for GET /api/v1/dnssec/ds.
type DSResponse struct {
	Enabled   bool       `json:"enabled"`
	Message   string     `json:"message,omitempty"`
	DSRecords []DSRecord `json:"ds_records,omitempty"`
}

// KeysResponse is the response for GET /api/v1/dnssec/keys.
type KeysResponse struct {
	Enabled bool              `json:"enabled"`
	Keys    []*dnssec.KeyPair `json:"keys,omitempty"`
}

// StatusResponse is the response for GET /api/v1/dnssec/status.
type StatusResponse struct {
	Enabled bool              `json:"enabled"`
	Keys    []*dnssec.KeyInfo `json:"keys,omitempty"`
	Sync    dnssec.SyncStatus `json:"sync,omitempty"`
}
