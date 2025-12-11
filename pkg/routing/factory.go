// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/loganrossus/OpenGSLB/pkg/geo"
)

// Algorithm names.
const (
	AlgorithmRoundRobin = "round-robin"
	AlgorithmWeighted   = "weighted"
	AlgorithmFailover   = "failover"
	// AlgorithmGeolocation is defined in geo.go
	AlgorithmLatency = "latency"
)

// NewRouter creates a router based on the algorithm name.
// Supported algorithms: round-robin, weighted, failover, geolocation, latency.
// For geolocation or latency routing with providers, use Factory.NewRouter().
func NewRouter(algorithm string) (Router, error) {
	switch strings.ToLower(algorithm) {
	case AlgorithmRoundRobin, "roundrobin", "rr":
		return NewRoundRobinRouter(), nil
	case AlgorithmWeighted, "weight":
		return NewWeightedRouter(), nil
	case AlgorithmFailover, "active-standby", "activestandby":
		return NewFailoverRouter(), nil
	case AlgorithmGeolocation, "geo":
		// Return a GeoRouter without resolver - must be configured later
		return NewGeoRouter(GeoRouterConfig{}), nil
	case AlgorithmLatency:
		// Return a LatencyRouter without provider - will fall back to round-robin
		// until a provider is set via SetProvider()
		return NewLatencyRouter(LatencyRouterConfig{}), nil
	default:
		return nil, fmt.Errorf("unknown routing algorithm: %s", algorithm)
	}
}

// Factory creates routers with access to shared resources like geo resolver.
type Factory struct {
	geoResolver       *geo.Resolver
	latencyProvider   LatencyProvider
	defaultRegion     string
	maxLatencyMs      int
	minLatencySamples int
	logger            *slog.Logger
}

// FactoryConfig contains configuration for creating a Factory.
type FactoryConfig struct {
	GeoResolver       *geo.Resolver
	LatencyProvider   LatencyProvider
	DefaultRegion     string
	MaxLatencyMs      int // Max latency threshold for latency routing (default: 500)
	MinLatencySamples int // Min samples required before using latency data (default: 3)
	Logger            *slog.Logger
}

// NewFactory creates a new router Factory.
func NewFactory(cfg FactoryConfig) *Factory {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Apply defaults for latency config
	maxLatencyMs := cfg.MaxLatencyMs
	if maxLatencyMs == 0 {
		maxLatencyMs = 500
	}
	minSamples := cfg.MinLatencySamples
	if minSamples == 0 {
		minSamples = 3
	}

	return &Factory{
		geoResolver:       cfg.GeoResolver,
		latencyProvider:   cfg.LatencyProvider,
		defaultRegion:     cfg.DefaultRegion,
		maxLatencyMs:      maxLatencyMs,
		minLatencySamples: minSamples,
		logger:            logger,
	}
}

// NewRouter creates a router based on the algorithm name.
// This factory method provides access to shared resources like geo resolver.
func (f *Factory) NewRouter(algorithm string) (Router, error) {
	switch strings.ToLower(algorithm) {
	case AlgorithmRoundRobin, "roundrobin", "rr":
		return NewRoundRobinRouter(), nil
	case AlgorithmWeighted, "weight":
		return NewWeightedRouter(), nil
	case AlgorithmFailover, "active-standby", "activestandby":
		return NewFailoverRouter(), nil
	case AlgorithmGeolocation, "geo":
		return NewGeoRouter(GeoRouterConfig{
			Resolver:      f.geoResolver,
			DefaultRegion: f.defaultRegion,
			Logger:        f.logger,
		}), nil
	case AlgorithmLatency:
		return NewLatencyRouter(LatencyRouterConfig{
			Provider:     f.latencyProvider,
			MaxLatencyMs: f.maxLatencyMs,
			MinSamples:   f.minLatencySamples,
			Logger:       f.logger,
		}), nil
	default:
		return nil, fmt.Errorf("unknown routing algorithm: %s", algorithm)
	}
}

// SetGeoResolver sets or updates the geo resolver for creating GeoRouters.
func (f *Factory) SetGeoResolver(resolver *geo.Resolver) {
	f.geoResolver = resolver
}

// SetDefaultRegion sets the default region for geolocation routing.
func (f *Factory) SetDefaultRegion(region string) {
	f.defaultRegion = region
}

// SetLatencyProvider sets or updates the latency provider for creating LatencyRouters.
func (f *Factory) SetLatencyProvider(provider LatencyProvider) {
	f.latencyProvider = provider
}

// SetLatencyConfig updates the latency routing configuration.
func (f *Factory) SetLatencyConfig(maxLatencyMs, minSamples int) {
	if maxLatencyMs > 0 {
		f.maxLatencyMs = maxLatencyMs
	}
	if minSamples > 0 {
		f.minLatencySamples = minSamples
	}
}
