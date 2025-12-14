// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overlord

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Override store for tracking overrides
var (
	overrideStore   = make(map[string]Override)
	overrideStoreMu sync.RWMutex
)

// handleOverrides handles GET /api/overrides and POST /api/overrides
func (h *Handlers) handleOverrides(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listOverrides(w, r)
	case http.MethodPost:
		h.createOverride(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// listOverrides handles GET /api/overrides
func (h *Handlers) listOverrides(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")

	overrides := make([]Override, 0)

	// Get overrides from registry
	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		backends := registry.GetAllBackends()
		for _, b := range backends {
			if b.OverrideStatus == nil {
				continue
			}

			// Apply service filter
			if service != "" && b.Service != service {
				continue
			}

			override := Override{
				ID:        fmt.Sprintf("%s:%d", b.Address, b.Port),
				Service:   b.Service,
				Address:   fmt.Sprintf("%s:%d", b.Address, b.Port),
				Healthy:   *b.OverrideStatus,
				Reason:    b.OverrideReason,
				Source:    "manual",
				Authority: b.OverrideBy,
				CreatedAt: b.OverrideAt,
			}
			overrides = append(overrides, override)
		}
	}

	// Also include overrides from local store
	overrideStoreMu.RLock()
	for id, override := range overrideStore {
		if service != "" && override.Service != service {
			continue
		}
		// Avoid duplicates
		found := false
		for _, o := range overrides {
			if o.ID == id {
				found = true
				break
			}
		}
		if !found {
			overrides = append(overrides, override)
		}
	}
	overrideStoreMu.RUnlock()

	writeJSON(w, http.StatusOK, OverridesResponse{Overrides: overrides})
}

// createOverride handles POST /api/overrides
func (h *Handlers) createOverride(w http.ResponseWriter, r *http.Request) {
	var req OverrideCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if req.Service == "" {
		writeError(w, http.StatusBadRequest, "service is required", "MISSING_FIELD")
		return
	}
	if req.Address == "" {
		writeError(w, http.StatusBadRequest, "address is required", "MISSING_FIELD")
		return
	}

	// Parse address to get host and port
	address, port := parseAddressPort(req.Address)
	if address == "" {
		writeError(w, http.StatusBadRequest, "invalid address format (expected host:port)", "INVALID_ADDRESS")
		return
	}

	// Set default authority
	authority := req.Authority
	if authority == "" {
		authority = getUser(r)
	}

	// Set default source
	source := req.Source
	if source == "" {
		source = "manual"
	}

	// Generate ID
	id := uuid.New().String()[:8]

	override := Override{
		ID:        id,
		Service:   req.Service,
		Address:   req.Address,
		Healthy:   req.Healthy,
		Reason:    req.Reason,
		Source:    source,
		Authority: authority,
		CreatedAt: time.Now(),
	}

	// Apply override to registry
	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		err := registry.SetOverride(req.Service, address, port, req.Healthy, req.Reason, authority)
		if err != nil {
			h.logger.Warn("failed to set override in registry", "error", err)
		}
	}

	// Store override
	overrideStoreMu.Lock()
	overrideStore[id] = override
	overrideStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryOverride, id,
		fmt.Sprintf("Created override for %s: %s", req.Service, req.Address),
		map[string]interface{}{
			"service": req.Service,
			"address": req.Address,
			"healthy": req.Healthy,
			"reason":  req.Reason,
		},
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusCreated, OverrideResponse{Override: override})
}

// handleOverrideByID handles DELETE /api/overrides/:id
func (h *Handlers) handleOverrideByID(w http.ResponseWriter, r *http.Request) {
	id := parsePathParam(r.URL.Path, "/api/overrides/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "override ID is required", "MISSING_PARAM")
		return
	}

	switch r.Method {
	case http.MethodDelete:
		h.deleteOverride(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// deleteOverride handles DELETE /api/overrides/:id
func (h *Handlers) deleteOverride(w http.ResponseWriter, r *http.Request, id string) {
	// Find the override
	overrideStoreMu.RLock()
	override, exists := overrideStore[id]
	overrideStoreMu.RUnlock()

	if !exists {
		// Check if it's an address-based ID from the registry
		registry := h.dataProvider.GetBackendRegistry()
		if registry != nil {
			backends := registry.GetAllBackends()
			for _, b := range backends {
				backendID := fmt.Sprintf("%s:%d", b.Address, b.Port)
				if backendID == id && b.OverrideStatus != nil {
					// Clear override in registry
					_ = registry.ClearOverride(b.Service, b.Address, b.Port)
					exists = true
					override = Override{
						ID:      id,
						Service: b.Service,
						Address: backendID,
					}
					break
				}
			}
		}
	}

	if !exists {
		writeError(w, http.StatusNotFound, "override not found", "OVERRIDE_NOT_FOUND")
		return
	}

	// Clear override from registry
	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		address, port := parseAddressPort(override.Address)
		if address != "" {
			_ = registry.ClearOverride(override.Service, address, port)
		}
	}

	// Remove from store
	overrideStoreMu.Lock()
	delete(overrideStore, id)
	overrideStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionDelete, AuditCategoryOverride, id,
		fmt.Sprintf("Deleted override %s", id),
		nil,
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// parseAddressPort parses "host:port" into host and port.
func parseAddressPort(addr string) (string, int) {
	parts := strings.Split(addr, ":")
	if len(parts) < 2 {
		return addr, 80 // Default port
	}

	// Handle potential IPv6
	if len(parts) > 2 {
		// Last part is port
		port, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			return addr, 80
		}
		host := strings.Join(parts[:len(parts)-1], ":")
		return host, port
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return parts[0], 80
	}
	return parts[0], port
}
