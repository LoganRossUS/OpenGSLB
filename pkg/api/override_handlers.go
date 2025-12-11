// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// OverrideHandlers provides HTTP handlers for the override API.
type OverrideHandlers struct {
	manager *OverrideManager
	logger  *slog.Logger
}

// NewOverrideHandlers creates a new OverrideHandlers instance.
func NewOverrideHandlers(manager *OverrideManager, logger *slog.Logger) *OverrideHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &OverrideHandlers{
		manager: manager,
		logger:  logger,
	}
}

// HandleOverrides handles requests to /api/v1/overrides and /api/v1/overrides/{service}/{address}.
// It routes based on the URL path and HTTP method.
func (h *OverrideHandlers) HandleOverrides(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	// Expected paths:
	//   GET  /api/v1/overrides           -> ListOverrides
	//   PUT  /api/v1/overrides/{service}/{address} -> SetOverride
	//   DELETE /api/v1/overrides/{service}/{address} -> ClearOverride

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/overrides")
	path = strings.TrimPrefix(path, "/")

	// If path is empty or "/", it's a list request
	if path == "" {
		h.HandleListOverrides(w, r)
		return
	}

	// Otherwise, parse service/address from path
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		h.writeError(w, http.StatusBadRequest, "invalid path: expected /api/v1/overrides/{service}/{address}")
		return
	}

	service := parts[0]
	address := parts[1]

	switch r.Method {
	case http.MethodPut:
		h.HandleSetOverride(w, r, service, address)
	case http.MethodDelete:
		h.HandleClearOverride(w, r, service, address)
	case http.MethodGet:
		h.HandleGetOverride(w, r, service, address)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleListOverrides handles GET /api/v1/overrides.
func (h *OverrideHandlers) HandleListOverrides(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	overrides := h.manager.ListOverrides()

	resp := OverridesListResponse{
		Overrides: overrides,
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// HandleSetOverride handles PUT /api/v1/overrides/{service}/{address}.
func (h *OverrideHandlers) HandleSetOverride(w http.ResponseWriter, r *http.Request, service, address string) {
	if r.Method != http.MethodPut {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req OverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.Source == "" {
		h.writeError(w, http.StatusBadRequest, "source is required")
		return
	}

	clientIP := h.extractClientIP(r)

	override, err := h.manager.SetOverride(r.Context(), service, address, req.Healthy, req.Reason, req.Source, clientIP)
	if err != nil {
		h.logger.Error("failed to set override",
			"service", service,
			"address", address,
			"error", err,
		)
		h.writeError(w, http.StatusInternalServerError, "failed to set override: "+err.Error())
		return
	}

	resp := OverrideResponse{
		Service:   override.Service,
		Address:   override.Address,
		Healthy:   override.Healthy,
		Reason:    override.Reason,
		Source:    override.Source,
		CreatedAt: override.CreatedAt,
		Authority: override.Authority,
	}

	h.logger.Info("override set via API",
		"service", service,
		"address", address,
		"healthy", req.Healthy,
		"source", req.Source,
		"client_ip", clientIP,
	)

	h.writeJSON(w, http.StatusOK, resp)
}

// HandleClearOverride handles DELETE /api/v1/overrides/{service}/{address}.
func (h *OverrideHandlers) HandleClearOverride(w http.ResponseWriter, r *http.Request, service, address string) {
	if r.Method != http.MethodDelete {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	clientIP := h.extractClientIP(r)

	if err := h.manager.ClearOverride(r.Context(), service, address, clientIP); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, http.StatusNotFound, "override not found")
			return
		}
		h.logger.Error("failed to clear override",
			"service", service,
			"address", address,
			"error", err,
		)
		h.writeError(w, http.StatusInternalServerError, "failed to clear override: "+err.Error())
		return
	}

	h.logger.Info("override cleared via API",
		"service", service,
		"address", address,
		"client_ip", clientIP,
	)

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetOverride handles GET /api/v1/overrides/{service}/{address}.
func (h *OverrideHandlers) HandleGetOverride(w http.ResponseWriter, r *http.Request, service, address string) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	override, exists := h.manager.GetOverride(service, address)
	if !exists {
		h.writeError(w, http.StatusNotFound, "override not found")
		return
	}

	resp := OverrideResponse{
		Service:   override.Service,
		Address:   override.Address,
		Healthy:   override.Healthy,
		Reason:    override.Reason,
		Source:    override.Source,
		CreatedAt: override.CreatedAt,
		Authority: override.Authority,
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// extractClientIP extracts the client IP from the request.
func (h *OverrideHandlers) extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeJSON writes a JSON response with the given status code.
func (h *OverrideHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *OverrideHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
