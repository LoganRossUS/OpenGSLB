// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/health"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
	"github.com/loganrossus/OpenGSLB/pkg/store"
	"github.com/loganrossus/OpenGSLB/pkg/version"
)

// Common errors for providers.
var (
	ErrNotFound       = errors.New("not found")
	ErrNotImplemented = errors.New("not implemented")
)

// =============================================================================
// Registry-based providers
// =============================================================================

// RegistryInterface defines the methods needed from the backend registry.
type RegistryInterface interface {
	GetAllBackends() []*overwatch.Backend
	GetBackends(service string) []*overwatch.Backend
	GetBackend(service, address string, port int) (*overwatch.Backend, bool)
}

// DNSRegistryInterface defines the methods needed from the DNS registry.
// This allows API-created domains to be dynamically registered with the DNS server.
type DNSRegistryInterface interface {
	// RegisterDomainDynamic creates and registers a new domain entry with the given parameters.
	// The routerFactory must return a routing.Router compatible type.
	RegisterDomainDynamic(name string, ttl uint32, algorithm string, routerFactory func(string) (interface{}, error)) error
	// UpdateDomainSettings updates a domain's TTL and routing algorithm while preserving servers.
	UpdateDomainSettings(name string, ttl uint32, algorithm string, routerFactory func(string) (interface{}, error)) error
	// Remove deletes a domain from the DNS registry.
	Remove(name string)
	// RegisterServer dynamically adds or updates a server in the DNS registry.
	RegisterServer(service string, address string, port int, weight int, region string) error
	// DeregisterServer removes a server from the DNS registry.
	DeregisterServer(service string, address string, port int) error
}

// RegistryDomainProvider implements DomainProvider using the backend registry.
type RegistryDomainProvider struct {
	registry      RegistryInterface
	config        *config.Config
	store         store.Store
	persistence   persistence
	logger        *slog.Logger
	dnsRegistry   DNSRegistryInterface
	routerFactory func(string) (interface{}, error)
	// servers owns the servers of every domain. The backends of a domain are
	// the servers whose service is the domain name.
	servers *RegistryServerProvider
}

// NewRegistryDomainProvider creates a new RegistryDomainProvider.
func NewRegistryDomainProvider(registry RegistryInterface, cfg *config.Config, logger *slog.Logger) *RegistryDomainProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &RegistryDomainProvider{
		registry: registry,
		config:   cfg,
		logger:   logger,
	}
}

// SetStore sets the store for persistence operations.
func (p *RegistryDomainProvider) SetStore(s store.Store) {
	p.store = s
}

// SetStoreUnavailable records why there is no store. The reason is part of
// the 503 that answers every write while the provider has no store.
func (p *RegistryDomainProvider) SetStoreUnavailable(reason string) {
	p.persistence.unavailable = reason
}

// SetServerProvider sets the provider that owns the servers. The backends of
// a domain are read from it and written through it, so a backend added to a
// domain and a server registered for that domain are the same object.
func (p *RegistryDomainProvider) SetServerProvider(servers *RegistryServerProvider) {
	p.servers = servers
}

// HasDomain reports whether name is a domain of the configuration file or a
// domain created through the API. A service that only has backends (for
// example from an agent) and no domain is not one: it is not served in DNS.
func (p *RegistryDomainProvider) HasDomain(name string) bool {
	if p.config != nil {
		for _, d := range p.config.Domains {
			if d.Name == name {
				return true
			}
		}
	}
	if p.store != nil {
		if _, err := p.store.Get(context.Background(), store.PrefixDomains+name); err == nil {
			return true
		}
	}
	return false
}

// SetDNSRegistry sets the DNS registry for dynamic domain registration.
// This enables API-created domains to be automatically registered with the DNS server.
func (p *RegistryDomainProvider) SetDNSRegistry(registry DNSRegistryInterface) {
	p.dnsRegistry = registry
	p.logger.Info("DNS registry set for domain provider",
		"registry_type", fmt.Sprintf("%T", registry),
		"registry_ptr", fmt.Sprintf("%p", registry),
	)
}

// SetRouterFactory sets the router factory for creating routers for new domains.
// The factory must return a routing.Router compatible type.
func (p *RegistryDomainProvider) SetRouterFactory(factory func(string) (interface{}, error)) {
	p.routerFactory = factory
}

// LoadStoredDomainsIntoDNS loads API-created domains and their backends from the store
// and registers them with the DNS registry. This should be called on startup after
// the DNS registry and router factory are set.
func (p *RegistryDomainProvider) LoadStoredDomainsIntoDNS() error {
	p.logger.Info("loading stored domains into DNS registry")

	if p.store == nil {
		p.logger.Warn("no store configured, skipping stored domain loading")
		return nil
	}
	if p.dnsRegistry == nil {
		p.logger.Warn("no DNS registry configured, skipping stored domain loading")
		return nil
	}
	if p.routerFactory == nil {
		p.logger.Warn("no router factory configured, skipping stored domain loading")
		return nil
	}

	// Load all stored domains
	pairs, err := p.store.List(context.Background(), store.PrefixDomains)
	if err != nil {
		return fmt.Errorf("failed to list stored domains: %w", err)
	}

	p.logger.Info("found stored domains to load", "count", len(pairs))

	loadedDomains := 0
	loadedBackends := 0

	for _, pair := range pairs {
		var domain Domain
		if err := json.Unmarshal(pair.Value, &domain); err != nil {
			p.logger.Warn("failed to unmarshal stored domain", "key", pair.Key, "error", err)
			continue
		}

		domain.normalizeRouting()

		// Register domain with DNS registry
		ttl := config.TTLSeconds(domain.TTL)
		if ttl == 0 {
			ttl = config.TTLSeconds(config.DefaultTTL)
		}
		algorithm := domain.RoutingAlgorithm
		if algorithm == "" {
			algorithm = config.DefaultRoutingAlgorithm
		}

		if err := p.dnsRegistry.RegisterDomainDynamic(domain.Name, ttl, algorithm, p.routerFactory); err != nil {
			p.logger.Warn("failed to register stored domain with DNS",
				"domain", domain.Name,
				"error", err,
			)
			continue
		}
		loadedDomains++

		// The servers of the domain live in the backend registry, which keeps
		// them in the store under backends/. Servers that are in the registry
		// already (this function may run again after a reload) are put back
		// into the new DNS entry; the rest follow when the registry loads its
		// store and reports each one. A disabled server is not answered.
		for _, backend := range p.registry.GetBackends(domain.Name) {
			if backend.Source == overwatch.SourceStatic || backend.Disabled {
				continue
			}
			if err := p.dnsRegistry.RegisterServer(
				domain.Name,
				backend.Address,
				backend.Port,
				backend.Weight,
				backend.Region,
			); err != nil {
				p.logger.Warn("failed to register server of stored domain with DNS",
					"domain", domain.Name,
					"address", backend.Address,
					"port", backend.Port,
					"error", err,
				)
				continue
			}
			loadedBackends++
		}
	}

	p.logger.Info("loaded stored domains into DNS registry",
		"domains", loadedDomains,
		"backends", loadedBackends,
	)

	return nil
}

// ListDomains returns all configured domains.
func (p *RegistryDomainProvider) ListDomains() []Domain {
	// Start with domains from config file
	domainMap := make(map[string]Domain)

	// Add domains from config
	if p.config != nil {
		for _, d := range p.config.Domains {
			// Count backends from config regions
			backendCount := 0
			for _, regionName := range d.Regions {
				for _, r := range p.config.Regions {
					if r.Name == regionName {
						backendCount += len(r.Servers)
					}
				}
			}

			ttl := d.TTL
			if ttl == 0 {
				ttl = config.DefaultTTL
			}

			routingPolicy := d.RoutingAlgorithm
			if routingPolicy == "" {
				routingPolicy = config.DefaultRoutingAlgorithm
			}

			domainMap[d.Name] = Domain{
				ID:               d.Name,
				Name:             d.Name,
				TTL:              ttl,
				RoutingAlgorithm: routingPolicy,
				RoutingPolicy:    routingPolicy,
				Enabled:          true,
				BackendCount:     backendCount,
				HealthyBackends:  backendCount, // Assume healthy until checked
			}
		}
	}

	// Add domains from store (API-created)
	if p.store != nil {
		pairs, err := p.store.List(context.Background(), store.PrefixDomains)
		if err == nil {
			for _, pair := range pairs {
				var domain Domain
				if err := json.Unmarshal(pair.Value, &domain); err == nil {
					domain.normalizeRouting()
					// Store domains override config domains
					domainMap[domain.Name] = domain
				}
			}
		}
	}

	// Add/update with data from backend registry (dynamic agent registrations)
	backends := p.registry.GetAllBackends()
	serviceMap := make(map[string][]*overwatch.Backend)
	for _, b := range backends {
		serviceMap[b.Service] = append(serviceMap[b.Service], b)
	}

	for service, serviceBackends := range serviceMap {
		healthyCount := 0
		for _, b := range serviceBackends {
			if b.EffectiveStatus == overwatch.StatusHealthy {
				healthyCount++
			}
		}

		// Update existing or add new
		existing, ok := domainMap[service]
		if ok {
			existing.BackendCount = len(serviceBackends)
			existing.HealthyBackends = healthyCount
			domainMap[service] = existing
		} else {
			domainMap[service] = Domain{
				ID:               service,
				Name:             service,
				TTL:              config.DefaultTTL,
				RoutingAlgorithm: config.DefaultRoutingAlgorithm,
				RoutingPolicy:    config.DefaultRoutingAlgorithm,
				Enabled:          true,
				BackendCount:     len(serviceBackends),
				HealthyBackends:  healthyCount,
			}
		}
	}

	domains := make([]Domain, 0, len(domainMap))
	for _, d := range domainMap {
		domains = append(domains, d)
	}

	return domains
}

// GetDomain returns a domain by name.
func (p *RegistryDomainProvider) GetDomain(name string) (*Domain, error) {
	// First check store for API-created domains
	if p.store != nil {
		key := store.PrefixDomains + name
		data, err := p.store.Get(context.Background(), key)
		if err == nil {
			var domain Domain
			if err := json.Unmarshal(data, &domain); err == nil {
				domain.normalizeRouting()
				// Update with backend health info from registry
				backends := p.registry.GetBackends(name)
				if len(backends) > 0 {
					domain.BackendCount = len(backends)
					healthyCount := 0
					for _, b := range backends {
						if b.EffectiveStatus == overwatch.StatusHealthy {
							healthyCount++
						}
					}
					domain.HealthyBackends = healthyCount
				}
				return &domain, nil
			}
		}
	}

	// Then check config
	if p.config != nil {
		for _, d := range p.config.Domains {
			if d.Name == name {
				// Count backends from config regions
				backendCount := 0
				for _, regionName := range d.Regions {
					for _, r := range p.config.Regions {
						if r.Name == regionName {
							backendCount += len(r.Servers)
						}
					}
				}

				ttl := d.TTL
				if ttl == 0 {
					ttl = config.DefaultTTL
				}

				routingPolicy := d.RoutingAlgorithm
				if routingPolicy == "" {
					routingPolicy = config.DefaultRoutingAlgorithm
				}

				// Check registry for dynamic health info
				backends := p.registry.GetBackends(name)
				healthyCount := backendCount
				if len(backends) > 0 {
					backendCount = len(backends)
					healthyCount = 0
					for _, b := range backends {
						if b.EffectiveStatus == overwatch.StatusHealthy {
							healthyCount++
						}
					}
				}

				return &Domain{
					ID:               name,
					Name:             name,
					TTL:              ttl,
					RoutingAlgorithm: routingPolicy,
					RoutingPolicy:    routingPolicy,
					Enabled:          true,
					BackendCount:     backendCount,
					HealthyBackends:  healthyCount,
				}, nil
			}
		}
	}

	// Check registry
	backends := p.registry.GetBackends(name)
	if len(backends) == 0 {
		return nil, ErrNotFound
	}

	healthyCount := 0
	for _, b := range backends {
		if b.EffectiveStatus == overwatch.StatusHealthy {
			healthyCount++
		}
	}

	return &Domain{
		ID:               name,
		Name:             name,
		TTL:              config.DefaultTTL,
		RoutingAlgorithm: config.DefaultRoutingAlgorithm,
		RoutingPolicy:    config.DefaultRoutingAlgorithm,
		Enabled:          true,
		BackendCount:     len(backends),
		HealthyBackends:  healthyCount,
	}, nil
}

// CreateDomain creates a new domain.
func (p *RegistryDomainProvider) CreateDomain(domain Domain) error {
	if p.store == nil {
		return p.persistence.errNoStore()
	}

	// Check if domain already exists in config
	if p.config != nil {
		for _, d := range p.config.Domains {
			if d.Name == domain.Name {
				return errConflict("domain %q already exists in the configuration file", domain.Name)
			}
		}
	}

	// Check if domain already exists in store
	key := store.PrefixDomains + domain.Name
	_, err := p.store.Get(context.Background(), key)
	if err == nil {
		return errConflict("domain %q already exists", domain.Name)
	}

	// Set ID and timestamps
	if domain.ID == "" {
		domain.ID = domain.Name
	}
	now := time.Now().UTC()
	domain.CreatedAt = now
	domain.UpdatedAt = now

	// Set defaults if not provided
	if domain.TTL == 0 {
		domain.TTL = config.DefaultTTL
	}
	domain.normalizeRouting()
	if domain.RoutingAlgorithm == "" {
		domain.RoutingAlgorithm = config.DefaultRoutingAlgorithm
		domain.RoutingPolicy = domain.RoutingAlgorithm
	}

	// An algorithm the router factory cannot build must not be stored: the
	// domain would exist in the API and never be served.
	if p.routerFactory != nil {
		if _, err := p.routerFactory(domain.RoutingAlgorithm); err != nil {
			return errInvalid("routing_algorithm %q cannot be used: %v", domain.RoutingAlgorithm, err)
		}
	}

	// Serialize and store
	data, err := json.Marshal(domain)
	if err != nil {
		return fmt.Errorf("failed to marshal domain: %w", err)
	}

	if err := p.store.Set(context.Background(), key, data); err != nil {
		return fmt.Errorf("failed to store domain: %w", err)
	}

	// Register with DNS registry if available
	if p.dnsRegistry != nil && p.routerFactory != nil {
		if err := p.dnsRegistry.RegisterDomainDynamic(
			domain.Name,
			config.TTLSeconds(domain.TTL),
			domain.RoutingAlgorithm,
			p.routerFactory,
		); err != nil {
			// Log the error but don't fail the domain creation
			// The domain is persisted and will be available after restart
			p.logger.Warn("failed to register domain with DNS server",
				"name", domain.Name,
				"error", err,
			)
		} else {
			p.logger.Info("domain registered with DNS server", "name", domain.Name)
		}
	}

	p.logger.Info("domain created via API", "name", domain.Name)
	return nil
}

// UpdateDomain updates an existing domain.
func (p *RegistryDomainProvider) UpdateDomain(name string, domain Domain) error {
	if p.store == nil {
		return p.persistence.errNoStore()
	}

	// Check if domain exists in store
	key := store.PrefixDomains + name
	existingData, err := p.store.Get(context.Background(), key)
	if err != nil {
		// Domain might exist in config but not in store
		if p.config != nil {
			for _, d := range p.config.Domains {
				if d.Name == name {
					// Can't update config-based domains
					return errConflict("domain %q is defined in the configuration file and cannot be changed through the API; change the file and reload", name)
				}
			}
		}
		return errMissing("domain %q not found", name)
	}

	// Preserve creation timestamp
	var existing Domain
	if err := json.Unmarshal(existingData, &existing); err == nil {
		domain.CreatedAt = existing.CreatedAt
	}
	domain.UpdatedAt = time.Now().UTC()
	domain.Name = name // Ensure name matches
	if domain.ID == "" {
		domain.ID = name
	}
	domain.normalizeRouting()
	// The counts are computed on read, not stored.
	domain.BackendCount = 0
	domain.HealthyBackends = 0

	ttl := config.TTLSeconds(domain.TTL)
	if ttl == 0 {
		ttl = config.TTLSeconds(config.DefaultTTL)
	}
	algorithm := domain.RoutingAlgorithm
	if algorithm == "" {
		algorithm = config.DefaultRoutingAlgorithm
	}
	if p.routerFactory != nil {
		if _, err := p.routerFactory(algorithm); err != nil {
			return errInvalid("routing_algorithm %q cannot be used: %v", algorithm, err)
		}
	}

	// Serialize and store
	data, err := json.Marshal(domain)
	if err != nil {
		return fmt.Errorf("failed to marshal domain: %w", err)
	}

	if err := p.store.Set(context.Background(), key, data); err != nil {
		return fmt.Errorf("failed to update domain: %w", err)
	}

	// Update DNS registry if available
	if p.dnsRegistry != nil && p.routerFactory != nil {
		if err := p.dnsRegistry.UpdateDomainSettings(name, ttl, algorithm, p.routerFactory); err != nil {
			// Log but don't fail - store is already updated
			p.logger.Warn("failed to update domain in DNS registry",
				"domain", name,
				"error", err,
			)
		} else {
			p.logger.Info("domain settings updated in DNS registry",
				"domain", name,
				"ttl", ttl,
				"algorithm", algorithm,
			)
		}
	}

	p.logger.Info("domain updated via API", "name", name)
	return nil
}

// DeleteDomain deletes a domain by name, together with the servers that were
// created for it through the API.
func (p *RegistryDomainProvider) DeleteDomain(name string) error {
	if p.store == nil {
		return p.persistence.errNoStore()
	}

	// Check if domain exists in config
	if p.config != nil {
		for _, d := range p.config.Domains {
			if d.Name == name {
				return errConflict("domain %q is defined in the configuration file and cannot be deleted through the API; remove it from the file and reload", name)
			}
		}
	}

	// Check if domain exists in store
	key := store.PrefixDomains + name
	_, err := p.store.Get(context.Background(), key)
	if err != nil {
		return errMissing("domain %q not found", name)
	}

	// Delete the servers created for the domain through the API first. While
	// the domain is still in the DNS registry each one is also taken out of
	// its DNS entry.
	if p.servers != nil {
		p.servers.deleteAPIServersOf(name)
	}
	// Records in the layout of version 1.1.10 and earlier. They are migrated
	// at startup, so this finds nothing unless the migration failed.
	backendPrefix := store.PrefixDomainBackends + name + "/"
	backends, _ := p.store.List(context.Background(), backendPrefix)
	for _, b := range backends {
		_ = p.store.Delete(context.Background(), b.Key)
	}

	// Delete the domain
	if err := p.store.Delete(context.Background(), key); err != nil {
		return fmt.Errorf("failed to delete domain: %w", err)
	}

	// Remove from DNS registry if available
	if p.dnsRegistry != nil {
		p.dnsRegistry.Remove(name)
		p.logger.Info("domain removed from DNS server", "name", name)
	}

	p.logger.Info("domain deleted via API", "name", name)
	return nil
}

// GetDomainBackends returns the servers of a domain. A domain that exists and
// has no servers returns an empty list.
func (p *RegistryDomainProvider) GetDomainBackends(name string) ([]BackendServer, error) {
	if p.servers == nil {
		return nil, &Error{Status: http.StatusServiceUnavailable, Message: "server provider not configured"}
	}
	servers := p.servers.listByService(name)
	if len(servers) == 0 {
		// No servers: the answer is an empty list when the domain exists and
		// 404 when it does not.
		if _, err := p.GetDomain(name); err != nil {
			return nil, errMissing("domain %q not found", name)
		}
	}
	return servers, nil
}

// AddDomainBackend creates a server for a domain.
func (p *RegistryDomainProvider) AddDomainBackend(domainName string, spec ServerSpec) (*BackendServer, error) {
	if p.servers == nil {
		return nil, &Error{Status: http.StatusServiceUnavailable, Message: "server provider not configured"}
	}
	spec.Service = domainName
	return p.servers.CreateServer(spec)
}

// RemoveDomainBackend deletes a server of a domain. backendID is the server
// ID, "{domain}:{address}:{port}".
func (p *RegistryDomainProvider) RemoveDomainBackend(domainName string, backendID string) error {
	if p.servers == nil {
		return &Error{Status: http.StatusServiceUnavailable, Message: "server provider not configured"}
	}
	service, _, _, err := parseServerID(backendID)
	if err != nil || service != domainName {
		return errMissing("backend %q not found for domain %q", backendID, domainName)
	}
	return p.servers.DeleteServer(backendID)
}

// =============================================================================
// Config-based providers
// =============================================================================

// ConfigRegionProvider implements RegionProvider using configuration.
type ConfigRegionProvider struct {
	config      *config.Config
	registry    RegistryInterface
	store       store.Store
	persistence persistence
	logger      *slog.Logger
}

// NewConfigRegionProvider creates a new ConfigRegionProvider.
func NewConfigRegionProvider(cfg *config.Config, registry RegistryInterface, logger *slog.Logger) *ConfigRegionProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &ConfigRegionProvider{
		config:   cfg,
		registry: registry,
		logger:   logger,
	}
}

// SetStore sets the store for persistence operations.
func (p *ConfigRegionProvider) SetStore(s store.Store) {
	p.store = s
}

// SetStoreUnavailable records why there is no store. The reason is part of
// the 503 that answers every write while the provider has no store.
func (p *ConfigRegionProvider) SetStoreUnavailable(reason string) {
	p.persistence.unavailable = reason
}

// ListRegions returns all configured regions.
func (p *ConfigRegionProvider) ListRegions() []Region {
	regionMap := make(map[string]struct {
		serverCount    int
		healthyServers int
		countries      []string
		continents     []string
	})

	// First add regions from config
	if p.config != nil {
		for _, r := range p.config.Regions {
			regionMap[r.Name] = struct {
				serverCount    int
				healthyServers int
				countries      []string
				continents     []string
			}{
				serverCount:    len(r.Servers),
				healthyServers: len(r.Servers), // Assume healthy until checked
				countries:      r.Countries,
				continents:     r.Continents,
			}
		}
	}

	// Add regions from store (API-created) - these are full Region objects
	storeRegions := make(map[string]Region)
	if p.store != nil {
		pairs, err := p.store.List(context.Background(), store.PrefixRegions)
		if err == nil {
			for _, pair := range pairs {
				var region Region
				if err := json.Unmarshal(pair.Value, &region); err == nil {
					storeRegions[region.ID] = region
				}
			}
		}
	}

	// Update with dynamic backend registry data
	backends := p.registry.GetAllBackends()
	for _, b := range backends {
		region := b.Region
		if region == "" {
			region = "default"
		}

		stats := regionMap[region]
		stats.serverCount++
		if b.EffectiveStatus == overwatch.StatusHealthy {
			stats.healthyServers++
		}
		regionMap[region] = stats
	}

	regions := make([]Region, 0, len(regionMap)+len(storeRegions))
	for name, stats := range regionMap {
		regions = append(regions, Region{
			ID:             name,
			Name:           name,
			Code:           name,
			Enabled:        true,
			ServerCount:    stats.serverCount,
			HealthyServers: stats.healthyServers,
			Countries:      stats.countries,
			Continent:      firstOrEmpty(stats.continents),
		})
	}

	// Add store regions that aren't already in the list
	for id, region := range storeRegions {
		found := false
		for i, r := range regions {
			if r.ID == id {
				// Update existing with store data (store takes precedence for metadata)
				regions[i].Description = region.Description
				if len(region.Countries) > 0 {
					regions[i].Countries = region.Countries
				}
				if region.Continent != "" {
					regions[i].Continent = region.Continent
				}
				found = true
				break
			}
		}
		if !found {
			regions = append(regions, region)
		}
	}

	return regions
}

func firstOrEmpty(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

// GetRegion returns a region by ID.
func (p *ConfigRegionProvider) GetRegion(id string) (*Region, error) {
	// Check store first for API-created regions
	if p.store != nil {
		key := store.PrefixRegions + id
		data, err := p.store.Get(context.Background(), key)
		if err == nil {
			var region Region
			if err := json.Unmarshal(data, &region); err == nil {
				return &region, nil
			}
		}
	}

	// Fall back to listing all regions (includes config and registry)
	regions := p.ListRegions()
	for _, r := range regions {
		if r.ID == id || r.Code == id {
			return &r, nil
		}
	}
	return nil, ErrNotFound
}

// CreateRegion creates a new region.
func (p *ConfigRegionProvider) CreateRegion(region Region) error {
	if p.store == nil {
		return p.persistence.errNoStore()
	}

	// Check if region already exists in config
	if p.config != nil {
		for _, r := range p.config.Regions {
			if r.Name == region.ID || r.Name == region.Name {
				return fmt.Errorf("region %q already exists in config", region.ID)
			}
		}
	}

	// Generate ID if not provided
	if region.ID == "" {
		region.ID = region.Name
	}
	if region.Code == "" {
		region.Code = region.ID
	}

	// Check if region already exists in store
	key := store.PrefixRegions + region.ID
	_, err := p.store.Get(context.Background(), key)
	if err == nil {
		return fmt.Errorf("region %q already exists", region.ID)
	}

	// Set timestamps
	now := time.Now().UTC()
	region.CreatedAt = now
	region.UpdatedAt = now

	// Serialize and store
	data, err := json.Marshal(region)
	if err != nil {
		return fmt.Errorf("failed to marshal region: %w", err)
	}

	if err := p.store.Set(context.Background(), key, data); err != nil {
		return fmt.Errorf("failed to store region: %w", err)
	}

	p.logger.Info("region created via API", "id", region.ID)
	return nil
}

// UpdateRegion updates an existing region.
func (p *ConfigRegionProvider) UpdateRegion(id string, region Region) error {
	if p.store == nil {
		return p.persistence.errNoStore()
	}

	// Check if region exists in store
	key := store.PrefixRegions + id
	existingData, err := p.store.Get(context.Background(), key)
	if err != nil {
		// Region might exist in config but not in store
		if p.config != nil {
			for _, r := range p.config.Regions {
				if r.Name == id {
					return fmt.Errorf("cannot update config-based region %q via API; use config file", id)
				}
			}
		}
		return fmt.Errorf("region %q not found", id)
	}

	// Preserve creation timestamp
	var existing Region
	if err := json.Unmarshal(existingData, &existing); err == nil {
		region.CreatedAt = existing.CreatedAt
	}
	region.UpdatedAt = time.Now().UTC()
	region.ID = id // Ensure ID matches
	if region.Code == "" {
		region.Code = id
	}

	// Serialize and store
	data, err := json.Marshal(region)
	if err != nil {
		return fmt.Errorf("failed to marshal region: %w", err)
	}

	if err := p.store.Set(context.Background(), key, data); err != nil {
		return fmt.Errorf("failed to update region: %w", err)
	}

	p.logger.Info("region updated via API", "id", id)
	return nil
}

// DeleteRegion deletes a region by ID.
func (p *ConfigRegionProvider) DeleteRegion(id string) error {
	if p.store == nil {
		return p.persistence.errNoStore()
	}

	// Check if region exists in config
	if p.config != nil {
		for _, r := range p.config.Regions {
			if r.Name == id {
				return fmt.Errorf("cannot delete config-based region %q via API; use config file", id)
			}
		}
	}

	// Check if region exists in store
	key := store.PrefixRegions + id
	_, err := p.store.Get(context.Background(), key)
	if err != nil {
		return fmt.Errorf("region %q not found", id)
	}

	// Delete the region
	if err := p.store.Delete(context.Background(), key); err != nil {
		return fmt.Errorf("failed to delete region: %w", err)
	}

	p.logger.Info("region deleted via API", "id", id)
	return nil
}

// =============================================================================
// Stub providers for features not fully implemented
// =============================================================================

// StubNodeProvider implements NodeProvider with stub data.
type StubNodeProvider struct {
	registry RegistryInterface
	logger   *slog.Logger
}

// NewStubNodeProvider creates a new StubNodeProvider.
func NewStubNodeProvider(registry RegistryInterface, logger *slog.Logger) *StubNodeProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &StubNodeProvider{
		registry: registry,
		logger:   logger,
	}
}

// ListOverwatchNodes returns all Overwatch nodes.
func (p *StubNodeProvider) ListOverwatchNodes() []OverwatchNode {
	// Return self as the only node for now
	return []OverwatchNode{
		{
			ID:     "local",
			Name:   "local-overwatch",
			Status: "active",
		},
	}
}

// GetOverwatchNode returns an Overwatch node by ID.
func (p *StubNodeProvider) GetOverwatchNode(id string) (*OverwatchNode, error) {
	if id == "local" {
		return &OverwatchNode{
			ID:     "local",
			Name:   "local-overwatch",
			Status: "active",
		}, nil
	}
	return nil, ErrNotFound
}

// ListAgentNodes returns all Agent nodes.
func (p *StubNodeProvider) ListAgentNodes() []AgentNode {
	// Extract unique agents from backends
	backends := p.registry.GetAllBackends()
	agentMap := make(map[string]*AgentNode)

	for _, b := range backends {
		if _, exists := agentMap[b.AgentID]; !exists {
			agentMap[b.AgentID] = &AgentNode{
				ID:       b.AgentID,
				Name:     b.AgentID,
				Region:   b.Region,
				Status:   "connected",
				LastSeen: b.AgentLastSeen,
			}
		}
	}

	agents := make([]AgentNode, 0, len(agentMap))
	for _, agent := range agentMap {
		agents = append(agents, *agent)
	}

	return agents
}

// GetAgentNode returns an Agent node by ID.
func (p *StubNodeProvider) GetAgentNode(id string) (*AgentNode, error) {
	agents := p.ListAgentNodes()
	for _, a := range agents {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, ErrNotFound
}

// RegisterAgentNode registers a new Agent node.
func (p *StubNodeProvider) RegisterAgentNode(_ AgentNode) error {
	return ErrNotImplemented
}

// DeregisterAgentNode removes an Agent node.
func (p *StubNodeProvider) DeregisterAgentNode(_ string) error {
	return ErrNotImplemented
}

// GetAgentCertificate retrieves the certificate for an Agent.
func (p *StubNodeProvider) GetAgentCertificate(_ string) (*AgentCertificate, error) {
	return nil, ErrNotImplemented
}

// RevokeAgentCertificate revokes an Agent's certificate.
func (p *StubNodeProvider) RevokeAgentCertificate(_ string) error {
	return ErrNotImplemented
}

// ReissueAgentCertificate issues a new certificate for an Agent.
func (p *StubNodeProvider) ReissueAgentCertificate(_ string) (*AgentCertificate, error) {
	return nil, ErrNotImplemented
}

// StubGossipProvider implements GossipProvider with stub data.
type StubGossipProvider struct {
	logger *slog.Logger
}

// NewStubGossipProvider creates a new StubGossipProvider.
func NewStubGossipProvider(logger *slog.Logger) *StubGossipProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &StubGossipProvider{logger: logger}
}

// ListGossipNodes returns all nodes in the gossip cluster.
func (p *StubGossipProvider) ListGossipNodes() []GossipNode {
	return []GossipNode{
		{
			ID:     "local",
			Name:   "local",
			Status: "alive",
		},
	}
}

// GetGossipNode returns a gossip node by ID.
func (p *StubGossipProvider) GetGossipNode(id string) (*GossipNode, error) {
	if id == "local" {
		return &GossipNode{
			ID:     "local",
			Name:   "local",
			Status: "alive",
		}, nil
	}
	return nil, ErrNotFound
}

// GetGossipConfig returns the gossip protocol configuration.
func (p *StubGossipProvider) GetGossipConfig() (*GossipConfig, error) {
	return &GossipConfig{
		Enabled:        true,
		BindAddress:    config.DefaultOverwatchGossipBindAddress,
		ProbeInterval:  int(config.DefaultOverwatchProbeInterval.Milliseconds()),
		ProbeTimeout:   int(config.DefaultOverwatchProbeTimeout.Milliseconds()),
		GossipInterval: int(config.DefaultOverwatchGossipInterval.Milliseconds()),
		RetransmitMult: 4,
	}, nil
}

// UpdateGossipConfig updates the gossip protocol configuration.
func (p *StubGossipProvider) UpdateGossipConfig(_ GossipConfig) error {
	return ErrNotImplemented
}

// StubAuditProvider implements AuditProvider with stub data.
type StubAuditProvider struct {
	logger *slog.Logger
}

// NewStubAuditProvider creates a new StubAuditProvider.
func NewStubAuditProvider(logger *slog.Logger) *StubAuditProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &StubAuditProvider{logger: logger}
}

// ListAuditLogs returns audit log entries with pagination and filtering.
func (p *StubAuditProvider) ListAuditLogs(_ AuditFilter) ([]AuditEntry, int, error) {
	return []AuditEntry{}, 0, nil
}

// GetAuditEntry returns a single audit entry by ID.
func (p *StubAuditProvider) GetAuditEntry(_ string) (*AuditEntry, error) {
	return nil, ErrNotFound
}

// GetAuditStats returns audit log statistics.
func (p *StubAuditProvider) GetAuditStats() (*AuditStats, error) {
	return &AuditStats{
		TotalEntries:   0,
		EntriesLast24h: 0,
		ByAction:       make(map[string]int64),
	}, nil
}

// ExportAuditLogs exports audit logs in the specified format.
func (p *StubAuditProvider) ExportAuditLogs(_ AuditFilter, _ string) ([]byte, error) {
	return []byte("[]"), nil
}

// StubMetricsProvider implements MetricsProvider with stub data.
// Deprecated: Use OverwatchMetricsProvider for real metrics.
type StubMetricsProvider struct {
	registry RegistryInterface
	logger   *slog.Logger
}

// NewStubMetricsProvider creates a new StubMetricsProvider.
// Deprecated: Use NewOverwatchMetricsProvider for real metrics.
func NewStubMetricsProvider(registry RegistryInterface, logger *slog.Logger) *StubMetricsProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &StubMetricsProvider{
		registry: registry,
		logger:   logger,
	}
}

// GetOverview returns the system metrics overview.
func (p *StubMetricsProvider) GetOverview() (*MetricsOverview, error) {
	backends := p.registry.GetAllBackends()
	healthyCount := 0
	for _, b := range backends {
		if b.EffectiveStatus == overwatch.StatusHealthy {
			healthyCount++
		}
	}

	return &MetricsOverview{
		Timestamp:        time.Now(),
		ActiveServers:    len(backends),
		HealthyServers:   healthyCount,
		UnhealthyServers: len(backends) - healthyCount,
	}, nil
}

// GetHistory returns historical metrics data.
func (p *StubMetricsProvider) GetHistory(_ MetricsHistoryFilter) ([]MetricsDataPoint, error) {
	return []MetricsDataPoint{}, nil
}

// GetNodeMetrics returns metrics for a specific node.
func (p *StubMetricsProvider) GetNodeMetrics(_ string) (*NodeMetrics, error) {
	return &NodeMetrics{
		NodeID:    "local",
		Timestamp: time.Now(),
	}, nil
}

// GetRegionMetrics returns metrics for a specific region.
func (p *StubMetricsProvider) GetRegionMetrics(regionID string) (*RegionMetrics, error) {
	backends := p.registry.GetAllBackends()

	serverCount := 0
	healthyCount := 0
	for _, b := range backends {
		region := b.Region
		if region == "" {
			region = "default"
		}
		if region == regionID {
			serverCount++
			if b.EffectiveStatus == overwatch.StatusHealthy {
				healthyCount++
			}
		}
	}

	return &RegionMetrics{
		RegionID:       regionID,
		Timestamp:      time.Now(),
		TotalServers:   serverCount,
		HealthyServers: healthyCount,
	}, nil
}

// GetRoutingStats returns routing decision statistics.
func (p *StubMetricsProvider) GetRoutingStats() (*RoutingStats, error) {
	return &RoutingStats{
		Timestamp:      time.Now(),
		TotalDecisions: 0,
		ByRegion:       make(map[string]int64),
	}, nil
}

// =============================================================================
// Overwatch Metrics Provider - Real implementation
// =============================================================================

// HealthManagerInterface defines the methods needed from the health manager.
type HealthManagerInterface interface {
	GetAllStatus() []health.Snapshot
	ServerCount() int
}

// OverwatchMetricsProvider implements MetricsProvider with real data from multiple sources.
type OverwatchMetricsProvider struct {
	registry      RegistryInterface
	config        *config.Config
	healthManager HealthManagerInterface
	startTime     time.Time
	logger        *slog.Logger
}

// OverwatchMetricsConfig holds configuration for the metrics provider.
type OverwatchMetricsConfig struct {
	Registry      RegistryInterface
	Config        *config.Config
	HealthManager HealthManagerInterface
	StartTime     time.Time
	Logger        *slog.Logger
}

// NewOverwatchMetricsProvider creates a new OverwatchMetricsProvider with all data sources.
func NewOverwatchMetricsProvider(cfg OverwatchMetricsConfig) *OverwatchMetricsProvider {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}
	return &OverwatchMetricsProvider{
		registry:      cfg.Registry,
		config:        cfg.Config,
		healthManager: cfg.HealthManager,
		startTime:     startTime,
		logger:        logger,
	}
}

// GetOverview returns the system metrics overview with real data.
func (p *OverwatchMetricsProvider) GetOverview() (*MetricsOverview, error) {
	now := time.Now()

	// Get backend stats from registry
	backends := p.registry.GetAllBackends()
	healthyCount := 0
	regionSet := make(map[string]struct{})
	agentSet := make(map[string]struct{})

	for _, b := range backends {
		if b.EffectiveStatus == overwatch.StatusHealthy {
			healthyCount++
		}
		region := b.Region
		if region == "" {
			region = "default"
		}
		regionSet[region] = struct{}{}
		if b.AgentID != "" {
			agentSet[b.AgentID] = struct{}{}
		}
	}

	// Get domain count from config
	activeDomains := 0
	if p.config != nil {
		activeDomains = len(p.config.Domains)
	}

	// Also count unique services from backends as domains
	serviceSet := make(map[string]struct{})
	for _, b := range backends {
		serviceSet[b.Service] = struct{}{}
	}
	if len(serviceSet) > activeDomains {
		activeDomains = len(serviceSet)
	}

	// Get region count from config if available, otherwise from backends
	activeRegions := len(regionSet)
	if p.config != nil && len(p.config.Regions) > activeRegions {
		activeRegions = len(p.config.Regions)
	}

	// Get health check stats from health manager
	healthChecksTotal := int64(0)
	healthManagerServerCount := 0
	if p.healthManager != nil {
		snapshots := p.healthManager.GetAllStatus()
		healthManagerServerCount = len(snapshots)
		// Each snapshot represents a health check target; count those with checks performed
		for _, snap := range snapshots {
			if !snap.LastCheck.IsZero() {
				healthChecksTotal++
			}
		}
	}

	// Calculate uptime
	uptime := int64(now.Sub(p.startTime).Seconds())

	// Get memory stats
	memStats := getMemoryStats()

	// Get CPU stats
	cpuStats := getCPUStats()

	// Determine server counts - prefer backend registry, fallback to health manager
	activeServers := len(backends)
	if activeServers == 0 && healthManagerServerCount > 0 {
		activeServers = healthManagerServerCount
		// If using health manager count, count healthy from snapshots
		if p.healthManager != nil {
			for _, snap := range p.healthManager.GetAllStatus() {
				if snap.Status == health.StatusHealthy {
					healthyCount++
				}
			}
		}
	}

	// Also check config for server counts as a fallback
	if activeServers == 0 && p.config != nil {
		for _, r := range p.config.Regions {
			activeServers += len(r.Servers)
		}
		// Assume config servers are healthy if no other data
		if healthyCount == 0 {
			healthyCount = activeServers
		}
	}

	return &MetricsOverview{
		Timestamp:          now,
		Uptime:             uptime,
		QueriesTotal:       0, // DNS metrics tracked separately via Prometheus
		QueriesPerSec:      0, // Would need a rate calculator
		HealthChecksTotal:  healthChecksTotal,
		HealthChecksPerSec: 0, // Would need a rate calculator
		ActiveDomains:      activeDomains,
		ActiveServers:      activeServers,
		HealthyServers:     healthyCount,
		UnhealthyServers:   activeServers - healthyCount,
		ActiveRegions:      activeRegions,
		OverwatchNodes:     1, // Local node
		AgentNodes:         len(agentSet),
		DNSSECEnabled:      p.config != nil && p.config.Overwatch.DNSSEC.Enabled,
		GossipEnabled:      true, // Gossip is always enabled in Overwatch mode
		ResponseTimes:      ResponseTimeStats{},
		ErrorRate:          0,
		CacheHitRate:       0,
		Memory:             memStats,
		CPU:                cpuStats,
	}, nil
}

// GetHistory returns historical metrics data.
func (p *OverwatchMetricsProvider) GetHistory(_ MetricsHistoryFilter) ([]MetricsDataPoint, error) {
	// Historical metrics would require a time-series store
	return []MetricsDataPoint{}, nil
}

// GetNodeMetrics returns metrics for a specific node.
func (p *OverwatchMetricsProvider) GetNodeMetrics(nodeID string) (*NodeMetrics, error) {
	now := time.Now()

	return &NodeMetrics{
		NodeID:    nodeID,
		NodeType:  "overwatch",
		Timestamp: now,
		Status:    "active",
		Uptime:    int64(now.Sub(p.startTime).Seconds()),
		Memory:    getMemoryStats(),
		CPU:       getCPUStats(),
	}, nil
}

// GetRegionMetrics returns metrics for a specific region.
func (p *OverwatchMetricsProvider) GetRegionMetrics(regionID string) (*RegionMetrics, error) {
	backends := p.registry.GetAllBackends()

	serverCount := 0
	healthyCount := 0
	for _, b := range backends {
		region := b.Region
		if region == "" {
			region = "default"
		}
		if region == regionID {
			serverCount++
			if b.EffectiveStatus == overwatch.StatusHealthy {
				healthyCount++
			}
		}
	}

	// Also check config for servers in this region
	if p.config != nil {
		for _, r := range p.config.Regions {
			if r.Name == regionID && len(r.Servers) > serverCount {
				serverCount = len(r.Servers)
				// If no backends registered, assume config servers are healthy
				if healthyCount == 0 {
					healthyCount = serverCount
				}
			}
		}
	}

	return &RegionMetrics{
		RegionID:       regionID,
		Timestamp:      time.Now(),
		TotalServers:   serverCount,
		HealthyServers: healthyCount,
	}, nil
}

// GetRoutingStats returns routing decision statistics.
func (p *OverwatchMetricsProvider) GetRoutingStats() (*RoutingStats, error) {
	// Routing stats would require tracking decisions over time
	byRegion := make(map[string]int64)

	// Populate regions from config
	if p.config != nil {
		for _, r := range p.config.Regions {
			byRegion[r.Name] = 0
		}
	}

	return &RoutingStats{
		Timestamp:      time.Now(),
		TotalDecisions: 0,
		ByRegion:       byRegion,
	}, nil
}

// getMemoryStats returns current memory statistics.
func getMemoryStats() MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Calculate used memory
	used := int64(m.Alloc) // #nosec G115 -- a process cannot hold 2^63 bytes of memory
	total := int64(m.Sys)  // #nosec G115 -- as above
	available := total - used

	var percent float64
	if total > 0 {
		percent = float64(used) / float64(total) * 100
	}

	return MemoryStats{
		Used:      used,
		Available: available,
		Total:     total,
		Percent:   percent,
	}
}

// getCPUStats returns current CPU statistics.
// Note: This provides a basic approximation. For accurate CPU metrics,
// a dedicated monitoring solution would be needed.
func getCPUStats() CPUStats {
	numCPU := runtime.NumCPU()

	// Note: Go doesn't provide direct CPU usage metrics.
	// For a production system, you'd want to use something like
	// gopsutil or read from /proc/stat directly.
	// For now, we return the number of CPU cores and placeholder values.
	return CPUStats{
		Used:   0,
		System: 0,
		User:   0,
		Idle:   100,
		Cores:  numCPU,
	}
}

// ConfigBasedConfigProvider implements ConfigProvider using application config.
type ConfigBasedConfigProvider struct {
	config    *config.Config
	logger    *slog.Logger
	startTime time.Time
	dataDir   string
	// dnsSettings reports the DNS settings in effect; nil when no DNS server runs.
	dnsSettings func() LiveDNSSettings
}

// NewConfigBasedConfigProvider creates a new ConfigBasedConfigProvider.
func NewConfigBasedConfigProvider(cfg *config.Config, logger *slog.Logger) *ConfigBasedConfigProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &ConfigBasedConfigProvider{
		config: cfg,
		logger: logger,
	}
}

// GetPreferences returns user preferences.
func (p *ConfigBasedConfigProvider) GetPreferences() (*Preferences, error) {
	return &Preferences{
		Theme:                "auto",
		Language:             "en",
		RefreshInterval:      30,
		NotificationsEnabled: true,
	}, nil
}

// UpdatePreferences updates user preferences.
func (p *ConfigBasedConfigProvider) UpdatePreferences(_ Preferences) error {
	return ErrNotImplemented
}

// SetNodeInfo sets what the configuration does not hold: when the process
// started and the directory of the persistent store. dataDir is empty when
// the node runs without persistence.
func (p *ConfigBasedConfigProvider) SetNodeInfo(startTime time.Time, dataDir string) {
	p.startTime = startTime
	p.dataDir = dataDir
}

// GetSystemConfig returns the identity of the node and the build it runs.
func (p *ConfigBasedConfigProvider) GetSystemConfig() (*SystemConfig, error) {
	sys := &SystemConfig{
		Version: version.GetVersion(),
		BuildInfo: BuildInfo{
			Version:   version.GetVersion(),
			GitCommit: vcsRevision(),
			GoVersion: runtime.Version(),
		},
		Mode:      string(p.config.GetEffectiveMode()),
		StartTime: p.startTime.UTC(),
		DataDir:   p.dataDir,
	}
	if p.config.GetEffectiveMode() == config.ModeAgent {
		sys.Region = p.config.Agent.Identity.Region
	} else {
		sys.NodeID = p.config.Overwatch.Identity.NodeID
		sys.Region = p.config.Overwatch.Identity.Region
	}
	if sys.NodeID == "" {
		// overwatch.identity.node_id defaults to the host name.
		if host, err := os.Hostname(); err == nil {
			sys.NodeID = host
		}
	}
	sys.NodeName = sys.NodeID
	return sys, nil
}

// vcsRevision returns the commit the binary was built from, when the Go
// toolchain recorded it (a build from a git checkout), and "" otherwise.
func vcsRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

// GetDNSConfig returns DNS-specific configuration.
func (p *ConfigBasedConfigProvider) GetDNSConfig() (*DNSConfig, error) {
	cfg := &DNSConfig{
		ListenAddress: p.config.DNS.ListenAddress,
		DefaultTTL:    p.config.DNS.DefaultTTL,
		Zones:         []DNSZone{},
	}
	if p.dnsSettings != nil {
		live := p.dnsSettings()
		cfg.MaxUDPSize = live.MaxUDPSize
		cfg.ReturnLastHealthy = live.ReturnLastHealthy
		if live.Zones != nil {
			cfg.Zones = live.Zones
		}
	}
	return cfg, nil
}

// LiveDNSSettings are the DNS settings in effect on the running DNS server.
// They can differ from the configuration the process started with after a
// reload.
type LiveDNSSettings struct {
	MaxUDPSize        int
	ReturnLastHealthy bool
	Zones             []DNSZone
}

// SetDNSSettings sets the function that reports the DNS settings in effect.
func (p *ConfigBasedConfigProvider) SetDNSSettings(fn func() LiveDNSSettings) {
	p.dnsSettings = fn
}

// UpdateDNSConfig updates DNS configuration.
func (p *ConfigBasedConfigProvider) UpdateDNSConfig(_ DNSConfig) error {
	return ErrNotImplemented
}

// GetHealthCheckConfig returns health check configuration.
func (p *ConfigBasedConfigProvider) GetHealthCheckConfig() (*HealthCheckConfig, error) {
	return &HealthCheckConfig{
		DefaultInterval:    int(config.DefaultHealthInterval.Seconds()),
		DefaultTimeout:     int(config.DefaultHealthTimeout.Seconds()),
		HealthyThreshold:   config.DefaultSuccessThreshold,
		UnhealthyThreshold: config.DefaultFailureThreshold,
		DefaultProtocol:    config.DefaultHealthCheckType,
	}, nil
}

// UpdateHealthCheckConfig updates health check configuration.
func (p *ConfigBasedConfigProvider) UpdateHealthCheckConfig(_ HealthCheckConfig) error {
	return ErrNotImplemented
}

// GetLoggingConfig returns logging configuration.
func (p *ConfigBasedConfigProvider) GetLoggingConfig() (*LoggingConfig, error) {
	return &LoggingConfig{
		Level:  p.config.Logging.Level,
		Format: p.config.Logging.Format,
	}, nil
}

// UpdateLoggingConfig updates logging configuration.
func (p *ConfigBasedConfigProvider) UpdateLoggingConfig(_ LoggingConfig) error {
	return ErrNotImplemented
}

// StubRoutingProvider implements RoutingProvider with stub data.
type StubRoutingProvider struct {
	logger *slog.Logger
}

// NewStubRoutingProvider creates a new StubRoutingProvider.
func NewStubRoutingProvider(logger *slog.Logger) *StubRoutingProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &StubRoutingProvider{logger: logger}
}

// ListAlgorithms returns available routing algorithms.
func (p *StubRoutingProvider) ListAlgorithms() []RoutingAlgorithm {
	return []RoutingAlgorithm{
		{
			ID:          "round-robin",
			Name:        "Round Robin",
			Description: "Distributes requests evenly across all healthy backends",
			Type:        "round-robin",
			Enabled:     true,
			Default:     true,
		},
		{
			ID:          "weighted",
			Name:        "Weighted",
			Description: "Distributes requests based on backend weights",
			Type:        "weighted",
			Enabled:     true,
			Default:     false,
		},
		{
			ID:          "geolocation",
			Name:        "Geolocation",
			Description: "Routes requests to the nearest geographic region",
			Type:        "geo",
			Enabled:     true,
			Default:     false,
		},
		{
			ID:          "latency",
			Name:        "Latency-based",
			Description: "Routes requests to backends with lowest latency",
			Type:        "latency",
			Enabled:     true,
			Default:     false,
		},
		{
			ID:          "failover",
			Name:        "Failover",
			Description: "Primary/backup routing with automatic failover",
			Type:        "failover",
			Enabled:     true,
			Default:     false,
		},
	}
}

// GetAlgorithm returns a specific routing algorithm by ID.
func (p *StubRoutingProvider) GetAlgorithm(id string) (*RoutingAlgorithm, error) {
	algorithms := p.ListAlgorithms()
	for _, a := range algorithms {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, ErrNotFound
}

// TestRouting simulates routing for a given request.
func (p *StubRoutingProvider) TestRouting(request RoutingTestRequest) (*RoutingTestResult, error) {
	return &RoutingTestResult{
		Domain:    request.Domain,
		ClientIP:  request.ClientIP,
		Algorithm: "round-robin",
		Decision:  "no_healthy_backend",
		Timestamp: time.Now(),
	}, nil
}

// GetDecisions returns recent routing decisions.
func (p *StubRoutingProvider) GetDecisions(_ RoutingDecisionFilter) ([]RoutingDecision, int, error) {
	return []RoutingDecision{}, 0, nil
}

// GetFlows returns traffic flow information.
func (p *StubRoutingProvider) GetFlows(_ FlowFilter) ([]TrafficFlow, error) {
	return []TrafficFlow{}, nil
}

// =============================================================================
// Latency Provider
// =============================================================================

// LatencyRegistryInterface defines the methods needed from the backend registry for latency data.
type LatencyRegistryInterface interface {
	GetLatency(address string, port int) overwatch.LatencyInfo
}

// RegistryLatencyProvider implements LatencyProvider using the backend registry.
type RegistryLatencyProvider struct {
	registry LatencyRegistryInterface
}

// NewRegistryLatencyProvider creates a new RegistryLatencyProvider.
func NewRegistryLatencyProvider(registry LatencyRegistryInterface) *RegistryLatencyProvider {
	return &RegistryLatencyProvider{registry: registry}
}

// GetLatency returns latency information for a server.
func (p *RegistryLatencyProvider) GetLatency(address string, port int) LatencyInfo {
	info := p.registry.GetLatency(address, port)
	return LatencyInfo{
		SmoothedLatency: info.SmoothedLatency,
		HasData:         info.HasData,
	}
}
