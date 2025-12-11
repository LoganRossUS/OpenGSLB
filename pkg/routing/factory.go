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
// Supported algorithms: round-robin, weighted, failover.
// For geolocation or latency routing, use NewRouterWithConfig or Factory.
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
		// Latency routing will be implemented in Sprint 6 Story 2
		return nil, fmt.Errorf("latency routing not yet implemented")
	default:
		return nil, fmt.Errorf("unknown routing algorithm: %s", algorithm)
	}
}

// Factory creates routers with access to shared resources like geo resolver.
type Factory struct {
	geoResolver   *geo.Resolver
	defaultRegion string
	logger        *slog.Logger
}

// FactoryConfig contains configuration for creating a Factory.
type FactoryConfig struct {
	GeoResolver   *geo.Resolver
	DefaultRegion string
	Logger        *slog.Logger
}

// NewFactory creates a new router Factory.
func NewFactory(cfg FactoryConfig) *Factory {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Factory{
		geoResolver:   cfg.GeoResolver,
		defaultRegion: cfg.DefaultRegion,
		logger:        logger,
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
		// Latency routing will be implemented in Sprint 6 Story 2
		return nil, fmt.Errorf("latency routing not yet implemented")
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
