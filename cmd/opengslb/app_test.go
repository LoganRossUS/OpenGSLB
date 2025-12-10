// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    → https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

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
		if app.healthManager == nil {
			t.Error("health manager should be initialized")
		}
		if app.dnsServer == nil {
			t.Error("DNS server should be initialized")
		}
		if app.dnsRegistry == nil {
			t.Error("DNS registry should be initialized")
		}
		// Verify domains are registered with routers
		if app.dnsRegistry.Count() != 1 {
			t.Errorf("expected 1 domain registered, got %d", app.dnsRegistry.Count())
		}
	})
}

// multiAlgorithmConfig tests per-domain routing
const multiAlgorithmConfig = `
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
      - address: "10.0.1.11"
        port: 80
        weight: 200
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health

  - name: us-west-1
    servers:
      - address: "10.0.2.10"
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health

domains:
  - name: roundrobin.example.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
    ttl: 30

  - name: weighted.example.com
    routing_algorithm: weighted
    regions:
      - us-east-1
    ttl: 30

  - name: failover.example.com
    routing_algorithm: failover
    regions:
      - us-east-1
      - us-west-1
    ttl: 30
`

func TestApplicationPerDomainRouting(t *testing.T) {
	t.Run("each domain gets its own router", func(t *testing.T) {
		cfg := loadTestConfig(t, multiAlgorithmConfig)
		app := NewApplication(cfg, nil)

		if err := app.Initialize(); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// Check each domain has the correct routing algorithm
		tests := []struct {
			domain    string
			algorithm string
		}{
			{"roundrobin.example.com.", "round-robin"},
			{"weighted.example.com.", "weighted"},
			{"failover.example.com.", "failover"},
		}

		for _, tc := range tests {
			entry := app.dnsRegistry.Lookup(tc.domain)
			if entry == nil {
				t.Errorf("domain %s not found in registry", tc.domain)
				continue
			}
			if entry.Router == nil {
				t.Errorf("domain %s has no router", tc.domain)
				continue
			}
			if entry.Router.Algorithm() != tc.algorithm {
				t.Errorf("domain %s: expected algorithm %s, got %s",
					tc.domain, tc.algorithm, entry.Router.Algorithm())
			}
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

// =============================================================================
