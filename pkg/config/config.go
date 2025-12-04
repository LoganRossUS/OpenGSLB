package dns

import (
	"fmt"
	"net"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// RouterFactory creates a router for a given algorithm name.
type RouterFactory func(algorithm string) (Router, error)

// BuildRegistry creates a domain registry from application configuration.
func BuildRegistry(cfg *config.Config, routerFactory RouterFactory) (*Registry, error) {
	registry := NewRegistry()

	// Build a map of region name -> servers for quick lookup
	regionServers := make(map[string][]ServerInfo)
	for _, region := range cfg.Regions {
		servers := make([]ServerInfo, 0, len(region.Servers))
		for _, s := range region.Servers {
			ip := net.ParseIP(s.Address)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP address %q in region %s", s.Address, region.Name)
			}
			servers = append(servers, ServerInfo{
				Address: ip,
				Port:    s.Port,
				Weight:  s.Weight,
				Region:  region.Name,
			})
		}
		regionServers[region.Name] = servers
	}

	// Build domain entries
	for _, domain := range cfg.Domains {
		var allServers []ServerInfo

		// Collect servers from all regions assigned to this domain
		for _, regionName := range domain.Regions {
			servers, ok := regionServers[regionName]
			if !ok {
				return nil, fmt.Errorf("domain %s references unknown region %s", domain.Name, regionName)
			}
			allServers = append(allServers, servers...)
		}

		if len(allServers) == 0 {
			return nil, fmt.Errorf("domain %s has no servers", domain.Name)
		}

		// Create router for this domain
		algorithm := domain.RoutingAlgorithm
		if algorithm == "" {
			algorithm = "round-robin"
		}

		router, err := routerFactory(algorithm)
		if err != nil {
			return nil, fmt.Errorf("failed to create router for domain %s: %w", domain.Name, err)
		}

		ttl := uint32(domain.TTL)

		entry := &DomainEntry{
			Name:             domain.Name,
			TTL:              ttl,
			RoutingAlgorithm: algorithm,
			Servers:          allServers,
			Router:           router,
		}

		registry.Register(entry)
	}

	return registry, nil
}