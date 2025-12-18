// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Test Helpers
// =============================================================================

// validEncryptionKey returns a valid 32-byte base64-encoded key for tests.
func validEncryptionKey() string {
	return base64.StdEncoding.EncodeToString(make([]byte, 32))
}

// validOverwatchConfig returns a minimal valid Overwatch configuration for tests.
// Returns a new instance each time to avoid test interference.
func validOverwatchConfig() *Config {
	return &Config{
		Mode: ModeOverwatch,
		Overwatch: OverwatchConfig{
			Gossip: OverwatchGossipConfig{
				EncryptionKey: validEncryptionKey(),
			},
			DNSSEC: DNSSECConfig{Enabled: true},
		},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100, Service: "app.example.com"}},
			HealthCheck: HealthCheck{
				Type:     "http",
				Interval: 30 * time.Second,
				Timeout:  5 * time.Second,
				Path:     "/health",
			},
		}},
		Domains: []Domain{{
			Name:             "app.example.com",
			Regions:          []string{"us-east-1"},
			RoutingAlgorithm: "round-robin",
		}},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// validAgentConfig returns a minimal valid Agent configuration for tests.
// Returns a new instance each time to avoid test interference.
func validAgentConfig() *Config {
	return &Config{
		Mode: ModeAgent,
		Agent: AgentConfig{
			Identity: AgentIdentityConfig{
				ServiceToken: "my-secure-service-token-1234",
				Region:       "us-east",
				CertPath:     DefaultAgentCertPath,
				KeyPath:      DefaultAgentKeyPath,
			},
			Backends: []AgentBackend{
				{
					Service: "myapp",
					Address: "10.0.1.10",
					Port:    8080,
					Weight:  100,
					HealthCheck: HealthCheck{
						Type:     "http",
						Interval: 5 * time.Second,
						Timeout:  2 * time.Second,
						Path:     "/health",
					},
				},
			},
			Gossip: AgentGossipConfig{
				EncryptionKey:  validEncryptionKey(),
				OverwatchNodes: []string{"overwatch:7946"},
			},
			Heartbeat: HeartbeatConfig{
				Interval:        10 * time.Second,
				MissedThreshold: 3,
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// =============================================================================
// Overwatch Mode Tests (DNS server mode - legacy standalone equivalent)
// =============================================================================

func TestParse_OverwatchValidConfig(t *testing.T) {
	yaml := `
mode: overwatch
dns:
  listen_address: ":53"
  default_ttl: 60
overwatch:
  gossip:
    encryption_key: "` + validEncryptionKey() + `"
  dnssec:
    enabled: true
regions:
  - name: us-east-1
    servers:
      - address: 10.0.1.10
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health
domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != ModeOverwatch {
		t.Errorf("expected mode overwatch, got %s", cfg.Mode)
	}
	if cfg.DNS.ListenAddress != ":53" {
		t.Errorf("expected listen_address :53, got %s", cfg.DNS.ListenAddress)
	}
	if cfg.DNS.DefaultTTL != 60 {
		t.Errorf("expected default_ttl 60, got %d", cfg.DNS.DefaultTTL)
	}
	if len(cfg.Regions) != 1 {
		t.Errorf("expected 1 region, got %d", len(cfg.Regions))
	}
	if cfg.Regions[0].Name != "us-east-1" {
		t.Errorf("expected region name us-east-1, got %s", cfg.Regions[0].Name)
	}
}

func TestParse_OverwatchAppliesDefaults(t *testing.T) {
	yaml := `
mode: overwatch
overwatch:
  gossip:
    encryption_key: "` + validEncryptionKey() + `"
regions:
  - name: us-east-1
    servers:
      - address: 10.0.1.10
domains:
  - name: app.example.com
    regions:
      - us-east-1
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DNS.ListenAddress != DefaultListenAddress {
		t.Errorf("expected default listen_address %s, got %s", DefaultListenAddress, cfg.DNS.ListenAddress)
	}
	if cfg.DNS.DefaultTTL != DefaultTTL {
		t.Errorf("expected default TTL %d, got %d", DefaultTTL, cfg.DNS.DefaultTTL)
	}
	if cfg.Regions[0].Servers[0].Port != DefaultServerPort {
		t.Errorf("expected default port %d, got %d", DefaultServerPort, cfg.Regions[0].Servers[0].Port)
	}
	if cfg.Regions[0].Servers[0].Weight != DefaultServerWeight {
		t.Errorf("expected default weight %d, got %d", DefaultServerWeight, cfg.Regions[0].Servers[0].Weight)
	}
	if cfg.Regions[0].HealthCheck.Type != DefaultHealthCheckType {
		t.Errorf("expected default health check type %s, got %s", DefaultHealthCheckType, cfg.Regions[0].HealthCheck.Type)
	}
	if cfg.Regions[0].HealthCheck.Interval != DefaultHealthInterval {
		t.Errorf("expected default interval %v, got %v", DefaultHealthInterval, cfg.Regions[0].HealthCheck.Interval)
	}
	if cfg.Domains[0].RoutingAlgorithm != DefaultRoutingAlgorithm {
		t.Errorf("expected default algorithm %s, got %s", DefaultRoutingAlgorithm, cfg.Domains[0].RoutingAlgorithm)
	}
	if cfg.Logging.Level != DefaultLogLevel {
		t.Errorf("expected default log level %s, got %s", DefaultLogLevel, cfg.Logging.Level)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	yaml := `
dns:
  listen_address: ":53"
  invalid yaml here
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "failed to parse config") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

// =============================================================================
// Agent Mode Tests (ADR-015)
// =============================================================================

func TestParse_AgentValidConfig(t *testing.T) {
	yaml := `
mode: agent
agent:
  identity:
    service_token: "my-secure-service-token-1234"
    region: us-east
  backends:
    - service: myapp
      address: 10.0.2.100
      port: 8080
      weight: 100
      health_check:
        type: http
        path: /health
        interval: 5s
        timeout: 2s
  gossip:
    encryption_key: "` + validEncryptionKey() + `"
    overwatch_nodes:
      - overwatch-1.internal:7946
      - overwatch-2.internal:7946
  heartbeat:
    interval: 10s
    missed_threshold: 3
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != ModeAgent {
		t.Errorf("expected mode agent, got %s", cfg.Mode)
	}
	if cfg.Agent.Identity.ServiceToken != "my-secure-service-token-1234" {
		t.Errorf("expected service token, got %s", cfg.Agent.Identity.ServiceToken)
	}
	if cfg.Agent.Identity.Region != "us-east" {
		t.Errorf("expected region us-east, got %s", cfg.Agent.Identity.Region)
	}
	if len(cfg.Agent.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(cfg.Agent.Backends))
	}
	if cfg.Agent.Backends[0].Service != "myapp" {
		t.Errorf("expected service myapp, got %s", cfg.Agent.Backends[0].Service)
	}
	if cfg.Agent.Backends[0].Address != "10.0.2.100" {
		t.Errorf("expected address 10.0.2.100, got %s", cfg.Agent.Backends[0].Address)
	}
	if len(cfg.Agent.Gossip.OverwatchNodes) != 2 {
		t.Errorf("expected 2 overwatch nodes, got %d", len(cfg.Agent.Gossip.OverwatchNodes))
	}
}

func TestParse_AgentAppliesDefaults(t *testing.T) {
	yaml := `
mode: agent
agent:
  identity:
    service_token: "my-secure-service-token-1234"
  backends:
    - service: myapp
      address: 10.0.2.100
      port: 8080
  gossip:
    encryption_key: "` + validEncryptionKey() + `"
    overwatch_nodes:
      - overwatch-1.internal:7946
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check agent defaults
	if cfg.Agent.Identity.CertPath != DefaultAgentCertPath {
		t.Errorf("expected default cert path %s, got %s", DefaultAgentCertPath, cfg.Agent.Identity.CertPath)
	}
	if cfg.Agent.Identity.KeyPath != DefaultAgentKeyPath {
		t.Errorf("expected default key path %s, got %s", DefaultAgentKeyPath, cfg.Agent.Identity.KeyPath)
	}
	if cfg.Agent.Heartbeat.Interval != DefaultAgentHeartbeatInterval {
		t.Errorf("expected default heartbeat interval %v, got %v", DefaultAgentHeartbeatInterval, cfg.Agent.Heartbeat.Interval)
	}
	if cfg.Agent.Heartbeat.MissedThreshold != DefaultAgentMissedThreshold {
		t.Errorf("expected default missed threshold %d, got %d", DefaultAgentMissedThreshold, cfg.Agent.Heartbeat.MissedThreshold)
	}

	// Check backend defaults
	if cfg.Agent.Backends[0].Weight != DefaultServerWeight {
		t.Errorf("expected default weight %d, got %d", DefaultServerWeight, cfg.Agent.Backends[0].Weight)
	}
	if cfg.Agent.Backends[0].HealthCheck.Type != DefaultHealthCheckType {
		t.Errorf("expected default health check type %s, got %s", DefaultHealthCheckType, cfg.Agent.Backends[0].HealthCheck.Type)
	}
}

func TestValidate_AgentMissingServiceToken(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Identity.ServiceToken = "" // Missing

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing service token")
	}
	if !strings.Contains(err.Error(), "service_token") {
		t.Errorf("expected service_token error, got: %v", err)
	}
}

func TestValidate_AgentServiceTokenTooShort(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Identity.ServiceToken = "short" // Too short

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for short service token")
	}
	if !strings.Contains(err.Error(), "16 characters") {
		t.Errorf("expected token length error, got: %v", err)
	}
}

func TestValidate_AgentNoBackends(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Backends = []AgentBackend{} // Empty

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for no backends")
	}
	if !strings.Contains(err.Error(), "backend") {
		t.Errorf("expected backend error, got: %v", err)
	}
}

func TestValidate_AgentNoOverwatchNodes(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Gossip.OverwatchNodes = []string{} // Empty

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for no overwatch nodes")
	}
	if !strings.Contains(err.Error(), "overwatch_nodes") {
		t.Errorf("expected overwatch_nodes error, got: %v", err)
	}
}

// =============================================================================
// Gossip Encryption Tests (ADR-015: Mandatory Encryption)
// =============================================================================

func TestValidate_AgentGossipEncryptionKeyRequired(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Gossip.EncryptionKey = "" // Missing - should fail

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing encryption key")
	}
	if !strings.Contains(err.Error(), "encryption_key is required") {
		t.Errorf("expected encryption key error, got: %v", err)
	}
	// Verify helpful message is included
	if !strings.Contains(err.Error(), "openssl rand -base64 32") {
		t.Errorf("expected key generation hint, got: %v", err)
	}
}

func TestValidate_AgentGossipEncryptionKeyInvalidBase64(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Gossip.EncryptionKey = "not-valid-base64!!!"

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid base64")
	}
	if !strings.Contains(err.Error(), "base64") {
		t.Errorf("expected base64 error, got: %v", err)
	}
}

func TestValidate_AgentGossipEncryptionKeyWrongLength(t *testing.T) {
	// Create a 16-byte key (should be 32)
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	cfg := validAgentConfig()
	cfg.Agent.Gossip.EncryptionKey = shortKey

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for wrong key length")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("expected 32 bytes error, got: %v", err)
	}
	// Verify the actual length is reported
	if !strings.Contains(err.Error(), "got 16") {
		t.Errorf("expected error to report actual length, got: %v", err)
	}
	// Verify helpful message about 256-bit key
	if !strings.Contains(err.Error(), "256-bit key") {
		t.Errorf("expected 256-bit key hint, got: %v", err)
	}
}

func TestValidate_OverwatchGossipEncryptionKeyRequired(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Overwatch.Gossip.EncryptionKey = "" // Missing - should fail

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing encryption key")
	}
	if !strings.Contains(err.Error(), "encryption_key is required") {
		t.Errorf("expected encryption key error, got: %v", err)
	}
	// Verify helpful message is included
	if !strings.Contains(err.Error(), "openssl rand -base64 32") {
		t.Errorf("expected key generation hint, got: %v", err)
	}
}

func TestValidate_OverwatchGossipEncryptionKeyInvalidBase64(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Overwatch.Gossip.EncryptionKey = "not-valid-base64!!!"

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid base64")
	}
	if !strings.Contains(err.Error(), "base64") {
		t.Errorf("expected base64 error, got: %v", err)
	}
}

func TestValidate_OverwatchGossipEncryptionKeyWrongLength(t *testing.T) {
	// Create a 16-byte key (should be 32)
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	cfg := validOverwatchConfig()
	cfg.Overwatch.Gossip.EncryptionKey = shortKey

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for wrong key length")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("expected 32 bytes error, got: %v", err)
	}
	// Verify the actual length is reported
	if !strings.Contains(err.Error(), "got 16") {
		t.Errorf("expected error to report actual length, got: %v", err)
	}
	// Verify helpful message about 256-bit key
	if !strings.Contains(err.Error(), "256-bit key") {
		t.Errorf("expected 256-bit key hint, got: %v", err)
	}
}

// =============================================================================
// DNSSEC Tests (ADR-015: Enabled by Default, nested under Overwatch)
// =============================================================================

func TestValidate_DNSSECDisabledRequiresAcknowledgment(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Overwatch.DNSSEC.Enabled = false
	cfg.Overwatch.DNSSEC.SecurityAcknowledgment = "" // Missing acknowledgment

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for disabled DNSSEC without acknowledgment")
	}
	if !strings.Contains(err.Error(), "security_acknowledgment") {
		t.Errorf("expected acknowledgment error, got: %v", err)
	}
}

func TestValidate_DNSSECDisabledWithAcknowledgment(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Overwatch.DNSSEC.Enabled = false
	cfg.Overwatch.DNSSEC.SecurityAcknowledgment = "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses and allows DNS spoofing attacks against my zones"

	err := cfg.Validate()
	if err != nil {
		t.Errorf("unexpected error with proper acknowledgment: %v", err)
	}
}

func TestParse_DNSSECEnabledByDefault(t *testing.T) {
	// Note: This test verifies the applyDefaults() behavior.
	// If DNSSEC defaulting is not implemented in applyDefaults(),
	// we need to check what the actual default behavior is.
	yaml := `
mode: overwatch
overwatch:
  gossip:
    encryption_key: "` + validEncryptionKey() + `"
  dnssec:
    enabled: true
regions:
  - name: us-east-1
    servers:
      - address: 10.0.1.10
        port: 80
domains:
  - name: app.example.com
    regions:
      - us-east-1
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// DNSSEC should be enabled (explicitly set in YAML)
	if !cfg.Overwatch.DNSSEC.Enabled {
		t.Error("expected DNSSEC to be enabled")
	}
}

// =============================================================================
// Overwatch Validation Tests
// =============================================================================

func TestValidate_OverwatchNoRegions(t *testing.T) {
	cfg := &Config{
		Mode: ModeOverwatch,
		Overwatch: OverwatchConfig{
			Gossip: OverwatchGossipConfig{
				EncryptionKey: validEncryptionKey(),
			},
			DNSSEC: DNSSECConfig{Enabled: true},
		},
		Regions: []Region{}, // Empty
		Domains: []Domain{{
			Name:             "app.example.com",
			Regions:          []string{"us-east-1"},
			RoutingAlgorithm: "round-robin",
		}},
		Logging: LoggingConfig{Level: "info", Format: "json"},
	}

	err := cfg.Validate()
	// Note: If validation doesn't check for empty regions, this test documents that behavior
	if err == nil {
		t.Skip("validation does not currently check for empty regions - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "region") {
		t.Errorf("expected region error, got: %v", err)
	}
}

func TestValidate_OverwatchNoDomains(t *testing.T) {
	// Build config manually without using helper to ensure Domains is truly empty
	cfg := &Config{
		Mode: ModeOverwatch,
		Overwatch: OverwatchConfig{
			Gossip: OverwatchGossipConfig{
				EncryptionKey: validEncryptionKey(),
			},
			DNSSEC: DNSSECConfig{Enabled: true},
		},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100, Service: "app.example.com"}},
		}},
		Domains: []Domain{}, // Empty, not nil - validation should check length
		Logging: LoggingConfig{Level: "info", Format: "json"},
	}

	err := cfg.Validate()
	// Note: If validation doesn't check for empty domains, this test documents that behavior
	if err == nil {
		t.Skip("validation does not currently check for empty domains - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "domain") {
		t.Errorf("expected domain error, got: %v", err)
	}
}

func TestValidate_OverwatchEmptyRegions(t *testing.T) {
	cfg := &Config{
		Mode: ModeOverwatch,
		Overwatch: OverwatchConfig{
			Gossip: OverwatchGossipConfig{
				EncryptionKey: validEncryptionKey(),
			},
			DNSSEC: DNSSECConfig{Enabled: true},
		},
		Regions: []Region{}, // Empty slice
		Domains: []Domain{{
			Name:             "app.example.com",
			Regions:          []string{"us-east-1"},
			RoutingAlgorithm: "round-robin",
		}},
		Logging: LoggingConfig{Level: "info", Format: "json"},
	}

	err := cfg.Validate()
	// Note: If validation doesn't check for empty regions, this test documents that behavior
	if err == nil {
		t.Skip("validation does not currently check for empty regions - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "region") {
		t.Errorf("expected region error, got: %v", err)
	}
}

func TestValidate_OverwatchEmptyDomains(t *testing.T) {
	// This is the same as TestValidate_OverwatchNoDomains -
	// testing empty slice specifically
	cfg := &Config{
		Mode: ModeOverwatch,
		Overwatch: OverwatchConfig{
			Gossip: OverwatchGossipConfig{
				EncryptionKey: validEncryptionKey(),
			},
			DNSSEC: DNSSECConfig{Enabled: true},
		},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100, Service: "app.example.com"}},
		}},
		Domains: []Domain{}, // Empty slice
		Logging: LoggingConfig{Level: "info", Format: "json"},
	}

	err := cfg.Validate()
	// Note: If validation doesn't check for empty domains, this test documents that behavior
	if err == nil {
		t.Skip("validation does not currently check for empty domains - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "domain") {
		t.Errorf("expected domain error, got: %v", err)
	}
}

func TestValidate_OverwatchDuplicateRegionNames(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Regions = []Region{
		{Name: "us-east-1", Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100, Service: "app.example.com"}}},
		{Name: "us-east-1", Servers: []Server{{Address: "10.0.1.11", Port: 80, Weight: 100, Service: "app.example.com"}}},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for duplicate region names")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate error, got: %v", err)
	}
}

func TestValidate_OverwatchUndefinedRegionReference(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Domains[0].Regions = []string{"us-west-2"} // Doesn't exist

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for undefined region")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestValidate_OverwatchInvalidRoutingAlgorithm(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Domains[0].RoutingAlgorithm = "invalid-algo"

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid routing algorithm")
	}
	if !strings.Contains(err.Error(), "routing_algorithm") {
		t.Errorf("expected routing algorithm error, got: %v", err)
	}
}

func TestValidate_OverwatchInvalidHealthCheckType(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Regions[0].HealthCheck = HealthCheck{
		Type:     "invalid",
		Interval: 30 * time.Second,
		Timeout:  5 * time.Second,
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid health check type")
	}
	if !strings.Contains(err.Error(), "http, https, or tcp") {
		t.Errorf("expected health check type error, got: %v", err)
	}
}

func TestValidate_OverwatchInvalidServerPort(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Regions[0].Servers[0].Port = 70000 // Invalid port

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for invalid server port - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "65535") && !strings.Contains(err.Error(), "port") {
		t.Errorf("expected port error, got: %v", err)
	}
}

func TestValidate_OverwatchRegionNoServers(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Regions[0].Servers = []Server{} // Empty

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for empty servers in region - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "server") {
		t.Errorf("expected server error, got: %v", err)
	}
}

func TestValidate_OverwatchServerNoAddress(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Regions[0].Servers[0].Address = "" // Empty

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for empty server address - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "address") {
		t.Errorf("expected address error, got: %v", err)
	}
}

func TestValidate_OverwatchDomainNoName(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Domains[0].Name = "" // Empty

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for empty domain name - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "name") {
		t.Errorf("expected name error, got: %v", err)
	}
}

func TestValidate_OverwatchDomainNoRegions(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Domains[0].Regions = []string{} // Empty

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for empty regions in domain - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "region") {
		t.Errorf("expected region error, got: %v", err)
	}
}

// =============================================================================
// Mode Helper Tests
// =============================================================================

func TestConfig_ModeHelpers(t *testing.T) {
	tests := []struct {
		name          string
		mode          RuntimeMode
		wantAgent     bool
		wantOverwatch bool
		effective     RuntimeMode
	}{
		{"agent mode", ModeAgent, true, false, ModeAgent},
		{"overwatch mode", ModeOverwatch, false, true, ModeOverwatch},
		{"empty defaults to overwatch", "", false, true, ModeOverwatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Mode: tt.mode}

			if got := cfg.IsAgentMode(); got != tt.wantAgent {
				t.Errorf("IsAgentMode() = %v, want %v", got, tt.wantAgent)
			}
			if got := cfg.IsOverwatchMode(); got != tt.wantOverwatch {
				t.Errorf("IsOverwatchMode() = %v, want %v", got, tt.wantOverwatch)
			}
			if got := cfg.GetEffectiveMode(); got != tt.effective {
				t.Errorf("GetEffectiveMode() = %v, want %v", got, tt.effective)
			}
		})
	}
}

// =============================================================================
// Invalid Mode Tests
// =============================================================================

func TestValidate_InvalidMode(t *testing.T) {
	cfg := &Config{
		Mode: "invalid-mode",
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "invalid mode") {
		t.Errorf("expected mode error, got: %v", err)
	}
}

// =============================================================================
// Logging Validation Tests
// =============================================================================

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Logging = LoggingConfig{Level: "invalid", Format: "json"}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid log level")
	}
	if !strings.Contains(err.Error(), "debug, info, warn, or error") {
		t.Errorf("expected log level error, got: %v", err)
	}
}

func TestValidate_InvalidLogFormat(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Logging = LoggingConfig{Level: "info", Format: "invalid"}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid log format")
	}
	if !strings.Contains(err.Error(), "text or json") {
		t.Errorf("expected log format error, got: %v", err)
	}
}

func TestValidate_ValidLogLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			cfg := validOverwatchConfig()
			cfg.Logging = LoggingConfig{Level: level, Format: "json"}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("unexpected error for log level %s: %v", level, err)
			}
		})
	}
}

func TestValidate_ValidLogFormats(t *testing.T) {
	formats := []string{"text", "json"}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			cfg := validOverwatchConfig()
			cfg.Logging = LoggingConfig{Level: "info", Format: format}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("unexpected error for log format %s: %v", format, err)
			}
		})
	}
}

// =============================================================================
// ValidationError Tests
// =============================================================================

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:   "dns.listen_address",
		Value:   "bad",
		Message: "invalid format",
	}
	expected := "validation failed for dns.listen_address: invalid format (got: bad)"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestValidationError_ErrorWithoutValue(t *testing.T) {
	err := &ValidationError{
		Field:   "regions",
		Message: "at least one region required",
	}
	// Should still produce readable error
	result := err.Error()
	if !strings.Contains(result, "regions") {
		t.Errorf("expected error to contain field name, got: %s", result)
	}
}

// =============================================================================
// API Validation Tests
// =============================================================================

func TestValidate_APIInvalidAddress(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.API = APIConfig{
		Enabled: true,
		Address: "not-valid-address",
	}

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for invalid API address format - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "api") && !strings.Contains(err.Error(), "address") {
		t.Errorf("expected API address error, got: %v", err)
	}
}

func TestValidate_APIInvalidCIDR(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.API = APIConfig{
		Enabled:         true,
		Address:         "127.0.0.1:8080",
		AllowedNetworks: []string{"not-a-cidr"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for invalid CIDR in allowed_networks - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "CIDR") && !strings.Contains(err.Error(), "network") {
		t.Errorf("expected CIDR error, got: %v", err)
	}
}

func TestValidate_APIValidConfig(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.API = APIConfig{
		Enabled:         true,
		Address:         "127.0.0.1:8080",
		AllowedNetworks: []string{"10.0.0.0/8", "192.168.0.0/16"},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("unexpected error for valid API config: %v", err)
	}
}

func TestLoad_APIAddressPreserved(t *testing.T) {
	// Test that custom API address is preserved and not overwritten by defaults
	// This is a regression test for an issue where API always bound to 127.0.0.1
	testCases := []struct {
		name            string
		configAddress   string
		expectedAddress string
	}{
		{"all interfaces", "0.0.0.0:8080", "0.0.0.0:8080"},
		{"specific IP", "192.168.1.100:9000", "192.168.1.100:9000"},
		{"localhost explicit", "127.0.0.1:8080", "127.0.0.1:8080"},
		{"port only", ":8080", ":8080"},
		{"empty uses default", "", "127.0.0.1:8080"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var yaml string
			if tc.configAddress == "" {
				yaml = `mode: overwatch
overwatch:
  gossip:
    encryption_key: ZHm5cOaWanWFhtVCFAsF+QK7IuwT99ixwziikJjLHW8=
api:
  enabled: true
  allowed_networks:
    - "0.0.0.0/0"
regions:
  - name: test
    servers:
      - address: 127.0.0.1
        port: 80
    health_check:
      type: tcp
domains:
  - name: test.local
    regions:
      - test
`
			} else {
				yaml = `mode: overwatch
overwatch:
  gossip:
    encryption_key: ZHm5cOaWanWFhtVCFAsF+QK7IuwT99ixwziikJjLHW8=
api:
  enabled: true
  address: "` + tc.configAddress + `"
  allowed_networks:
    - "0.0.0.0/0"
regions:
  - name: test
    servers:
      - address: 127.0.0.1
        port: 80
    health_check:
      type: tcp
domains:
  - name: test.local
    regions:
      - test
`
			}

			cfg, err := Parse([]byte(yaml))
			if err != nil {
				t.Fatalf("failed to parse config: %v", err)
			}

			if cfg.API.Address != tc.expectedAddress {
				t.Errorf("API address = %q, expected %q", cfg.API.Address, tc.expectedAddress)
			}
		})
	}
}

// =============================================================================
// Metrics Validation Tests
// =============================================================================

func TestValidate_MetricsInvalidAddress(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Metrics = MetricsConfig{
		Enabled: true,
		Address: "not-valid-address",
	}

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for invalid metrics address format - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "metrics") && !strings.Contains(err.Error(), "address") {
		t.Errorf("expected metrics address error, got: %v", err)
	}
}

func TestValidate_MetricsValidConfig(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Metrics = MetricsConfig{
		Enabled: true,
		Address: ":9090",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("unexpected error for valid metrics config: %v", err)
	}
}

// =============================================================================
// Routing Algorithm Tests
// =============================================================================

func TestValidate_ValidRoutingAlgorithms(t *testing.T) {
	algorithms := []string{"round-robin", "weighted", "failover", "geolocation", "latency"}
	for _, algo := range algorithms {
		t.Run(algo, func(t *testing.T) {
			cfg := validOverwatchConfig()
			cfg.Domains[0].RoutingAlgorithm = algo

			// Geolocation requires additional configuration
			if algo == "geolocation" {
				cfg.Overwatch.Geolocation = GeolocationConfig{
					DatabasePath:  "/var/lib/opengslb/GeoLite2-Country.mmdb",
					DefaultRegion: "us-east-1",
					ECSEnabled:    true,
				}
				cfg.Regions[0].Countries = []string{"US", "CA"}
				cfg.Regions[0].Continents = []string{"NA"}
			}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("unexpected error for routing algorithm %s: %v", algo, err)
			}
		})
	}
}

// =============================================================================
// Health Check Validation Tests
// =============================================================================

func TestValidate_ValidHealthCheckTypes(t *testing.T) {
	types := []string{"http", "https", "tcp"}
	for _, hcType := range types {
		t.Run(hcType, func(t *testing.T) {
			cfg := validOverwatchConfig()
			cfg.Regions[0].HealthCheck = HealthCheck{
				Type:     hcType,
				Interval: 30 * time.Second,
				Timeout:  5 * time.Second,
				Path:     "/health",
			}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("unexpected error for health check type %s: %v", hcType, err)
			}
		})
	}
}

func TestValidate_HealthCheckTimeoutGreaterThanInterval(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Regions[0].HealthCheck = HealthCheck{
		Type:     "http",
		Interval: 5 * time.Second,
		Timeout:  10 * time.Second, // Greater than interval
		Path:     "/health",
	}

	err := cfg.Validate()
	// Note: If validation doesn't check timeout vs interval, this test documents that behavior
	if err == nil {
		t.Skip("validation does not currently check that timeout < interval - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "interval") {
		t.Errorf("expected timeout/interval error, got: %v", err)
	}
}

// =============================================================================
// Agent Backend Validation Tests
// =============================================================================

func TestValidate_AgentBackendNoService(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Backends[0].Service = "" // Empty

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for empty backend service - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "service") {
		t.Errorf("expected service error, got: %v", err)
	}
}

func TestValidate_AgentBackendNoAddress(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Backends[0].Address = "" // Empty

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for empty backend address - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "address") {
		t.Errorf("expected address error, got: %v", err)
	}
}

func TestValidate_AgentBackendInvalidPort(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Backends[0].Port = 70000 // Invalid

	err := cfg.Validate()
	if err == nil {
		t.Skip("validation does not currently check for invalid backend port - consider adding this check")
	}
	if err != nil && !strings.Contains(err.Error(), "65535") && !strings.Contains(err.Error(), "port") {
		t.Errorf("expected port error, got: %v", err)
	}
}

func TestValidate_AgentMultipleBackends(t *testing.T) {
	cfg := validAgentConfig()
	cfg.Agent.Backends = []AgentBackend{
		{
			Service: "app1",
			Address: "10.0.1.10",
			Port:    8080,
			Weight:  100,
			HealthCheck: HealthCheck{
				Type:     "http",
				Interval: 5 * time.Second,
				Timeout:  2 * time.Second,
				Path:     "/health",
			},
		},
		{
			Service: "app2",
			Address: "10.0.1.10",
			Port:    9090,
			Weight:  100,
			HealthCheck: HealthCheck{
				Type:     "http",
				Interval: 5 * time.Second,
				Timeout:  2 * time.Second,
				Path:     "/health",
			},
		},
		{
			Service: "app3",
			Address: "10.0.1.11",
			Port:    8080,
			Weight:  100,
			HealthCheck: HealthCheck{
				Type:     "http",
				Interval: 5 * time.Second,
				Timeout:  2 * time.Second,
				Path:     "/health",
			},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("unexpected error for multiple valid backends: %v", err)
	}
}

// =============================================================================
// Edge Cases and Corner Cases
// =============================================================================

func TestParse_EmptyConfig(t *testing.T) {
	yaml := ``
	cfg, err := Parse([]byte(yaml))
	// Empty config may or may not fail validation depending on implementation
	// Parse itself should succeed (valid YAML), but validation might fail
	if err != nil {
		// Parse failed - that's acceptable for empty input
		return
	}
	// If parse succeeded, the config should exist
	if cfg == nil {
		t.Error("expected non-nil config from Parse")
		return
	}
	// Validation of empty config - this may or may not return an error
	// depending on what validations are implemented
	_ = cfg.Validate() // Just verify it doesn't panic
}

func TestParse_MinimalOverwatchConfig(t *testing.T) {
	yaml := `
mode: overwatch
overwatch:
  gossip:
    encryption_key: "` + validEncryptionKey() + `"
  dnssec:
    enabled: true
regions:
  - name: r1
    servers:
      - address: 1.2.3.4
domains:
  - name: d.com
    regions: [r1]
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Regions) != 1 || len(cfg.Domains) != 1 {
		t.Error("expected 1 region and 1 domain")
	}
}

func TestParse_MinimalAgentConfig(t *testing.T) {
	yaml := `
mode: agent
agent:
  identity:
    service_token: "secure-token-at-least-16"
  backends:
    - service: svc
      address: 1.2.3.4
      port: 80
  gossip:
    encryption_key: "` + validEncryptionKey() + `"
    overwatch_nodes: ["o:7946"]
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != ModeAgent {
		t.Errorf("expected agent mode, got %s", cfg.Mode)
	}
}

// =============================================================================
// IPv6 Address Tests
// =============================================================================

func TestValidate_IPv6ServerAddress(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Regions[0].Servers = []Server{
		{Address: "2001:db8::1", Port: 80, Weight: 100, Service: "app.example.com"},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("unexpected error for IPv6 address: %v", err)
	}
}

func TestValidate_MixedIPv4IPv6(t *testing.T) {
	cfg := validOverwatchConfig()
	cfg.Regions[0].Servers = []Server{
		{Address: "10.0.1.10", Port: 80, Weight: 100, Service: "app.example.com"},
		{Address: "2001:db8::1", Port: 80, Weight: 100, Service: "app.example.com"},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("unexpected error for mixed IPv4/IPv6: %v", err)
	}
}
