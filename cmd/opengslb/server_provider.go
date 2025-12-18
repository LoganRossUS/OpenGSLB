// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/api"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// backendRegistryServerProvider adapts the backend registry to the API server provider interface.
// v1.1.0: Enables CRUD operations on servers via REST API.
type backendRegistryServerProvider struct {
	registry *overwatch.Registry
}

// newBackendRegistryServerProvider creates a new server provider adapter.
func newBackendRegistryServerProvider(registry *overwatch.Registry) *backendRegistryServerProvider {
	return &backendRegistryServerProvider{
		registry: registry,
	}
}

// ListServers returns all registered backends as API BackendServer objects.
func (p *backendRegistryServerProvider) ListServers() []api.BackendServer {
	backends := p.registry.GetAllBackends()
	servers := make([]api.BackendServer, 0, len(backends))

	for _, backend := range backends {
		id := fmt.Sprintf("%s:%s:%d", backend.Service, backend.Address, backend.Port)
		servers = append(servers, p.backendToAPIServer(id, backend))
	}
	return servers
}

// GetServer returns a specific server by ID (format: service:address:port).
func (p *backendRegistryServerProvider) GetServer(id string) (*api.BackendServer, error) {
	backends := p.registry.GetAllBackends()

	for _, backend := range backends {
		backendID := fmt.Sprintf("%s:%s:%d", backend.Service, backend.Address, backend.Port)
		if backendID == id {
			server := p.backendToAPIServer(id, backend)
			return &server, nil
		}
	}

	return nil, errors.New("server not found")
}

// CreateServer creates a new server via API registration (Source=SourceAPI).
// v1.1.0: Requires service field to map server to domain.
func (p *backendRegistryServerProvider) CreateServer(server api.BackendServer) error {
	if server.Address == "" {
		return errors.New("address is required")
	}
	if server.Port == 0 {
		return errors.New("port is required")
	}
	if server.Region == "" {
		return errors.New("region is required")
	}

	// v1.1.0: Derive service from metadata or use Name
	service := server.Name
	if svc, ok := server.Metadata["service"]; ok {
		service = svc
	}
	if service == "" {
		return errors.New("service is required (set via name or metadata.service)")
	}

	weight := server.Weight
	if weight == 0 {
		weight = 100 // Default weight
	}

	// Register via API source
	return p.registry.RegisterAPI(service, server.Address, server.Port, weight, server.Region)
}

// UpdateServer updates an existing server.
func (p *backendRegistryServerProvider) UpdateServer(id string, server api.BackendServer) error {
	// For now, just support updating weight
	// TODO: Add UpdateWeight method to registry or implement full update logic
	return errors.New("update not yet implemented")
}

// DeleteServer removes a server from the registry.
func (p *backendRegistryServerProvider) DeleteServer(id string) error {
	// Parse ID (format: service:address:port)
	var service, address string
	var port int

	_, err := fmt.Sscanf(id, "%s:%s:%d", &service, &address, &port)
	if err != nil {
		return fmt.Errorf("invalid server ID format (expected service:address:port): %w", err)
	}

	return p.registry.Deregister(service, address, port)
}

// GetServerHealthCheck returns health check configuration for a server.
// Currently returns a default config since health checks are managed per-region.
func (p *backendRegistryServerProvider) GetServerHealthCheck(id string) (*api.ServerHealthCheck, error) {
	// Verify server exists
	if _, err := p.GetServer(id); err != nil {
		return nil, err
	}

	// Return default health check config (managed by external validation)
	return &api.ServerHealthCheck{
		Enabled:            true,
		Type:               "http",
		Path:               "/health",
		Interval:           3 * time.Second,
		Timeout:            5 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
	}, nil
}

// UpdateServerHealthCheck updates health check configuration for a server.
// Currently a no-op since health checks are managed per-region in config.
func (p *backendRegistryServerProvider) UpdateServerHealthCheck(id string, config api.ServerHealthCheck) error {
	// Verify server exists
	if _, err := p.GetServer(id); err != nil {
		return err
	}

	// Accept the request but don't apply changes
	return nil
}

// backendToAPIServer converts a Backend to an api.BackendServer.
func (p *backendRegistryServerProvider) backendToAPIServer(id string, backend *overwatch.Backend) api.BackendServer {
	var agentHealthy string
	if backend.AgentHealthy {
		agentHealthy = "healthy"
	} else {
		agentHealthy = "unhealthy"
	}

	return api.BackendServer{
		ID:       id,
		Name:     backend.Service,
		Address:  backend.Address,
		Port:     backend.Port,
		Protocol: "tcp",
		Weight:   backend.Weight,
		Region:   backend.Region,
		Enabled:  backend.EffectiveStatus != overwatch.StatusDraining,
		Healthy:  backend.EffectiveStatus == overwatch.StatusHealthy,
		Metadata: map[string]string{
			"service":          backend.Service,
			"source":           string(backend.Source),
			"effective_status": string(backend.EffectiveStatus),
			"agent_healthy":    agentHealthy,
			"agent_last_seen":  backend.AgentLastSeen.Format(time.RFC3339),
			"validation_check": backend.ValidationLastCheck.Format(time.RFC3339),
			"smoothed_latency": backend.SmoothedLatency.String(),
		},
		CreatedAt: backend.CreatedAt,
		UpdatedAt: backend.UpdatedAt,
	}
}
