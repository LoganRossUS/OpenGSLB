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

// Package config provides configuration loading and validation for OpenGSLB.
package config

import "time"

// Config is the root configuration structure for OpenGSLB.
type Config struct {
	DNS     DNSConfig     `yaml:"dns"`
	Regions []Region      `yaml:"regions"`
	Domains []Domain      `yaml:"domains"`
	Logging LoggingConfig `yaml:"logging"`
	Metrics MetricsConfig `yaml:"metrics"`
	API     APIConfig     `yaml:"api"`
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
}

// HealthCheck defines health check configuration for a region.
type HealthCheck struct {
	Type             string        `yaml:"type"`
	Interval         time.Duration `yaml:"interval"`
	Timeout          time.Duration `yaml:"timeout"`
	Path             string        `yaml:"path"`
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
