// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"
)

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	// Mode validation handled separately by main.go after flag override
	// Here we validate the content assuming mode is set

	// Validate shared sections
	if err := c.validateLogging(); err != nil {
		return fmt.Errorf("logging: %w", err)
	}

	if err := c.validateMetrics(); err != nil {
		return fmt.Errorf("metrics: %w", err)
	}

	// Mode-specific validation
	switch c.Mode {
	case ModeAgent:
		if err := c.validateAgentMode(); err != nil {
			return fmt.Errorf("agent: %w", err)
		}
	case ModeOverwatch:
		if err := c.validateOverwatchMode(); err != nil {
			return err
		}
	case "":
		// Mode not set in config file - will be defaulted later
		// Still validate overwatch-mode sections if present
		if len(c.Regions) > 0 || len(c.Domains) > 0 {
			if err := c.validateOverwatchMode(); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid mode %q: must be 'agent' or 'overwatch'", c.Mode)
	}

	return nil
}

// validateLogging validates logging configuration.
func (c *Config) validateLogging() error {
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "": true,
	}
	if !validLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("invalid level %q: must be debug, info, warn, or error", c.Logging.Level)
	}

	validFormats := map[string]bool{
		"text": true, "json": true, "": true,
	}
	if !validFormats[strings.ToLower(c.Logging.Format)] {
		return fmt.Errorf("invalid format %q: must be text or json", c.Logging.Format)
	}

	return nil
}

// validateMetrics validates metrics configuration.
func (c *Config) validateMetrics() error {
	if c.Metrics.Enabled && c.Metrics.Address != "" {
		if _, _, err := net.SplitHostPort(c.Metrics.Address); err != nil {
			// Try adding default host
			if _, _, err := net.SplitHostPort("0.0.0.0" + c.Metrics.Address); err != nil {
				return fmt.Errorf("invalid address %q: %w", c.Metrics.Address, err)
			}
		}
	}
	return nil
}

// validateAgentMode validates agent-specific configuration.
func (c *Config) validateAgentMode() error {
	// Identity validation
	if c.Agent.Identity.ServiceToken == "" {
		return fmt.Errorf("identity.service_token is required")
	}
	if len(c.Agent.Identity.ServiceToken) < 16 {
		return fmt.Errorf("identity.service_token must be at least 16 characters")
	}

	// Backends validation
	if len(c.Agent.Backends) == 0 {
		return fmt.Errorf("at least one backend is required")
	}
	for i, backend := range c.Agent.Backends {
		if err := validateAgentBackend(backend, i); err != nil {
			return err
		}
	}

	// Gossip validation (mandatory encryption)
	if err := validateGossipEncryptionKey(c.Agent.Gossip.EncryptionKey); err != nil {
		return fmt.Errorf("gossip: %w", err)
	}
	if len(c.Agent.Gossip.OverwatchNodes) == 0 {
		return fmt.Errorf("gossip.overwatch_nodes must have at least one address")
	}
	for i, node := range c.Agent.Gossip.OverwatchNodes {
		if _, _, err := net.SplitHostPort(node); err != nil {
			return fmt.Errorf("gossip.overwatch_nodes[%d] %q: invalid address: %w", i, node, err)
		}
	}

	// Heartbeat validation
	if c.Agent.Heartbeat.Interval > 0 && c.Agent.Heartbeat.Interval < time.Second {
		return fmt.Errorf("heartbeat.interval must be at least 1s")
	}

	return nil
}

// validateAgentBackend validates a single agent backend configuration.
func validateAgentBackend(b AgentBackend, index int) error {
	prefix := fmt.Sprintf("backends[%d]", index)

	if b.Service == "" {
		return fmt.Errorf("%s.service is required", prefix)
	}
	if b.Address == "" {
		return fmt.Errorf("%s.address is required", prefix)
	}
	if net.ParseIP(b.Address) == nil {
		// Try to resolve hostname
		if _, err := net.LookupHost(b.Address); err != nil {
			return fmt.Errorf("%s.address %q: invalid IP or hostname", prefix, b.Address)
		}
	}
	if b.Port <= 0 || b.Port > 65535 {
		return fmt.Errorf("%s.port must be between 1 and 65535", prefix)
	}
	if b.Weight < 0 {
		return fmt.Errorf("%s.weight must be non-negative", prefix)
	}

	// Health check validation
	hc := b.HealthCheck
	validTypes := map[string]bool{"http": true, "https": true, "tcp": true, "": true}
	if !validTypes[strings.ToLower(hc.Type)] {
		return fmt.Errorf("%s.health_check.type %q: must be http, https, or tcp", prefix, hc.Type)
	}
	if hc.Interval > 0 && hc.Interval < time.Second {
		return fmt.Errorf("%s.health_check.interval must be at least 1s", prefix)
	}
	if hc.Timeout > 0 && hc.Timeout < 100*time.Millisecond {
		return fmt.Errorf("%s.health_check.timeout must be at least 100ms", prefix)
	}

	return nil
}

// validateOverwatchMode validates overwatch-specific configuration.
func (c *Config) validateOverwatchMode() error {
	// Gossip validation (mandatory encryption)
	if err := validateGossipEncryptionKey(c.Overwatch.Gossip.EncryptionKey); err != nil {
		return fmt.Errorf("overwatch.gossip: %w", err)
	}

	// DNSSEC validation
	if !c.Overwatch.DNSSEC.Enabled {
		expectedAck := "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses and allows DNS spoofing attacks against my zones"
		if c.Overwatch.DNSSEC.SecurityAcknowledgment != expectedAck {
			return fmt.Errorf("dnssec: to disable DNSSEC, security_acknowledgment must be set to: %q", expectedAck)
		}
	}

	// DNS validation
	if err := c.validateDNS(); err != nil {
		return fmt.Errorf("dns: %w", err)
	}

	// Regions validation
	if err := c.validateRegions(); err != nil {
		return err
	}

	// Domains validation
	if err := c.validateDomains(); err != nil {
		return err
	}

	// Geolocation validation (only if any domain uses geolocation)
	if err := c.validateGeolocation(); err != nil {
		return fmt.Errorf("geolocation: %w", err)
	}

	// API validation
	if err := c.validateAPI(); err != nil {
		return fmt.Errorf("api: %w", err)
	}

	return nil
}

// validateGossipEncryptionKey validates the gossip encryption key.
// ADR-015: Encryption is MANDATORY - no opt-out.
// AES-256 requires exactly 32 bytes (256 bits).
func validateGossipEncryptionKey(key string) error {
	if key == "" {
		return fmt.Errorf("encryption_key is required. OpenGSLB requires encrypted gossip communication.\n       Generate a key with: openssl rand -base64 32")
	}

	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return fmt.Errorf("encryption_key must be valid base64: %w", err)
	}

	if len(decoded) != 32 {
		return fmt.Errorf("encryption_key must be exactly 32 bytes (got %d).\n       Ensure you're using a 256-bit key. Generate with: openssl rand -base64 32", len(decoded))
	}

	return nil
}

// validateDNS validates DNS configuration.
func (c *Config) validateDNS() error {
	if c.DNS.ListenAddress != "" {
		if _, _, err := net.SplitHostPort(c.DNS.ListenAddress); err != nil {
			return fmt.Errorf("invalid listen_address %q: %w", c.DNS.ListenAddress, err)
		}
	}

	if c.DNS.DefaultTTL < 0 {
		return fmt.Errorf("default_ttl must be non-negative")
	}

	return nil
}

// validateRegions validates region configurations.
func (c *Config) validateRegions() error {
	regionNames := make(map[string]bool)

	for i, region := range c.Regions {
		prefix := fmt.Sprintf("regions[%d]", i)

		if region.Name == "" {
			return fmt.Errorf("%s.name is required", prefix)
		}
		if regionNames[region.Name] {
			return fmt.Errorf("%s: duplicate region name %q", prefix, region.Name)
		}
		regionNames[region.Name] = true

		if len(region.Servers) == 0 {
			return fmt.Errorf("%s: at least one server required", prefix)
		}

		for j, server := range region.Servers {
			serverPrefix := fmt.Sprintf("%s.servers[%d]", prefix, j)
			if server.Address == "" {
				return fmt.Errorf("%s.address is required", serverPrefix)
			}
			if server.Port <= 0 || server.Port > 65535 {
				return fmt.Errorf("%s.port must be between 1 and 65535", serverPrefix)
			}
		}

		// Health check validation
		hc := region.HealthCheck
		validTypes := map[string]bool{"http": true, "https": true, "tcp": true, "": true}
		if !validTypes[strings.ToLower(hc.Type)] {
			return fmt.Errorf("%s.health_check.type %q: must be http, https, or tcp", prefix, hc.Type)
		}
	}

	return nil
}

// validateDomains validates domain configurations.
func (c *Config) validateDomains() error {
	// Build region name set for validation
	regionNames := make(map[string]bool)
	for _, region := range c.Regions {
		regionNames[region.Name] = true
	}

	domainNames := make(map[string]bool)

	for i, domain := range c.Domains {
		prefix := fmt.Sprintf("domains[%d]", i)

		if domain.Name == "" {
			return fmt.Errorf("%s.name is required", prefix)
		}
		if domainNames[domain.Name] {
			return fmt.Errorf("%s: duplicate domain name %q", prefix, domain.Name)
		}
		domainNames[domain.Name] = true

		// Validate routing algorithm
		validAlgorithms := map[string]bool{
			"round-robin": true, "weighted": true, "failover": true,
			"geolocation": true, "latency": true, "": true,
		}
		if !validAlgorithms[strings.ToLower(domain.RoutingAlgorithm)] {
			return fmt.Errorf("%s.routing_algorithm %q: must be round-robin, weighted, failover, geolocation, or latency",
				prefix, domain.RoutingAlgorithm)
		}

		// Validate regions exist
		if len(domain.Regions) == 0 {
			return fmt.Errorf("%s: at least one region required", prefix)
		}
		for _, regionName := range domain.Regions {
			if !regionNames[regionName] {
				return fmt.Errorf("%s: region %q not found", prefix, regionName)
			}
		}
	}

	return nil
}

// validateAPI validates API configuration.
func (c *Config) validateAPI() error {
	if !c.API.Enabled {
		return nil
	}

	if c.API.Address != "" {
		if _, _, err := net.SplitHostPort(c.API.Address); err != nil {
			return fmt.Errorf("invalid address %q: %w", c.API.Address, err)
		}
	}

	for i, network := range c.API.AllowedNetworks {
		if _, _, err := net.ParseCIDR(network); err != nil {
			return fmt.Errorf("allowed_networks[%d] %q: invalid CIDR: %w", i, network, err)
		}
	}

	return nil
}

// validateGeolocation validates geolocation configuration.
// Only validates if any domain uses geolocation routing.
func (c *Config) validateGeolocation() error {
	// Check if any domain uses geolocation routing
	usesGeo := false
	for _, domain := range c.Domains {
		if strings.ToLower(domain.RoutingAlgorithm) == "geolocation" {
			usesGeo = true
			break
		}
	}

	if !usesGeo {
		return nil
	}

	geo := c.Overwatch.Geolocation

	// Database path is required for geolocation routing
	if geo.DatabasePath == "" {
		return fmt.Errorf("database_path is required when using geolocation routing")
	}

	// Default region is required
	if geo.DefaultRegion == "" {
		return fmt.Errorf("default_region is required")
	}

	// Validate default region exists
	regionExists := false
	for _, region := range c.Regions {
		if region.Name == geo.DefaultRegion {
			regionExists = true
			break
		}
	}
	if !regionExists {
		return fmt.Errorf("default_region %q does not exist in regions", geo.DefaultRegion)
	}

	// Validate custom mappings
	for i, mapping := range geo.CustomMappings {
		prefix := fmt.Sprintf("custom_mappings[%d]", i)

		if mapping.CIDR == "" {
			return fmt.Errorf("%s.cidr is required", prefix)
		}
		if _, _, err := net.ParseCIDR(mapping.CIDR); err != nil {
			return fmt.Errorf("%s.cidr %q: invalid CIDR: %w", prefix, mapping.CIDR, err)
		}
		if mapping.Region == "" {
			return fmt.Errorf("%s.region is required", prefix)
		}

		// Validate region exists
		mappingRegionExists := false
		for _, region := range c.Regions {
			if region.Name == mapping.Region {
				mappingRegionExists = true
				break
			}
		}
		if !mappingRegionExists {
			return fmt.Errorf("%s.region %q does not exist in regions", prefix, mapping.Region)
		}
	}

	// Validate regions have countries/continents defined (for domains using geolocation)
	for _, domain := range c.Domains {
		if strings.ToLower(domain.RoutingAlgorithm) != "geolocation" {
			continue
		}

		for _, regionName := range domain.Regions {
			for _, region := range c.Regions {
				if region.Name == regionName {
					// Region must have either countries or continents defined
					// (or be referenced only via custom mappings, which is valid)
					// We allow regions without geo mapping for custom mapping use
					if len(region.Countries) == 0 && len(region.Continents) == 0 {
						// This is allowed - region can be used only via custom mappings
						continue
					}

					// Validate continent codes if specified
					validContinents := map[string]bool{
						"AF": true, "AN": true, "AS": true, "EU": true,
						"NA": true, "OC": true, "SA": true,
					}
					for _, cont := range region.Continents {
						if !validContinents[strings.ToUpper(cont)] {
							return fmt.Errorf("region %q: invalid continent code %q (valid: AF, AN, AS, EU, NA, OC, SA)",
								region.Name, cont)
						}
					}

					// Validate country codes are 2 characters (basic check)
					for _, country := range region.Countries {
						if len(country) != 2 {
							return fmt.Errorf("region %q: invalid country code %q (must be 2-letter ISO code)",
								region.Name, country)
						}
					}
				}
			}
		}
	}

	return nil
}
