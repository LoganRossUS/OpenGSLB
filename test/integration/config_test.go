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

// TestConfigLoading tests basic config loading functionality.
func TestConfigLoading(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "opengslb-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("loads valid config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "valid.yaml")
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

		cfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if len(cfg.Regions) != 1 {
			t.Errorf("expected 1 region, got %d", len(cfg.Regions))
		}
		if len(cfg.Domains) != 1 {
			t.Errorf("expected 1 domain, got %d", len(cfg.Domains))
		}
	})

	t.Run("validates config", func(t *testing.T) {
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

		cfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		// Validate the config
		if err := cfg.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})

	t.Run("rejects invalid config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "invalid.yaml")
		// Missing required fields
		configContent := `
dns:
  listen_address: "127.0.0.1:15353"
`
		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := config.Load(configPath)
		if err != nil {
			// Expected to fail loading or validation
			return
		}

		// If loading succeeded, validation should fail
		if err := cfg.Validate(); err == nil {
			t.Error("expected validation to fail for incomplete config")
		}
	})
}
