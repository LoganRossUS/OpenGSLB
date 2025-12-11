// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Default configuration values.
const (
	// DNS defaults
	DefaultListenAddress = ":53"
	DefaultTTL           = 60

	// Server defaults
	DefaultServerPort   = 80
	DefaultServerWeight = 100

	// Health check defaults
	DefaultHealthCheckType  = "http"
	DefaultHealthInterval   = 30 * time.Second
	DefaultHealthTimeout    = 5 * time.Second
	DefaultHealthPath       = "/health"
	DefaultFailureThreshold = 3
	DefaultSuccessThreshold = 2

	// Routing defaults
	DefaultRoutingAlgorithm = "round-robin"

	// Logging defaults
	DefaultLogLevel  = "info"
	DefaultLogFormat = "json"

	// API defaults
	DefaultAPIAddress = "127.0.0.1:8080"

	// Agent defaults (ADR-015)
	DefaultAgentHeartbeatInterval = 10 * time.Second
	DefaultAgentMissedThreshold   = 3
	DefaultAgentCertPath          = "/var/lib/opengslb/agent.crt"
	DefaultAgentKeyPath           = "/var/lib/opengslb/agent.key"

	// Overwatch defaults (ADR-015)
	DefaultOverwatchGossipBindAddress = "0.0.0.0:7946"
	DefaultOverwatchProbeInterval     = 1 * time.Second
	DefaultOverwatchProbeTimeout      = 500 * time.Millisecond
	DefaultOverwatchGossipInterval    = 200 * time.Millisecond
	DefaultValidationCheckInterval    = 30 * time.Second
	DefaultValidationCheckTimeout     = 5 * time.Second
	DefaultStaleThreshold             = 30 * time.Second
	DefaultStaleRemoveAfter           = 5 * time.Minute
	DefaultDNSSECAlgorithm            = "ECDSAP256SHA256"
	DefaultDNSSECKeySyncPollInterval  = 1 * time.Hour
	DefaultDNSSECKeySyncTimeout       = 30 * time.Second

	// Predictive health defaults
	DefaultPredictiveCPUThreshold       = 90.0
	DefaultPredictiveMemoryThreshold    = 85.0
	DefaultPredictiveErrorRateThreshold = 10.0
	DefaultPredictiveBleedDuration      = 30 * time.Second
	DefaultPredictiveErrorWindow        = 60 * time.Second
	DefaultPredictiveCheckInterval      = 10 * time.Second
)

// DefaultAPIAllowedNetworks defines the default networks allowed to access the API.
var DefaultAPIAllowedNetworks = []string{"127.0.0.1/32", "::1/128"}

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s (got: %v)", e.Field, e.Message, e.Value)
}

// Load reads and parses a configuration file from the given path.
func Load(path string) (*Config, error) {
	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return Parse(data)
}

// Parse parses configuration from YAML bytes.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

// Validate validates the configuration (called separately after Parse if needed).
func Validate(cfg *Config) error {
	return cfg.Validate()
}

func applyDefaults(cfg *Config) {
	// Logging defaults (both modes)
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = DefaultLogLevel
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = DefaultLogFormat
	}

	// Mode-specific defaults
	switch cfg.Mode {
	case ModeAgent:
		applyAgentDefaults(cfg)
	case ModeOverwatch, "":
		applyOverwatchDefaults(cfg)
	}
}

func applyAgentDefaults(cfg *Config) {
	// Identity defaults
	if cfg.Agent.Identity.CertPath == "" {
		cfg.Agent.Identity.CertPath = DefaultAgentCertPath
	}
	if cfg.Agent.Identity.KeyPath == "" {
		cfg.Agent.Identity.KeyPath = DefaultAgentKeyPath
	}

	// Heartbeat defaults
	if cfg.Agent.Heartbeat.Interval == 0 {
		cfg.Agent.Heartbeat.Interval = DefaultAgentHeartbeatInterval
	}
	if cfg.Agent.Heartbeat.MissedThreshold == 0 {
		cfg.Agent.Heartbeat.MissedThreshold = DefaultAgentMissedThreshold
	}

	// Backend health check defaults
	for i := range cfg.Agent.Backends {
		applyBackendHealthCheckDefaults(&cfg.Agent.Backends[i].HealthCheck)
		if cfg.Agent.Backends[i].Weight == 0 {
			cfg.Agent.Backends[i].Weight = DefaultServerWeight
		}
	}

	// Predictive health defaults
	if cfg.Agent.Predictive.Enabled {
		applyPredictiveDefaults(&cfg.Agent.Predictive)
	}
}

func applyOverwatchDefaults(cfg *Config) {
	// DNS defaults
	if cfg.DNS.ListenAddress == "" {
		cfg.DNS.ListenAddress = DefaultListenAddress
	}
	if cfg.DNS.DefaultTTL == 0 {
		cfg.DNS.DefaultTTL = DefaultTTL
	}

	// Gossip defaults
	if cfg.Overwatch.Gossip.BindAddress == "" {
		cfg.Overwatch.Gossip.BindAddress = DefaultOverwatchGossipBindAddress
	}
	if cfg.Overwatch.Gossip.ProbeInterval == 0 {
		cfg.Overwatch.Gossip.ProbeInterval = DefaultOverwatchProbeInterval
	}
	if cfg.Overwatch.Gossip.ProbeTimeout == 0 {
		cfg.Overwatch.Gossip.ProbeTimeout = DefaultOverwatchProbeTimeout
	}
	if cfg.Overwatch.Gossip.GossipInterval == 0 {
		cfg.Overwatch.Gossip.GossipInterval = DefaultOverwatchGossipInterval
	}

	// Validation defaults
	if cfg.Overwatch.Validation.CheckInterval == 0 {
		cfg.Overwatch.Validation.CheckInterval = DefaultValidationCheckInterval
	}
	if cfg.Overwatch.Validation.CheckTimeout == 0 {
		cfg.Overwatch.Validation.CheckTimeout = DefaultValidationCheckTimeout
	}

	// Stale defaults
	if cfg.Overwatch.Stale.Threshold == 0 {
		cfg.Overwatch.Stale.Threshold = DefaultStaleThreshold
	}
	if cfg.Overwatch.Stale.RemoveAfter == 0 {
		cfg.Overwatch.Stale.RemoveAfter = DefaultStaleRemoveAfter
	}

	// DNSSEC defaults (enabled by default per ADR-015)
	if cfg.Overwatch.DNSSEC.Algorithm == "" {
		cfg.Overwatch.DNSSEC.Algorithm = DefaultDNSSECAlgorithm
	}
	if cfg.Overwatch.DNSSEC.KeySync.PollInterval == 0 {
		cfg.Overwatch.DNSSEC.KeySync.PollInterval = DefaultDNSSECKeySyncPollInterval
	}
	if cfg.Overwatch.DNSSEC.KeySync.Timeout == 0 {
		cfg.Overwatch.DNSSEC.KeySync.Timeout = DefaultDNSSECKeySyncTimeout
	}

	// API defaults
	applyAPIDefaults(&cfg.API)

	// Region defaults
	for i := range cfg.Regions {
		applyRegionDefaults(&cfg.Regions[i])
	}

	// Domain defaults
	for i := range cfg.Domains {
		applyDomainDefaults(&cfg.Domains[i], cfg.DNS.DefaultTTL)
	}
}

func applyBackendHealthCheckDefaults(hc *HealthCheck) {
	if hc.Type == "" {
		hc.Type = DefaultHealthCheckType
	}
	if hc.Interval == 0 {
		hc.Interval = DefaultHealthInterval
	}
	if hc.Timeout == 0 {
		hc.Timeout = DefaultHealthTimeout
	}
	if hc.Path == "" && (hc.Type == "http" || hc.Type == "https") {
		hc.Path = DefaultHealthPath
	}
	if hc.FailureThreshold == 0 {
		hc.FailureThreshold = DefaultFailureThreshold
	}
	if hc.SuccessThreshold == 0 {
		hc.SuccessThreshold = DefaultSuccessThreshold
	}
}

func applyPredictiveDefaults(ph *PredictiveHealthConfig) {
	if ph.CheckInterval == 0 {
		ph.CheckInterval = DefaultPredictiveCheckInterval
	}
	if ph.CPU.Threshold == 0 {
		ph.CPU.Threshold = DefaultPredictiveCPUThreshold
	}
	if ph.CPU.BleedDuration == 0 {
		ph.CPU.BleedDuration = DefaultPredictiveBleedDuration
	}
	if ph.Memory.Threshold == 0 {
		ph.Memory.Threshold = DefaultPredictiveMemoryThreshold
	}
	if ph.Memory.BleedDuration == 0 {
		ph.Memory.BleedDuration = DefaultPredictiveBleedDuration
	}
	if ph.ErrorRate.Threshold == 0 {
		ph.ErrorRate.Threshold = DefaultPredictiveErrorRateThreshold
	}
	if ph.ErrorRate.Window == 0 {
		ph.ErrorRate.Window = DefaultPredictiveErrorWindow
	}
	if ph.ErrorRate.BleedDuration == 0 {
		ph.ErrorRate.BleedDuration = DefaultPredictiveBleedDuration
	}
}

func applyAPIDefaults(api *APIConfig) {
	if !api.Enabled {
		return
	}
	if api.Address == "" {
		api.Address = DefaultAPIAddress
	}
	if len(api.AllowedNetworks) == 0 {
		api.AllowedNetworks = DefaultAPIAllowedNetworks
	}
}

func applyRegionDefaults(r *Region) {
	for j := range r.Servers {
		if r.Servers[j].Port == 0 {
			r.Servers[j].Port = DefaultServerPort
		}
		if r.Servers[j].Weight == 0 {
			r.Servers[j].Weight = DefaultServerWeight
		}
	}

	if r.HealthCheck.Type == "" {
		r.HealthCheck.Type = DefaultHealthCheckType
	}
	if r.HealthCheck.Interval == 0 {
		r.HealthCheck.Interval = DefaultHealthInterval
	}
	if r.HealthCheck.Timeout == 0 {
		r.HealthCheck.Timeout = DefaultHealthTimeout
	}
	if r.HealthCheck.Path == "" && (r.HealthCheck.Type == "http" || r.HealthCheck.Type == "https") {
		r.HealthCheck.Path = DefaultHealthPath
	}
	if r.HealthCheck.FailureThreshold == 0 {
		r.HealthCheck.FailureThreshold = DefaultFailureThreshold
	}
	if r.HealthCheck.SuccessThreshold == 0 {
		r.HealthCheck.SuccessThreshold = DefaultSuccessThreshold
	}
}

func applyDomainDefaults(d *Domain, defaultTTL int) {
	if d.RoutingAlgorithm == "" {
		d.RoutingAlgorithm = DefaultRoutingAlgorithm
	}
	if d.TTL == 0 {
		d.TTL = defaultTTL
	}
}
