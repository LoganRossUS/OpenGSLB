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
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/agent"
	"github.com/loganrossus/OpenGSLB/pkg/api"
	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/geo"
	"github.com/loganrossus/OpenGSLB/pkg/gossip"
	"github.com/loganrossus/OpenGSLB/pkg/health"
	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/loganrossus/OpenGSLB/pkg/store"
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

	// Overwatch mode components (Story 3)
	backendRegistry    *overwatch.Registry
	overwatchValidator *overwatch.Validator
	gossipHandler      *overwatch.GossipHandler
	gossipReceiver     *gossip.MemberlistReceiver
	overwatchStore     store.Store

	// Agent mode components (Story 2)
	agentInstance *agent.Agent
	gossipSender  *gossip.MemberlistSender

	// Geolocation resolver (Demo 4: GeoIP routing)
	geoResolver *geo.Resolver

	// Application lifecycle
	startTime  time.Time
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
		startTime:  time.Now(),
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

	// Initialize gossip sender if configured
	var gossipSender agent.GossipSender
	if a.config.Agent.Gossip.EncryptionKey != "" && len(a.config.Agent.Gossip.OverwatchNodes) > 0 {
		sender, err := gossip.NewMemberlistSender(gossip.SenderConfig{
			NodeName:       "", // Will be set from identity after agent creation
			BindAddress:    "0.0.0.0:0",
			OverwatchNodes: a.config.Agent.Gossip.OverwatchNodes,
			EncryptionKey:  a.config.Agent.Gossip.EncryptionKey,
			Region:         a.config.Agent.Identity.Region,
			Logger:         a.logger,
		})
		if err != nil {
			return fmt.Errorf("failed to create gossip sender: %w", err)
		}
		a.gossipSender = sender
		gossipSender = sender
		a.logger.Info("gossip sender initialized",
			"overwatch_nodes", a.config.Agent.Gossip.OverwatchNodes,
		)
	} else {
		a.logger.Warn("gossip not configured - agent will not communicate with Overwatch nodes",
			"has_encryption_key", a.config.Agent.Gossip.EncryptionKey != "",
			"overwatch_nodes", len(a.config.Agent.Gossip.OverwatchNodes),
		)
	}

	// Create the agent instance
	agentInstance, err := agent.NewAgent(agent.AgentConfig{
		Config: a.config,
		Logger: a.logger,
		Gossip: gossipSender,
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}
	a.agentInstance = agentInstance

	// Initialize metrics server
	if err := a.initializeMetricsServer(); err != nil {
		return fmt.Errorf("failed to initialize metrics server: %w", err)
	}

	// Log agent identity information
	identity := agentInstance.GetIdentity()
	a.logger.Info("agent mode initialized",
		"agent_id", identity.AgentID,
		"region", identity.Region,
		"cert_fingerprint", identity.Fingerprint,
		"backends", agentInstance.GetBackendManager().BackendCount(),
	)

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

	// Initialize backend registry (Story 3)
	if err := a.initializeBackendRegistry(); err != nil {
		return fmt.Errorf("failed to initialize backend registry: %w", err)
	}

	// Initialize external validator (Story 3)
	if err := a.initializeValidator(); err != nil {
		return fmt.Errorf("failed to initialize validator: %w", err)
	}

	// Initialize gossip handler (Story 3 - placeholder for Story 4)
	if err := a.initializeGossipHandler(); err != nil {
		return fmt.Errorf("failed to initialize gossip handler: %w", err)
	}

	// Initialize geolocation resolver (Demo 4: GeoIP routing)
	if err := a.initializeGeoResolver(); err != nil {
		return fmt.Errorf("failed to initialize geo resolver: %w", err)
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

	a.logger.Info("overwatch mode initialized",
		"validation_enabled", a.config.Overwatch.Validation.Enabled,
		"stale_threshold", a.config.Overwatch.Stale.Threshold,
	)
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

// initializeGeoResolver creates and configures the geolocation resolver for GeoIP-based routing.
func (a *Application) initializeGeoResolver() error {
	geoCfg := a.config.Overwatch.Geolocation
	if geoCfg.DatabasePath == "" {
		a.logger.Info("geolocation disabled: no database path configured")
		return nil
	}

	resolver, err := geo.NewResolver(geo.ResolverConfig{
		DatabasePath:   geoCfg.DatabasePath,
		DefaultRegion:  geoCfg.DefaultRegion,
		CustomMappings: geoCfg.CustomMappings,
		Regions:        a.config.Regions,
		Logger:         a.logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create geo resolver: %w", err)
	}

	a.geoResolver = resolver

	a.logger.Info("geolocation resolver initialized",
		"database_path", geoCfg.DatabasePath,
		"default_region", geoCfg.DefaultRegion,
		"custom_mappings", len(geoCfg.CustomMappings),
		"regions", len(a.config.Regions),
	)
	return nil
}

// initializeBackendRegistry creates and configures the backend registry for agent registrations.
func (a *Application) initializeBackendRegistry() error {
	// Initialize bbolt store for persistence
	dataDir := a.config.Overwatch.DataDir
	if dataDir == "" {
		dataDir = "/var/lib/opengslb"
	}

	if err := os.MkdirAll(dataDir, 0750); err != nil {
		a.logger.Warn("failed to create data directory, running without persistence", "error", err)
		// Continue without persistence - don't fall back to temp dir to avoid conflicts
	} else {
		storePath := filepath.Join(dataDir, "overwatch.db")
		bboltStore, err := store.NewBboltStore(storePath)
		if err != nil {
			a.logger.Warn("failed to initialize bbolt store, running without persistence", "error", err)
			// Continue without persistence
		} else {
			a.overwatchStore = bboltStore
		}
	}

	// Configure registry with stale thresholds from config
	staleThreshold := a.config.Overwatch.Stale.Threshold
	if staleThreshold == 0 {
		staleThreshold = 30 * time.Second
	}
	removeAfter := a.config.Overwatch.Stale.RemoveAfter
	if removeAfter == 0 {
		removeAfter = 5 * time.Minute
	}

	registryCfg := overwatch.RegistryConfig{
		StaleThreshold: staleThreshold,
		RemoveAfter:    removeAfter,
		Logger:         a.logger,
	}

	a.backendRegistry = overwatch.NewRegistry(registryCfg, a.overwatchStore)

	// Set up status change callback for metrics
	a.backendRegistry.OnStatusChange(func(backend *overwatch.Backend, oldStatus, newStatus overwatch.BackendStatus) {
		overwatch.RecordBackendStatusChange(backend.Service, oldStatus, newStatus)
		// Update registry metrics
		overwatch.UpdateRegistryMetrics(a.backendRegistry)
	})

	a.logger.Info("backend registry initialized",
		"stale_threshold", staleThreshold,
		"remove_after", removeAfter,
		"persistence", a.overwatchStore != nil,
	)
	return nil
}

// initializeValidator creates and configures the external health validator.
func (a *Application) initializeValidator() error {
	if !a.config.Overwatch.Validation.Enabled {
		a.logger.Info("external validation disabled")
		return nil
	}

	checkInterval := a.config.Overwatch.Validation.CheckInterval
	if checkInterval == 0 {
		checkInterval = 30 * time.Second
	}
	checkTimeout := a.config.Overwatch.Validation.CheckTimeout
	if checkTimeout == 0 {
		checkTimeout = 5 * time.Second
	}

	// Create a composite checker for validation
	checker := health.NewCompositeChecker()
	checker.Register("http", health.NewHTTPChecker())
	checker.Register("tcp", health.NewTCPChecker())

	validatorCfg := overwatch.ValidatorConfig{
		Enabled:       true,
		CheckInterval: checkInterval,
		CheckTimeout:  checkTimeout,
		Logger:        a.logger,
	}

	a.overwatchValidator = overwatch.NewValidator(validatorCfg, a.backendRegistry, checker)

	a.logger.Info("external validator initialized",
		"check_interval", checkInterval,
		"check_timeout", checkTimeout,
	)
	return nil
}

// initializeGossipHandler creates and configures the gossip message handler.
func (a *Application) initializeGossipHandler() error {
	a.gossipHandler = overwatch.NewGossipHandler(a.backendRegistry, a.logger)

	// Initialize gossip receiver if configured
	if a.config.Overwatch.Gossip.EncryptionKey != "" {
		bindAddr := a.config.Overwatch.Gossip.BindAddress
		if bindAddr == "" {
			bindAddr = "0.0.0.0:7946"
		}

		receiver, err := gossip.NewMemberlistReceiver(gossip.ReceiverConfig{
			NodeName:       a.config.Overwatch.Identity.NodeID,
			BindAddress:    bindAddr,
			EncryptionKey:  a.config.Overwatch.Gossip.EncryptionKey,
			ProbeInterval:  a.config.Overwatch.Gossip.ProbeInterval,
			ProbeTimeout:   a.config.Overwatch.Gossip.ProbeTimeout,
			GossipInterval: a.config.Overwatch.Gossip.GossipInterval,
			Logger:         a.logger,
		})
		if err != nil {
			return fmt.Errorf("failed to create gossip receiver: %w", err)
		}
		a.gossipReceiver = receiver
		a.logger.Info("gossip receiver initialized",
			"bind_address", bindAddr,
		)
	} else {
		a.logger.Warn("gossip not configured - Overwatch will not receive agent updates",
			"has_encryption_key", a.config.Overwatch.Gossip.EncryptionKey != "",
		)
	}

	a.logger.Info("gossip handler initialized")
	return nil
}

// initializeDNSServer creates and configures the DNS server.
func (a *Application) initializeDNSServer() error {
	// Create a routing factory with access to the health manager for latency-based routing
	latencyProvider := &healthManagerLatencyProvider{manager: a.healthManager}
	routerFactory := routing.NewFactory(routing.FactoryConfig{
		LatencyProvider:   latencyProvider,
		MinLatencySamples: 1,             // Health manager tracks one sample at a time
		GeoResolver:       a.geoResolver, // Demo 4: GeoIP-based routing
		Logger:            a.logger,
	})

	registry, err := dns.BuildRegistry(a.config, routerFactory.NewRouter)
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
		ECSEnabled:     a.config.Overwatch.Geolocation.ECSEnabled, // Demo 4: EDNS Client Subnet for GeoIP
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

	// Create latency provider if backend registry is available
	var latencyProvider api.LatencyProvider
	if a.backendRegistry != nil {
		latencyProvider = api.NewRegistryLatencyProvider(a.backendRegistry)
	}

	handlers := api.NewHandlers(
		a.healthManager,
		&readinessChecker{app: a},
		&regionMapper{cfg: a.config},
		latencyProvider,
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

	// Set Overwatch API handlers (Story 3)
	if a.backendRegistry != nil {
		overwatchHandlers := overwatch.NewAPIHandlers(a.backendRegistry, a.overwatchValidator)
		server.SetOverwatchHandlers(overwatchHandlers)
		a.logger.Debug("overwatch API handlers registered")

		// Set up dashboard/management API handlers
		// Domain handlers - provides domain/service information from the registry
		domainProvider := api.NewRegistryDomainProvider(a.backendRegistry, a.config, a.logger)
		server.SetDomainHandlers(api.NewDomainHandlers(domainProvider, a.logger))
		a.logger.Debug("domain API handlers registered")

		// Server handlers - provides backend server information from the registry
		serverProvider := api.NewRegistryServerProvider(a.backendRegistry, a.config, a.logger)
		server.SetServerHandlers(api.NewServerHandlers(serverProvider, a.logger))
		a.logger.Debug("server API handlers registered")

		// Region handlers - provides region information derived from backends
		regionProvider := api.NewConfigRegionProvider(a.config, a.backendRegistry, a.logger)
		server.SetRegionHandlers(api.NewRegionHandlers(regionProvider, a.logger))
		a.logger.Debug("region API handlers registered")

		// Node handlers - provides Overwatch and Agent node information
		nodeProvider := api.NewStubNodeProvider(a.backendRegistry, a.logger)
		server.SetNodeHandlers(api.NewNodeHandlers(nodeProvider, a.logger))
		a.logger.Debug("node API handlers registered")

		// Gossip handlers - provides gossip cluster information
		gossipProvider := api.NewStubGossipProvider(a.logger)
		server.SetGossipHandlers(api.NewGossipHandlers(gossipProvider, a.logger))
		a.logger.Debug("gossip API handlers registered")

		// Audit handlers - provides audit log information
		auditProvider := api.NewStubAuditProvider(a.logger)
		server.SetAuditHandlers(api.NewAuditHandlers(auditProvider, a.logger))
		a.logger.Debug("audit API handlers registered")

		// Metrics handlers - provides system metrics
		metricsProvider := api.NewOverwatchMetricsProvider(api.OverwatchMetricsConfig{
			Registry:      a.backendRegistry,
			Config:        a.config,
			HealthManager: a.healthManager,
			StartTime:     a.startTime,
			Logger:        a.logger,
		})
		server.SetMetricsHandlers(api.NewMetricsHandlers(metricsProvider, a.logger))
		a.logger.Debug("metrics API handlers registered")

		// Config handlers - provides system configuration
		configProvider := api.NewConfigBasedConfigProvider(a.config, a.logger)
		server.SetConfigHandlers(api.NewConfigHandlers(configProvider, a.logger))
		a.logger.Debug("config API handlers registered")

		// Routing handlers - provides routing algorithm information
		routingProvider := api.NewStubRoutingProvider(a.logger)
		server.SetRoutingHandlers(api.NewRoutingHandlers(routingProvider, a.logger))
		a.logger.Debug("routing API handlers registered")

		// Override handlers - provides health override management
		overrideManager := api.NewOverrideManager(a.overwatchStore, a.logger)
		server.SetOverrideHandlers(api.NewOverrideHandlers(overrideManager, a.logger))
		a.logger.Debug("override API handlers registered")
	}

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
	// Start the agent (starts health checks, heartbeat, gossip)
	if err := a.agentInstance.Start(ctx); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	identity := a.agentInstance.GetIdentity()
	a.logger.Info("agent started",
		"agent_id", identity.AgentID,
		"region", identity.Region,
		"backends", a.agentInstance.GetBackendManager().BackendCount(),
	)

	// Start metrics server
	if a.metricsServer != nil {
		go func() {
			if err := a.metricsServer.Start(ctx); err != nil {
				a.logger.Error("metrics server error", "error", err)
			}
		}()
	}

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

	// Start backend registry (Story 3)
	if a.backendRegistry != nil {
		if err := a.backendRegistry.Start(); err != nil {
			return fmt.Errorf("failed to start backend registry: %w", err)
		}
		a.logger.Info("backend registry started")
	}

	// Start external validator (Story 3)
	if a.overwatchValidator != nil {
		if err := a.overwatchValidator.Start(); err != nil {
			return fmt.Errorf("failed to start validator: %w", err)
		}
		a.logger.Info("external validator started")
	}

	// Start gossip receiver and handler
	if a.gossipReceiver != nil {
		if err := a.gossipReceiver.Start(ctx); err != nil {
			return fmt.Errorf("failed to start gossip receiver: %w", err)
		}
		a.logger.Info("gossip receiver started",
			"bind_address", a.config.Overwatch.Gossip.BindAddress,
		)

		// Start the gossip handler to process messages
		if err := a.gossipHandler.Start(a.gossipReceiver); err != nil {
			return fmt.Errorf("failed to start gossip handler: %w", err)
		}
		a.logger.Info("gossip handler started")
	}

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
		if err := a.shutdownAgentMode(ctx); err != nil {
			shutdownErr = err
		}
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

// shutdownAgentMode stops agent-specific components.
func (a *Application) shutdownAgentMode(ctx context.Context) error {
	if a.agentInstance == nil {
		return nil
	}

	a.logger.Debug("stopping agent")

	// Create a timeout context for agent shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := a.agentInstance.Stop(shutdownCtx); err != nil {
		a.logger.Error("error stopping agent", "error", err)
		return err
	}

	// Stop gossip sender (agent.Stop() will have sent deregistration)
	if a.gossipSender != nil {
		a.logger.Debug("stopping gossip sender")
		if err := a.gossipSender.Stop(); err != nil {
			a.logger.Error("error stopping gossip sender", "error", err)
		}
	}

	a.logger.Info("agent stopped", "agent_id", a.agentInstance.GetIdentity().AgentID)
	return nil
}

// shutdownOverwatchMode stops overwatch-specific components.
func (a *Application) shutdownOverwatchMode(ctx context.Context) error {
	var shutdownErr error

	// Stop gossip handler first
	if a.gossipHandler != nil {
		a.logger.Debug("stopping gossip handler")
		if err := a.gossipHandler.Stop(); err != nil {
			a.logger.Error("error stopping gossip handler", "error", err)
			shutdownErr = err
		}
	}

	// Stop gossip receiver
	if a.gossipReceiver != nil {
		a.logger.Debug("stopping gossip receiver")
		if err := a.gossipReceiver.Stop(); err != nil {
			a.logger.Error("error stopping gossip receiver", "error", err)
			shutdownErr = err
		}
	}

	// Stop external validator (Story 3)
	if a.overwatchValidator != nil {
		a.logger.Debug("stopping external validator")
		if err := a.overwatchValidator.Stop(); err != nil {
			a.logger.Error("error stopping validator", "error", err)
			shutdownErr = err
		}
	}

	// Stop backend registry (Story 3)
	if a.backendRegistry != nil {
		a.logger.Debug("stopping backend registry")
		if err := a.backendRegistry.Stop(); err != nil {
			a.logger.Error("error stopping backend registry", "error", err)
			shutdownErr = err
		}
	}

	// Close store (Story 3)
	if a.overwatchStore != nil {
		a.logger.Debug("closing store")
		if err := a.overwatchStore.Close(); err != nil {
			a.logger.Error("error closing store", "error", err)
			shutdownErr = err
		}
	}

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
		if err := a.reloadAgentMode(newCfg); err != nil {
			metrics.RecordReload(false)
			return err
		}
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

// reloadAgentMode reloads agent-specific configuration.
func (a *Application) reloadAgentMode(newCfg *config.Config) error {
	if a.agentInstance == nil {
		return nil
	}

	// Compare backend configurations
	oldBackends := a.config.Agent.Backends
	newBackends := newCfg.Agent.Backends

	a.logger.Info("reloading agent configuration",
		"old_backends", len(oldBackends),
		"new_backends", len(newBackends),
	)

	// For now, log what would change
	// Full reload implementation would:
	// 1. Stop health checks for removed backends
	// 2. Start health checks for new backends
	// 3. Update configuration for modified backends

	// backendKey generates a unique key for a backend
	backendKey := func(service, address string, port int) string {
		return fmt.Sprintf("%s:%s:%d", service, address, port)
	}

	// Build maps for comparison using interface{} to avoid type dependency
	type backendInfo struct {
		Service string
		Address string
		Port    int
		Weight  int
	}

	oldMap := make(map[string]backendInfo)
	for _, b := range oldBackends {
		key := backendKey(b.Service, b.Address, b.Port)
		oldMap[key] = backendInfo{
			Service: b.Service,
			Address: b.Address,
			Port:    b.Port,
			Weight:  b.Weight,
		}
	}

	newMap := make(map[string]backendInfo)
	for _, b := range newBackends {
		key := backendKey(b.Service, b.Address, b.Port)
		newMap[key] = backendInfo{
			Service: b.Service,
			Address: b.Address,
			Port:    b.Port,
			Weight:  b.Weight,
		}
	}

	// Find added backends
	var added, removed, modified int
	for key := range newMap {
		if _, exists := oldMap[key]; !exists {
			added++
			a.logger.Info("backend added", "backend", key)
		}
	}

	// Find removed backends
	for key := range oldMap {
		if _, exists := newMap[key]; !exists {
			removed++
			a.logger.Info("backend removed", "backend", key)
		}
	}

	// Find modified backends (same key but different config)
	for key, newB := range newMap {
		if oldB, exists := oldMap[key]; exists {
			if newB.Weight != oldB.Weight {
				modified++
				a.logger.Info("backend modified", "backend", key,
					"old_weight", oldB.Weight, "new_weight", newB.Weight)
			}
		}
	}

	a.logger.Info("agent configuration reload complete",
		"added", added, "removed", removed, "modified", modified)

	// TODO: Implement actual backend manager reconfiguration
	// This would call a.agentInstance.GetBackendManager().Reconfigure(newBackends)

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
	// Create a routing factory with access to the health manager for latency-based routing
	latencyProvider := &healthManagerLatencyProvider{manager: a.healthManager}
	routerFactory := routing.NewFactory(routing.FactoryConfig{
		LatencyProvider:   latencyProvider,
		MinLatencySamples: 1,             // Health manager tracks one sample at a time
		GeoResolver:       a.geoResolver, // Demo 4: GeoIP-based routing
		Logger:            a.logger,
	})

	newRegistry, err := dns.BuildRegistry(newCfg, routerFactory.NewRouter)
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

// GetAgent returns the agent instance (for testing and introspection).
// Returns nil if not running in agent mode.
func (a *Application) GetAgent() *agent.Agent {
	return a.agentInstance
}

// GetBackendRegistry returns the backend registry (for testing and introspection).
// Returns nil if not running in overwatch mode.
func (a *Application) GetBackendRegistry() *overwatch.Registry {
	return a.backendRegistry
}

// GetValidator returns the external validator (for testing and introspection).
// Returns nil if not running in overwatch mode or validation is disabled.
func (a *Application) GetValidator() *overwatch.Validator {
	return a.overwatchValidator
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

// healthManagerLatencyProvider implements routing.LatencyProvider using the health.Manager.
type healthManagerLatencyProvider struct {
	manager *health.Manager
}

// GetLatency returns latency information for a server from the health manager.
func (p *healthManagerLatencyProvider) GetLatency(address string, port int) routing.LatencyInfo {
	snapshot, found := p.manager.GetStatus(address, port)
	if !found {
		return routing.LatencyInfo{HasData: false}
	}
	if snapshot.LastLatency <= 0 {
		return routing.LatencyInfo{HasData: false}
	}
	return routing.LatencyInfo{
		SmoothedLatency: snapshot.LastLatency,
		LastLatency:     snapshot.LastLatency,
		Samples:         1, // Health manager tracks one sample at a time
		HasData:         true,
	}
}
