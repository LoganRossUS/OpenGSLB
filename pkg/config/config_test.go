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
