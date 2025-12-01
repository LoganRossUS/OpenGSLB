package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/version"
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

	// Set up structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("OpenGSLB starting",
		"version", version.Version,
		"config", *configPath,
	)

	// Create and initialize application
	app := NewApplication(*configPath, logger)
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
