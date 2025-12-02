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

func main() {
	// Parse command-line flags
	configPath := flag.String("config", DefaultConfigPath, "path to configuration file")
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
		"config", *configPath,
	)

	// Check config file permissions before loading
	if err := checkConfigPermissions(*configPath, bootstrapLogger); err != nil {
		bootstrapLogger.Error("configuration file security check failed", "error", err)
		os.Exit(1)
	}

	// Load configuration to get logging settings
	cfg, err := config.Load(*configPath)
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

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start application in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- app.Start(ctx)
	}()

	logger.Info("OpenGSLB running, press Ctrl+C to stop")

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		if err != nil {
			logger.Error("application error", "error", err)
		}
	}

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

	logger.Debug("config file permissions verified", "path", configPath, "mode", fmt.Sprintf("%04o", mode))
	return nil
}