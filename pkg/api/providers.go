// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
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

// RegistryDomainProvider implements DomainProvider using the backend registry.
type RegistryDomainProvider struct {
	registry RegistryInterface
	config   *config.Config
	logger   *slog.Logger
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

// ListDomains returns all configured domains.
func (p *RegistryDomainProvider) ListDomains() []Domain {
	backends := p.registry.GetAllBackends()

	// Group backends by service (domain)
	serviceMap := make(map[string][]*overwatch.Backend)
	for _, b := range backends {
		serviceMap[b.Service] = append(serviceMap[b.Service], b)
	}

	domains := make([]Domain, 0, len(serviceMap))
	for service, serviceBackends := range serviceMap {
		healthyCount := 0
		for _, b := range serviceBackends {
			if b.EffectiveStatus == overwatch.StatusHealthy {
				healthyCount++
			}
		}

		domains = append(domains, Domain{
			ID:              service,
			Name:            service,
			TTL:             config.DefaultTTL,
			RoutingPolicy:   config.DefaultRoutingAlgorithm,
			Enabled:         true,
			BackendCount:    len(serviceBackends),
			HealthyBackends: healthyCount,
		})
	}

	return domains
}

// GetDomain returns a domain by name.
func (p *RegistryDomainProvider) GetDomain(name string) (*Domain, error) {
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
		ID:              name,
		Name:            name,
		TTL:             config.DefaultTTL,
		RoutingPolicy:   config.DefaultRoutingAlgorithm,
		Enabled:         true,
		BackendCount:    len(backends),
		HealthyBackends: healthyCount,
	}, nil
}

// CreateDomain creates a new domain.
func (p *RegistryDomainProvider) CreateDomain(_ Domain) error {
	return ErrNotImplemented
}

// UpdateDomain updates an existing domain.
func (p *RegistryDomainProvider) UpdateDomain(_ string, _ Domain) error {
	return ErrNotImplemented
}

// DeleteDomain deletes a domain by name.
func (p *RegistryDomainProvider) DeleteDomain(_ string) error {
	return ErrNotImplemented
}

// GetDomainBackends returns the backends for a domain.
func (p *RegistryDomainProvider) GetDomainBackends(name string) ([]DomainBackend, error) {
	backends := p.registry.GetBackends(name)
	if len(backends) == 0 {
		return nil, ErrNotFound
	}

	result := make([]DomainBackend, 0, len(backends))
	for _, b := range backends {
		result = append(result, DomainBackend{
			ID:        fmt.Sprintf("%s:%s:%d", b.Service, b.Address, b.Port),
			Address:   b.Address,
			Port:      b.Port,
			Weight:    b.Weight,
			Region:    b.Region,
			Healthy:   b.EffectiveStatus == overwatch.StatusHealthy,
			Enabled:   true,
			LastCheck: b.ValidationLastCheck,
		})
	}

	return result, nil
}

// RegistryServerProvider implements BackendServerProvider using the backend registry.
type RegistryServerProvider struct {
	registry RegistryInterface
	logger   *slog.Logger
}

// NewRegistryServerProvider creates a new RegistryServerProvider.
func NewRegistryServerProvider(registry RegistryInterface, logger *slog.Logger) *RegistryServerProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &RegistryServerProvider{
		registry: registry,
		logger:   logger,
	}
}

// ListServers returns all configured backend servers.
func (p *RegistryServerProvider) ListServers() []BackendServer {
	backends := p.registry.GetAllBackends()

	servers := make([]BackendServer, 0, len(backends))
	for _, b := range backends {
		servers = append(servers, p.backendToServer(b))
	}

	return servers
}

// GetServer returns a server by ID.
func (p *RegistryServerProvider) GetServer(id string) (*BackendServer, error) {
	backends := p.registry.GetAllBackends()

	for _, b := range backends {
		serverID := fmt.Sprintf("%s:%s:%d", b.Service, b.Address, b.Port)
		if serverID == id {
			server := p.backendToServer(b)
			return &server, nil
		}
	}

	return nil, ErrNotFound
}

// CreateServer creates a new server.
func (p *RegistryServerProvider) CreateServer(_ BackendServer) error {
	return ErrNotImplemented
}

// UpdateServer updates an existing server.
func (p *RegistryServerProvider) UpdateServer(_ string, _ BackendServer) error {
	return ErrNotImplemented
}

// DeleteServer deletes a server by ID.
func (p *RegistryServerProvider) DeleteServer(_ string) error {
	return ErrNotImplemented
}

// GetServerHealthCheck returns the health check configuration for a server.
func (p *RegistryServerProvider) GetServerHealthCheck(id string) (*ServerHealthCheck, error) {
	_, err := p.GetServer(id)
	if err != nil {
		return nil, err
	}

	// Return default health check config
	return &ServerHealthCheck{
		Enabled:            true,
		Type:               config.DefaultHealthCheckType,
		Path:               config.DefaultHealthPath,
		Interval:           config.DefaultHealthInterval,
		Timeout:            config.DefaultHealthTimeout,
		HealthyThreshold:   config.DefaultSuccessThreshold,
		UnhealthyThreshold: config.DefaultFailureThreshold,
	}, nil
}

// UpdateServerHealthCheck updates the health check configuration for a server.
func (p *RegistryServerProvider) UpdateServerHealthCheck(_ string, _ ServerHealthCheck) error {
	return ErrNotImplemented
}

func (p *RegistryServerProvider) backendToServer(b *overwatch.Backend) BackendServer {
	lastCheck := b.ValidationLastCheck
	healthy := b.EffectiveStatus == overwatch.StatusHealthy

	return BackendServer{
		ID:       fmt.Sprintf("%s:%s:%d", b.Service, b.Address, b.Port),
		Name:     fmt.Sprintf("%s-%s", b.Service, b.Address),
		Address:  b.Address,
		Port:     b.Port,
		Protocol: "http",
		Weight:   b.Weight,
		Region:   b.Region,
		Enabled:  true,
		Healthy:  healthy,
		Status: &ServerStatus{
			Healthy:   healthy,
			LastCheck: &lastCheck,
			LastError: b.ValidationError,
		},
	}
}

// =============================================================================
// Config-based providers
// =============================================================================

// ConfigRegionProvider implements RegionProvider using configuration.
type ConfigRegionProvider struct {
	config   *config.Config
	registry RegistryInterface
	logger   *slog.Logger
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

// ListRegions returns all configured regions.
func (p *ConfigRegionProvider) ListRegions() []Region {
	// Extract unique regions from backends
	backends := p.registry.GetAllBackends()
	regionMap := make(map[string]struct {
		serverCount    int
		healthyServers int
	})

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

	regions := make([]Region, 0, len(regionMap))
	for name, stats := range regionMap {
		regions = append(regions, Region{
			ID:             name,
			Name:           name,
			Code:           name,
			Enabled:        true,
			ServerCount:    stats.serverCount,
			HealthyServers: stats.healthyServers,
		})
	}

	return regions
}

// GetRegion returns a region by ID.
func (p *ConfigRegionProvider) GetRegion(id string) (*Region, error) {
	regions := p.ListRegions()
	for _, r := range regions {
		if r.ID == id || r.Code == id {
			return &r, nil
		}
	}
	return nil, ErrNotFound
}

// CreateRegion creates a new region.
func (p *ConfigRegionProvider) CreateRegion(_ Region) error {
	return ErrNotImplemented
}

// UpdateRegion updates an existing region.
func (p *ConfigRegionProvider) UpdateRegion(_ string, _ Region) error {
	return ErrNotImplemented
}

// DeleteRegion deletes a region by ID.
func (p *ConfigRegionProvider) DeleteRegion(_ string) error {
	return ErrNotImplemented
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
type StubMetricsProvider struct {
	registry RegistryInterface
	logger   *slog.Logger
}

// NewStubMetricsProvider creates a new StubMetricsProvider.
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

// ConfigBasedConfigProvider implements ConfigProvider using application config.
type ConfigBasedConfigProvider struct {
	config *config.Config
	logger *slog.Logger
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

// GetSystemConfig returns the system configuration.
func (p *ConfigBasedConfigProvider) GetSystemConfig() (*SystemConfig, error) {
	return &SystemConfig{
		Mode:    string(p.config.GetEffectiveMode()),
		Version: "1.0.0",
	}, nil
}

// GetDNSConfig returns DNS-specific configuration.
func (p *ConfigBasedConfigProvider) GetDNSConfig() (*DNSConfig, error) {
	return &DNSConfig{
		ListenAddress: p.config.DNS.ListenAddress,
		DefaultTTL:    p.config.DNS.DefaultTTL,
	}, nil
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
