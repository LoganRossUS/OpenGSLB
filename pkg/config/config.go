package config

import (
	"fmt"
	"os"
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
)

// Load reads and parses a configuration file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
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

	for i := range cfg.Regions {
		applyRegionDefaults(&cfg.Regions[i])
	}

	for i := range cfg.Domains {
		applyDomainDefaults(&cfg.Domains[i], cfg.DNS.DefaultTTL)
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
