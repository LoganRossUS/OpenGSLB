package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/logging"
	"github.com/loganrossus/OpenGSLB/pkg/version"
)

const (
	// DefaultConfigPath is the default location for the configuration file.
	DefaultConfigPath = "/etc/opengslb/config.yaml"

	// MaxInsecureFileMode represents the most permissive acceptable file mode.
	// Config files must not be world-readable (no 'other' read permission).
	MaxInsecureFileMode fs.FileMode = 0o004
)

// configPath is stored at package level so reload handler can access it.
var configPath string

func main() {
	// Parse command-line flags
	flag.StringVar(&configPath, "config", DefaultConfigPath, "path to configuration file")
	showVersion := flag.Bool("version", false, "show version information")
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("OpenGSLB %s\n", version.Version)
		os.Exit(0)
	}

	// Bootstrap logger for startup (before config is loaded)
	bootstrapLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	bootstrapLogger.Info("OpenGSLB starting",
		"version", version.Version,
		"config", configPath,
	)

	// Check config file permissions before loading
	if err := checkConfigPermissions(configPath, bootstrapLogger); err != nil {
		bootstrapLogger.Error("configuration file security check failed", "error", err)
		os.Exit(1)
	}

	// Load configuration to get logging settings
	cfg, err := config.Load(configPath)
	if err != nil {
		bootstrapLogger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Create configured logger
	logger, err := logging.NewLogger(logging.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})
	if err != nil {
		bootstrapLogger.Error("failed to create logger", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	logger.Info("configuration loaded",
		"log_level", cfg.Logging.Level,
		"log_format", cfg.Logging.Format,
		"dns_listen", cfg.DNS.ListenAddress,
		"regions", len(cfg.Regions),
		"domains", len(cfg.Domains),
	)

	// Create and initialize application
	app := NewApplication(cfg, logger)
	if err := app.Initialize(); err != nil {
		logger.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	// Set up context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals (SIGINT, SIGTERM) and reload signal (SIGHUP)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Start application in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- app.Start(ctx)
	}()

	logger.Info("OpenGSLB running",
		"pid", os.Getpid(),
		"reload", "send SIGHUP to reload configuration",
	)

	// Main event loop
	for {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGHUP:
				logger.Info("received SIGHUP, reloading configuration")
				if err := handleReload(app, logger); err != nil {
					logger.Error("configuration reload failed", "error", err)
					// Continue running with old configuration
				}
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("received shutdown signal", "signal", sig)
				goto shutdown
			}
		case err := <-errChan:
			if err != nil {
				logger.Error("application error", "error", err)
			}
			goto shutdown
		}
	}

shutdown:
	// Graceful shutdown with timeout
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := app.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	logger.Info("OpenGSLB stopped")
}

// handleReload loads and applies a new configuration.
func handleReload(app *Application, logger *slog.Logger) error {
	// Check file permissions first
	if err := checkConfigPermissions(configPath, logger); err != nil {
		return fmt.Errorf("config file security check failed: %w", err)
	}

	// Load and validate new configuration
	newCfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Apply the new configuration
	if err := app.Reload(newCfg); err != nil {
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	logger.Info("configuration reloaded successfully")
	return nil
}

// checkConfigPermissions verifies the config file has secure permissions.
func checkConfigPermissions(configPath string, logger *slog.Logger) error {
	info, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	mode := info.Mode().Perm()
	if mode&MaxInsecureFileMode != 0 {
		return fmt.Errorf(
			"config file %s has insecure permissions %04o (world-readable); "+
				"run 'chmod 640 %s' or 'chmod 600 %s' to fix",
			configPath, mode, configPath, configPath,
		)
	}

	if logger != nil {
		logger.Debug("config file permissions verified", "path", configPath, "mode", fmt.Sprintf("%04o", mode))
	}

	return nil
}
