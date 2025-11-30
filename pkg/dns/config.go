package dns

import (
	"fmt"
	"net"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// BuildRegistry creates a domain registry from application configuration.
// It maps each domain to its servers from the configured regions.
func BuildRegistry(cfg *config.Config) (*Registry, error) {
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

		// Determine TTL - use domain-specific if set, otherwise will use default
		ttl := uint32(domain.TTL)

		entry := &DomainEntry{
			Name:             domain.Name,
			TTL:              ttl,
			RoutingAlgorithm: domain.RoutingAlgorithm,
			Servers:          allServers,
		}

		registry.Register(entry)
	}

	return registry, nil
}
