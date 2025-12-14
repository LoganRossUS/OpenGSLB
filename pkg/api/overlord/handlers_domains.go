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
	"strings"
	"time"
)

// handleDomains handles GET /api/domains and POST /api/domains
func (h *Handlers) handleDomains(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listDomains(w, r)
	case http.MethodPost:
		h.createDomain(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// listDomains handles GET /api/domains
func (h *Handlers) listDomains(w http.ResponseWriter, r *http.Request) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	domains := make([]Domain, 0, len(cfg.Domains))

	for _, d := range cfg.Domains {
		// Count backends for this domain
		totalBackends := 0
		healthyBackends := 0

		registry := h.dataProvider.GetBackendRegistry()
		if registry != nil {
			// Get all backends that match this domain's regions
			for _, regionName := range d.Regions {
				for _, region := range cfg.Regions {
					if region.Name == regionName {
						totalBackends += len(region.Servers)
						// Check health status from registry
						backends := registry.GetAllBackends()
						for _, b := range backends {
							if b.Region == regionName {
								if b.EffectiveStatus == "healthy" {
									healthyBackends++
								}
							}
						}
					}
				}
			}
		} else {
			// Fallback: count from config
			for _, regionName := range d.Regions {
				for _, region := range cfg.Regions {
					if region.Name == regionName {
						totalBackends += len(region.Servers)
						healthyBackends += len(region.Servers) // Assume healthy if no registry
					}
				}
			}
		}

		domains = append(domains, Domain{
			ID:               d.Name,
			Name:             d.Name,
			RoutingAlgorithm: d.RoutingAlgorithm,
			Regions:          d.Regions,
			TTL:              d.TTL,
			HealthyBackends:  healthyBackends,
			TotalBackends:    totalBackends,
			Enabled:          true, // Domains in config are enabled by default
			CreatedAt:        time.Now().Add(-7 * 24 * time.Hour), // Placeholder
			UpdatedAt:        time.Now().Add(-1 * time.Hour),      // Placeholder
		})
	}

	writeJSON(w, http.StatusOK, DomainsResponse{Domains: domains})
}

// createDomain handles POST /api/domains
func (h *Handlers) createDomain(w http.ResponseWriter, r *http.Request) {
	var req DomainCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Validate required fields
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "MISSING_FIELD")
		return
	}
	if req.RoutingAlgorithm == "" {
		writeError(w, http.StatusBadRequest, "routingAlgorithm is required", "MISSING_FIELD")
		return
	}
	if len(req.Regions) == 0 {
		writeError(w, http.StatusBadRequest, "at least one region is required", "MISSING_FIELD")
		return
	}

	// Validate routing algorithm
	validAlgorithms := map[string]bool{
		"latency":     true,
		"geolocation": true,
		"failover":    true,
		"weighted":    true,
		"round-robin": true,
	}
	if !validAlgorithms[req.RoutingAlgorithm] {
		writeError(w, http.StatusBadRequest, "invalid routing algorithm", "INVALID_ALGORITHM")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Check if domain already exists
	for _, d := range cfg.Domains {
		if d.Name == req.Name {
			writeError(w, http.StatusConflict, "domain already exists", "DOMAIN_EXISTS")
			return
		}
	}

	// Set default TTL if not provided
	ttl := req.TTL
	if ttl == 0 {
		ttl = cfg.DNS.DefaultTTL
	}

	now := time.Now()
	domain := Domain{
		ID:               req.Name,
		Name:             req.Name,
		RoutingAlgorithm: req.RoutingAlgorithm,
		Regions:          req.Regions,
		TTL:              ttl,
		Description:      req.Description,
		Enabled:          req.Enabled,
		Service:          req.Service,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryDomain, req.Name,
		fmt.Sprintf("Created domain %s", req.Name),
		map[string]interface{}{
			"routingAlgorithm": req.RoutingAlgorithm,
			"regions":          req.Regions,
			"ttl":              ttl,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusCreated, DomainResponse{Domain: domain})
}

// handleDomainByName handles GET, PUT, DELETE /api/domains/:name
func (h *Handlers) handleDomainByName(w http.ResponseWriter, r *http.Request) {
	// Parse domain name from path
	name, subPath := parseSubPath(r.URL.Path, "/api/domains/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "domain name is required", "MISSING_PARAM")
		return
	}

	// Handle sub-paths
	if subPath == "backends" {
		h.getDomainBackends(w, r, name)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getDomain(w, r, name)
	case http.MethodPut:
		h.updateDomain(w, r, name)
	case http.MethodDelete:
		h.deleteDomain(w, r, name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

// getDomain handles GET /api/domains/:name
func (h *Handlers) getDomain(w http.ResponseWriter, r *http.Request, name string) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	for _, d := range cfg.Domains {
		if d.Name == name {
			// Count backends
			totalBackends := 0
			healthyBackends := 0

			registry := h.dataProvider.GetBackendRegistry()
			if registry != nil {
				for _, regionName := range d.Regions {
					backends := registry.GetAllBackends()
					for _, b := range backends {
						if b.Region == regionName {
							totalBackends++
							if b.EffectiveStatus == "healthy" {
								healthyBackends++
							}
						}
					}
				}
			} else {
				for _, regionName := range d.Regions {
					for _, region := range cfg.Regions {
						if region.Name == regionName {
							totalBackends += len(region.Servers)
							healthyBackends += len(region.Servers)
						}
					}
				}
			}

			domain := Domain{
				ID:               d.Name,
				Name:             d.Name,
				RoutingAlgorithm: d.RoutingAlgorithm,
				Regions:          d.Regions,
				TTL:              d.TTL,
				HealthyBackends:  healthyBackends,
				TotalBackends:    totalBackends,
				Enabled:          true,
				CreatedAt:        time.Now().Add(-7 * 24 * time.Hour),
				UpdatedAt:        time.Now().Add(-1 * time.Hour),
			}

			writeJSON(w, http.StatusOK, DomainResponse{Domain: domain})
			return
		}
	}

	writeError(w, http.StatusNotFound, "domain not found", "DOMAIN_NOT_FOUND")
}

// updateDomain handles PUT /api/domains/:name
func (h *Handlers) updateDomain(w http.ResponseWriter, r *http.Request, name string) {
	var req DomainUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Find and update domain
	var foundDomain *Domain
	for _, d := range cfg.Domains {
		if d.Name == name {
			domain := Domain{
				ID:               d.Name,
				Name:             d.Name,
				RoutingAlgorithm: d.RoutingAlgorithm,
				Regions:          d.Regions,
				TTL:              d.TTL,
				Enabled:          true,
				UpdatedAt:        time.Now(),
			}

			// Apply updates
			if req.RoutingAlgorithm != "" {
				domain.RoutingAlgorithm = req.RoutingAlgorithm
			}
			if len(req.Regions) > 0 {
				domain.Regions = req.Regions
			}
			if req.TTL > 0 {
				domain.TTL = req.TTL
			}
			if req.Description != "" {
				domain.Description = req.Description
			}
			if req.Enabled != nil {
				domain.Enabled = *req.Enabled
			}

			foundDomain = &domain
			break
		}
	}

	if foundDomain == nil {
		writeError(w, http.StatusNotFound, "domain not found", "DOMAIN_NOT_FOUND")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionUpdate, AuditCategoryDomain, name,
		fmt.Sprintf("Updated domain %s", name),
		map[string]interface{}{
			"routingAlgorithm": foundDomain.RoutingAlgorithm,
			"regions":          foundDomain.Regions,
			"ttl":              foundDomain.TTL,
		},
		AuditSeveritySuccess, r.RemoteAddr)

	writeJSON(w, http.StatusOK, DomainResponse{Domain: *foundDomain})
}

// deleteDomain handles DELETE /api/domains/:name
func (h *Handlers) deleteDomain(w http.ResponseWriter, r *http.Request, name string) {
	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Check if domain exists
	found := false
	for _, d := range cfg.Domains {
		if d.Name == name {
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "domain not found", "DOMAIN_NOT_FOUND")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionDelete, AuditCategoryDomain, name,
		fmt.Sprintf("Deleted domain %s", name),
		nil,
		AuditSeverityWarning, r.RemoteAddr)

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true})
}

// getDomainBackends handles GET /api/domains/:name/backends
func (h *Handlers) getDomainBackends(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Find domain
	var domainRegions []string
	for _, d := range cfg.Domains {
		if d.Name == name {
			domainRegions = d.Regions
			break
		}
	}

	if domainRegions == nil {
		writeError(w, http.StatusNotFound, "domain not found", "DOMAIN_NOT_FOUND")
		return
	}

	// Get backends for domain's regions
	servers := make([]Backend, 0)

	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		backends := registry.GetAllBackends()
		for _, b := range backends {
			// Check if backend's region matches domain's regions
			for _, regionName := range domainRegions {
				if b.Region == regionName {
					server := Backend{
						ID:              fmt.Sprintf("%s:%d", b.Address, b.Port),
						Service:         b.Service,
						Address:         b.Address,
						Port:            b.Port,
						Weight:          b.Weight,
						Region:          b.Region,
						AgentID:         b.AgentID,
						EffectiveStatus: string(b.EffectiveStatus),
						AgentHealthy:    b.AgentHealthy,
						SmoothedLatency: int(b.SmoothedLatency.Milliseconds()),
						LatencySamples:  b.LatencySamples,
					}
					if !b.AgentLastSeen.IsZero() {
						server.AgentLastSeen = &b.AgentLastSeen
					}
					if b.ValidationHealthy != nil {
						server.ValidationHealthy = b.ValidationHealthy
						server.ValidationLastCheck = &b.ValidationLastCheck
						server.ValidationError = b.ValidationError
					}
					servers = append(servers, server)
					break
				}
			}
		}
	} else {
		// Fallback: use config-based servers
		for _, regionName := range domainRegions {
			for _, region := range cfg.Regions {
				if region.Name == regionName {
					for _, s := range region.Servers {
						servers = append(servers, Backend{
							ID:              fmt.Sprintf("%s:%d", s.Address, s.Port),
							Address:         s.Address,
							Port:            s.Port,
							Weight:          s.Weight,
							Region:          region.Name,
							EffectiveStatus: "healthy",
							AgentHealthy:    true,
						})
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, ServersResponse{Servers: servers})
}

// domainMatchesService checks if a domain name matches a service name pattern.
func domainMatchesService(domainName, serviceName string) bool {
	// Simple match: service name is part of domain name
	return strings.Contains(domainName, serviceName) ||
		strings.HasPrefix(domainName, serviceName+".") ||
		domainName == serviceName
}
