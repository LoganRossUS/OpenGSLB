package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

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
	dnsRegistry   *dns.Registry
	healthManager *health.Manager
	metricsServer *metrics.Server
	logger        *slog.Logger
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
	a.logger.Info("initializing application")

	metrics.SetAppInfo(version.Version)

	serverCount := 0
	for _, region := range a.config.Regions {
		serverCount += len(region.Servers)
	}
	metrics.SetConfigMetrics(len(a.config.Domains), serverCount, float64(time.Now().Unix()))

	if err := a.initializeHealthManager(); err != nil {
		return fmt.Errorf("failed to initialize health manager: %w", err)
	}

	if err := a.initializeDNSServer(); err != nil {
		return fmt.Errorf("failed to initialize DNS server: %w", err)
	}

	if err := a.initializeMetricsServer(); err != nil {
		return fmt.Errorf("failed to initialize metrics server: %w", err)
	}

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
	// Build DNS registry with per-domain routers
	registry, err := dns.BuildRegistry(a.config, routing.NewRouter)
	if err != nil {
		return fmt.Errorf("failed to build DNS registry: %w", err)
	}
	a.dnsRegistry = registry

	// Log the routing algorithms configured for each domain
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

	handler := dns.NewHandler(dns.HandlerConfig{
		Registry:       registry,
		HealthProvider: a.healthManager,
		DefaultTTL:     uint32(a.config.DNS.DefaultTTL),
		Logger:         a.logger,
	})

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

// Start begins all application components.
func (a *Application) Start(ctx context.Context) error {
	a.logger.Info("starting application")

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

	a.logger.Info("starting DNS server", "address", a.config.DNS.ListenAddress)
	if err := a.dnsServer.Start(ctx); err != nil {
		return fmt.Errorf("DNS server error: %w", err)
	}

	return nil
}

// Reload applies a new configuration without restarting.
func (a *Application) Reload(newCfg *config.Config) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	oldCfg := a.config

	a.logger.Info("reloading configuration",
		"old_domains", len(oldCfg.Domains),
		"new_domains", len(newCfg.Domains),
		"old_regions", len(oldCfg.Regions),
		"new_regions", len(newCfg.Regions),
	)

	if oldCfg.DNS.ListenAddress != newCfg.DNS.ListenAddress {
		a.logger.Warn("DNS listen address change requires restart",
			"old", oldCfg.DNS.ListenAddress,
			"new", newCfg.DNS.ListenAddress,
		)
	}
	if oldCfg.Metrics.Address != newCfg.Metrics.Address {
		a.logger.Warn("metrics address change requires restart",
			"old", oldCfg.Metrics.Address,
			"new", newCfg.Metrics.Address,
		)
	}

	// Update DNS registry (includes per-domain routers)
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

	a.logger.Info("configuration reload complete",
		"domains", len(newCfg.Domains),
		"servers", serverCount,
	)

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
			a.logger.Debug("domain reload",
				"domain", entry.Name,
				"algorithm", entry.Router.Algorithm(),
			)
		}
	}

	a.dnsRegistry.ReplaceAll(entries)

	a.logger.Debug("DNS registry updated",
		"domains", a.dnsRegistry.Count(),
	)

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
		"total", a.healthManager.ServerCount(),
	)

	return nil
}

// Shutdown gracefully stops all application components.
func (a *Application) Shutdown(ctx context.Context) error {
	a.logger.Info("shutting down application")

	var shutdownErr error

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