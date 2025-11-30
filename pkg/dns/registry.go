// Package dns provides the DNS server implementation for OpenGSLB.
package dns

import (
	"net"
	"sync"
)

// ServerInfo contains information about a backend server.
type ServerInfo struct {
	Address net.IP
	Port    int
	Weight  int
	Region  string
}

// DomainEntry contains configuration for a single domain.
type DomainEntry struct {
	Name             string
	TTL              uint32
	RoutingAlgorithm string
	Servers          []ServerInfo
}

// Registry provides thread-safe lookup of domain configurations.
type Registry struct {
	mu      sync.RWMutex
	domains map[string]*DomainEntry
}

// NewRegistry creates a new empty domain registry.
func NewRegistry() *Registry {
	return &Registry{
		domains: make(map[string]*DomainEntry),
	}
}

// Register adds or updates a domain entry in the registry.
func (r *Registry) Register(entry *DomainEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Normalize domain name to ensure consistent lookups
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

// normalizeDomain ensures domain names are in a consistent format.
// DNS queries typically have a trailing dot (FQDN), so we ensure all
// stored and queried names use the same format.
func normalizeDomain(name string) string {
	if len(name) == 0 {
		return name
	}
	// Ensure trailing dot for FQDN format
	if name[len(name)-1] != '.' {
		return name + "."
	}
	return name
}
