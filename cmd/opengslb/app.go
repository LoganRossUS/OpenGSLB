// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/api"
	"github.com/loganrossus/OpenGSLB/pkg/cluster"
	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/health"
	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/loganrossus/OpenGSLB/pkg/version"
)

// Application manages the lifecycle of all OpenGSLB components.
type Application struct {
	config        *config.Config
	configMu      sync.RWMutex
	dnsServer     *dns.Server
	dnsHandler    *dns.Handler
	dnsRegistry   *dns.Registry
	healthManager *health.Manager
	metricsServer *metrics.Server
	apiServer     *api.Server
	logger        *slog.Logger

	// Cluster mode components
	raftNode *cluster.RaftNode
}

// NewApplication creates a new Application instance with pre-loaded configuration.
func NewApplication(cfg *config.Config, logger *slog.Logger) *Application {
	if logger == nil {
		logger = slog.Default()
	}
	return &Application{
		config: cfg,
		logger: logger,
	}
}

// Initialize sets up all components using the loaded configuration.
func (a *Application) Initialize() error {
	a.logger.Info("initializing application", "mode", a.config.Cluster.Mode)

	metrics.SetAppInfo(version.Version)

	serverCount := 0
	for _, region := range a.config.Regions {
		serverCount += len(region.Servers)
	}
	metrics.SetConfigMetrics(len(a.config.Domains), serverCount, float64(time.Now().Unix()))

	// Mode-specific initialization
	if a.config.Cluster.IsClusterMode() {
		if err := a.initializeClusterMode(); err != nil {
			return fmt.Errorf("failed to initialize cluster mode: %w", err)
		}
		// Set cluster mode metric
		metrics.SetClusterMode(true)
	} else {
		a.logger.Info("running in standalone mode")
		// Set standalone mode metric
		metrics.SetClusterMode(false)
	}

	if err := a.initializeHealthManager(); err != nil {
		return fmt.Errorf("failed to initialize health manager: %w", err)
	}

	if err := a.initializeDNSServer(); err != nil {
		return fmt.Errorf("failed to initialize DNS server: %w", err)
	}

	if err := a.initializeMetricsServer(); err != nil {
		return fmt.Errorf("failed to initialize metrics server: %w", err)
	}

	if err := a.initializeAPIServer(); err != nil {
		return fmt.Errorf("failed to initialize API server: %w", err)
	}

	return nil
}

// initializeClusterMode sets up cluster-specific components.
func (a *Application) initializeClusterMode() error {
	a.logger.Info("initializing cluster mode components",
		"node_name", a.config.Cluster.NodeName,
		"bind_address", a.config.Cluster.BindAddress,
		"bootstrap", a.config.Cluster.Bootstrap,
	)

	// Determine data directory for Raft
	dataDir := a.config.Cluster.Raft.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(".", "data", a.config.Cluster.NodeName)
	}

	// Create Raft node configuration
	raftCfg := cluster.DefaultConfig()
	raftCfg.NodeID = a.config.Cluster.NodeName
	raftCfg.NodeName = a.config.Cluster.NodeName
	raftCfg.BindAddress = a.config.Cluster.BindAddress
	raftCfg.AdvertiseAddress = a.config.Cluster.AdvertiseAddress
	raftCfg.DataDir = dataDir
	raftCfg.Bootstrap = a.config.Cluster.Bootstrap
	raftCfg.Join = a.config.Cluster.Join

	// Apply timeouts from config if set
	if a.config.Cluster.Raft.HeartbeatTimeout > 0 {
		raftCfg.HeartbeatTimeout = a.config.Cluster.Raft.HeartbeatTimeout
	}
	if a.config.Cluster.Raft.ElectionTimeout > 0 {
		raftCfg.ElectionTimeout = a.config.Cluster.Raft.ElectionTimeout
	}

	// Create Raft node
	raftNode, err := cluster.NewRaftNode(raftCfg, a.logger.With("component", "raft"))
	if err != nil {
		return fmt.Errorf("failed to create raft node: %w", err)
	}
	a.raftNode = raftNode

	// Set node info metric
	metrics.SetClusterNodeInfo(a.config.Cluster.NodeName, a.config.Cluster.BindAddress)

	// Register leader observer for metrics and DNS server activation
	a.raftNode.RegisterLeaderObserver(func(isLeader bool) {
		a.onLeadershipChange(isLeader)
	})

	a.logger.Info("cluster mode components initialized")
	return nil
}

// onLeadershipChange is called when this node's leadership status changes.
// It updates metrics and logs the transition.
func (a *Application) onLeadershipChange(isLeader bool) {
	a.logger.Info("leadership changed", "is_leader", isLeader)

	// Update metrics
	metrics.SetClusterLeader(isLeader)
	if isLeader {
		metrics.SetClusterState("leader")
		a.logger.Info("this node is now the cluster leader - DNS queries will be served")
	} else {
		metrics.SetClusterState("follower")
		a.logger.Info("this node is now a follower - DNS queries will be refused")
	}
	metrics.RecordLeaderChange()
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

	// Create leader checker - in cluster mode, use raftNode; in standalone, use nil (defaults to always leader)
	var leaderChecker dns.LeaderChecker
	if a.config.Cluster.IsClusterMode() && a.raftNode != nil {
		leaderChecker = a.raftNode
	}

	handler := dns.NewHandler(dns.HandlerConfig{
		Registry:       registry,
		HealthProvider: a.healthManager,
		LeaderChecker:  leaderChecker,
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
		"cluster_mode", a.config.Cluster.IsClusterMode(),
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

	// Set up cluster handlers if in cluster mode
	if a.config.Cluster.IsClusterMode() && a.raftNode != nil {
		clusterHandlers := api.NewClusterHandlers(
			a.raftNode,
			string(a.config.Cluster.Mode),
			a.logger.With("component", "cluster-api"),
		)
		server.SetClusterHandlers(clusterHandlers)
		a.logger.Debug("cluster API handlers configured")
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
	a.logger.Info("starting application", "mode", a.config.Cluster.Mode)

	// In cluster mode, start Raft first
	if a.config.Cluster.IsClusterMode() && a.raftNode != nil {
		if err := a.raftNode.Start(ctx); err != nil {
			return fmt.Errorf("failed to start Raft node: %w", err)
		}
		a.logger.Info("Raft node started")

		// Wait for leader election
		leaderCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := a.raftNode.WaitForLeader(leaderCtx); err != nil {
			a.logger.Warn("timeout waiting for leader election", "error", err)
		} else {
			leader, _ := a.raftNode.Leader()
			a.logger.Info("leader elected", "leader_id", leader.NodeID)
		}
	}

	if err := a.healthManager.Start(); err != nil {
		return fmt.Errorf("failed to start health manager: %w", err)
	}
	a.logger.Info("health manager started")

	if a.metricsServer != nil {
		go func() {
			if err := a.metricsServer.Start(ctx); err != nil {
				a.logger.Error("metrics server error", "error", err)
			}
		}()
	}

	if a.apiServer != nil {
		go func() {
			if err := a.apiServer.Start(ctx); err != nil {
				a.logger.Error("API server error", "error", err)
			}
		}()
	}

	// Start DNS server
	// In cluster mode, the server always runs but the handler checks leadership
	// before processing queries. Non-leaders return REFUSED.
	a.logger.Info("starting DNS server",
		"address", a.config.DNS.ListenAddress,
		"leader_check", a.config.Cluster.IsClusterMode(),
	)
	if err := a.dnsServer.Start(ctx); err != nil {
		return fmt.Errorf("DNS server error: %w", err)
	}

	return nil
}

// Shutdown gracefully stops all application components.
func (a *Application) Shutdown(ctx context.Context) error {
	a.logger.Info("shutting down application")

	var shutdownErr error

	// Stop Raft node if in cluster mode
	if a.raftNode != nil {
		a.logger.Debug("stopping Raft node")
		if err := a.raftNode.Stop(ctx); err != nil {
			a.logger.Error("error stopping Raft node", "error", err)
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

	select {
	case <-ctx.Done():
		a.logger.Warn("shutdown deadline exceeded")
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}

	a.logger.Info("application shutdown complete")
	return shutdownErr
}

// IsLeader returns true if this node is the Raft leader.
// In standalone mode, always returns true.
func (a *Application) IsLeader() bool {
	if a.config.Cluster.IsStandaloneMode() {
		return true
	}
	if a.raftNode == nil {
		return true
	}
	return a.raftNode.IsLeader()
}

// GetRaftNode returns the Raft node for cluster operations.
// Returns nil in standalone mode.
func (a *Application) GetRaftNode() *cluster.RaftNode {
	return a.raftNode
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
	if oldCfg.Cluster.Mode != newCfg.Cluster.Mode {
		a.logger.Warn("cluster mode change requires restart")
	}

	if err := a.reloadDNSRegistry(newCfg); err != nil {
		metrics.RecordReload(false)
		return fmt.Errorf("failed to reload DNS registry: %w", err)
	}

	if err := a.reloadHealthManager(newCfg); err != nil {
		metrics.RecordReload(false)
		return fmt.Errorf("failed to reload health manager: %w", err)
	}

	serverCount := 0
	for _, region := range newCfg.Regions {
		serverCount += len(region.Servers)
	}
	metrics.SetConfigMetrics(len(newCfg.Domains), serverCount, float64(time.Now().Unix()))

	a.config = newCfg
	metrics.RecordReload(true)

	a.logger.Info("configuration reload complete")
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
