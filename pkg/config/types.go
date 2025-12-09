// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package config provides configuration loading and validation for OpenGSLB.
package config

import "time"

// RuntimeMode defines the operational mode of OpenGSLB.
type RuntimeMode string

const (
	// ModeStandalone runs OpenGSLB as a single node (default).
	ModeStandalone RuntimeMode = "standalone"
	// ModeCluster runs OpenGSLB as part of a distributed cluster.
	ModeCluster RuntimeMode = "cluster"
)

// Config is the root configuration structure for OpenGSLB.
type Config struct {
	DNS     DNSConfig     `yaml:"dns"`
	Regions []Region      `yaml:"regions"`
	Domains []Domain      `yaml:"domains"`
	Logging LoggingConfig `yaml:"logging"`
	Metrics MetricsConfig `yaml:"metrics"`
	API     APIConfig     `yaml:"api"`
	Cluster ClusterConfig `yaml:"cluster"`
}

// DNSConfig defines the DNS server settings.
type DNSConfig struct {
	ListenAddress     string `yaml:"listen_address"`
	DefaultTTL        int    `yaml:"default_ttl"`
	ReturnLastHealthy bool   `yaml:"return_last_healthy"`
}

// Region defines a geographic region with its servers and health check configuration.
type Region struct {
	Name        string      `yaml:"name"`
	Servers     []Server    `yaml:"servers"`
	HealthCheck HealthCheck `yaml:"health_check"`
}

// Server defines a backend server within a region.
type Server struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
	Weight  int    `yaml:"weight"`
	Host    string `yaml:"host"`
}

// HealthCheck defines health check configuration for a region.
type HealthCheck struct {
	Type             string        `yaml:"type"`
	Interval         time.Duration `yaml:"interval"`
	Timeout          time.Duration `yaml:"timeout"`
	Path             string        `yaml:"path"`
	Host             string        `yaml:"host"`
	FailureThreshold int           `yaml:"failure_threshold"`
	SuccessThreshold int           `yaml:"success_threshold"`
}

// Domain defines a domain and its routing configuration.
type Domain struct {
	Name             string   `yaml:"name"`
	RoutingAlgorithm string   `yaml:"routing_algorithm"`
	Regions          []string `yaml:"regions"`
	TTL              int      `yaml:"ttl"`
}

// LoggingConfig defines logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// MetricsConfig defines Prometheus metrics settings.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Address string `yaml:"address"`
}

// APIConfig defines the HTTP API server settings.
type APIConfig struct {
	Enabled           bool     `yaml:"enabled"`
	Address           string   `yaml:"address"`
	AllowedNetworks   []string `yaml:"allowed_networks"`
	TrustProxyHeaders bool     `yaml:"trust_proxy_headers"`
}

// ClusterConfig defines distributed cluster settings (ADR-012, ADR-014).
type ClusterConfig struct {
	// Mode specifies the runtime mode: "standalone" or "cluster".
	// Can be overridden by --mode flag. Default: "standalone"
	Mode RuntimeMode `yaml:"mode"`

	// NodeName is a unique identifier for this node. Defaults to hostname.
	NodeName string `yaml:"node_name"`

	// BindAddress is the address for Raft and gossip communication.
	// Format: "ip:port" (e.g., "10.0.1.10:7946")
	BindAddress string `yaml:"bind_address"`

	// AdvertiseAddress is the address other nodes use to reach this node.
	// Defaults to BindAddress if not set.
	AdvertiseAddress string `yaml:"advertise_address"`

	// Bootstrap indicates this node should initialize a new cluster.
	// Set on exactly one node during initial cluster formation.
	// Can be overridden by --bootstrap flag.
	Bootstrap bool `yaml:"bootstrap"`

	// Join specifies addresses of existing cluster nodes to join.
	// Can be overridden by --join flag.
	Join []string `yaml:"join"`

	// Raft contains Raft consensus settings.
	Raft RaftConfig `yaml:"raft"`

	// Gossip contains memberlist gossip settings.
	Gossip GossipConfig `yaml:"gossip"`

	// AnycastVIP is the virtual IP advertised by all cluster nodes.
	// Only the Raft leader responds to DNS queries on this VIP.
	AnycastVIP string `yaml:"anycast_vip"`
}

// RaftConfig defines Raft consensus settings.
type RaftConfig struct {
	// DataDir is the directory for Raft state and logs.
	// Default: "/var/lib/opengslb/raft"
	DataDir string `yaml:"data_dir"`

	// HeartbeatTimeout is the time between heartbeats.
	// Default: 1s
	HeartbeatTimeout time.Duration `yaml:"heartbeat_timeout"`

	// ElectionTimeout is the time before a new election starts.
	// Default: 1s
	ElectionTimeout time.Duration `yaml:"election_timeout"`

	// SnapshotInterval is the minimum time between snapshots.
	// Default: 120s
	SnapshotInterval time.Duration `yaml:"snapshot_interval"`

	// SnapshotThreshold is the number of log entries before snapshot.
	// Default: 8192
	SnapshotThreshold uint64 `yaml:"snapshot_threshold"`
}

// GossipConfig defines memberlist gossip settings.
type GossipConfig struct {
	// EncryptionKey is an optional 32-byte base64-encoded encryption key.
	EncryptionKey string `yaml:"encryption_key"`
}

// IsClusterMode returns true if the configuration is for cluster mode.
func (c *ClusterConfig) IsClusterMode() bool {
	return c.Mode == ModeCluster
}

// IsStandaloneMode returns true if the configuration is for standalone mode.
func (c *ClusterConfig) IsStandaloneMode() bool {
	return c.Mode == ModeStandalone || c.Mode == ""
}
