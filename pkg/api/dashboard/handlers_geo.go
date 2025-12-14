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

	"github.com/google/uuid"
)

// Geo mapping store
var (
	geoMappingStore   = make(map[string]GeoMapping)
	geoMappingStoreMu sync.RWMutex
)

// handleGeoMappings handles GET, POST /api/geo-mappings
func (h *Handlers) handleGeoMappings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listGeoMappings(w, r)
	case http.MethodPost:
		h.createGeoMapping(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// listGeoMappings handles GET /api/geo-mappings
func (h *Handlers) listGeoMappings(w http.ResponseWriter, r *http.Request) {
	mappings := make([]GeoMapping, 0)

	// Get mappings from config
	cfg := h.dataProvider.GetConfig()
	if cfg != nil && len(cfg.Overwatch.Geolocation.CustomMappings) > 0 {
		for i, cm := range cfg.Overwatch.Geolocation.CustomMappings {
			mappings = append(mappings, GeoMapping{
				ID:      fmt.Sprintf("config-%d", i),
				CIDR:    cm.CIDR,
				Region:  cm.Region,
				Comment: cm.Comment,
			})
		}
	}

	// Get mappings from store
	geoMappingStoreMu.RLock()
	for _, mapping := range geoMappingStore {
		mappings = append(mappings, mapping)
	}
	geoMappingStoreMu.RUnlock()

	writeJSON(w, http.StatusOK, GeoMappingsResponse{Mappings: mappings})
}

// createGeoMapping handles POST /api/geo-mappings
func (h *Handlers) createGeoMapping(w http.ResponseWriter, r *http.Request) {
	var mapping GeoMapping
	if err := json.NewDecoder(r.Body).Decode(&mapping); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if mapping.CIDR == "" {
		writeError(w, http.StatusBadRequest, "cidr is required", "MISSING_FIELD")
		return
	}
	if mapping.Region == "" {
		writeError(w, http.StatusBadRequest, "region is required", "MISSING_FIELD")
		return
	}

	// Generate ID if not provided
	if mapping.ID == "" {
		mapping.ID = uuid.New().String()[:8]
	}

	// Store mapping
	geoMappingStoreMu.Lock()
	geoMappingStore[mapping.ID] = mapping
	geoMappingStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryGeo, mapping.ID,
		fmt.Sprintf("Created geo mapping %s -> %s", mapping.CIDR, mapping.Region),
		map[string]interface{}{
			"cidr":    mapping.CIDR,
			"region":  mapping.Region,
			"comment": mapping.Comment,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusCreated, GeoMappingResponse{Mapping: mapping})
}

// handleGeoMappingByID handles GET, PUT, DELETE /api/geo-mappings/:id
func (h *Handlers) handleGeoMappingByID(w http.ResponseWriter, r *http.Request) {
	id := parsePathParam(r.URL.Path, "/api/geo-mappings/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "mapping ID is required", "MISSING_PARAM")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getGeoMapping(w, r, id)
	case http.MethodPut:
		h.updateGeoMapping(w, r, id)
	case http.MethodDelete:
		h.deleteGeoMapping(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getGeoMapping handles GET /api/geo-mappings/:id
func (h *Handlers) getGeoMapping(w http.ResponseWriter, r *http.Request, id string) {
	geoMappingStoreMu.RLock()
	mapping, exists := geoMappingStore[id]
	geoMappingStoreMu.RUnlock()

	if !exists {
		writeError(w, http.StatusNotFound, "geo mapping not found", "MAPPING_NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, GeoMappingResponse{Mapping: mapping})
}

// updateGeoMapping handles PUT /api/geo-mappings/:id
func (h *Handlers) updateGeoMapping(w http.ResponseWriter, r *http.Request, id string) {
	var mapping GeoMapping
	if err := json.NewDecoder(r.Body).Decode(&mapping); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	geoMappingStoreMu.Lock()
	existingMapping, exists := geoMappingStore[id]
	if !exists {
		geoMappingStoreMu.Unlock()
		writeError(w, http.StatusNotFound, "geo mapping not found", "MAPPING_NOT_FOUND")
		return
	}

	// Update fields
	if mapping.CIDR != "" {
		existingMapping.CIDR = mapping.CIDR
	}
	if mapping.Region != "" {
		existingMapping.Region = mapping.Region
	}
	if mapping.Comment != "" {
		existingMapping.Comment = mapping.Comment
	}

	geoMappingStore[id] = existingMapping
	geoMappingStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryGeo, id,
		fmt.Sprintf("Updated geo mapping %s", id),
		map[string]interface{}{
			"cidr":    existingMapping.CIDR,
			"region":  existingMapping.Region,
			"comment": existingMapping.Comment,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, GeoMappingResponse{Mapping: existingMapping})
}

// deleteGeoMapping handles DELETE /api/geo-mappings/:id
func (h *Handlers) deleteGeoMapping(w http.ResponseWriter, r *http.Request, id string) {
	geoMappingStoreMu.Lock()
	_, exists := geoMappingStore[id]
	if !exists {
		geoMappingStoreMu.Unlock()
		writeError(w, http.StatusNotFound, "geo mapping not found", "MAPPING_NOT_FOUND")
		return
	}
	delete(geoMappingStore, id)
	geoMappingStoreMu.Unlock()

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionDelete, AuditCategoryGeo, id,
		fmt.Sprintf("Deleted geo mapping %s", id),
		nil,
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// handleGeolocationConfig handles GET, PUT /api/geolocation/config
func (h *Handlers) handleGeolocationConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getGeolocationConfig(w, r)
	case http.MethodPut:
		h.updateGeolocationConfig(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getGeolocationConfig handles GET /api/geolocation/config
func (h *Handlers) getGeolocationConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	geoCfg := GeoConfig{
		DatabasePath:  cfg.Overwatch.Geolocation.DatabasePath,
		DefaultRegion: cfg.Overwatch.Geolocation.DefaultRegion,
		ECSEnabled:    cfg.Overwatch.Geolocation.ECSEnabled,
	}

	writeJSON(w, http.StatusOK, GeoConfigResponse{Config: geoCfg})
}

// updateGeolocationConfig handles PUT /api/geolocation/config
func (h *Handlers) updateGeolocationConfig(w http.ResponseWriter, r *http.Request) {
	var geoCfg GeoConfig
	if err := json.NewDecoder(r.Body).Decode(&geoCfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryConfig, "geolocation",
		"Updated geolocation configuration",
		map[string]interface{}{
			"databasePath":  geoCfg.DatabasePath,
			"defaultRegion": geoCfg.DefaultRegion,
			"ecsEnabled":    geoCfg.ECSEnabled,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, GeoConfigResponse{Config: geoCfg})
}

// handleGeolocationLookup handles POST /api/geolocation/lookup
func (h *Handlers) handleGeolocationLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	var req GeoLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if req.IP == "" {
		writeError(w, http.StatusBadRequest, "ip is required", "MISSING_FIELD")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Check custom mappings first
	geoMappingStoreMu.RLock()
	for _, mapping := range geoMappingStore {
		// In a real implementation, this would check if IP is in CIDR range
		// For now, return default region
		_ = mapping
	}
	geoMappingStoreMu.RUnlock()

	// Return result (placeholder - real implementation would use GeoIP database)
	response := GeoLookupResponse{
		Region:  cfg.Overwatch.Geolocation.DefaultRegion,
		Country: "US",
		City:    "Unknown",
	}

	// Simple heuristics based on IP ranges (placeholder)
	if len(req.IP) > 0 {
		switch req.IP[0] {
		case '1':
			response.Country = "AU"
			response.Region = "ap-southeast-2"
		case '2':
			response.Country = "EU"
			response.Region = "eu-west-1"
		case '3', '4', '5':
			response.Country = "US"
			response.Region = "us-east-1"
		}
	}

	writeJSON(w, http.StatusOK, response)
}
