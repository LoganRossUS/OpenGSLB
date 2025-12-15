// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
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
	configPath  string
	runtimeMode string
)

func main() {
	// Parse command-line flags
	flag.StringVar(&configPath, "config", DefaultConfigPath, "path to configuration file")
	flag.StringVar(&runtimeMode, "mode", "", "runtime mode: agent or overwatch")
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

	// Apply command-line overrides
	applyModeOverride(cfg, bootstrapLogger)

	// Validate mode configuration
	if err := validateModeFlags(cfg); err != nil {
		bootstrapLogger.Error("mode configuration invalid", "error", err)
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
	effectiveMode := cfg.Mode
	if effectiveMode == "" {
		effectiveMode = config.ModeOverwatch // Default to overwatch for backward compat
	}

	logger.Info("configuration loaded",
		"mode", effectiveMode,
		"log_level", cfg.Logging.Level,
		"log_format", cfg.Logging.Format,
	)

	// Mode-specific logging
	switch effectiveMode {
	case config.ModeAgent:
		logger.Info("agent mode enabled",
			"region", cfg.Agent.Identity.Region,
			"backends", len(cfg.Agent.Backends),
		)
	case config.ModeOverwatch:
		logger.Info("overwatch mode enabled",
			"dns_listen", cfg.DNS.ListenAddress,
			"regions", len(cfg.Regions),
			"domains", len(cfg.Domains),
		)
		// Log API configuration for debugging binding issues
		if cfg.API.Enabled {
			logger.Info("API configuration loaded",
				"api_address", cfg.API.Address,
				"api_allowed_networks", cfg.API.AllowedNetworks,
				"api_trust_proxy", cfg.API.TrustProxyHeaders,
			)
		}
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

// applyModeOverride applies command-line flags to configuration.
func applyModeOverride(cfg *config.Config, logger *slog.Logger) {
	// --mode flag overrides config file
	if runtimeMode != "" {
		cfg.Mode = config.RuntimeMode(runtimeMode)
		if logger != nil {
			logger.Debug("mode overridden by flag", "mode", runtimeMode)
		}
	}

	// Default to overwatch if mode not specified anywhere (backward compat)
	if cfg.Mode == "" {
		cfg.Mode = config.ModeOverwatch
	}

	// Set default node name from hostname if not specified (for overwatch)
	if cfg.Mode == config.ModeOverwatch && cfg.Overwatch.Identity.NodeID == "" {
		if hostname, err := os.Hostname(); err == nil {
			cfg.Overwatch.Identity.NodeID = hostname
		}
	}
}

// validateModeFlags validates mode-specific command-line flag combinations.
func validateModeFlags(cfg *config.Config) error {
	switch cfg.Mode {
	case config.ModeAgent:
		// Agent mode validation
		if cfg.Agent.Identity.ServiceToken == "" {
			return fmt.Errorf("agent mode requires identity.service_token")
		}
		if len(cfg.Agent.Backends) == 0 {
			return fmt.Errorf("agent mode requires at least one backend")
		}
		if cfg.Agent.Gossip.EncryptionKey == "" {
			return fmt.Errorf("agent mode requires gossip.encryption_key (generate with: openssl rand -base64 32)")
		}
		if len(cfg.Agent.Gossip.OverwatchNodes) == 0 {
			return fmt.Errorf("agent mode requires at least one gossip.overwatch_nodes address")
		}
	case config.ModeOverwatch:
		// Overwatch mode validation
		if cfg.Overwatch.Gossip.EncryptionKey == "" {
			return fmt.Errorf("overwatch mode requires gossip.encryption_key (generate with: openssl rand -base64 32)")
		}
	default:
		return fmt.Errorf("invalid mode %q: must be 'agent' or 'overwatch'", cfg.Mode)
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

	// Reapply mode override (flags take precedence)
	applyModeOverride(newCfg, logger)

	// Mode change requires restart
	if app.config.Mode != newCfg.Mode {
		logger.Warn("runtime mode change requires restart",
			"old", app.config.Mode,
			"new", newCfg.Mode,
		)
		return fmt.Errorf("mode change requires restart")
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
