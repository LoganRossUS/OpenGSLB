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

//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// TestMultiFileConfiguration tests the config includes functionality.
// This verifies that configuration can be split across multiple files.
func TestMultiFileConfiguration(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "opengslb-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("loads single file config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "single.yaml")
		configContent := `
mode: overwatch
dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 30

overwatch:
  gossip:
    encryption_key: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
    bind_address: "127.0.0.1:17946"
  dnssec:
    enabled: false
    security_acknowledgment: "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses and allows DNS spoofing attacks against my zones"

regions:
  - name: test-region
    servers:
      - address: "10.0.1.10"
        port: 80

domains:
  - name: test.example.com
    routing_algorithm: round-robin
    regions: [test-region]
`
		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := config.LoadFromFile(configPath)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if len(cfg.Regions) != 1 {
			t.Errorf("expected 1 region, got %d", len(cfg.Regions))
		}
		if len(cfg.Domains) != 1 {
			t.Errorf("expected 1 domain, got %d", len(cfg.Domains))
		}
		if cfg.Domains[0].Name != "test.example.com" {
			t.Errorf("expected domain 'test.example.com', got %q", cfg.Domains[0].Name)
		}
	})

	t.Run("loads config with includes", func(t *testing.T) {
		// Create regions config file
		regionsDir := filepath.Join(tmpDir, "regions")
		if err := os.MkdirAll(regionsDir, 0755); err != nil {
			t.Fatalf("failed to create regions dir: %v", err)
		}

		regionsContent := `
regions:
  - name: us-east
    servers:
      - address: "10.0.1.10"
        port: 80
        weight: 100
  - name: eu-west
    servers:
      - address: "10.0.2.10"
        port: 80
        weight: 100
`
		regionsFile := filepath.Join(regionsDir, "regions.yaml")
		if err := os.WriteFile(regionsFile, []byte(regionsContent), 0600); err != nil {
			t.Fatalf("failed to write regions config: %v", err)
		}

		// Create domains config file
		domainsDir := filepath.Join(tmpDir, "domains")
		if err := os.MkdirAll(domainsDir, 0755); err != nil {
			t.Fatalf("failed to create domains dir: %v", err)
		}

		domainsContent := `
domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions: [us-east, eu-west]
  - name: api.example.com
    routing_algorithm: weighted
    regions: [us-east]
`
		domainsFile := filepath.Join(domainsDir, "domains.yaml")
		if err := os.WriteFile(domainsFile, []byte(domainsContent), 0600); err != nil {
			t.Fatalf("failed to write domains config: %v", err)
		}

		// Create main config with includes
		mainConfig := `
mode: overwatch

includes:
  - ` + regionsFile + `
  - ` + domainsFile + `

dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 60

overwatch:
  gossip:
    encryption_key: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
    bind_address: "127.0.0.1:17946"
  dnssec:
    enabled: false
    security_acknowledgment: "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses and allows DNS spoofing attacks against my zones"
`
		mainConfigPath := filepath.Join(tmpDir, "main.yaml")
		if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0600); err != nil {
			t.Fatalf("failed to write main config: %v", err)
		}

		cfg, err := config.LoadFromFile(mainConfigPath)
		if err != nil {
			t.Fatalf("failed to load config with includes: %v", err)
		}

		// Verify regions were loaded from includes
		if len(cfg.Regions) != 2 {
			t.Errorf("expected 2 regions, got %d", len(cfg.Regions))
		}

		// Verify domains were loaded from includes
		if len(cfg.Domains) != 2 {
			t.Errorf("expected 2 domains, got %d", len(cfg.Domains))
		}

		// Check specific values
		foundUsEast := false
		foundEuWest := false
		for _, r := range cfg.Regions {
			if r.Name == "us-east" {
				foundUsEast = true
			}
			if r.Name == "eu-west" {
				foundEuWest = true
			}
		}
		if !foundUsEast {
			t.Error("missing us-east region from includes")
		}
		if !foundEuWest {
			t.Error("missing eu-west region from includes")
		}
	})

	t.Run("handles glob patterns in includes", func(t *testing.T) {
		// Create config.d directory with multiple files
		configD := filepath.Join(tmpDir, "config.d")
		if err := os.MkdirAll(configD, 0755); err != nil {
			t.Fatalf("failed to create config.d dir: %v", err)
		}

		// Create multiple region files
		region1 := `
regions:
  - name: region-1
    servers:
      - address: "10.1.0.1"
        port: 80
`
		region2 := `
regions:
  - name: region-2
    servers:
      - address: "10.2.0.1"
        port: 80
`
		if err := os.WriteFile(filepath.Join(configD, "01-region1.yaml"), []byte(region1), 0600); err != nil {
			t.Fatalf("failed to write region1: %v", err)
		}
		if err := os.WriteFile(filepath.Join(configD, "02-region2.yaml"), []byte(region2), 0600); err != nil {
			t.Fatalf("failed to write region2: %v", err)
		}

		// Create main config with glob pattern
		mainConfig := `
mode: overwatch

includes:
  - ` + filepath.Join(configD, "*.yaml") + `

dns:
  listen_address: "127.0.0.1:15353"

overwatch:
  gossip:
    encryption_key: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
    bind_address: "127.0.0.1:17946"
  dnssec:
    enabled: false
    security_acknowledgment: "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses and allows DNS spoofing attacks against my zones"

domains:
  - name: glob-test.example.com
    routing_algorithm: round-robin
    regions: [region-1, region-2]
`
		mainConfigPath := filepath.Join(tmpDir, "glob-main.yaml")
		if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0600); err != nil {
			t.Fatalf("failed to write glob main config: %v", err)
		}

		cfg, err := config.LoadFromFile(mainConfigPath)
		if err != nil {
			t.Fatalf("failed to load config with glob includes: %v", err)
		}

		// Should have loaded both region files via glob
		if len(cfg.Regions) != 2 {
			t.Errorf("expected 2 regions from glob, got %d", len(cfg.Regions))
		}
	})

	t.Run("validates configuration", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "validated.yaml")
		configContent := `
mode: overwatch
dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 30

overwatch:
  gossip:
    encryption_key: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
    bind_address: "127.0.0.1:17946"
  dnssec:
    enabled: false
    security_acknowledgment: "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses and allows DNS spoofing attacks against my zones"

regions:
  - name: valid-region
    servers:
      - address: "10.0.1.10"
        port: 80
    health_check:
      type: http
      interval: 5s
      timeout: 2s
      path: /health

domains:
  - name: valid.example.com
    routing_algorithm: round-robin
    regions: [valid-region]
    ttl: 30
`
		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := config.LoadFromFile(configPath)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		// Validate the config
		if err := cfg.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})
}

// TestConfigEnvironmentVariables tests environment variable expansion in config.
func TestConfigEnvironmentVariables(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "opengslb-env-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("expands environment variables", func(t *testing.T) {
		// Set test environment variable
		os.Setenv("TEST_GSLB_PORT", "15353")
		defer os.Unsetenv("TEST_GSLB_PORT")

		configPath := filepath.Join(tmpDir, "env.yaml")
		configContent := `
mode: overwatch
dns:
  listen_address: "127.0.0.1:${TEST_GSLB_PORT}"
  default_ttl: 30

overwatch:
  gossip:
    encryption_key: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
    bind_address: "127.0.0.1:17946"
  dnssec:
    enabled: false
    security_acknowledgment: "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses and allows DNS spoofing attacks against my zones"

regions:
  - name: test-region
    servers:
      - address: "10.0.1.10"
        port: 80

domains:
  - name: env-test.example.com
    routing_algorithm: round-robin
    regions: [test-region]
`
		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := config.LoadFromFile(configPath)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		// Check that environment variable was expanded
		if cfg.DNS.ListenAddress != "127.0.0.1:15353" {
			t.Errorf("expected listen address '127.0.0.1:15353', got %q", cfg.DNS.ListenAddress)
		}
	})
}
