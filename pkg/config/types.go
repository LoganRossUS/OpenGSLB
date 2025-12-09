// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package config provides configuration loading and validation for OpenGSLB.
package config

import (
	"net"
	"strconv"
	"time"
)

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

	// PredictiveHealth contains predictive health monitoring settings.
	PredictiveHealth PredictiveHealthConfig `yaml:"predictive_health"`

	// AnycastVIP is the virtual IP advertised by all cluster nodes.
	// Only the Raft leader responds to DNS queries on this VIP.
	AnycastVIP string `yaml:"anycast_vip"`

	// Overwatch contains configuration for the leader's health validation.
	Overwatch OverwatchConfig `yaml:"overwatch"`
}

// OverwatchConfig defines settings for the leader's validation of agent health claims.
type OverwatchConfig struct {
	// ExternalCheckInterval is the frequency of leader-initiated external checks.
	// Default: 10s
	ExternalCheckInterval time.Duration `yaml:"external_check_interval"`

	// VetoMode controls how disagreements between agent and external checks are resolved.
	// Options: "strict", "balanced", "permissive"
	// Default: "balanced"
	VetoMode string `yaml:"veto_mode"`

	// VetoThreshold is the number of consecutive external check failures
	// before the overwatch will veto an agent's healthy claim.
	// Default: 3
	VetoThreshold int `yaml:"veto_threshold"`
}

// PredictiveHealthConfig defines predictive health monitoring settings.
// When enabled, the agent monitors local system metrics and signals
// when thresholds are exceeded to enable proactive traffic bleeding.
type PredictiveHealthConfig struct {
	// Enabled controls whether predictive health monitoring is active.
	// Default: false
	Enabled bool `yaml:"enabled"`

	// CPU contains CPU utilization monitoring settings.
	CPU PredictiveMetricConfig `yaml:"cpu"`

	// Memory contains memory utilization monitoring settings.
	Memory PredictiveMetricConfig `yaml:"memory"`

	// ErrorRate contains health check error rate monitoring settings.
	ErrorRate PredictiveErrorRateConfig `yaml:"error_rate"`
}

// PredictiveMetricConfig defines threshold settings for CPU or memory metrics.
type PredictiveMetricConfig struct {
	// Threshold is the percentage (0-100) at which to trigger bleeding.
	// Default: 90 for CPU, 85 for memory
	Threshold float64 `yaml:"threshold"`

	// BleedDuration is the time over which to gradually reduce traffic.
	// Default: 30s
	BleedDuration time.Duration `yaml:"bleed_duration"`
}

// PredictiveErrorRateConfig defines threshold settings for error rate monitoring.
type PredictiveErrorRateConfig struct {
	// Threshold is the error count per minute at which to trigger bleeding.
	// Default: 10
	Threshold float64 `yaml:"threshold"`

	// Window is the time window over which to measure error rate.
	// Default: 60s
	Window time.Duration `yaml:"window"`

	// BleedDuration is the time over which to gradually reduce traffic.
	// Default: 60s
	BleedDuration time.Duration `yaml:"bleed_duration"`
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
	// Enabled controls whether gossip is enabled in cluster mode.
	// Default: true (when in cluster mode)
	Enabled bool `yaml:"enabled"`

	// BindPort is the port for gossip communication.
	// If not set, uses the port from ClusterConfig.BindAddress.
	// Default: 7946
	BindPort int `yaml:"bind_port"`

	// AdvertisePort is the port advertised to other nodes.
	// Defaults to BindPort if not set.
	AdvertisePort int `yaml:"advertise_port"`

	// EncryptionKey is an optional 32-byte base64-encoded encryption key.
	// Generate with: head -c 32 /dev/urandom | base64
	EncryptionKey string `yaml:"encryption_key"`

	// ProbeInterval is the interval between failure probes.
	// Lower values detect failures faster but increase network traffic.
	// Default: 1s
	ProbeInterval time.Duration `yaml:"probe_interval"`

	// ProbeTimeout is the timeout for a single probe.
	// Default: 500ms
	ProbeTimeout time.Duration `yaml:"probe_timeout"`

	// GossipInterval is the interval between gossip messages.
	// Lower values propagate updates faster but increase network traffic.
	// Default: 200ms
	GossipInterval time.Duration `yaml:"gossip_interval"`

	// PushPullInterval is the interval for full state synchronization.
	// Default: 30s
	PushPullInterval time.Duration `yaml:"push_pull_interval"`

	// RetransmitMult controls message retransmission.
	// Higher values improve reliability but increase bandwidth.
	// Default: 4
	RetransmitMult int `yaml:"retransmit_mult"`
}

// IsClusterMode returns true if the configuration is for cluster mode.
func (c *ClusterConfig) IsClusterMode() bool {
	return c.Mode == ModeCluster
}

// IsStandaloneMode returns true if the configuration is for standalone mode.
func (c *ClusterConfig) IsStandaloneMode() bool {
	return c.Mode == ModeStandalone || c.Mode == ""
}

// IsGossipEnabled returns true if gossip should be enabled.
// Gossip is enabled by default in cluster mode unless explicitly disabled.
func (c *ClusterConfig) IsGossipEnabled() bool {
	if c.IsStandaloneMode() {
		return false
	}
	// In cluster mode, gossip is enabled by default
	// It can be explicitly disabled by setting gossip.enabled: false
	return c.Gossip.Enabled || c.Gossip.BindPort > 0 || c.Gossip.EncryptionKey != ""
}

// GetGossipBindPort returns the gossip bind port, with defaults applied.
func (c *ClusterConfig) GetGossipBindPort() int {
	if c.Gossip.BindPort > 0 {
		return c.Gossip.BindPort
	}

	// Try to derive from BindAddress
	if c.BindAddress != "" {
		_, portStr, err := net.SplitHostPort(c.BindAddress)
		if err == nil {
			if port, err := strconv.Atoi(portStr); err == nil {
				return port + 100
			}
		}
	}

	return 7946 // Default memberlist port
}

// GetGossipAdvertisePort returns the gossip advertise port, with defaults applied.
func (c *ClusterConfig) GetGossipAdvertisePort() int {
	if c.Gossip.AdvertisePort > 0 {
		return c.Gossip.AdvertisePort
	}
	return c.GetGossipBindPort()
}
