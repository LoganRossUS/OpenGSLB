package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/health"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
)

const (
	// DefaultConfigPath is the default location for the configuration file.
	DefaultConfigPath = "/etc/opengslb/config.yaml"

	// MaxInsecureFileMode represents the most permissive acceptable file mode.
	// Config files must not be world-readable (no 'other' read permission).
	MaxInsecureFileMode fs.FileMode = 0o004
)

// Application manages the lifecycle of all OpenGSLB components.
type Application struct {
	configPath    string
	config        *config.Config
	dnsServer     *dns.Server
	healthManager *health.Manager
	router        dns.Router
	logger        *slog.Logger
}

// NewApplication creates a new Application instance.
func NewApplication(configPath string, logger *slog.Logger) *Application {
	if logger == nil {
		logger = slog.Default()
	}
	if configPath == "" {
		configPath = DefaultConfigPath
	}
	return &Application{
		configPath: configPath,
		logger:     logger,
	}
}

// Initialize loads configuration and sets up all components.
func (a *Application) Initialize() error {
	a.logger.Info("initializing application", "config_path", a.configPath)

	// Check config file permissions
	if err := a.checkConfigPermissions(); err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	a.config = cfg
	a.logger.Info("configuration loaded",
		"regions", len(cfg.Regions),
		"domains", len(cfg.Domains),
	)

	// Initialize router
	a.router = routing.NewRoundRobin()
	a.logger.Info("router initialized", "algorithm", a.router.Algorithm())

	// Initialize health manager
	if err := a.initializeHealthManager(); err != nil {
		return fmt.Errorf("failed to initialize health manager: %w", err)
	}

	// Initialize DNS server
	if err := a.initializeDNSServer(); err != nil {
		return fmt.Errorf("failed to initialize DNS server: %w", err)
	}

	return nil
}

// checkConfigPermissions verifies the config file has secure permissions.
func (a *Application) checkConfigPermissions() error {
	info, err := os.Stat(a.configPath)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	mode := info.Mode().Perm()
	if mode&MaxInsecureFileMode != 0 {
		return fmt.Errorf(
			"config file %s has insecure permissions %04o (world-readable); "+
				"run 'chmod 640 %s' or 'chmod 600 %s' to fix",
			a.configPath, mode, a.configPath, a.configPath,
		)
	}

	a.logger.Debug("config file permissions verified", "path", a.configPath, "mode", fmt.Sprintf("%04o", mode))
	return nil
}

// initializeHealthManager creates and configures the health manager.
func (a *Application) initializeHealthManager() error {
	checker := health.NewHTTPChecker()

	// Build manager config from first region's health check settings
	// (assuming consistent thresholds across regions for now)
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

	// Register all servers from all regions for health checking
	for _, region := range a.config.Regions {
		hc := region.HealthCheck
		for _, server := range region.Servers {
			serverCfg := health.ServerConfig{
				Address:  server.Address,
				Port:     server.Port,
				Path:     hc.Path,
				Scheme:   "http", // Default to HTTP; HTTPS support can be added later
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
			)
		}
	}

	a.logger.Info("health manager initialized",
		"servers", a.healthManager.ServerCount(),
	)
	return nil
}

// initializeDNSServer creates and configures the DNS server.
func (a *Application) initializeDNSServer() error {
	// Build DNS registry from configuration
	registry, err := dns.BuildRegistry(a.config)
	if err != nil {
		return fmt.Errorf("failed to build DNS registry: %w", err)
	}

	// Create DNS handler with all dependencies
	handler := dns.NewHandler(dns.HandlerConfig{
		Registry:       registry,
		Router:         a.router,
		HealthProvider: a.healthManager,
		DefaultTTL:     uint32(a.config.DNS.DefaultTTL),
		Logger:         a.logger,
	})

	// Create DNS server
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

// Start begins all application components.
func (a *Application) Start(ctx context.Context) error {
	a.logger.Info("starting application")

	// Start health manager
	if err := a.healthManager.Start(); err != nil {
		return fmt.Errorf("failed to start health manager: %w", err)
	}
	a.logger.Info("health manager started")

	// Start DNS server (blocks until context is canceled)
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

	// Note: DNS server shutdown is handled by context cancellation in Start()
	// We just need to stop the health manager here

	// Stop health manager
	if a.healthManager != nil {
		a.logger.Debug("stopping health manager")
		if err := a.healthManager.Stop(); err != nil {
			a.logger.Error("error stopping health manager", "error", err)
			shutdownErr = err
		}
	}

	// Give components time to clean up
	select {
	case <-ctx.Done():
		a.logger.Warn("shutdown deadline exceeded")
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		// Brief pause to allow goroutines to finish
	}

	a.logger.Info("application shutdown complete")
	return shutdownErr
}
