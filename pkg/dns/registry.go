// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/miekg/dns"
)

// RouterFactory is a function type that creates routers by algorithm name.
type RouterFactory func(algorithm string) (routing.Router, error)

// Registry provides thread-safe lookup of domain configurations.
type Registry struct {
	mu      sync.RWMutex
	domains map[string]*DomainEntry
	// below counts, for every proper ancestor of a registered domain, how
	// many registered domains lie beneath it. A name with a non-zero count
	// exists in the DNS tree even when it is not registered itself (an
	// "empty non-terminal") and must not be answered with NXDOMAIN.
	below map[string]int
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	r := &Registry{
		domains: make(map[string]*DomainEntry),
		below:   make(map[string]int),
	}
	slog.Debug("DNS registry created", "registry_ptr", fmt.Sprintf("%p", r))
	return r
}

// BuildRegistry creates a registry from configuration.
func BuildRegistry(cfg *config.Config, routerFactory RouterFactory) (*Registry, error) {
	registry := NewRegistry()

	// v1.1.0: Build a map of (region, service) -> servers for filtered lookup
	// This allows servers in the same region to be filtered by service
	type regionServiceKey struct {
		region  string
		service string
	}
	regionServiceServers := make(map[regionServiceKey][]ServerInfo)

	for _, region := range cfg.Regions {
		for _, server := range region.Servers {
			ip := net.ParseIP(server.Address)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP address for server in region %s: %s", region.Name, server.Address)
			}

			key := regionServiceKey{
				region:  region.Name,
				service: server.Service, // v1.1.0: Service is required
			}

			regionServiceServers[key] = append(regionServiceServers[key], ServerInfo{
				Address: ip,
				Port:    server.Port,
				Weight:  server.Weight,
				Region:  region.Name,
			})
		}
	}

	// Build domain entries
	for _, domain := range cfg.Domains {
		router, err := routerFactory(domain.RoutingAlgorithm)
		if err != nil {
			return nil, fmt.Errorf("failed to create router for domain %s: %w", domain.Name, err)
		}

		// v1.1.0: Collect servers that match this domain's service name from specified regions
		var servers []ServerInfo
		for _, regionName := range domain.Regions {
			key := regionServiceKey{
				region:  regionName,
				service: domain.Name, // Match servers where service == domain name
			}
			if regionServerList, ok := regionServiceServers[key]; ok {
				servers = append(servers, regionServerList...)
			}
		}

		ttl := config.TTLSeconds(domain.TTL)
		if ttl == 0 {
			ttl = config.TTLSeconds(cfg.DNS.DefaultTTL)
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
	if _, exists := r.domains[name]; !exists {
		countAncestors(r.below, name, 1)
	}
	r.domains[name] = entry
	// Log registry state after registration (INFO level for runtime visibility)
	slog.Info("domain registered in DNS registry",
		"name", name,
		"registry_count", len(r.domains),
		"registry_ptr", fmt.Sprintf("%p", r),
	)
}

// Lookup retrieves a domain entry by name.
// Returns nil if the domain is not registered.
//
// Lookup runs for every DNS query, so it only logs at DEBUG: a miss is an
// ordinary event (any query for an unknown name) and logging it at INFO
// floods the log under load. The level check comes first so that the log
// arguments cost nothing when debug logging is off.
func (r *Registry) Lookup(name string) *DomainEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	normalizedName := normalizeDomain(name)
	entry := r.domains[normalizedName]
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		msg := "DNS registry lookup - found"
		if entry == nil {
			msg = "DNS registry lookup - domain not found"
		}
		slog.Debug(msg,
			"query_name", name,
			"normalized_name", normalizedName,
			"registry_count", len(r.domains),
			"registry_ptr", fmt.Sprintf("%p", r),
		)
	}
	return entry
}

// closestDomain returns the registered domain that is name itself or its
// closest ancestor, or "" when there is none. name must be canonical. It is
// used to find the implicit zone of a name outside every configured zone and
// does not log.
func (r *Registry) closestDomain(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for off, end := 0, false; !end; off, end = dns.NextLabel(name, off) {
		if _, ok := r.domains[name[off:]]; ok {
			return name[off:]
		}
	}
	return ""
}

// Remove deletes a domain from the registry.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name = normalizeDomain(name)
	if _, exists := r.domains[name]; exists {
		countAncestors(r.below, name, -1)
		delete(r.domains, name)
	}
}

// hasDescendant reports whether a registered domain lies beneath name.
// name must be canonical.
func (r *Registry) hasDescendant(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.below[name] > 0
}

// countAncestors adds delta to the count of every proper ancestor of name,
// up to but not including the root, and drops counts that reach zero.
func countAncestors(below map[string]int, name string, delta int) {
	for off, end := dns.NextLabel(name, 0); !end; off, end = dns.NextLabel(name, off) {
		ancestor := name[off:]
		if n := below[ancestor] + delta; n > 0 {
			below[ancestor] = n
		} else {
			delete(below, ancestor)
		}
	}
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
	r.below = make(map[string]int)
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
	newBelow := make(map[string]int)
	for name := range newDomains {
		countAncestors(newBelow, name, 1)
	}
	r.mu.Lock()
	r.domains = newDomains
	r.below = newBelow
	r.mu.Unlock()
}

// RegisterServer dynamically adds or updates a server in the DNS registry.
// This is called when:
// - An agent heartbeat is received (agent self-registration)
// - A server is added via API
// v1.1.0: Enables dynamic server registration for unified architecture
func (r *Registry) RegisterServer(service string, address string, port int, weight int, region string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	domainName := normalizeDomain(service)
	entry, exists := r.domains[domainName]
	if !exists {
		return fmt.Errorf("domain %q not configured", service)
	}

	// Parse IP address
	ip := net.ParseIP(address)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", address)
	}

	// Create server info
	serverInfo := ServerInfo{
		Address: ip,
		Port:    port,
		Weight:  weight,
		Region:  region,
	}

	// Work on a copy: query handlers read the published entry without a lock.
	entry = cloneEntry(entry, 1)

	// Check if server already exists, update if so
	serverKey := fmt.Sprintf("%s:%d", address, port)
	found := false
	for i, existingServer := range entry.Servers {
		existingKey := fmt.Sprintf("%s:%d", existingServer.Address.String(), existingServer.Port)
		if existingKey == serverKey {
			entry.Servers[i] = serverInfo
			found = true
			break
		}
	}

	// Add if not found
	if !found {
		entry.Servers = append(entry.Servers, serverInfo)
	}

	r.domains[domainName] = entry
	return nil
}

// cloneEntry returns a copy of entry with its own server slice, with room for
// extra more servers.
//
// Lookup hands out the entry pointer and its callers read the entry after the
// registry lock is released, so a published entry must never be written to.
// Every change to a domain is made on a copy that then replaces the published
// entry under the write lock.
func cloneEntry(entry *DomainEntry, extra int) *DomainEntry {
	clone := *entry
	clone.Servers = make([]ServerInfo, len(entry.Servers), len(entry.Servers)+extra)
	copy(clone.Servers, entry.Servers)
	return &clone
}

// DeregisterServer removes a server from the DNS registry.
// This is called when:
// - An agent goes stale/deregisters
// - A server is removed via API
// v1.1.0: Enables dynamic server removal for unified architecture
func (r *Registry) DeregisterServer(service string, address string, port int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	domainName := normalizeDomain(service)
	entry, exists := r.domains[domainName]
	if !exists {
		return fmt.Errorf("domain %q not configured", service)
	}

	// Find and remove server
	serverKey := fmt.Sprintf("%s:%d", address, port)
	for i, server := range entry.Servers {
		existingKey := fmt.Sprintf("%s:%d", server.Address.String(), server.Port)
		if existingKey == serverKey {
			// Remove by swapping with last element and truncating, on a
			// copy (see cloneEntry)
			entry = cloneEntry(entry, 0)
			entry.Servers[i] = entry.Servers[len(entry.Servers)-1]
			entry.Servers = entry.Servers[:len(entry.Servers)-1]
			r.domains[domainName] = entry
			return nil
		}
	}

	return fmt.Errorf("server %s:%d not found in domain %q", address, port, service)
}

// UpdateServerWeight updates the weight of an existing server.
// v1.1.0: Enables dynamic weight adjustment
func (r *Registry) UpdateServerWeight(service string, address string, port int, weight int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	domainName := normalizeDomain(service)
	entry, exists := r.domains[domainName]
	if !exists {
		return fmt.Errorf("domain %q not configured", service)
	}

	// Find and update server
	serverKey := fmt.Sprintf("%s:%d", address, port)
	for i, server := range entry.Servers {
		existingKey := fmt.Sprintf("%s:%d", server.Address.String(), server.Port)
		if existingKey == serverKey {
			entry = cloneEntry(entry, 0)
			entry.Servers[i].Weight = weight
			r.domains[domainName] = entry
			return nil
		}
	}

	return fmt.Errorf("server %s:%d not found in domain %q", address, port, service)
}

// RegisterDomain creates and registers a new domain entry with the given parameters.
// This is used for dynamic domain creation via API.
// The domain is created with an empty server list; servers are added via RegisterServer.
func (r *Registry) RegisterDomain(name string, ttl uint32, algorithm string, routerFactory RouterFactory) error {
	router, err := routerFactory(algorithm)
	if err != nil {
		return fmt.Errorf("failed to create router for domain %s: %w", name, err)
	}

	entry := &DomainEntry{
		Name:             name,
		TTL:              ttl,
		RoutingAlgorithm: algorithm,
		Router:           router,
		Servers:          []ServerInfo{}, // Start with no servers
	}

	r.Register(entry)
	return nil
}

// RegisterDomainDynamic creates and registers a new domain entry with a generic factory.
// This is used by the API layer where the concrete routing.Router type isn't available.
// The routerFactory must return a routing.Router compatible type.
func (r *Registry) RegisterDomainDynamic(name string, ttl uint32, algorithm string, routerFactory func(string) (interface{}, error)) error {
	// Wrap the generic factory to return the typed Router
	typedFactory := func(alg string) (routing.Router, error) {
		result, err := routerFactory(alg)
		if err != nil {
			return nil, err
		}
		router, ok := result.(routing.Router)
		if !ok {
			return nil, fmt.Errorf("router factory returned non-Router type: %T", result)
		}
		return router, nil
	}
	return r.RegisterDomain(name, ttl, algorithm, typedFactory)
}

// UpdateDomainSettings updates a domain's TTL and routing algorithm while preserving servers.
// This is used by the API layer when a domain is updated.
func (r *Registry) UpdateDomainSettings(name string, ttl uint32, algorithm string, routerFactory func(string) (interface{}, error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	domainName := normalizeDomain(name)
	existing, exists := r.domains[domainName]
	if !exists {
		return fmt.Errorf("domain %q not found in DNS registry", name)
	}

	// Create new router if algorithm changed
	result, err := routerFactory(algorithm)
	if err != nil {
		return fmt.Errorf("failed to create router: %w", err)
	}
	router, ok := result.(routing.Router)
	if !ok {
		return fmt.Errorf("router factory returned non-Router type: %T", result)
	}

	// Update settings while preserving servers, on a copy (see cloneEntry)
	existing = cloneEntry(existing, 0)
	existing.TTL = ttl
	existing.Router = router
	r.domains[domainName] = existing

	slog.Info("domain settings updated in DNS registry",
		"name", domainName,
		"ttl", ttl,
		"algorithm", algorithm,
	)

	return nil
}

// normalizeDomain returns the canonical form used as the registry key:
// lower case with a trailing dot. DNS names compare without regard to case
// (RFC 1035 section 2.3.3, RFC 4343), so "APP.Example.COM" and
// "app.example.com." are the same name.
func normalizeDomain(name string) string {
	if len(name) == 0 {
		return name
	}
	// strings.ToLower returns its argument unchanged (no allocation) when the
	// name is already lower case, which is the common case on the query path.
	name = strings.ToLower(name)
	if name[len(name)-1] != '.' {
		return name + "."
	}
	return name
}
