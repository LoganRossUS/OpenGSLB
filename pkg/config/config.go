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
	DefaultListenAddress    = ":53"
	DefaultTTL              = 60
	DefaultServerPort       = 80
	DefaultServerWeight     = 100
	DefaultHealthCheckType  = "http"
	DefaultHealthInterval   = 30 * time.Second
	DefaultHealthTimeout    = 5 * time.Second
	DefaultHealthPath       = "/health"
	DefaultFailureThreshold = 3
	DefaultSuccessThreshold = 2
	DefaultRoutingAlgorithm = "round-robin"
	DefaultLogLevel         = "info"
	DefaultLogFormat        = "json"
	DefaultAPIAddress       = "127.0.0.1:8080"

	// Cluster defaults
	DefaultClusterMode           = ModeStandalone
	DefaultRaftDataDir           = "/var/lib/opengslb/raft"
	DefaultRaftHeartbeatTimeout  = 1 * time.Second
	DefaultRaftElectionTimeout   = 1 * time.Second
	DefaultRaftSnapshotInterval  = 120 * time.Second
	DefaultRaftSnapshotThreshold = 8192

	// Predictive health defaults
	DefaultPredictiveCPUThreshold       = 90.0
	DefaultPredictiveMemoryThreshold    = 85.0
	DefaultPredictiveErrorRateThreshold = 10.0
	DefaultPredictiveBleedDuration      = 30 * time.Second
	DefaultPredictiveErrorWindow        = 60 * time.Second
	DefaultPredictiveErrorBleedDuration = 60 * time.Second

	// Overwatch defaults
	DefaultOverwatchCheckInterval = 10 * time.Second
	DefaultOverwatchVetoMode      = "balanced"
)

// DefaultAPIAllowedNetworks defines the default networks allowed to access the API.
// By default, only localhost is permitted.
var DefaultAPIAllowedNetworks = []string{"127.0.0.1/32", "::1/128"}

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

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.DNS.ListenAddress == "" {
		cfg.DNS.ListenAddress = DefaultListenAddress
	}
	if cfg.DNS.DefaultTTL == 0 {
		cfg.DNS.DefaultTTL = DefaultTTL
	}

	if cfg.Logging.Level == "" {
		cfg.Logging.Level = DefaultLogLevel
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = DefaultLogFormat
	}

	applyAPIDefaults(&cfg.API)
	applyClusterDefaults(&cfg.Cluster)

	for i := range cfg.Regions {
		applyRegionDefaults(&cfg.Regions[i])
	}

	for i := range cfg.Domains {
		applyDomainDefaults(&cfg.Domains[i], cfg.DNS.DefaultTTL)
	}
}

func applyAPIDefaults(api *APIConfig) {
	// Only apply defaults if API is enabled
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

func applyClusterDefaults(cluster *ClusterConfig) {
	// Mode defaults to standalone
	if cluster.Mode == "" {
		cluster.Mode = DefaultClusterMode
	}

	// NodeName defaults to hostname (applied at runtime if still empty)
	// We don't set it here to allow validation to detect missing node_name in cluster mode

	// AdvertiseAddress defaults to BindAddress
	if cluster.AdvertiseAddress == "" && cluster.BindAddress != "" {
		cluster.AdvertiseAddress = cluster.BindAddress
	}

	// Raft defaults (only matter in cluster mode, but set anyway)
	if cluster.Raft.DataDir == "" {
		cluster.Raft.DataDir = DefaultRaftDataDir
	}
	if cluster.Raft.HeartbeatTimeout == 0 {
		cluster.Raft.HeartbeatTimeout = DefaultRaftHeartbeatTimeout
	}
	if cluster.Raft.ElectionTimeout == 0 {
		cluster.Raft.ElectionTimeout = DefaultRaftElectionTimeout
	}
	if cluster.Raft.SnapshotInterval == 0 {
		cluster.Raft.SnapshotInterval = DefaultRaftSnapshotInterval
	}
	if cluster.Raft.SnapshotThreshold == 0 {
		cluster.Raft.SnapshotThreshold = DefaultRaftSnapshotThreshold
	}

	// Predictive health defaults (only apply if enabled)
	if cluster.PredictiveHealth.Enabled {
		applyPredictiveHealthDefaults(&cluster.PredictiveHealth)
	}

	applyOverwatchDefaults(&cluster.Overwatch)
}

func applyOverwatchDefaults(o *OverwatchConfig) {
	if o.ExternalCheckInterval == 0 {
		o.ExternalCheckInterval = DefaultOverwatchCheckInterval
	}
	if o.VetoMode == "" {
		o.VetoMode = DefaultOverwatchVetoMode
	}
}

func applyPredictiveHealthDefaults(ph *PredictiveHealthConfig) {
	// CPU defaults
	if ph.CPU.Threshold == 0 {
		ph.CPU.Threshold = DefaultPredictiveCPUThreshold
	}
	if ph.CPU.BleedDuration == 0 {
		ph.CPU.BleedDuration = DefaultPredictiveBleedDuration
	}

	// Memory defaults
	if ph.Memory.Threshold == 0 {
		ph.Memory.Threshold = DefaultPredictiveMemoryThreshold
	}
	if ph.Memory.BleedDuration == 0 {
		ph.Memory.BleedDuration = DefaultPredictiveBleedDuration
	}

	// Error rate defaults
	if ph.ErrorRate.Threshold == 0 {
		ph.ErrorRate.Threshold = DefaultPredictiveErrorRateThreshold
	}
	if ph.ErrorRate.Window == 0 {
		ph.ErrorRate.Window = DefaultPredictiveErrorWindow
	}
	if ph.ErrorRate.BleedDuration == 0 {
		ph.ErrorRate.BleedDuration = DefaultPredictiveErrorBleedDuration
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
	if r.HealthCheck.Path == "" && r.HealthCheck.Type == "http" {
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
