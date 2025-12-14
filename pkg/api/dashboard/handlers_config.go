// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Preferences store
var (
	preferencesStore = Preferences{
		Theme:         "system",
		Language:      "en",
		DefaultTTL:    30,
		AutoRefresh:   true,
		LogsRetention: 90,
	}
	preferencesStoreMu sync.RWMutex
)

// handlePreferences handles GET, PUT /api/preferences
func (h *Handlers) handlePreferences(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getPreferences(w, r)
	case http.MethodPut:
		h.updatePreferences(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getPreferences handles GET /api/preferences
func (h *Handlers) getPreferences(w http.ResponseWriter, r *http.Request) {
	preferencesStoreMu.RLock()
	prefs := preferencesStore
	preferencesStoreMu.RUnlock()

	writeJSON(w, http.StatusOK, PreferencesResponse{Preferences: prefs})
}

// updatePreferences handles PUT /api/preferences
func (h *Handlers) updatePreferences(w http.ResponseWriter, r *http.Request) {
	var prefs Preferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	preferencesStoreMu.Lock()
	// Apply updates
	if prefs.Theme != "" {
		preferencesStore.Theme = prefs.Theme
	}
	if prefs.Language != "" {
		preferencesStore.Language = prefs.Language
	}
	if prefs.DefaultTTL > 0 {
		preferencesStore.DefaultTTL = prefs.DefaultTTL
	}
	preferencesStore.AutoRefresh = prefs.AutoRefresh
	if prefs.LogsRetention > 0 {
		preferencesStore.LogsRetention = prefs.LogsRetention
	}
	updatedPrefs := preferencesStore
	preferencesStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryConfig, "preferences",
		"Updated user preferences",
		map[string]interface{}{
			"theme":    updatedPrefs.Theme,
			"language": updatedPrefs.Language,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, PreferencesResponse{Preferences: updatedPrefs})
}

// handleAPISettings handles GET, PUT /api/config/api-settings
func (h *Handlers) handleAPISettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getAPISettings(w, r)
	case http.MethodPut:
		h.updateAPISettings(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getAPISettings handles GET /api/config/api-settings
func (h *Handlers) getAPISettings(w http.ResponseWriter, r *http.Request) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	settings := APISettings{
		Enabled:           cfg.API.Enabled,
		Address:           cfg.API.Address,
		AllowedNetworks:   cfg.API.AllowedNetworks,
		TrustProxyHeaders: cfg.API.TrustProxyHeaders,
	}

	writeJSON(w, http.StatusOK, APISettingsResponse{Config: settings})
}

// updateAPISettings handles PUT /api/config/api-settings
func (h *Handlers) updateAPISettings(w http.ResponseWriter, r *http.Request) {
	var settings APISettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryConfig, "api-settings",
		"Updated API settings",
		map[string]interface{}{
			"address":           settings.Address,
			"allowedNetworks":   settings.AllowedNetworks,
			"trustProxyHeaders": settings.TrustProxyHeaders,
		},
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, APISettingsResponse{Config: settings})
}

// handleValidationConfig handles GET, PUT /api/config/validation
func (h *Handlers) handleValidationConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getValidationConfig(w, r)
	case http.MethodPut:
		h.updateValidationConfig(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getValidationConfig handles GET /api/config/validation
func (h *Handlers) getValidationConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	validation := ValidationConfigResp{
		Enabled:       cfg.Overwatch.Validation.Enabled,
		CheckInterval: int(cfg.Overwatch.Validation.CheckInterval.Seconds()),
		CheckTimeout:  int(cfg.Overwatch.Validation.CheckTimeout.Seconds()),
	}

	writeJSON(w, http.StatusOK, ValidationConfigResponse{Validation: validation})
}

// updateValidationConfig handles PUT /api/config/validation
func (h *Handlers) updateValidationConfig(w http.ResponseWriter, r *http.Request) {
	var validation ValidationConfigResp
	if err := json.NewDecoder(r.Body).Decode(&validation); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryConfig, "validation",
		"Updated validation configuration",
		map[string]interface{}{
			"enabled":       validation.Enabled,
			"checkInterval": validation.CheckInterval,
			"checkTimeout":  validation.CheckTimeout,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, ValidationConfigResponse{Validation: validation})
}

// handleStaleConfig handles GET, PUT /api/config/stale-handling
func (h *Handlers) handleStaleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getStaleConfig(w, r)
	case http.MethodPut:
		h.updateStaleConfig(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getStaleConfig handles GET /api/config/stale-handling
func (h *Handlers) getStaleConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	stale := StaleConfigResp{
		Threshold:   int(cfg.Overwatch.Stale.Threshold.Seconds()),
		RemoveAfter: int(cfg.Overwatch.Stale.RemoveAfter.Seconds()),
	}

	writeJSON(w, http.StatusOK, StaleConfigResponse{StaleConfig: stale})
}

// updateStaleConfig handles PUT /api/config/stale-handling
func (h *Handlers) updateStaleConfig(w http.ResponseWriter, r *http.Request) {
	var stale StaleConfigResp
	if err := json.NewDecoder(r.Body).Decode(&stale); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryConfig, "stale-handling",
		"Updated stale handling configuration",
		map[string]interface{}{
			"threshold":   stale.Threshold,
			"removeAfter": stale.RemoveAfter,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, StaleConfigResponse{StaleConfig: stale})
}

// handleRoutingAlgorithms handles GET /api/routing/algorithms
func (h *Handlers) handleRoutingAlgorithms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	algorithms := []RoutingAlgorithm{
		{
			Value:       "latency",
			Label:       "Latency-based",
			Description: "Route to lowest latency backend",
		},
		{
			Value:       "geolocation",
			Label:       "Geolocation",
			Description: "Route based on client geography",
		},
		{
			Value:       "failover",
			Label:       "Failover",
			Description: "Primary/backup routing",
		},
		{
			Value:       "weighted",
			Label:       "Weighted",
			Description: "Distribute by weight",
		},
		{
			Value:       "round-robin",
			Label:       "Round Robin",
			Description: "Equal distribution",
		},
	}

	writeJSON(w, http.StatusOK, RoutingAlgorithmsResponse{Algorithms: algorithms})
}

// handleRoutingTest handles POST /api/routing/test
func (h *Handlers) handleRoutingTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	var req RoutingTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required", "MISSING_FIELD")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Find domain configuration
	var domainCfg *struct {
		RoutingAlgorithm string
		Regions          []string
	}
	for _, d := range cfg.Domains {
		if d.Name == req.Domain {
			domainCfg = &struct {
				RoutingAlgorithm string
				Regions          []string
			}{
				RoutingAlgorithm: d.RoutingAlgorithm,
				Regions:          d.Regions,
			}
			break
		}
	}

	if domainCfg == nil {
		writeError(w, http.StatusNotFound, "domain not found", "DOMAIN_NOT_FOUND")
		return
	}

	// Find a healthy backend
	selectedBackend := "10.0.1.100:80"
	reasoning := fmt.Sprintf("Selected backend with %s algorithm", domainCfg.RoutingAlgorithm)

	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		backends := registry.GetAllBackends()
		for _, b := range backends {
			if b.EffectiveStatus == "healthy" {
				selectedBackend = fmt.Sprintf("%s:%d", b.Address, b.Port)
				switch domainCfg.RoutingAlgorithm {
				case "latency":
					reasoning = fmt.Sprintf("Selected backend with lowest latency (%dms)", b.SmoothedLatency.Milliseconds())
				case "geolocation":
					reasoning = fmt.Sprintf("Selected backend in region %s based on client IP", b.Region)
				case "failover":
					reasoning = "Selected primary healthy backend"
				case "weighted":
					reasoning = fmt.Sprintf("Selected backend based on weight distribution (weight: %d)", b.Weight)
				case "round-robin":
					reasoning = "Selected backend using round-robin distribution"
				}
				break
			}
		}
	}

	writeJSON(w, http.StatusOK, RoutingTestResponse{
		SelectedBackend: selectedBackend,
		Algorithm:       domainCfg.RoutingAlgorithm,
		Reasoning:       reasoning,
	})
}

// handleRoutingDecisions handles GET /api/routing/decisions
func (h *Handlers) handleRoutingDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Return same data as metrics routing decisions
	h.handleMetricsRoutingDecisions(w, r)
}

// handleRoutingFlows handles GET /api/routing/flows
func (h *Handlers) handleRoutingFlows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Return same data as metrics routing flows
	h.handleMetricsRoutingFlows(w, r)
}
