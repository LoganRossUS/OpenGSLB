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

package config

import (
	"strings"
	"testing"
	"time"
)

func TestParse_ValidConfig(t *testing.T) {
	yaml := `
dns:
  listen_address: ":53"
  default_ttl: 60
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

func TestParse_AppliesDefaults(t *testing.T) {
	yaml := `
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

func TestValidate_NoRegions(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for no regions")
	}
	if !strings.Contains(err.Error(), "at least one region must be defined") {
		t.Errorf("expected region error, got: %v", err)
	}
}

func TestValidate_NoDomains(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for no domains")
	}
	if !strings.Contains(err.Error(), "at least one domain must be defined") {
		t.Errorf("expected domain error, got: %v", err)
	}
}

func TestValidate_InvalidServerAddress(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "not-an-ip", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid IP")
	}
	if !strings.Contains(err.Error(), "invalid IP address") {
		t.Errorf("expected IP error, got: %v", err)
	}
}

func TestValidate_DuplicateRegionNames(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Regions: []Region{
			{Name: "us-east-1", Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
				HealthCheck: HealthCheck{Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second, Path: "/health", FailureThreshold: 3, SuccessThreshold: 2}},
			{Name: "us-east-1", Servers: []Server{{Address: "10.0.1.11", Port: 80, Weight: 100}},
				HealthCheck: HealthCheck{Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second, Path: "/health", FailureThreshold: 3, SuccessThreshold: 2}},
		},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for duplicate region")
	}
	if !strings.Contains(err.Error(), "duplicate region name") {
		t.Errorf("expected duplicate error, got: %v", err)
	}
}

func TestValidate_UndefinedRegionReference(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-west-2"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for undefined region reference")
	}
	if !strings.Contains(err.Error(), "references undefined region") {
		t.Errorf("expected undefined region error, got: %v", err)
	}
}

func TestValidate_InvalidRoutingAlgorithm(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "invalid-algo", Regions: []string{"us-east-1"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid algorithm")
	}
	if !strings.Contains(err.Error(), "must be one of: round-robin") {
		t.Errorf("expected algorithm error, got: %v", err)
	}
}

func TestValidate_InvalidHealthCheckType(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "invalid", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid health check type")
	}
	if !strings.Contains(err.Error(), "must be one of: http, https, tcp") {
		t.Errorf("expected health check type error, got: %v", err)
	}
}

func TestValidate_TimeoutGreaterThanInterval(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 5 * time.Second, Timeout: 10 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for timeout >= interval")
	}
	if !strings.Contains(err.Error(), "must be less than interval") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 70000, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid port")
	}
	if !strings.Contains(err.Error(), "must be between 1 and 65535") {
		t.Errorf("expected port error, got: %v", err)
	}
}

func TestValidate_InvalidTTL(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 100000},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid TTL")
	}
	if !strings.Contains(err.Error(), "must be between 1 and 86400") {
		t.Errorf("expected TTL error, got: %v", err)
	}
}

func TestValidate_HTTPPathWithoutSlash(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for path without /")
	}
	if !strings.Contains(err.Error(), "must start with /") {
		t.Errorf("expected path error, got: %v", err)
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := &Config{
		DNS:     DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Logging: LoggingConfig{Level: "invalid", Format: "json"},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid log level")
	}
	if !strings.Contains(err.Error(), "must be one of: debug, info, warn, error") {
		t.Errorf("expected log level error, got: %v", err)
	}
}

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

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		DNS: DNSConfig{ListenAddress: ":53", DefaultTTL: 100000},
		Regions: []Region{{
			Name:    "",
			Servers: []Server{{Address: "not-an-ip", Port: 70000, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "invalid", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "", RoutingAlgorithm: "invalid", Regions: []string{}, TTL: 60}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected multiple errors")
	}

	errStr := err.Error()
	expectedErrors := []string{"default_ttl", "name", "IP address", "port", "http, https, tcp", "algorithm"}
	for _, e := range expectedErrors {
		if !strings.Contains(errStr, e) {
			t.Errorf("expected error to contain %q, got: %s", e, errStr)
		}
	}
}

// =============================================================================
// Cluster Configuration Tests
// =============================================================================

func TestParse_ClusterConfigDefaults(t *testing.T) {
	yaml := `
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

	// Cluster defaults
	if cfg.Cluster.Mode != ModeStandalone {
		t.Errorf("expected default mode %s, got %s", ModeStandalone, cfg.Cluster.Mode)
	}
	if cfg.Cluster.Raft.DataDir != DefaultRaftDataDir {
		t.Errorf("expected default raft data_dir %s, got %s", DefaultRaftDataDir, cfg.Cluster.Raft.DataDir)
	}
	if cfg.Cluster.Raft.HeartbeatTimeout != DefaultRaftHeartbeatTimeout {
		t.Errorf("expected default heartbeat_timeout %v, got %v", DefaultRaftHeartbeatTimeout, cfg.Cluster.Raft.HeartbeatTimeout)
	}
	if cfg.Cluster.Raft.ElectionTimeout != DefaultRaftElectionTimeout {
		t.Errorf("expected default election_timeout %v, got %v", DefaultRaftElectionTimeout, cfg.Cluster.Raft.ElectionTimeout)
	}
	if cfg.Cluster.Raft.SnapshotThreshold != DefaultRaftSnapshotThreshold {
		t.Errorf("expected default snapshot_threshold %d, got %d", DefaultRaftSnapshotThreshold, cfg.Cluster.Raft.SnapshotThreshold)
	}
}

func TestParse_ClusterConfigExplicit(t *testing.T) {
	yaml := `
regions:
  - name: us-east-1
    servers:
      - address: 10.0.1.10
domains:
  - name: app.example.com
    regions:
      - us-east-1

cluster:
  mode: cluster
  node_name: node-1
  bind_address: "10.0.1.10:7946"
  advertise_address: "10.0.1.10:7946"
  anycast_vip: "10.99.99.1"
  raft:
    data_dir: "/custom/raft"
    heartbeat_timeout: 500ms
    election_timeout: 1s
    snapshot_interval: 60s
    snapshot_threshold: 4096
  gossip:
    encryption_key: "dGVzdGtleQ=="
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Cluster.Mode != ModeCluster {
		t.Errorf("expected mode cluster, got %s", cfg.Cluster.Mode)
	}
	if cfg.Cluster.NodeName != "node-1" {
		t.Errorf("expected node_name node-1, got %s", cfg.Cluster.NodeName)
	}
	if cfg.Cluster.BindAddress != "10.0.1.10:7946" {
		t.Errorf("expected bind_address 10.0.1.10:7946, got %s", cfg.Cluster.BindAddress)
	}
	if cfg.Cluster.AnycastVIP != "10.99.99.1" {
		t.Errorf("expected anycast_vip 10.99.99.1, got %s", cfg.Cluster.AnycastVIP)
	}
	if cfg.Cluster.Raft.DataDir != "/custom/raft" {
		t.Errorf("expected data_dir /custom/raft, got %s", cfg.Cluster.Raft.DataDir)
	}
	if cfg.Cluster.Raft.HeartbeatTimeout != 500*time.Millisecond {
		t.Errorf("expected heartbeat_timeout 500ms, got %v", cfg.Cluster.Raft.HeartbeatTimeout)
	}
	if cfg.Cluster.Raft.SnapshotThreshold != 4096 {
		t.Errorf("expected snapshot_threshold 4096, got %d", cfg.Cluster.Raft.SnapshotThreshold)
	}
	if cfg.Cluster.Gossip.EncryptionKey != "dGVzdGtleQ==" {
		t.Errorf("expected encryption_key dGVzdGtleQ==, got %s", cfg.Cluster.Gossip.EncryptionKey)
	}
}

func TestValidate_ClusterModeRequiresBindAddress(t *testing.T) {
	cfg := &Config{
		DNS:     DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Logging: LoggingConfig{Level: "info", Format: "json"},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
		Cluster: ClusterConfig{
			Mode: ModeCluster,
			// BindAddress intentionally omitted
			Raft: RaftConfig{
				DataDir:          "/var/lib/opengslb/raft",
				HeartbeatTimeout: 1 * time.Second,
				ElectionTimeout:  1 * time.Second,
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for cluster mode without bind_address")
	}
	if !strings.Contains(err.Error(), "bind_address") {
		t.Errorf("expected bind_address error, got: %v", err)
	}
}

func TestValidate_ClusterModeInvalidBindAddress(t *testing.T) {
	cfg := &Config{
		DNS:     DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Logging: LoggingConfig{Level: "info", Format: "json"},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
		Cluster: ClusterConfig{
			Mode:        ModeCluster,
			BindAddress: "not-valid-address",
			Raft: RaftConfig{
				DataDir:          "/var/lib/opengslb/raft",
				HeartbeatTimeout: 1 * time.Second,
				ElectionTimeout:  1 * time.Second,
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid bind_address")
	}
	if !strings.Contains(err.Error(), "bind_address") {
		t.Errorf("expected bind_address error, got: %v", err)
	}
}

func TestValidate_ClusterModeInvalidMode(t *testing.T) {
	cfg := &Config{
		DNS:     DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Logging: LoggingConfig{Level: "info", Format: "json"},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
		Cluster: ClusterConfig{
			Mode: "invalid-mode",
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "cluster.mode") {
		t.Errorf("expected mode error, got: %v", err)
	}
}

func TestValidate_ClusterModeValidConfig(t *testing.T) {
	cfg := &Config{
		DNS:     DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Logging: LoggingConfig{Level: "info", Format: "json"},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
		Cluster: ClusterConfig{
			Mode:        ModeCluster,
			NodeName:    "node-1",
			BindAddress: "10.0.1.10:7946",
			Raft: RaftConfig{
				DataDir:          "/var/lib/opengslb/raft",
				HeartbeatTimeout: 1 * time.Second,
				ElectionTimeout:  1 * time.Second,
			},
		},
	}
	err := Validate(cfg)
	if err != nil {
		t.Errorf("unexpected error for valid cluster config: %v", err)
	}
}

func TestValidate_StandaloneModeSkipsClusterValidation(t *testing.T) {
	cfg := &Config{
		DNS:     DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Logging: LoggingConfig{Level: "info", Format: "json"},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
		Cluster: ClusterConfig{
			Mode: ModeStandalone,
			// No bind_address - should be OK in standalone mode
		},
	}
	err := Validate(cfg)
	if err != nil {
		t.Errorf("standalone mode should not require bind_address: %v", err)
	}
}

func TestClusterConfig_ModeHelpers(t *testing.T) {
	tests := []struct {
		name           string
		mode           RuntimeMode
		wantCluster    bool
		wantStandalone bool
	}{
		{"cluster mode", ModeCluster, true, false},
		{"standalone mode", ModeStandalone, false, true},
		{"empty mode defaults to standalone", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ClusterConfig{Mode: tt.mode}

			if got := cfg.IsClusterMode(); got != tt.wantCluster {
				t.Errorf("IsClusterMode() = %v, want %v", got, tt.wantCluster)
			}
			if got := cfg.IsStandaloneMode(); got != tt.wantStandalone {
				t.Errorf("IsStandaloneMode() = %v, want %v", got, tt.wantStandalone)
			}
		})
	}
}

func TestValidate_RaftElectionTimeoutLessThanHeartbeat(t *testing.T) {
	cfg := &Config{
		DNS:     DNSConfig{ListenAddress: ":53", DefaultTTL: 60},
		Logging: LoggingConfig{Level: "info", Format: "json"},
		Regions: []Region{{
			Name:    "us-east-1",
			Servers: []Server{{Address: "10.0.1.10", Port: 80, Weight: 100}},
			HealthCheck: HealthCheck{
				Type: "http", Interval: 30 * time.Second, Timeout: 5 * time.Second,
				Path: "/health", FailureThreshold: 3, SuccessThreshold: 2,
			},
		}},
		Domains: []Domain{{Name: "app.example.com", RoutingAlgorithm: "round-robin", Regions: []string{"us-east-1"}, TTL: 60}},
		Cluster: ClusterConfig{
			Mode:        ModeCluster,
			BindAddress: "10.0.1.10:7946",
			Raft: RaftConfig{
				DataDir:          "/var/lib/opengslb/raft",
				HeartbeatTimeout: 2 * time.Second,
				ElectionTimeout:  1 * time.Second, // Less than heartbeat - invalid
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for election_timeout < heartbeat_timeout")
	}
	if !strings.Contains(err.Error(), "election_timeout") {
		t.Errorf("expected election_timeout error, got: %v", err)
	}
}
