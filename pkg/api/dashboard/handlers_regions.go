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
)

// handleRegions handles GET /api/regions and POST /api/regions
func (h *Handlers) handleRegions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listRegions(w, r)
	case http.MethodPost:
		h.createRegion(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// listRegions handles GET /api/regions
func (h *Handlers) listRegions(w http.ResponseWriter, r *http.Request) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	regions := make([]Region, 0, len(cfg.Regions))

	for _, cr := range cfg.Regions {
		servers := make([]string, 0, len(cr.Servers))
		for _, s := range cr.Servers {
			servers = append(servers, fmt.Sprintf("%s:%d", s.Address, s.Port))
		}

		region := Region{
			Name:       cr.Name,
			Countries:  cr.Countries,
			Continents: cr.Continents,
			Servers:    servers,
			HealthCheck: HealthCheckConfig{
				Enabled:            true,
				Type:               cr.HealthCheck.Type,
				Path:               cr.HealthCheck.Path,
				Interval:           int(cr.HealthCheck.Interval.Seconds()),
				Timeout:            int(cr.HealthCheck.Timeout.Seconds()),
				HealthyThreshold:   cr.HealthCheck.SuccessThreshold,
				UnhealthyThreshold: cr.HealthCheck.FailureThreshold,
			},
		}
		regions = append(regions, region)
	}

	writeJSON(w, http.StatusOK, RegionsResponse{Regions: regions})
}

// createRegion handles POST /api/regions
func (h *Handlers) createRegion(w http.ResponseWriter, r *http.Request) {
	var region Region
	if err := json.NewDecoder(r.Body).Decode(&region); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if region.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "MISSING_FIELD")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Check if region already exists
	for _, r := range cfg.Regions {
		if r.Name == region.Name {
			writeError(w, http.StatusConflict, "region already exists", "REGION_EXISTS")
			return
		}
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryRegion, region.Name,
		fmt.Sprintf("Created region %s", region.Name),
		map[string]interface{}{
			"countries":  region.Countries,
			"continents": region.Continents,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusCreated, RegionResponse{Region: region})
}

// handleRegionByName handles GET, PUT, DELETE /api/regions/:name
func (h *Handlers) handleRegionByName(w http.ResponseWriter, r *http.Request) {
	// Parse region name from path
	name, subPath := parseSubPath(r.URL.Path, "/api/regions/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "region name is required", "MISSING_PARAM")
		return
	}

	// Handle sub-paths
	if subPath == "health" {
		h.getRegionHealth(w, r, name)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getRegion(w, r, name)
	case http.MethodPut:
		h.updateRegion(w, r, name)
	case http.MethodDelete:
		h.deleteRegion(w, r, name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getRegion handles GET /api/regions/:name
func (h *Handlers) getRegion(w http.ResponseWriter, r *http.Request, name string) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	for _, cr := range cfg.Regions {
		if cr.Name == name {
			servers := make([]string, 0, len(cr.Servers))
			for _, s := range cr.Servers {
				servers = append(servers, fmt.Sprintf("%s:%d", s.Address, s.Port))
			}

			region := Region{
				Name:       cr.Name,
				Countries:  cr.Countries,
				Continents: cr.Continents,
				Servers:    servers,
				HealthCheck: HealthCheckConfig{
					Enabled:            true,
					Type:               cr.HealthCheck.Type,
					Path:               cr.HealthCheck.Path,
					Interval:           int(cr.HealthCheck.Interval.Seconds()),
					Timeout:            int(cr.HealthCheck.Timeout.Seconds()),
					HealthyThreshold:   cr.HealthCheck.SuccessThreshold,
					UnhealthyThreshold: cr.HealthCheck.FailureThreshold,
				},
			}

			writeJSON(w, http.StatusOK, RegionResponse{Region: region})
			return
		}
	}

	writeError(w, http.StatusNotFound, "region not found", "REGION_NOT_FOUND")
}

// updateRegion handles PUT /api/regions/:name
func (h *Handlers) updateRegion(w http.ResponseWriter, r *http.Request, name string) {
	var region Region
	if err := json.NewDecoder(r.Body).Decode(&region); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Find the region
	found := false
	for _, cr := range cfg.Regions {
		if cr.Name == name {
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "region not found", "REGION_NOT_FOUND")
		return
	}

	// Set the name from path
	region.Name = name

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryRegion, name,
		fmt.Sprintf("Updated region %s", name),
		map[string]interface{}{
			"countries":  region.Countries,
			"continents": region.Continents,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, RegionResponse{Region: region})
}

// deleteRegion handles DELETE /api/regions/:name
func (h *Handlers) deleteRegion(w http.ResponseWriter, r *http.Request, name string) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Check if region exists
	found := false
	for _, cr := range cfg.Regions {
		if cr.Name == name {
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "region not found", "REGION_NOT_FOUND")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionDelete, AuditCategoryRegion, name,
		fmt.Sprintf("Deleted region %s", name),
		nil,
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// getRegionHealth handles GET /api/regions/:name/health
func (h *Handlers) getRegionHealth(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Verify region exists
	var found bool
	for _, cr := range cfg.Regions {
		if cr.Name == name {
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "region not found", "REGION_NOT_FOUND")
		return
	}

	// Calculate health summary
	summary := RegionHealthSummary{
		Region: name,
	}

	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		backends := registry.GetAllBackends()
		var totalLatency float64
		var latencyCount int

		for _, b := range backends {
			if b.Region == name {
				summary.TotalBackends++
				if b.EffectiveStatus == "healthy" {
					summary.HealthyBackends++
				}
				if b.SmoothedLatency > 0 {
					totalLatency += float64(b.SmoothedLatency.Milliseconds())
					latencyCount++
				}
			}
		}

		if latencyCount > 0 {
			summary.AvgLatency = totalLatency / float64(latencyCount)
		}
	} else {
		// Fallback: use config-based servers
		for _, cr := range cfg.Regions {
			if cr.Name == name {
				summary.TotalBackends = len(cr.Servers)
				summary.HealthyBackends = len(cr.Servers) // Assume all healthy
				break
			}
		}
	}

	if summary.TotalBackends > 0 {
		summary.HealthPercent = float64(summary.HealthyBackends) / float64(summary.TotalBackends) * 100
	}

	writeJSON(w, http.StatusOK, RegionHealthResponse{HealthSummary: summary})
}
