package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// validConfigContent provides a minimal valid configuration for testing.
const validConfigContent = `
dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 60

logging:
  level: info
  format: text

regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health
      failure_threshold: 3
      success_threshold: 2

domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
    ttl: 30
`

// loadTestConfig is a helper that writes config content to a temp file and loads it.
func loadTestConfig(t *testing.T, content string) *config.Config {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	return cfg
}

func TestCheckConfigPermissions(t *testing.T) {
	t.Run("secure permissions allowed", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		if err := os.WriteFile(configPath, []byte(validConfigContent), 0600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		if err := checkConfigPermissions(configPath, nil); err != nil {
			t.Errorf("expected no error for 0600, got: %v", err)
		}
	})

	t.Run("group readable allowed", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		if err := os.WriteFile(configPath, []byte(validConfigContent), 0640); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		if err := checkConfigPermissions(configPath, nil); err != nil {
			t.Errorf("expected no error for 0640, got: %v", err)
		}
	})

	t.Run("world readable rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		if err := os.WriteFile(configPath, []byte(validConfigContent), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		err := checkConfigPermissions(configPath, nil)
		if err == nil {
			t.Error("expected error for world-readable config")
		}
	})

	t.Run("world readable with group rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		if err := os.WriteFile(configPath, []byte(validConfigContent), 0604); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		err := checkConfigPermissions(configPath, nil)
		if err == nil {
			t.Error("expected error for world-readable config")
		}
	})

	t.Run("missing file returns error", func(t *testing.T) {
		err := checkConfigPermissions("/nonexistent/path/config.yaml", nil)
		if err == nil {
			t.Error("expected error for missing config file")
		}
	})
}

func TestNewApplication(t *testing.T) {
	t.Run("accepts config and logger", func(t *testing.T) {
		cfg := loadTestConfig(t, validConfigContent)
		app := NewApplication(cfg, nil)

		if app.config != cfg {
			t.Error("expected config to be set")
		}
		if app.logger == nil {
			t.Error("expected default logger when nil provided")
		}
	})

	t.Run("uses default logger when nil", func(t *testing.T) {
		cfg := loadTestConfig(t, validConfigContent)
		app := NewApplication(cfg, nil)

		if app.logger == nil {
			t.Error("expected logger to be set to default")
		}
	})
}

func TestApplicationInitialize(t *testing.T) {
	t.Run("initializes with valid config", func(t *testing.T) {
		cfg := loadTestConfig(t, validConfigContent)
		app := NewApplication(cfg, nil)

		if err := app.Initialize(); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// Verify components are initialized
		if app.config == nil {
			t.Error("config should be set")
		}
		if app.router == nil {
			t.Error("router should be initialized")
		}
		if app.healthManager == nil {
			t.Error("health manager should be initialized")
		}
		if app.dnsServer == nil {
			t.Error("DNS server should be initialized")
		}
	})
}

// lifecycleTestConfig uses a unique port to avoid conflicts with other tests.
const lifecycleTestConfig = `
dns:
  listen_address: "127.0.0.1:25353"
  default_ttl: 60

logging:
  level: info
  format: text

regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 60s
      timeout: 5s
      path: /health
      failure_threshold: 3
      success_threshold: 2

domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
    ttl: 30
`

func TestApplicationLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping lifecycle test in short mode")
	}

	t.Run("start and shutdown", func(t *testing.T) {
		cfg := loadTestConfig(t, lifecycleTestConfig)
		app := NewApplication(cfg, nil)

		if err := app.Initialize(); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// Start in background
		ctx, cancel := context.WithCancel(context.Background())
		errChan := make(chan error, 1)

		go func() {
			errChan <- app.Start(ctx)
		}()

		// Give it time to start
		time.Sleep(200 * time.Millisecond)

		// Trigger shutdown
		cancel()

		// Wait for Start to return first (it handles DNS shutdown)
		select {
		case err := <-errChan:
			if err != nil {
				t.Errorf("Start returned error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Start did not return after shutdown")
		}

		// Then shutdown remaining components (health manager)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := app.Shutdown(shutdownCtx); err != nil {
			t.Errorf("Shutdown error: %v", err)
		}
	})
}
