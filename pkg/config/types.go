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
}

// Server defines a backend server within a region.
type Server struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
	Weight  int    `yaml:"weight"`
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