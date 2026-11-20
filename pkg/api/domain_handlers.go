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
	// GetDomainBackends returns the servers of a domain. A domain that exists
	// and has no servers returns an empty list, not an error.
	GetDomainBackends(name string) ([]BackendServer, error)
	// AddDomainBackend creates a server for a domain. spec.Service is ignored:
	// the domain name is the service.
	AddDomainBackend(domainName string, spec ServerSpec) (*BackendServer, error)
	// RemoveDomainBackend deletes a server of a domain by its server ID.
	RemoveDomainBackend(domainName string, backendID string) error
}

// Domain represents a DNS domain configuration.
type Domain struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	TTL  int    `json:"ttl"`
	// RoutingAlgorithm selects the routing algorithm. It has the name and the
	// values of the routing_algorithm key of a domain in the configuration file.
	RoutingAlgorithm string `json:"routing_algorithm"`
	// RoutingPolicy is the deprecated former name of RoutingAlgorithm. It
	// always holds the same value and will be removed in a later API version.
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

// normalizeRouting makes the canonical and the deprecated routing field agree.
// Domains stored by earlier versions carry only routing_policy.
func (d *Domain) normalizeRouting() {
	if d.RoutingAlgorithm == "" {
		d.RoutingAlgorithm = d.RoutingPolicy
	}
	d.RoutingPolicy = d.RoutingAlgorithm
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
	Backends    []BackendServer `json:"backends"`
	Total       int             `json:"total"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// DomainBackendCreateRequest is the request body for adding a backend to a
// domain. It is a server create request without the service, which is the
// domain in the path.
type DomainBackendCreateRequest struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight"`
	// Priority is stored and returned; no routing algorithm reads it.
	Priority int    `json:"priority"`
	Region   string `json:"region"`
	// Enabled defaults to true when it is left out.
	Enabled     *bool                     `json:"enabled,omitempty"`
	HealthCheck *ServerHealthCheckRequest `json:"health_check,omitempty"`
}

// DomainBackendResponse is the response for single backend operations.
type DomainBackendResponse struct {
	Backend BackendServer `json:"backend"`
}

// DomainCreateRequest is the request body for creating a domain.
type DomainCreateRequest struct {
	Name string `json:"name"`
	TTL  int    `json:"ttl"`
	// RoutingAlgorithm selects the routing algorithm.
	RoutingAlgorithm string `json:"routing_algorithm"`
	// RoutingPolicy is the deprecated former name of RoutingAlgorithm.
	RoutingPolicy string          `json:"routing_policy"`
	DNSSECEnabled bool            `json:"dnssec_enabled"`
	Enabled       bool            `json:"enabled"`
	Description   string          `json:"description,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	Settings      *DomainSettings `json:"settings,omitempty"`
}

// DomainUpdateRequest is the request body for updating a domain.
type DomainUpdateRequest struct {
	TTL *int `json:"ttl,omitempty"`
	// RoutingAlgorithm selects the routing algorithm.
	RoutingAlgorithm *string `json:"routing_algorithm,omitempty"`
	// RoutingPolicy is the deprecated former name of RoutingAlgorithm.
	RoutingPolicy *string         `json:"routing_policy,omitempty"`
	DNSSECEnabled *bool           `json:"dnssec_enabled,omitempty"`
	Enabled       *bool           `json:"enabled,omitempty"`
	Description   *string         `json:"description,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	Settings      *DomainSettings `json:"settings,omitempty"`
}

// routingAlgorithms are the values of routing_algorithm, as in the
// configuration file (pkg/routing).
var routingAlgorithms = []string{"round-robin", "weighted", "failover", "geolocation", "latency", "learned_latency"}

// resolveRoutingAlgorithm returns the algorithm a request names. algorithm is
// the canonical field and policy its deprecated alias; either may be empty.
// Naming two different algorithms is refused, as is an unknown one.
func resolveRoutingAlgorithm(algorithm, policy string) (string, error) {
	algorithm = strings.ToLower(strings.TrimSpace(algorithm))
	policy = strings.ToLower(strings.TrimSpace(policy))
	if algorithm != "" && policy != "" && algorithm != policy {
		return "", errInvalid("routing_algorithm (%q) and its deprecated alias routing_policy (%q) name different algorithms; send routing_algorithm only", algorithm, policy)
	}
	if algorithm == "" {
		algorithm = policy
	}
	if algorithm == "" {
		return "", nil
	}
	for _, known := range routingAlgorithms {
		if algorithm == known {
			return algorithm, nil
		}
	}
	return "", errInvalid("unknown routing_algorithm %q; must be one of %s", algorithm, strings.Join(routingAlgorithms, ", "))
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

	// Check for sub-resources: backends and backends/{id}
	if len(parts) == 2 {
		subResource := parts[1]
		switch {
		case subResource == "backends":
			h.handleDomainBackends(w, r, domainName, "")
		case strings.HasPrefix(subResource, "backends/"):
			h.handleDomainBackends(w, r, domainName, strings.TrimPrefix(subResource, "backends/"))
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
	if strings.ContainsAny(req.Name, ":/ \t\r\n") {
		h.writeError(w, http.StatusBadRequest, "name must be a DNS name; it cannot contain ':', '/' or white space")
		return
	}
	if req.TTL < 0 {
		h.writeError(w, http.StatusBadRequest, "ttl must not be negative")
		return
	}
	algorithm, err := resolveRoutingAlgorithm(req.RoutingAlgorithm, req.RoutingPolicy)
	if err != nil {
		h.writeProviderError(w, err, "create domain")
		return
	}

	domain := Domain{
		ID:               req.Name,
		Name:             req.Name,
		TTL:              req.TTL,
		RoutingAlgorithm: algorithm,
		DNSSECEnabled:    req.DNSSECEnabled,
		Enabled:          req.Enabled,
		Description:      req.Description,
		Tags:             req.Tags,
		Settings:         req.Settings,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	// Set defaults
	if domain.TTL == 0 {
		domain.TTL = 300
	}
	if domain.RoutingAlgorithm == "" {
		domain.RoutingAlgorithm = "round-robin"
	}
	domain.normalizeRouting()

	if err := h.provider.CreateDomain(domain); err != nil {
		h.writeProviderError(w, err, "create domain")
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
		if *req.TTL < 0 {
			h.writeError(w, http.StatusBadRequest, "ttl must not be negative")
			return
		}
		existing.TTL = *req.TTL
	}
	if req.RoutingAlgorithm != nil || req.RoutingPolicy != nil {
		var algorithm, policy string
		if req.RoutingAlgorithm != nil {
			algorithm = *req.RoutingAlgorithm
		}
		if req.RoutingPolicy != nil {
			policy = *req.RoutingPolicy
		}
		resolved, err := resolveRoutingAlgorithm(algorithm, policy)
		if err != nil {
			h.writeProviderError(w, err, "update domain")
			return
		}
		if resolved != "" {
			existing.RoutingAlgorithm = resolved
		}
	}
	existing.normalizeRouting()
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
		h.writeProviderError(w, err, "update domain")
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
		h.writeProviderError(w, err, "delete domain")
		return
	}

	h.logger.Info("domain deleted", "name", name)
	w.WriteHeader(http.StatusNoContent)
}

// handleDomainBackends handles /api/v1/domains/{name}/backends and
// /api/v1/domains/{name}/backends/{id}.
func (h *DomainHandlers) handleDomainBackends(w http.ResponseWriter, r *http.Request, domainName, backendID string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "domain provider not configured")
		return
	}

	if backendID != "" {
		// A single backend supports DELETE only. Read and change it as a
		// server: /api/v1/servers/{id}.
		if r.Method != http.MethodDelete {
			h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.removeDomainBackend(w, r, domainName, backendID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.listDomainBackends(w, r, domainName)
	case http.MethodPost:
		h.addDomainBackend(w, r, domainName)
	case http.MethodDelete:
		h.writeError(w, http.StatusBadRequest, "backend ID required for delete")
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listDomainBackends handles GET /api/v1/domains/{name}/backends.
func (h *DomainHandlers) listDomainBackends(w http.ResponseWriter, r *http.Request, domainName string) {
	backends, err := h.provider.GetDomainBackends(domainName)
	if err != nil {
		h.writeProviderError(w, err, "list backends")
		return
	}
	if backends == nil {
		backends = []BackendServer{}
	}

	resp := DomainBackendsResponse{
		Backends:    backends,
		Total:       len(backends),
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// addDomainBackend handles POST /api/v1/domains/{name}/backends.
func (h *DomainHandlers) addDomainBackend(w http.ResponseWriter, r *http.Request, domainName string) {
	var req DomainBackendCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.Address == "" {
		h.writeError(w, http.StatusBadRequest, "address is required")
		return
	}
	if req.Port == 0 {
		h.writeError(w, http.StatusBadRequest, "port is required")
		return
	}

	backend, err := h.provider.AddDomainBackend(domainName, ServerSpec{
		Service:     domainName,
		Address:     req.Address,
		Port:        req.Port,
		Weight:      req.Weight,
		Region:      req.Region,
		Priority:    req.Priority,
		Enabled:     req.Enabled == nil || *req.Enabled,
		HealthCheck: req.HealthCheck,
	})
	if err != nil {
		h.writeProviderError(w, err, "add backend")
		return
	}

	h.logger.Info("backend added to domain", "domain", domainName, "address", req.Address, "port", req.Port)
	h.writeJSON(w, http.StatusCreated, DomainBackendResponse{Backend: *backend})
}

// removeDomainBackend handles DELETE /api/v1/domains/{name}/backends/{id}.
func (h *DomainHandlers) removeDomainBackend(w http.ResponseWriter, r *http.Request, domainName, backendID string) {
	if err := h.provider.RemoveDomainBackend(domainName, backendID); err != nil {
		h.writeProviderError(w, err, "remove backend")
		return
	}

	h.logger.Info("backend removed from domain", "domain", domainName, "backend", backendID)
	w.WriteHeader(http.StatusNoContent)
}

// writeProviderError answers with the status a provider error maps to.
func (h *DomainHandlers) writeProviderError(w http.ResponseWriter, err error, action string) {
	status, message := errorStatus(err, action)
	if status >= http.StatusInternalServerError {
		h.logger.Error("domain API request failed", "action", action, "status", status, "error", err)
	}
	h.writeError(w, status, message)
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
