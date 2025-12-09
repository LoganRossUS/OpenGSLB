// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/logging"
	"github.com/loganrossus/OpenGSLB/pkg/version"
)

const (
	DefaultConfigPath               = "/etc/opengslb/config.yaml"
	MaxInsecureFileMode fs.FileMode = 0o004
)

// Command-line flags stored at package level for reload handler access.
var (
	configPath    string
	runtimeMode   string
	bootstrapFlag bool
	joinAddresses string
)

func main() {
	// Parse command-line flags
	flag.StringVar(&configPath, "config", DefaultConfigPath, "path to configuration file")
	flag.StringVar(&runtimeMode, "mode", "", "runtime mode: standalone (default) or cluster")
	flag.BoolVar(&bootstrapFlag, "bootstrap", false, "bootstrap a new cluster (cluster mode only)")
	flag.StringVar(&joinAddresses, "join", "", "comma-separated list of cluster nodes to join (cluster mode only)")
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

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		bootstrapLogger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Apply command-line overrides to cluster config
	applyClusterOverrides(cfg, bootstrapLogger)

	// Validate cluster configuration (command-line specific validation)
	if err := validateClusterFlags(cfg); err != nil {
		bootstrapLogger.Error("cluster configuration invalid", "error", err)
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

	// Log effective mode
	effectiveMode := cfg.Cluster.Mode
	if effectiveMode == "" {
		effectiveMode = config.ModeStandalone
	}

	logger.Info("configuration loaded",
		"mode", effectiveMode,
		"log_level", cfg.Logging.Level,
		"log_format", cfg.Logging.Format,
		"dns_listen", cfg.DNS.ListenAddress,
		"regions", len(cfg.Regions),
		"domains", len(cfg.Domains),
	)

	// Log cluster-specific information
	if cfg.Cluster.IsClusterMode() {
		logger.Info("cluster mode enabled",
			"node_name", cfg.Cluster.NodeName,
			"bind_address", cfg.Cluster.BindAddress,
			"bootstrap", cfg.Cluster.Bootstrap,
			"join", cfg.Cluster.Join,
		)
	}

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
		"mode", effectiveMode,
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
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := app.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	logger.Info("OpenGSLB stopped")
}

// applyClusterOverrides applies command-line flags to cluster configuration.
func applyClusterOverrides(cfg *config.Config, logger *slog.Logger) {
	// --mode flag overrides config file
	if runtimeMode != "" {
		cfg.Cluster.Mode = config.RuntimeMode(runtimeMode)
		if logger != nil {
			logger.Debug("mode overridden by flag", "mode", runtimeMode)
		}
	}

	// --bootstrap flag overrides config file
	if bootstrapFlag {
		cfg.Cluster.Bootstrap = true
		if logger != nil {
			logger.Debug("bootstrap enabled by flag")
		}
	}

	// --join flag overrides config file
	if joinAddresses != "" {
		cfg.Cluster.Join = strings.Split(joinAddresses, ",")
		if logger != nil {
			logger.Debug("join addresses set by flag", "addresses", cfg.Cluster.Join)
		}
	}

	// Default to standalone if mode not specified anywhere
	if cfg.Cluster.Mode == "" {
		cfg.Cluster.Mode = config.ModeStandalone
	}

	// Set default node name from hostname if not specified
	if cfg.Cluster.NodeName == "" {
		if hostname, err := os.Hostname(); err == nil {
			cfg.Cluster.NodeName = hostname
		}
	}
}

// validateClusterFlags validates cluster-specific command-line flag combinations.
// This is separate from config.Validate() which handles YAML validation.
func validateClusterFlags(cfg *config.Config) error {
	if cfg.Cluster.IsStandaloneMode() {
		// Standalone mode: warn if cluster flags were provided
		if bootstrapFlag {
			return fmt.Errorf("--bootstrap flag requires --mode=cluster")
		}
		if joinAddresses != "" {
			return fmt.Errorf("--join flag requires --mode=cluster")
		}
		return nil
	}

	// Cluster mode validation
	if cfg.Cluster.Bootstrap && len(cfg.Cluster.Join) > 0 {
		return fmt.Errorf("--bootstrap and --join are mutually exclusive")
	}

	if !cfg.Cluster.Bootstrap && len(cfg.Cluster.Join) == 0 {
		return fmt.Errorf("cluster mode requires either --bootstrap or --join")
	}

	return nil
}

// handleReload loads and applies a new configuration.
func handleReload(app *Application, logger *slog.Logger) error {
	if err := checkConfigPermissions(configPath, logger); err != nil {
		return fmt.Errorf("config file security check failed: %w", err)
	}

	newCfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Reapply cluster overrides (flags take precedence)
	applyClusterOverrides(newCfg, logger)

	// Mode change requires restart
	if app.config.Cluster.Mode != newCfg.Cluster.Mode {
		logger.Warn("runtime mode change requires restart",
			"old", app.config.Cluster.Mode,
			"new", newCfg.Cluster.Mode,
		)
	}

	if err := app.Reload(newCfg); err != nil {
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	logger.Info("configuration reloaded successfully")
	return nil
}

// checkConfigPermissions verifies the config file has secure permissions.
func checkConfigPermissions(path string, logger *slog.Logger) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	mode := info.Mode().Perm()
	if mode&MaxInsecureFileMode != 0 {
		return fmt.Errorf(
			"config file %s has insecure permissions %04o (world-readable); "+
				"run 'chmod 640 %s' or 'chmod 600 %s' to fix",
			path, mode, path, path,
		)
	}

	if logger != nil {
		logger.Debug("config file permissions verified", "path", path, "mode", fmt.Sprintf("%04o", mode))
	}

	return nil
}
