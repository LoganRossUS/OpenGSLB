// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package config provides configuration loading and validation for OpenGSLB.
package config

import (
	"time"
)

// RuntimeMode defines the operational mode of OpenGSLB (ADR-015).
type RuntimeMode string

const (
	// ModeAgent runs OpenGSLB as a health-reporting agent on application servers.
	ModeAgent RuntimeMode = "agent"
	// ModeOverwatch runs OpenGSLB as a DNS-serving, health-validating authority.
	ModeOverwatch RuntimeMode = "overwatch"
)

// Config is the root configuration structure for OpenGSLB.
type Config struct {
	// Mode specifies the runtime mode: "agent" or "overwatch" (ADR-015)
	Mode RuntimeMode `yaml:"mode"`

	// Includes is a list of glob patterns for additional configuration files
	// to merge into this configuration. Patterns are relative to the main config file.
	// Example: ["regions/*.yaml", "domains/**/*.yaml"]
	Includes []string `yaml:"includes,omitempty"`

	// Agent configuration (only used when mode=agent)
	Agent AgentConfig `yaml:"agent"`

	// Overwatch configuration (only used when mode=overwatch)
	Overwatch OverwatchConfig `yaml:"overwatch"`

	// DNS server settings (overwatch mode only)
	DNS DNSConfig `yaml:"dns"`

	// Regions define backend server pools (overwatch mode, or agent can reference)
	Regions []Region `yaml:"regions"`

	// Domains define GSLB-managed zones (overwatch mode only)
	Domains []Domain `yaml:"domains"`

	// Logging settings (both modes)
	Logging LoggingConfig `yaml:"logging"`

	// Metrics settings (both modes)
	Metrics MetricsConfig `yaml:"metrics"`

	// API settings (overwatch mode only)
	API APIConfig `yaml:"api"`
}

// ============================================================================
// Agent Mode Configuration (ADR-015)
// ============================================================================

// AgentConfig defines configuration for agent mode.
type AgentConfig struct {
	// Identity contains agent identification settings
	Identity AgentIdentityConfig `yaml:"identity"`

	// Backends are the services this agent monitors
	Backends []AgentBackend `yaml:"backends"`

	// Predictive contains predictive health monitoring settings
	Predictive PredictiveHealthConfig `yaml:"predictive"`

	// Gossip contains settings for communicating with Overwatch nodes
	Gossip AgentGossipConfig `yaml:"gossip"`

	// Heartbeat contains keepalive settings
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`

	// LatencyLearning contains passive latency learning settings (ADR-017)
	LatencyLearning LatencyLearningConfig `yaml:"latency_learning"`
}

// AgentIdentityConfig defines agent identity settings.
type AgentIdentityConfig struct {
	// ServiceToken is the pre-shared token for initial authentication
	ServiceToken string `yaml:"service_token"`

	// Region is the geographic region this agent belongs to
	Region string `yaml:"region"`

	// CertPath is the path to store/load the agent's certificate
	// If empty, defaults to /var/lib/opengslb/agent.crt
	CertPath string `yaml:"cert_path"`

	// KeyPath is the path to store/load the agent's private key
	// If empty, defaults to /var/lib/opengslb/agent.key
	KeyPath string `yaml:"key_path"`
}

// AgentBackend defines a service backend monitored by this agent.
type AgentBackend struct {
	// Service is the service name (used for DNS domain mapping)
	Service string `yaml:"service"`

	// Address is the backend server IP address
	Address string `yaml:"address"`

	// Port is the backend server port
	Port int `yaml:"port"`

	// Weight is the routing weight (higher = more traffic)
	Weight int `yaml:"weight"`

	// HealthCheck defines how to check this backend's health
	HealthCheck HealthCheck `yaml:"health_check"`
}

// AgentGossipConfig defines gossip settings for agents.
type AgentGossipConfig struct {
	// EncryptionKey is a REQUIRED 32-byte base64-encoded encryption key
	// Generate with: openssl rand -base64 32
	EncryptionKey string `yaml:"encryption_key"`

	// OverwatchNodes is a list of Overwatch gossip addresses to connect to
	// Format: "host:port" (e.g., "overwatch-1.internal:7946")
	OverwatchNodes []string `yaml:"overwatch_nodes"`
}

// HeartbeatConfig defines agent heartbeat settings.
type HeartbeatConfig struct {
	// Interval is the time between heartbeat messages
	// Default: 10s
	Interval time.Duration `yaml:"interval"`

	// MissedThreshold is the number of missed heartbeats before deregistration
	// Default: 3
	MissedThreshold int `yaml:"missed_threshold"`
}

// ============================================================================
// Overwatch Mode Configuration (ADR-015)
// ============================================================================

// OverwatchConfig defines configuration for overwatch mode.
type OverwatchConfig struct {
	// Identity contains overwatch node identification settings
	Identity OverwatchIdentityConfig `yaml:"identity"`

	// AgentTokens maps service names to their authentication tokens
	AgentTokens map[string]string `yaml:"agent_tokens"`

	// Gossip contains settings for receiving agent gossip
	Gossip OverwatchGossipConfig `yaml:"gossip"`

	// Validation contains external health validation settings
	Validation ValidationConfig `yaml:"validation"`

	// Stale contains settings for detecting stale backends
	Stale StaleConfig `yaml:"stale"`

	// DNSSEC contains DNSSEC signing settings
	DNSSEC DNSSECConfig `yaml:"dnssec"`

	// Geolocation contains geolocation routing settings
	Geolocation GeolocationConfig `yaml:"geolocation"`

	// DataDir is the directory for persistent data (bbolt database)
	// Default: /var/lib/opengslb
	DataDir string `yaml:"data_dir"`
}

// GeolocationConfig defines geolocation routing settings.
type GeolocationConfig struct {
	// DatabasePath is the path to the MaxMind GeoLite2-Country database
	// Required for geolocation routing to work
	DatabasePath string `yaml:"database_path"`

	// DefaultRegion is the fallback region when geo lookup fails
	// Required
	DefaultRegion string `yaml:"default_region"`

	// ECSEnabled enables parsing of EDNS Client Subnet for more accurate geo
	// Default: true
	ECSEnabled bool `yaml:"ecs_enabled"`

	// CustomMappings define CIDR-to-region mappings (evaluated BEFORE GeoIP)
	CustomMappings []CustomMapping `yaml:"custom_mappings"`
}

// CustomMapping defines a CIDR-to-region mapping for custom geolocation rules.
type CustomMapping struct {
	// CIDR is the IP range in CIDR notation (e.g., "10.1.0.0/16")
	CIDR string `yaml:"cidr"`

	// Region is the target region for this CIDR
	Region string `yaml:"region"`

	// Comment is an optional description
	Comment string `yaml:"comment,omitempty"`

	// Source indicates where this mapping came from ("config" or "api")
	// This is set automatically, not from config
	Source string `yaml:"-"`
}

// OverwatchIdentityConfig defines overwatch node identity settings.
type OverwatchIdentityConfig struct {
	// NodeID is a unique identifier for this overwatch node
	// Defaults to hostname if not specified
	NodeID string `yaml:"node_id"`

	// Region is the geographic region this overwatch node serves
	Region string `yaml:"region"`
}

// OverwatchGossipConfig defines gossip settings for overwatch nodes.
type OverwatchGossipConfig struct {
	// BindAddress is the address to listen for gossip (host:port)
	// Default: "0.0.0.0:7946"
	BindAddress string `yaml:"bind_address"`

	// EncryptionKey is a REQUIRED 32-byte base64-encoded encryption key
	// Must match the key used by agents
	EncryptionKey string `yaml:"encryption_key"`

	// ProbeInterval is the interval between failure probes
	// Default: 1s
	ProbeInterval time.Duration `yaml:"probe_interval"`

	// ProbeTimeout is the timeout for a single probe
	// Default: 500ms
	ProbeTimeout time.Duration `yaml:"probe_timeout"`

	// GossipInterval is the interval between gossip messages
	// Default: 200ms
	GossipInterval time.Duration `yaml:"gossip_interval"`
}

// ValidationConfig defines external health validation settings.
type ValidationConfig struct {
	// Enabled controls whether external validation is active
	// Default: true
	Enabled bool `yaml:"enabled"`

	// CheckInterval is the frequency of validation checks
	// Default: 30s
	CheckInterval time.Duration `yaml:"check_interval"`

	// CheckTimeout is the timeout for validation checks
	// Default: 5s
	CheckTimeout time.Duration `yaml:"check_timeout"`
}

// StaleConfig defines settings for detecting stale backends.
type StaleConfig struct {
	// Threshold is the time after which a backend with no heartbeat is stale
	// Default: 30s
	Threshold time.Duration `yaml:"threshold"`

	// RemoveAfter is the time after which a stale backend is removed
	// Default: 5m
	RemoveAfter time.Duration `yaml:"remove_after"`
}

// DNSSECConfig defines DNSSEC signing settings.
type DNSSECConfig struct {
	// Enabled controls whether DNSSEC is active
	// Default: true (secure by default)
	Enabled bool `yaml:"enabled"`

	// SecurityAcknowledgment is REQUIRED if Enabled=false
	// Must contain specific text acknowledging security implications
	SecurityAcknowledgment string `yaml:"security_acknowledgment"`

	// Algorithm is the DNSSEC signing algorithm
	// Default: ECDSAP256SHA256
	Algorithm string `yaml:"algorithm"`

	// KeySync contains settings for syncing keys between Overwatch nodes
	KeySync DNSSECKeySyncConfig `yaml:"key_sync"`
}

// DNSSECKeySyncConfig defines DNSSEC key synchronization settings.
type DNSSECKeySyncConfig struct {
	// Peers are the API addresses of other Overwatch nodes for key sync
	Peers []string `yaml:"peers"`

	// PollInterval is the time between key sync polls
	// Default: 1h
	PollInterval time.Duration `yaml:"poll_interval"`

	// Timeout is the timeout for key sync requests
	// Default: 30s
	Timeout time.Duration `yaml:"timeout"`
}

// ============================================================================
// Shared Configuration (both modes)
// ============================================================================

// DNSConfig defines the DNS server settings (overwatch mode).
type DNSConfig struct {
	ListenAddress     string   `yaml:"listen_address"`
	DefaultTTL        int      `yaml:"default_ttl"`
	ReturnLastHealthy bool     `yaml:"return_last_healthy"`
	Zones             []string `yaml:"zones"`
}

// Region defines a geographic region with its servers and health check configuration.
type Region struct {
	Name        string      `yaml:"name"`
	Servers     []Server    `yaml:"servers"`
	HealthCheck HealthCheck `yaml:"health_check"`

	// Countries is a list of ISO 3166-1 alpha-2 country codes for geolocation routing
	// e.g., ["US", "CA", "MX"]
	Countries []string `yaml:"countries,omitempty"`

	// Continents is a list of continent codes for geolocation routing
	// Valid codes: AF (Africa), AN (Antarctica), AS (Asia), EU (Europe),
	// NA (North America), OC (Oceania), SA (South America)
	Continents []string `yaml:"continents,omitempty"`
}

// Server defines a backend server within a region.
type Server struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
	Weight  int    `yaml:"weight"`
	Service string `yaml:"service"` // Required in v1.1.0: Domain/service this server belongs to
	Host    string `yaml:"host"`
}

// HealthCheck defines health check configuration.
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
	Name             string         `yaml:"name"`
	RoutingAlgorithm string         `yaml:"routing_algorithm"`
	Regions          []string       `yaml:"regions"`
	TTL              int            `yaml:"ttl"`
	LatencyConfig    *LatencyConfig `yaml:"latency_config,omitempty"`
}

// LatencyConfig defines configuration for latency-based routing.
type LatencyConfig struct {
	// SmoothingFactor is the EMA alpha (0-1), higher = more responsive
	// Default: 0.3
	SmoothingFactor float64 `yaml:"smoothing_factor"`
	// MaxLatencyMs excludes servers above this threshold
	// Default: 500
	MaxLatencyMs int `yaml:"max_latency_ms"`
	// MinSamples is the minimum number of samples before using latency data
	// Default: 3
	MinSamples int `yaml:"min_samples"`
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

// APIConfig defines the HTTP API server settings (overwatch mode).
type APIConfig struct {
	Enabled           bool     `yaml:"enabled"`
	Address           string   `yaml:"address"`
	AllowedNetworks   []string `yaml:"allowed_networks"`
	TrustProxyHeaders bool     `yaml:"trust_proxy_headers"`
}

// PredictiveHealthConfig defines predictive health monitoring settings.
type PredictiveHealthConfig struct {
	Enabled       bool                      `yaml:"enabled"`
	CPU           PredictiveMetricConfig    `yaml:"cpu"`
	Memory        PredictiveMetricConfig    `yaml:"memory"`
	ErrorRate     PredictiveErrorRateConfig `yaml:"error_rate"`
	CheckInterval time.Duration             `yaml:"check_interval"`
}

// PredictiveMetricConfig defines threshold settings for CPU or memory metrics.
type PredictiveMetricConfig struct {
	Threshold     float64       `yaml:"threshold"`
	BleedDuration time.Duration `yaml:"bleed_duration"`
}

// PredictiveErrorRateConfig defines threshold settings for error rate monitoring.
type PredictiveErrorRateConfig struct {
	Threshold     float64       `yaml:"threshold"`
	Window        time.Duration `yaml:"window"`
	BleedDuration time.Duration `yaml:"bleed_duration"`
}

// LatencyLearningConfig defines passive latency learning settings (ADR-017).
// Agents learn client-to-backend latency by reading TCP RTT data from the OS.
type LatencyLearningConfig struct {
	// Enabled controls whether latency learning is active.
	// Default: false
	Enabled bool `yaml:"enabled"`

	// PollInterval is how often to poll the OS for TCP connection data.
	// Default: 10s
	PollInterval time.Duration `yaml:"poll_interval"`

	// MinConnectionAge is the minimum connection age before collecting RTT.
	// Newly established connections have unstable RTT measurements.
	// Default: 5s
	MinConnectionAge time.Duration `yaml:"min_connection_age"`

	// IPv4Prefix is the prefix length for IPv4 subnet aggregation.
	// Default: 24 (e.g., /24 subnets)
	IPv4Prefix int `yaml:"ipv4_prefix"`

	// IPv6Prefix is the prefix length for IPv6 subnet aggregation.
	// Default: 48 (e.g., /48 subnets)
	IPv6Prefix int `yaml:"ipv6_prefix"`

	// EWMAAlpha is the smoothing factor for EWMA calculation (0-1).
	// Higher values give more weight to recent samples.
	// Default: 0.3
	EWMAAlpha float64 `yaml:"ewma_alpha"`

	// MaxSubnets is the maximum number of subnets to track.
	// Prevents unbounded memory growth.
	// Default: 100000
	MaxSubnets int `yaml:"max_subnets"`

	// SubnetTTL is how long to keep subnet entries without updates.
	// Default: 168h (7 days)
	SubnetTTL time.Duration `yaml:"subnet_ttl"`

	// MinSamples is the minimum samples before reporting a subnet.
	// Default: 5
	MinSamples int `yaml:"min_samples"`

	// ReportInterval is how often to send latency reports to overwatches.
	// Default: 30s
	ReportInterval time.Duration `yaml:"report_interval"`
}

// ============================================================================
// Helper Methods
// ============================================================================

// IsAgentMode returns true if the configuration is for agent mode.
func (c *Config) IsAgentMode() bool {
	return c.Mode == ModeAgent
}

// IsOverwatchMode returns true if the configuration is for overwatch mode.
func (c *Config) IsOverwatchMode() bool {
	return c.Mode == ModeOverwatch || c.Mode == ""
}

// GetEffectiveMode returns the runtime mode, defaulting to overwatch.
func (c *Config) GetEffectiveMode() RuntimeMode {
	if c.Mode == "" {
		return ModeOverwatch
	}
	return c.Mode
}
