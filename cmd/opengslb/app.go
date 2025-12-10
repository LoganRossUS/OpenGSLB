// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/api"
	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/health"
	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/loganrossus/OpenGSLB/pkg/version"
)

// Application manages the lifecycle of all OpenGSLB components.
// ADR-015: Simplified architecture - no Raft, no cluster coordination.
type Application struct {
	config   *config.Config
	configMu sync.RWMutex
	logger   *slog.Logger

	// Overwatch mode components
	dnsServer     *dns.Server
	dnsHandler    *dns.Handler
	dnsRegistry   *dns.Registry
	healthManager *health.Manager
	metricsServer *metrics.Server
	apiServer     *api.Server

	// Agent mode components (Story 2 will add these)
	// agentManager *agent.Manager

	// Gossip - used in both modes but differently (Story 2/3 will add)
	// gossipManager *gossip.Manager

	// shutdownCh is closed when the application is shutting down.
	shutdownCh chan struct{}
}

// NewApplication creates a new Application instance with pre-loaded configuration.
func NewApplication(cfg *config.Config, logger *slog.Logger) *Application {
	if logger == nil {
		logger = slog.Default()
	}
	return &Application{
		config:     cfg,
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}
}

// Initialize sets up all components using the loaded configuration.
func (a *Application) Initialize() error {
	a.logger.Info("initializing application", "mode", a.config.GetEffectiveMode())

	metrics.SetAppInfo(version.Version)

	// Mode-specific initialization
	switch a.config.GetEffectiveMode() {
	case config.ModeAgent:
		if err := a.initializeAgentMode(); err != nil {
			return fmt.Errorf("failed to initialize agent mode: %w", err)
		}
	case config.ModeOverwatch:
		if err := a.initializeOverwatchMode(); err != nil {
			return fmt.Errorf("failed to initialize overwatch mode: %w", err)
		}
	default:
		return fmt.Errorf("unknown mode: %s", a.config.Mode)
	}

	return nil
}

// initializeAgentMode sets up agent-specific components.
// ADR-015: Agent monitors local backends, gossips to overwatch nodes.
func (a *Application) initializeAgentMode() error {
	a.logger.Info("initializing agent mode",
		"region", a.config.Agent.Identity.Region,
		"backends", len(a.config.Agent.Backends),
	)

	// Story 2 will implement:
	// - Multi-backend health checking
	// - Heartbeat mechanism
	// - Gossip to overwatch nodes
	// - Predictive health monitoring

	// For now, just initialize metrics
	if err := a.initializeMetricsServer(); err != nil {
		return fmt.Errorf("failed to initialize metrics server: %w", err)
	}

	a.logger.Info("agent mode initialized (stub - Story 2 will complete)")
	return nil
}

// initializeOverwatchMode sets up overwatch-specific components.
// ADR-015: Overwatch serves DNS, validates health, receives agent gossip.
func (a *Application) initializeOverwatchMode() error {
	a.logger.Info("initializing overwatch mode",
		"node_id", a.config.Overwatch.Identity.NodeID,
		"region", a.config.Overwatch.Identity.Region,
	)

	// Set config metrics
	serverCount := 0
	for _, region := range a.config.Regions {
		serverCount += len(region.Servers)
	}
	metrics.SetConfigMetrics(len(a.config.Domains), serverCount, float64(time.Now().Unix()))

	// Initialize health manager (for external validation)
	if err := a.initializeHealthManager(); err != nil {
		return fmt.Errorf("failed to initialize health manager: %w", err)
	}

	// Initialize DNS server
	if err := a.initializeDNSServer(); err != nil {
		return fmt.Errorf("failed to initialize DNS server: %w", err)
	}

	// Initialize metrics server
	if err := a.initializeMetricsServer(); err != nil {
		return fmt.Errorf("failed to initialize metrics server: %w", err)
	}

	// Initialize API server
	if err := a.initializeAPIServer(); err != nil {
		return fmt.Errorf("failed to initialize API server: %w", err)
	}

	// Story 3 will add:
	// - Gossip receiver for agent messages
	// - Backend registry from agent registrations
	// - External validation of agent health claims
	// - Override API

	a.logger.Info("overwatch mode initialized")
	return nil
}

// initializeHealthManager creates and configures the health manager.
func (a *Application) initializeHealthManager() error {
	checker := health.NewCompositeChecker()
	checker.Register("http", health.NewHTTPChecker())
	checker.Register("tcp", health.NewTCPChecker())

	a.logger.Debug("registered health checkers", "types", checker.RegisteredTypes())

	mgrCfg := health.DefaultManagerConfig()
	if len(a.config.Regions) > 0 {
		hc := a.config.Regions[0].HealthCheck
		if hc.FailureThreshold > 0 {
			mgrCfg.FailThreshold = hc.FailureThreshold
		}
		if hc.SuccessThreshold > 0 {
			mgrCfg.PassThreshold = hc.SuccessThreshold
		}
	}

	a.healthManager = health.NewManager(checker, mgrCfg)

	// Add servers to health manager
	if err := a.registerHealthCheckServers(); err != nil {
		return err
	}

	a.logger.Info("health manager initialized",
		"servers", a.healthManager.ServerCount(),
	)
	return nil
}

// registerHealthCheckServers registers all configured servers with the health manager.
func (a *Application) registerHealthCheckServers() error {
	for _, region := range a.config.Regions {
		hc := region.HealthCheck
		for _, server := range region.Servers {
			scheme := hc.Type
			if scheme == "" {
				scheme = "http"
			}

			serverCfg := health.ServerConfig{
				Address:  server.Address,
				Port:     server.Port,
				Path:     hc.Path,
				Scheme:   scheme,
				Host:     server.Host,
				Interval: hc.Interval,
				Timeout:  hc.Timeout,
			}

			if err := a.healthManager.AddServer(serverCfg); err != nil {
				return fmt.Errorf("failed to add server %s:%d to health manager: %w",
					server.Address, server.Port, err)
			}

			a.logger.Debug("registered server for health checks",
				"region", region.Name,
				"address", server.Address,
				"port", server.Port,
				"check_type", scheme,
			)
		}
	}
	return nil
}

// initializeDNSServer creates and configures the DNS server.
func (a *Application) initializeDNSServer() error {
	registry, err := dns.BuildRegistry(a.config, routing.NewRouter)
	if err != nil {
		return fmt.Errorf("failed to build DNS registry: %w", err)
	}
	a.dnsRegistry = registry

	for _, domainName := range registry.Domains() {
		entry := registry.Lookup(domainName)
		if entry != nil {
			a.logger.Info("domain configured",
				"domain", entry.Name,
				"algorithm", entry.Router.Algorithm(),
				"servers", len(entry.Servers),
			)
		}
	}

	// ADR-015: No leader checker needed - all Overwatch nodes serve DNS independently
	handler := dns.NewHandler(dns.HandlerConfig{
		Registry:       registry,
		HealthProvider: a.healthManager,
		LeaderChecker:  nil, // Standalone mode - always serve
		DefaultTTL:     uint32(a.config.DNS.DefaultTTL),
		Logger:         a.logger,
	})
	a.dnsHandler = handler

	a.dnsServer = dns.NewServer(dns.ServerConfig{
		Address: a.config.DNS.ListenAddress,
		Handler: handler,
		Logger:  a.logger,
	})

	a.logger.Info("DNS server initialized",
		"address", a.config.DNS.ListenAddress,
		"domains", registry.Count(),
	)
	return nil
}

// initializeMetricsServer creates and configures the metrics server.
func (a *Application) initializeMetricsServer() error {
	if !a.config.Metrics.Enabled {
		a.logger.Info("metrics server disabled")
		return nil
	}

	address := a.config.Metrics.Address
	if address == "" {
		address = ":9090"
	}

	a.metricsServer = metrics.NewServer(metrics.ServerConfig{
		Address: address,
		Logger:  a.logger,
	})

	a.logger.Info("metrics server initialized", "address", address)
	return nil
}

// initializeAPIServer creates and configures the API server.
func (a *Application) initializeAPIServer() error {
	if !a.config.API.Enabled {
		a.logger.Info("API server disabled")
		return nil
	}

	handlers := api.NewHandlers(
		a.healthManager,
		&readinessChecker{app: a},
		&regionMapper{cfg: a.config},
	)

	server, err := api.NewServer(api.ServerConfig{
		Address:           a.config.API.Address,
		AllowedNetworks:   a.config.API.AllowedNetworks,
		TrustProxyHeaders: a.config.API.TrustProxyHeaders,
		Logger:            a.logger,
	}, handlers)
	if err != nil {
		return fmt.Errorf("failed to create API server: %w", err)
	}

	// ADR-015: No cluster handlers - removed
	a.apiServer = server

	a.logger.Info("API server initialized",
		"address", a.config.API.Address,
		"allowed_networks", a.config.API.AllowedNetworks,
	)
	return nil
}

// Start begins all application components.
func (a *Application) Start(ctx context.Context) error {
	a.logger.Info("starting application", "mode", a.config.GetEffectiveMode())

	switch a.config.GetEffectiveMode() {
	case config.ModeAgent:
		return a.startAgentMode(ctx)
	case config.ModeOverwatch:
		return a.startOverwatchMode(ctx)
	default:
		return fmt.Errorf("unknown mode: %s", a.config.Mode)
	}
}

// startAgentMode starts agent-specific components.
func (a *Application) startAgentMode(ctx context.Context) error {
	// Story 2 will implement:
	// - Start gossip to overwatch nodes
	// - Start health checks for local backends
	// - Start heartbeat sender
	// - Start predictive health monitoring

	if a.metricsServer != nil {
		go func() {
			if err := a.metricsServer.Start(ctx); err != nil {
				a.logger.Error("metrics server error", "error", err)
			}
		}()
	}

	a.logger.Info("agent mode started (stub - Story 2 will complete)")

	// Block until context is canceled
	<-ctx.Done()
	return nil
}

// startOverwatchMode starts overwatch-specific components.
func (a *Application) startOverwatchMode(ctx context.Context) error {
	// Start health manager
	if err := a.healthManager.Start(); err != nil {
		return fmt.Errorf("failed to start health manager: %w", err)
	}
	a.logger.Info("health manager started")

	// Start metrics server
	if a.metricsServer != nil {
		go func() {
			for i := 0; i < 30; i++ {
				if err := a.metricsServer.Start(ctx); err != nil {
					a.logger.Warn("metrics server failed to start, retrying", "error", err, "attempt", i+1)
					select {
					case <-ctx.Done():
						return
					case <-time.After(1 * time.Second):
						continue
					}
				}
				return
			}
			a.logger.Error("metrics server failed to start after 30 attempts")
		}()
	}

	// Start API server
	if a.apiServer != nil {
		go func() {
			if err := a.apiServer.Start(ctx); err != nil {
				a.logger.Error("API server error", "error", err)
			}
		}()
	}

	// Story 3 will add:
	// - Start gossip receiver
	// - Start external validation loop

	// Start DNS server (blocks until shutdown)
	a.logger.Info("starting DNS server", "address", a.config.DNS.ListenAddress)
	if err := a.dnsServer.Start(ctx); err != nil {
		return fmt.Errorf("DNS server error: %w", err)
	}

	return nil
}

// Shutdown gracefully stops all application components.
func (a *Application) Shutdown(ctx context.Context) error {
	a.logger.Info("shutting down application")

	var shutdownErr error

	// Mode-specific shutdown
	switch a.config.GetEffectiveMode() {
	case config.ModeAgent:
		// Story 2 will add agent-specific shutdown
	case config.ModeOverwatch:
		if err := a.shutdownOverwatchMode(ctx); err != nil {
			shutdownErr = err
		}
	}

	// Common shutdown
	if a.metricsServer != nil {
		a.logger.Debug("stopping metrics server")
		// Metrics server doesn't have explicit shutdown
	}

	a.logger.Info("application shutdown complete")
	return shutdownErr
}

// shutdownOverwatchMode stops overwatch-specific components.
func (a *Application) shutdownOverwatchMode(ctx context.Context) error {
	var shutdownErr error

	// Story 3 will add:
	// - Stop gossip receiver

	if a.apiServer != nil {
		a.logger.Debug("stopping API server")
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := a.apiServer.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("error stopping API server", "error", err)
			shutdownErr = err
		}
		cancel()
	}

	if a.healthManager != nil {
		a.logger.Debug("stopping health manager")
		if err := a.healthManager.Stop(); err != nil {
			a.logger.Error("error stopping health manager", "error", err)
			shutdownErr = err
		}
	}

	// DNS server shuts down when context is canceled

	return shutdownErr
}

// Reload applies a new configuration without restarting.
func (a *Application) Reload(newCfg *config.Config) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	oldCfg := a.config

	a.logger.Info("reloading configuration",
		"old_domains", len(oldCfg.Domains),
		"new_domains", len(newCfg.Domains),
	)

	// Check for changes that require restart
	if oldCfg.DNS.ListenAddress != newCfg.DNS.ListenAddress {
		a.logger.Warn("DNS listen address change requires restart")
	}

	// Mode-specific reload
	switch a.config.GetEffectiveMode() {
	case config.ModeAgent:
		// Story 2 will add agent-specific reload
	case config.ModeOverwatch:
		if err := a.reloadOverwatchMode(newCfg); err != nil {
			metrics.RecordReload(false)
			return err
		}
	}

	a.config = newCfg
	metrics.RecordReload(true)
	return nil
}

// reloadOverwatchMode reloads overwatch-specific configuration.
func (a *Application) reloadOverwatchMode(newCfg *config.Config) error {
	if err := a.reloadDNSRegistry(newCfg); err != nil {
		return fmt.Errorf("failed to reload DNS registry: %w", err)
	}

	if err := a.reloadHealthManager(newCfg); err != nil {
		return fmt.Errorf("failed to reload health manager: %w", err)
	}

	return nil
}

// reloadDNSRegistry updates the DNS registry with new domain configuration.
func (a *Application) reloadDNSRegistry(newCfg *config.Config) error {
	newRegistry, err := dns.BuildRegistry(newCfg, routing.NewRouter)
	if err != nil {
		return fmt.Errorf("failed to build new registry: %w", err)
	}

	var entries []*dns.DomainEntry
	for _, name := range newRegistry.Domains() {
		if entry := newRegistry.Lookup(name); entry != nil {
			entries = append(entries, entry)
		}
	}

	a.dnsRegistry.ReplaceAll(entries)
	return nil
}

// reloadHealthManager updates health checks for the new server configuration.
func (a *Application) reloadHealthManager(newCfg *config.Config) error {
	var newServers []health.ServerConfig

	for _, region := range newCfg.Regions {
		hc := region.HealthCheck
		for _, server := range region.Servers {
			scheme := hc.Type
			if scheme == "" {
				scheme = "http"
			}

			newServers = append(newServers, health.ServerConfig{
				Address:  server.Address,
				Port:     server.Port,
				Path:     hc.Path,
				Scheme:   scheme,
				Host:     server.Host,
				Interval: hc.Interval,
				Timeout:  hc.Timeout,
			})
		}
	}

	added, removed, updated := a.healthManager.Reconfigure(newServers)

	a.logger.Info("health manager reconfigured",
		"added", added,
		"removed", removed,
		"updated", updated,
	)

	return nil
}

// readinessChecker implements api.ReadinessChecker for the Application.
type readinessChecker struct {
	app *Application
}

func (r *readinessChecker) IsDNSReady() bool {
	return r.app.dnsServer != nil
}

func (r *readinessChecker) IsHealthCheckReady() bool {
	if r.app.healthManager == nil {
		return false
	}
	snapshots := r.app.healthManager.GetAllStatus()
	if len(snapshots) == 0 {
		return true
	}
	for _, snap := range snapshots {
		if !snap.LastCheck.IsZero() {
			return true
		}
	}
	return false
}

// regionMapper implements api.RegionMapper for the Application.
type regionMapper struct {
	cfg *config.Config
}

func (r *regionMapper) GetServerRegion(address string, port int) string {
	for _, region := range r.cfg.Regions {
		for _, server := range region.Servers {
			if server.Address == address && server.Port == port {
				return region.Name
			}
		}
	}
	return ""
}
