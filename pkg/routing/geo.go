// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"log/slog"
	"net"
	"sync"

	"github.com/loganrossus/OpenGSLB/pkg/geo"
	"github.com/loganrossus/OpenGSLB/pkg/metrics"
)

// Context keys for geolocation routing.
type geoContextKey string

const (
	// ClientIPKey is the context key for the client IP address.
	ClientIPKey geoContextKey = "clientIP"

	// DomainKey is the context key for the domain being queried.
	DomainKey geoContextKey = "domain"

	// AlgorithmGeolocation is the algorithm name for geolocation routing.
	AlgorithmGeolocation = "geolocation"
)

// WithClientIP adds the client IP to the context for geolocation routing.
func WithClientIP(ctx context.Context, ip net.IP) context.Context {
	return context.WithValue(ctx, ClientIPKey, ip)
}

// GetClientIP retrieves the client IP from the context.
func GetClientIP(ctx context.Context) net.IP {
	if ip, ok := ctx.Value(ClientIPKey).(net.IP); ok {
		return ip
	}
	return nil
}

// WithDomain adds the domain name to the context for metric recording.
func WithDomain(ctx context.Context, domain string) context.Context {
	return context.WithValue(ctx, DomainKey, domain)
}

// GetDomain retrieves the domain from the context.
func GetDomain(ctx context.Context) string {
	if domain, ok := ctx.Value(DomainKey).(string); ok {
		return domain
	}
	return ""
}

// GeoRouter implements geolocation-based server selection.
// It uses a geo.Resolver to determine the client's region and selects
// a server from that region.
type GeoRouter struct {
	mu            sync.RWMutex
	resolver      *geo.Resolver
	defaultRegion string
	fallback      Router
	logger        *slog.Logger
}

// GeoRouterConfig contains configuration for creating a GeoRouter.
type GeoRouterConfig struct {
	Resolver      *geo.Resolver
	DefaultRegion string
	Logger        *slog.Logger
}

// NewGeoRouter creates a new geolocation-based router.
func NewGeoRouter(cfg GeoRouterConfig) *GeoRouter {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &GeoRouter{
		resolver:      cfg.Resolver,
		defaultRegion: cfg.DefaultRegion,
		fallback:      NewRoundRobinRouter(),
		logger:        logger,
	}
}

// Route selects a server based on the client's geographic location.
// The client IP should be provided in the context using WithClientIP().
// If no client IP is available, falls back to round-robin.
func (r *GeoRouter) Route(ctx context.Context, pool ServerPool) (*Server, error) {
	servers := pool.Servers()
	if len(servers) == 0 {
		return nil, ErrNoHealthyServers
	}

	// Get domain from context for metrics
	domain := GetDomain(ctx)

	// Get client IP from context
	clientIP := GetClientIP(ctx)
	if clientIP == nil {
		r.logger.Debug("no client IP in context, using round-robin fallback")
		if domain != "" {
			metrics.RecordGeoFallback(domain, "no_client_ip")
		}
		return r.fallback.Route(ctx, pool)
	}

	r.mu.RLock()
	resolver := r.resolver
	r.mu.RUnlock()

	if resolver == nil {
		r.logger.Warn("geo resolver not configured, using round-robin fallback")
		if domain != "" {
			metrics.RecordGeoFallback(domain, "no_resolver")
		}
		return r.fallback.Route(ctx, pool)
	}

	// Resolve client IP to region
	match := resolver.Resolve(clientIP)

	r.logger.Debug("geolocation resolution",
		"clientIP", clientIP.String(),
		"region", match.Region,
		"matchType", match.MatchType,
	)

	// Record metrics based on match type
	if domain != "" {
		switch match.MatchType {
		case geo.MatchTypeCustomMapping:
			metrics.RecordGeoCustomHit(domain, match.Region, match.MatchedCIDR)
			metrics.RecordGeoRoutingDecision(domain, "", "", match.Region)
		case geo.MatchTypeGeoIP:
			metrics.RecordGeoRoutingDecision(domain, match.Country, match.Continent, match.Region)
		case geo.MatchTypeDefault:
			metrics.RecordGeoFallback(domain, "lookup_failed")
			metrics.RecordGeoRoutingDecision(domain, "", "", match.Region)
		}
	}

	// Find servers in the matched region
	regionServers := r.filterByRegion(servers, match.Region)
	if len(regionServers) > 0 {
		// Use round-robin among servers in the matched region
		regionPool := NewSimpleServerPool(regionServers)
		return r.fallback.Route(ctx, regionPool)
	}

	// No servers in matched region, try default region
	if match.Region != r.defaultRegion {
		r.logger.Debug("no servers in matched region, trying default",
			"matchedRegion", match.Region,
			"defaultRegion", r.defaultRegion,
		)
		if domain != "" {
			metrics.RecordGeoFallback(domain, "no_servers_in_region")
		}
		defaultServers := r.filterByRegion(servers, r.defaultRegion)
		if len(defaultServers) > 0 {
			defaultPool := NewSimpleServerPool(defaultServers)
			return r.fallback.Route(ctx, defaultPool)
		}
	}

	// Fall back to any available server
	r.logger.Debug("no servers in region or default, using any available server",
		"matchedRegion", match.Region,
	)
	if domain != "" {
		metrics.RecordGeoFallback(domain, "no_match")
	}
	return r.fallback.Route(ctx, pool)
}

// filterByRegion returns servers that belong to the specified region.
func (r *GeoRouter) filterByRegion(servers []*Server, region string) []*Server {
	var result []*Server
	for _, s := range servers {
		if s.Region == region {
			result = append(result, s)
		}
	}
	return result
}

// Algorithm returns the algorithm name.
func (r *GeoRouter) Algorithm() string {
	return AlgorithmGeolocation
}

// SetResolver sets or updates the geo resolver.
func (r *GeoRouter) SetResolver(resolver *geo.Resolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolver = resolver
}

// GetResolver returns the current geo resolver.
func (r *GeoRouter) GetResolver() *geo.Resolver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resolver
}
