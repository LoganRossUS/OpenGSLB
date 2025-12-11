// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/geo"
)

// GeoProvider provides geolocation functionality.
type GeoProvider interface {
	// Resolve resolves an IP to a region.
	Resolve(ip net.IP) *geo.RegionMatch

	// GetCustomMappings returns the custom mappings manager.
	GetCustomMappings() *geo.CustomMappings
}

// GeoHandlers contains handlers for geolocation API endpoints.
type GeoHandlers struct {
	geoProvider GeoProvider
}

// NewGeoHandlers creates a new GeoHandlers instance.
func NewGeoHandlers(provider GeoProvider) *GeoHandlers {
	return &GeoHandlers{
		geoProvider: provider,
	}
}

// GeoMappingResponse represents a custom mapping in API responses.
type GeoMappingResponse struct {
	CIDR    string `json:"cidr"`
	Region  string `json:"region"`
	Comment string `json:"comment,omitempty"`
	Source  string `json:"source"` // "config" or "api"
}

// GeoMappingsResponse is the response for listing all mappings.
type GeoMappingsResponse struct {
	Mappings    []GeoMappingResponse `json:"mappings"`
	GeneratedAt time.Time            `json:"generated_at"`
}

// GeoMappingRequest is the request body for creating/updating a mapping.
type GeoMappingRequest struct {
	CIDR    string `json:"cidr"`
	Region  string `json:"region"`
	Comment string `json:"comment,omitempty"`
}

// GeoTestResponse is the response for testing geo routing.
type GeoTestResponse struct {
	IP          string `json:"ip"`
	Region      string `json:"region"`
	MatchType   string `json:"match_type"` // "custom_mapping", "geoip", "default"
	MatchedCIDR string `json:"matched_cidr,omitempty"`
	Comment     string `json:"comment,omitempty"`
	Country     string `json:"country,omitempty"`
	Continent   string `json:"continent,omitempty"`
}

// HandleMappings routes /api/v1/geo/mappings based on HTTP method.
func (h *GeoHandlers) HandleMappings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listMappings(w, r)
	case http.MethodPut, http.MethodPost:
		h.addMapping(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listMappings handles GET /api/v1/geo/mappings
func (h *GeoHandlers) listMappings(w http.ResponseWriter, r *http.Request) {

	if h.geoProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "geolocation not configured")
		return
	}

	mappings := h.geoProvider.GetCustomMappings().List()

	response := GeoMappingsResponse{
		Mappings:    make([]GeoMappingResponse, 0, len(mappings)),
		GeneratedAt: time.Now().UTC(),
	}

	for _, m := range mappings {
		response.Mappings = append(response.Mappings, GeoMappingResponse{
			CIDR:    m.CIDR,
			Region:  m.Region,
			Comment: m.Comment,
			Source:  m.Source,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

// addMapping handles PUT /api/v1/geo/mappings
func (h *GeoHandlers) addMapping(w http.ResponseWriter, r *http.Request) {
	if h.geoProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "geolocation not configured")
		return
	}

	var req GeoMappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate CIDR
	if req.CIDR == "" {
		writeError(w, http.StatusBadRequest, "cidr is required")
		return
	}
	if _, _, err := net.ParseCIDR(req.CIDR); err != nil {
		writeError(w, http.StatusBadRequest, "invalid cidr: "+err.Error())
		return
	}

	// Validate region
	if req.Region == "" {
		writeError(w, http.StatusBadRequest, "region is required")
		return
	}

	mapping := geo.CustomMapping{
		CIDR:    req.CIDR,
		Region:  req.Region,
		Comment: req.Comment,
		Source:  "api",
	}

	if err := h.geoProvider.GetCustomMappings().Add(mapping); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add mapping: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, GeoMappingResponse{
		CIDR:    mapping.CIDR,
		Region:  mapping.Region,
		Comment: mapping.Comment,
		Source:  mapping.Source,
	})
}

// DeleteMapping handles DELETE /api/v1/geo/mappings/{cidr}
// The CIDR should be URL-encoded (e.g., 10.1.0.0%2F16)
func (h *GeoHandlers) DeleteMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.geoProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "geolocation not configured")
		return
	}

	// Extract CIDR from path (URL-encoded)
	// Path format: /api/v1/geo/mappings/{cidr}
	path := r.URL.Path
	prefix := "/api/v1/geo/mappings/"
	if len(path) <= len(prefix) {
		writeError(w, http.StatusBadRequest, "cidr is required in path")
		return
	}

	cidrEncoded := path[len(prefix):]
	cidr, err := url.PathUnescape(cidrEncoded)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cidr encoding: "+err.Error())
		return
	}

	if err := h.geoProvider.GetCustomMappings().Remove(cidr); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestIP handles GET /api/v1/geo/test?ip=x.x.x.x
func (h *GeoHandlers) TestIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.geoProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "geolocation not configured")
		return
	}

	ipStr := r.URL.Query().Get("ip")
	if ipStr == "" {
		writeError(w, http.StatusBadRequest, "ip query parameter is required")
		return
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		writeError(w, http.StatusBadRequest, "invalid IP address: "+ipStr)
		return
	}

	match := h.geoProvider.Resolve(ip)

	response := GeoTestResponse{
		IP:        ipStr,
		Region:    match.Region,
		MatchType: string(match.MatchType),
	}

	switch match.MatchType {
	case geo.MatchTypeCustomMapping:
		response.MatchedCIDR = match.MatchedCIDR
		response.Comment = match.Comment
	case geo.MatchTypeGeoIP:
		response.Country = match.Country
		response.Continent = match.Continent
	}

	writeJSON(w, http.StatusOK, response)
}
