// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"fmt"
	"net"
	"sync"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
)

// RouterFactory is a function type that creates routers by algorithm name.
type RouterFactory func(algorithm string) (routing.Router, error)

// Registry provides thread-safe lookup of domain configurations.
type Registry struct {
	mu      sync.RWMutex
	domains map[string]*DomainEntry
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	return &Registry{
		domains: make(map[string]*DomainEntry),
	}
}

// BuildRegistry creates a registry from configuration.
func BuildRegistry(cfg *config.Config, routerFactory RouterFactory) (*Registry, error) {
	registry := NewRegistry()

	// Build a map of region name -> servers for quick lookup
	regionServers := make(map[string][]ServerInfo)
	for _, region := range cfg.Regions {
		var servers []ServerInfo
		for _, server := range region.Servers {
			ip := net.ParseIP(server.Address)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP address for server in region %s: %s", region.Name, server.Address)
			}
			servers = append(servers, ServerInfo{
				Address: ip,
				Port:    server.Port,
				Weight:  server.Weight,
				Region:  region.Name,
			})
		}
		regionServers[region.Name] = servers
	}

	// Build domain entries
	for _, domain := range cfg.Domains {
		router, err := routerFactory(domain.RoutingAlgorithm)
		if err != nil {
			return nil, fmt.Errorf("failed to create router for domain %s: %w", domain.Name, err)
		}

		// Collect servers from all regions for this domain
		var servers []ServerInfo
		for _, regionName := range domain.Regions {
			if regionServerList, ok := regionServers[regionName]; ok {
				servers = append(servers, regionServerList...)
			}
		}

		ttl := uint32(domain.TTL)
		if ttl == 0 {
			ttl = uint32(cfg.DNS.DefaultTTL)
		}

		entry := &DomainEntry{
			Name:             domain.Name,
			TTL:              ttl,
			RoutingAlgorithm: domain.RoutingAlgorithm,
			Router:           router,
			Servers:          servers,
		}

		registry.Register(entry)
	}

	return registry, nil
}

// Register adds or updates a domain entry in the registry.
func (r *Registry) Register(entry *DomainEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := normalizeDomain(entry.Name)
	entry.Name = name
	r.domains[name] = entry
}

// Lookup retrieves a domain entry by name.
// Returns nil if the domain is not registered.
func (r *Registry) Lookup(name string) *DomainEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.domains[normalizeDomain(name)]
}

// Remove deletes a domain from the registry.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.domains, normalizeDomain(name))
}

// Domains returns a list of all registered domain names.
func (r *Registry) Domains() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.domains))
	for name := range r.domains {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered domains.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.domains)
}

// Clear removes all domains from the registry.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.domains = make(map[string]*DomainEntry)
}

// ReplaceAll atomically replaces all domain entries in the registry.
// This is used during configuration hot-reload.
func (r *Registry) ReplaceAll(entries []*DomainEntry) {
	newDomains := make(map[string]*DomainEntry, len(entries))
	for _, entry := range entries {
		name := normalizeDomain(entry.Name)
		entry.Name = name
		newDomains[name] = entry
	}
	r.mu.Lock()
	r.domains = newDomains
	r.mu.Unlock()
}

// normalizeDomain ensures domain names are in a consistent format.
func normalizeDomain(name string) string {
	if len(name) == 0 {
		return name
	}
	if name[len(name)-1] != '.' {
		return name + "."
	}
	return name
}
