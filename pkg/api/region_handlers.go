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
)

// RegionProvider defines the interface for region management operations.
type RegionProvider interface {
	// ListRegions returns all configured regions.
	ListRegions() []Region
	// GetRegion returns a region by ID.
	GetRegion(id string) (*Region, error)
	// CreateRegion creates a new region.
	CreateRegion(region Region) error
	// UpdateRegion updates an existing region.
	UpdateRegion(id string, region Region) error
	// DeleteRegion deletes a region by ID.
	DeleteRegion(id string) error
}

// Region represents a geographic region configuration.
type Region struct {
	ID             string    `json:"id,omitempty"`
	Name           string    `json:"name"`
	Code           string    `json:"code"`
	Description    string    `json:"description,omitempty"`
	Latitude       float64   `json:"latitude,omitempty"`
	Longitude      float64   `json:"longitude,omitempty"`
	Continent      string    `json:"continent,omitempty"`
	Countries      []string  `json:"countries,omitempty"`
	Enabled        bool      `json:"enabled"`
	Priority       int       `json:"priority"`
	ServerCount    int       `json:"server_count,omitempty"`
	HealthyServers int       `json:"healthy_servers,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

// RegionListResponse is the response for GET /api/v1/regions.
type RegionListResponse struct {
	Regions     []Region  `json:"regions"`
	Total       int       `json:"total"`
	GeneratedAt time.Time `json:"generated_at"`
}

// RegionResponse is the response for single region operations.
type RegionResponse struct {
	Region Region `json:"region"`
}

// RegionCreateRequest is the request body for creating a region.
type RegionCreateRequest struct {
	Name        string   `json:"name"`
	Code        string   `json:"code"`
	Description string   `json:"description,omitempty"`
	Latitude    float64  `json:"latitude,omitempty"`
	Longitude   float64  `json:"longitude,omitempty"`
	Continent   string   `json:"continent,omitempty"`
	Countries   []string `json:"countries,omitempty"`
	Enabled     bool     `json:"enabled"`
	Priority    int      `json:"priority"`
}

// RegionUpdateRequest is the request body for updating a region.
type RegionUpdateRequest struct {
	Name        *string  `json:"name,omitempty"`
	Code        *string  `json:"code,omitempty"`
	Description *string  `json:"description,omitempty"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	Continent   *string  `json:"continent,omitempty"`
	Countries   []string `json:"countries,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
}

// RegionHandlers provides HTTP handlers for region API endpoints.
type RegionHandlers struct {
	provider RegionProvider
	logger   *slog.Logger
}

// NewRegionHandlers creates a new RegionHandlers instance.
func NewRegionHandlers(provider RegionProvider, logger *slog.Logger) *RegionHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &RegionHandlers{
		provider: provider,
		logger:   logger,
	}
}

// HandleRegions routes /api/v1/regions requests based on HTTP method and path.
func (h *RegionHandlers) HandleRegions(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/regions")
	path = strings.TrimPrefix(path, "/")

	// If path is empty, it's a list/create request
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			h.listRegions(w, r)
		case http.MethodPost:
			h.createRegion(w, r)
		default:
			h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Single region operations
	regionID := path
	switch r.Method {
	case http.MethodGet:
		h.getRegion(w, r, regionID)
	case http.MethodPut, http.MethodPatch:
		h.updateRegion(w, r, regionID)
	case http.MethodDelete:
		h.deleteRegion(w, r, regionID)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listRegions handles GET /api/v1/regions.
func (h *RegionHandlers) listRegions(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "region provider not configured")
		return
	}

	regions := h.provider.ListRegions()

	// Apply filters from query parameters
	enabledFilter := r.URL.Query().Get("enabled")
	continentFilter := r.URL.Query().Get("continent")

	if enabledFilter != "" || continentFilter != "" {
		filtered := make([]Region, 0, len(regions))
		for _, reg := range regions {
			if enabledFilter == "true" && !reg.Enabled {
				continue
			}
			if enabledFilter == "false" && reg.Enabled {
				continue
			}
			if continentFilter != "" && reg.Continent != continentFilter {
				continue
			}
			filtered = append(filtered, reg)
		}
		regions = filtered
	}

	resp := RegionListResponse{
		Regions:     regions,
		Total:       len(regions),
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// getRegion handles GET /api/v1/regions/{id}.
func (h *RegionHandlers) getRegion(w http.ResponseWriter, r *http.Request, id string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "region provider not configured")
		return
	}

	region, err := h.provider.GetRegion(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "region not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, RegionResponse{Region: *region})
}

// createRegion handles POST /api/v1/regions.
func (h *RegionHandlers) createRegion(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "region provider not configured")
		return
	}

	var req RegionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.Name == "" {
		h.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Code == "" {
		h.writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	region := Region{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		Latitude:    req.Latitude,
		Longitude:   req.Longitude,
		Continent:   req.Continent,
		Countries:   req.Countries,
		Enabled:     req.Enabled,
		Priority:    req.Priority,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := h.provider.CreateRegion(region); err != nil {
		h.logger.Error("failed to create region", "name", req.Name, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to create region: "+err.Error())
		return
	}

	h.logger.Info("region created", "name", req.Name, "code", req.Code)
	h.writeJSON(w, http.StatusCreated, RegionResponse{Region: region})
}

// updateRegion handles PUT/PATCH /api/v1/regions/{id}.
func (h *RegionHandlers) updateRegion(w http.ResponseWriter, r *http.Request, id string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "region provider not configured")
		return
	}

	// Get existing region
	existing, err := h.provider.GetRegion(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "region not found: "+err.Error())
		return
	}

	var req RegionUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Apply updates
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Code != nil {
		existing.Code = *req.Code
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Latitude != nil {
		existing.Latitude = *req.Latitude
	}
	if req.Longitude != nil {
		existing.Longitude = *req.Longitude
	}
	if req.Continent != nil {
		existing.Continent = *req.Continent
	}
	if req.Countries != nil {
		existing.Countries = req.Countries
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	existing.UpdatedAt = time.Now().UTC()

	if err := h.provider.UpdateRegion(id, *existing); err != nil {
		h.logger.Error("failed to update region", "id", id, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to update region: "+err.Error())
		return
	}

	h.logger.Info("region updated", "id", id)
	h.writeJSON(w, http.StatusOK, RegionResponse{Region: *existing})
}

// deleteRegion handles DELETE /api/v1/regions/{id}.
func (h *RegionHandlers) deleteRegion(w http.ResponseWriter, r *http.Request, id string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "region provider not configured")
		return
	}

	if err := h.provider.DeleteRegion(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, http.StatusNotFound, "region not found")
			return
		}
		h.logger.Error("failed to delete region", "id", id, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to delete region: "+err.Error())
		return
	}

	h.logger.Info("region deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// writeJSON writes a JSON response with the given status code.
func (h *RegionHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *RegionHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
