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

// DomainProvider defines the interface for domain management operations.
type DomainProvider interface {
	// ListDomains returns all configured domains.
	ListDomains() []Domain
	// GetDomain returns a domain by name.
	GetDomain(name string) (*Domain, error)
	// CreateDomain creates a new domain.
	CreateDomain(domain Domain) error
	// UpdateDomain updates an existing domain.
	UpdateDomain(name string, domain Domain) error
	// DeleteDomain deletes a domain by name.
	DeleteDomain(name string) error
	// GetDomainBackends returns the backends for a domain.
	GetDomainBackends(name string) ([]DomainBackend, error)
}

// Domain represents a DNS domain configuration.
type Domain struct {
	ID              string          `json:"id,omitempty"`
	Name            string          `json:"name"`
	TTL             int             `json:"ttl"`
	RoutingPolicy   string          `json:"routing_policy"`
	HealthCheckID   string          `json:"health_check_id,omitempty"`
	DNSSECEnabled   bool            `json:"dnssec_enabled"`
	Enabled         bool            `json:"enabled"`
	Description     string          `json:"description,omitempty"`
	Tags            []string        `json:"tags,omitempty"`
	CreatedAt       time.Time       `json:"created_at,omitempty"`
	UpdatedAt       time.Time       `json:"updated_at,omitempty"`
	BackendCount    int             `json:"backend_count,omitempty"`
	HealthyBackends int             `json:"healthy_backends,omitempty"`
	Settings        *DomainSettings `json:"settings,omitempty"`
}

// DomainSettings holds advanced domain settings.
type DomainSettings struct {
	GeoRoutingEnabled   bool   `json:"geo_routing_enabled"`
	FailoverEnabled     bool   `json:"failover_enabled"`
	FailoverThreshold   int    `json:"failover_threshold"`
	LoadBalancingMethod string `json:"load_balancing_method"`
}

// DomainBackend represents a backend server for a domain.
type DomainBackend struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	Port      int       `json:"port"`
	Weight    int       `json:"weight"`
	Priority  int       `json:"priority"`
	Region    string    `json:"region"`
	Healthy   bool      `json:"healthy"`
	Enabled   bool      `json:"enabled"`
	LastCheck time.Time `json:"last_check,omitempty"`
}

// DomainListResponse is the response for GET /api/v1/domains.
type DomainListResponse struct {
	Domains     []Domain  `json:"domains"`
	Total       int       `json:"total"`
	GeneratedAt time.Time `json:"generated_at"`
}

// DomainResponse is the response for single domain operations.
type DomainResponse struct {
	Domain Domain `json:"domain"`
}

// DomainBackendsResponse is the response for GET /api/v1/domains/{name}/backends.
type DomainBackendsResponse struct {
	Backends    []DomainBackend `json:"backends"`
	Total       int             `json:"total"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// DomainCreateRequest is the request body for creating a domain.
type DomainCreateRequest struct {
	Name          string          `json:"name"`
	TTL           int             `json:"ttl"`
	RoutingPolicy string          `json:"routing_policy"`
	DNSSECEnabled bool            `json:"dnssec_enabled"`
	Enabled       bool            `json:"enabled"`
	Description   string          `json:"description,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	Settings      *DomainSettings `json:"settings,omitempty"`
}

// DomainUpdateRequest is the request body for updating a domain.
type DomainUpdateRequest struct {
	TTL           *int            `json:"ttl,omitempty"`
	RoutingPolicy *string         `json:"routing_policy,omitempty"`
	DNSSECEnabled *bool           `json:"dnssec_enabled,omitempty"`
	Enabled       *bool           `json:"enabled,omitempty"`
	Description   *string         `json:"description,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	Settings      *DomainSettings `json:"settings,omitempty"`
}

// DomainHandlers provides HTTP handlers for domain API endpoints.
type DomainHandlers struct {
	provider DomainProvider
	logger   *slog.Logger
}

// NewDomainHandlers creates a new DomainHandlers instance.
func NewDomainHandlers(provider DomainProvider, logger *slog.Logger) *DomainHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &DomainHandlers{
		provider: provider,
		logger:   logger,
	}
}

// HandleDomains routes /api/v1/domains requests based on HTTP method and path.
func (h *DomainHandlers) HandleDomains(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/domains")
	path = strings.TrimPrefix(path, "/")

	// If path is empty, it's a list/create request
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			h.listDomains(w, r)
		case http.MethodPost:
			h.createDomain(w, r)
		default:
			h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Parse domain name and optional sub-resource
	parts := strings.SplitN(path, "/", 2)
	domainName := parts[0]

	// Check for sub-resources
	if len(parts) == 2 {
		subResource := parts[1]
		switch subResource {
		case "backends":
			h.handleDomainBackends(w, r, domainName)
		default:
			h.writeError(w, http.StatusNotFound, "endpoint not found")
		}
		return
	}

	// Single domain operations
	switch r.Method {
	case http.MethodGet:
		h.getDomain(w, r, domainName)
	case http.MethodPut, http.MethodPatch:
		h.updateDomain(w, r, domainName)
	case http.MethodDelete:
		h.deleteDomain(w, r, domainName)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listDomains handles GET /api/v1/domains.
func (h *DomainHandlers) listDomains(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "domain provider not configured")
		return
	}

	domains := h.provider.ListDomains()

	resp := DomainListResponse{
		Domains:     domains,
		Total:       len(domains),
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// getDomain handles GET /api/v1/domains/{name}.
func (h *DomainHandlers) getDomain(w http.ResponseWriter, r *http.Request, name string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "domain provider not configured")
		return
	}

	domain, err := h.provider.GetDomain(name)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "domain not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, DomainResponse{Domain: *domain})
}

// createDomain handles POST /api/v1/domains.
func (h *DomainHandlers) createDomain(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "domain provider not configured")
		return
	}

	var req DomainCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.Name == "" {
		h.writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	domain := Domain{
		Name:          req.Name,
		TTL:           req.TTL,
		RoutingPolicy: req.RoutingPolicy,
		DNSSECEnabled: req.DNSSECEnabled,
		Enabled:       req.Enabled,
		Description:   req.Description,
		Tags:          req.Tags,
		Settings:      req.Settings,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Set defaults
	if domain.TTL == 0 {
		domain.TTL = 300
	}
	if domain.RoutingPolicy == "" {
		domain.RoutingPolicy = "round-robin"
	}

	if err := h.provider.CreateDomain(domain); err != nil {
		h.logger.Error("failed to create domain", "name", req.Name, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to create domain: "+err.Error())
		return
	}

	h.logger.Info("domain created", "name", req.Name)
	h.writeJSON(w, http.StatusCreated, DomainResponse{Domain: domain})
}

// updateDomain handles PUT/PATCH /api/v1/domains/{name}.
func (h *DomainHandlers) updateDomain(w http.ResponseWriter, r *http.Request, name string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "domain provider not configured")
		return
	}

	// Get existing domain
	existing, err := h.provider.GetDomain(name)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "domain not found: "+err.Error())
		return
	}

	var req DomainUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Apply updates
	if req.TTL != nil {
		existing.TTL = *req.TTL
	}
	if req.RoutingPolicy != nil {
		existing.RoutingPolicy = *req.RoutingPolicy
	}
	if req.DNSSECEnabled != nil {
		existing.DNSSECEnabled = *req.DNSSECEnabled
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Tags != nil {
		existing.Tags = req.Tags
	}
	if req.Settings != nil {
		existing.Settings = req.Settings
	}
	existing.UpdatedAt = time.Now().UTC()

	if err := h.provider.UpdateDomain(name, *existing); err != nil {
		h.logger.Error("failed to update domain", "name", name, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to update domain: "+err.Error())
		return
	}

	h.logger.Info("domain updated", "name", name)
	h.writeJSON(w, http.StatusOK, DomainResponse{Domain: *existing})
}

// deleteDomain handles DELETE /api/v1/domains/{name}.
func (h *DomainHandlers) deleteDomain(w http.ResponseWriter, r *http.Request, name string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "domain provider not configured")
		return
	}

	if err := h.provider.DeleteDomain(name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeError(w, http.StatusNotFound, "domain not found")
			return
		}
		h.logger.Error("failed to delete domain", "name", name, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to delete domain: "+err.Error())
		return
	}

	h.logger.Info("domain deleted", "name", name)
	w.WriteHeader(http.StatusNoContent)
}

// handleDomainBackends handles /api/v1/domains/{name}/backends.
func (h *DomainHandlers) handleDomainBackends(w http.ResponseWriter, r *http.Request, domainName string) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "domain provider not configured")
		return
	}

	backends, err := h.provider.GetDomainBackends(domainName)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "domain not found: "+err.Error())
		return
	}

	resp := DomainBackendsResponse{
		Backends:    backends,
		Total:       len(backends),
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// writeJSON writes a JSON response with the given status code.
func (h *DomainHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *DomainHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
