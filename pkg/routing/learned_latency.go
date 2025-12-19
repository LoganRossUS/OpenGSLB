// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
)

// LearnedLatencyData contains latency information for a backend from a client subnet.
// This mirrors overwatch.BackendLatency to avoid circular imports.
type LearnedLatencyData struct {
	// Backend is the backend service name.
	Backend string
	// EWMA is the smoothed RTT.
	EWMA time.Duration
	// SampleCount is the number of samples.
	SampleCount uint64
	// LastUpdated is when this entry was last updated.
	LastUpdated time.Time
}

// LearnedLatencyProvider provides learned latency data for client-backend pairs.
// This is implemented by overwatch.LearnedLatencyTable.
type LearnedLatencyProvider interface {
	// GetLatencyForBackend returns the learned latency for a specific client->backend pair.
	GetLatencyForBackend(clientIP netip.Addr, backend string) (*LearnedLatencyData, bool)
}

// LearnedLatencyRouterConfig contains configuration for the LearnedLatencyRouter.
type LearnedLatencyRouterConfig struct {
	// Provider is the source of learned latency data.
	Provider LearnedLatencyProvider

	// MaxLatencyMs excludes servers above this threshold (0 = no limit).
	// Default: 500
	MaxLatencyMs int

	// MinSamples is the minimum number of samples before using latency data.
	// Servers with fewer samples fall back to the fallback router.
	// Default: 5
	MinSamples int

	// StaleThreshold is how long since last update before data is considered stale.
	// Default: 168h (7 days)
	StaleThreshold time.Duration

	// FallbackRouter is used when no learned latency data is available.
	// Default: round-robin
	FallbackRouter Router

	// Logger for routing decisions.
	Logger *slog.Logger
}

// DefaultLearnedLatencyRouterConfig returns sensible defaults.
func DefaultLearnedLatencyRouterConfig() LearnedLatencyRouterConfig {
	return LearnedLatencyRouterConfig{
		MaxLatencyMs:   500,
		MinSamples:     5,
		StaleThreshold: 168 * time.Hour,
		Logger:         slog.Default(),
	}
}

// LearnedLatencyRouter implements routing based on learned client-to-backend latency (ADR-017).
// It uses TCP RTT data collected by agents to route clients to the fastest backend.
type LearnedLatencyRouter struct {
	mu       sync.RWMutex
	provider LearnedLatencyProvider
	config   LearnedLatencyRouterConfig
	fallback Router
	logger   *slog.Logger
}

// NewLearnedLatencyRouter creates a new learned latency-based router.
func NewLearnedLatencyRouter(cfg LearnedLatencyRouterConfig) *LearnedLatencyRouter {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Apply defaults
	if cfg.MaxLatencyMs == 0 {
		cfg.MaxLatencyMs = 500
	}
	if cfg.MinSamples == 0 {
		cfg.MinSamples = 5
	}
	if cfg.StaleThreshold == 0 {
		cfg.StaleThreshold = 168 * time.Hour
	}

	fallback := cfg.FallbackRouter
	if fallback == nil {
		fallback = NewRoundRobinRouter()
	}

	return &LearnedLatencyRouter{
		provider: cfg.Provider,
		config:   cfg,
		fallback: fallback,
		logger:   logger,
	}
}

// serverLearnedLatency pairs a server with its learned latency info.
type serverLearnedLatency struct {
	server  *Server
	latency *LearnedLatencyData
}

// Route selects the server with the lowest learned latency for the client.
// Falls back to the fallback router when:
// - No learned latency provider is configured
// - No client IP is available in context
// - No servers have sufficient latency samples
// - All servers are above the maximum latency threshold
func (r *LearnedLatencyRouter) Route(ctx context.Context, pool ServerPool) (*Server, error) {
	servers := pool.Servers()
	if len(servers) == 0 {
		return nil, ErrNoHealthyServers
	}

	// Get domain and client IP from context
	domain := GetDomain(ctx)
	clientIPOld := GetClientIP(ctx)

	// Convert net.IP to netip.Addr
	var clientIP netip.Addr
	if clientIPOld != nil {
		var ok bool
		clientIP, ok = netip.AddrFromSlice(clientIPOld)
		if !ok {
			r.logger.Debug("could not convert client IP to netip.Addr, using fallback")
			if domain != "" {
				metrics.RecordLatencyFallback(domain, "invalid_client_ip")
			}
			return r.fallback.Route(ctx, pool)
		}
		// Normalize IPv4-mapped IPv6 to IPv4
		if clientIP.Is4In6() {
			clientIP = clientIP.Unmap()
		}
	}

	r.mu.RLock()
	provider := r.provider
	maxLatency := time.Duration(r.config.MaxLatencyMs) * time.Millisecond
	minSamples := r.config.MinSamples
	staleThreshold := r.config.StaleThreshold
	r.mu.RUnlock()

	// If no provider or no client IP, fall back
	if provider == nil {
		r.logger.Debug("no learned latency provider configured, using fallback")
		if domain != "" {
			metrics.RecordLatencyFallback(domain, "no_provider")
		}
		return r.fallback.Route(ctx, pool)
	}

	if !clientIP.IsValid() {
		r.logger.Debug("no client IP in context, using fallback")
		if domain != "" {
			metrics.RecordLatencyFallback(domain, "no_client_ip")
		}
		return r.fallback.Route(ctx, pool)
	}

	// Collect learned latency data for all servers
	var withLatency []serverLearnedLatency
	now := time.Now()

	for _, server := range servers {
		// Use server address as backend identifier
		backendKey := fmt.Sprintf("%s:%d", server.Address, server.Port)
		data, hasData := provider.GetLatencyForBackend(clientIP, backendKey)

		if !hasData {
			continue
		}

		// Check minimum samples
		if data.SampleCount < uint64(minSamples) {
			continue
		}

		// Check staleness
		if now.Sub(data.LastUpdated) > staleThreshold {
			continue
		}

		withLatency = append(withLatency, serverLearnedLatency{
			server:  server,
			latency: data,
		})
	}

	// If no servers have learned latency data, fall back
	if len(withLatency) == 0 {
		r.logger.Debug("no servers with learned latency data, using fallback",
			"total_servers", len(servers),
			"client_ip", clientIP.String(),
		)
		if domain != "" {
			metrics.RecordLatencyFallback(domain, "no_learned_data")
		}
		return r.fallback.Route(ctx, pool)
	}

	// Filter by max latency threshold (if configured)
	var withinThreshold []serverLearnedLatency
	if maxLatency > 0 {
		for _, sl := range withLatency {
			if sl.latency.EWMA <= maxLatency {
				withinThreshold = append(withinThreshold, sl)
			}
		}

		// If all servers are above threshold, use all servers anyway but log warning
		if len(withinThreshold) == 0 {
			r.logger.Warn("all servers above learned latency threshold, using lowest",
				"threshold_ms", r.config.MaxLatencyMs,
				"servers_above_threshold", len(withLatency),
			)
			withinThreshold = withLatency
		}
	} else {
		withinThreshold = withLatency
	}

	// Select server with lowest learned latency
	selected := r.selectLowestLatency(withinThreshold)

	r.logger.Debug("learned latency routing decision",
		"selected_address", selected.server.Address,
		"selected_latency_ms", selected.latency.EWMA.Milliseconds(),
		"candidates", len(withinThreshold),
		"total_servers", len(servers),
		"client_subnet", clientIP.String(),
	)

	// Record the routing decision
	if domain != "" {
		serverAddr := fmt.Sprintf("%s:%d", selected.server.Address, selected.server.Port)
		metrics.RecordLatencyRoutingDecision(domain, serverAddr, float64(selected.latency.EWMA.Milliseconds()))
	}

	return selected.server, nil
}

// selectLowestLatency returns the server with the lowest learned latency.
func (r *LearnedLatencyRouter) selectLowestLatency(servers []serverLearnedLatency) serverLearnedLatency {
	if len(servers) == 0 {
		return serverLearnedLatency{}
	}

	lowest := servers[0]
	for _, sl := range servers[1:] {
		if sl.latency.EWMA < lowest.latency.EWMA {
			lowest = sl
		}
	}
	return lowest
}

// Algorithm returns the algorithm name.
func (r *LearnedLatencyRouter) Algorithm() string {
	return "learned_latency"
}

// SetProvider sets or updates the learned latency provider.
func (r *LearnedLatencyRouter) SetProvider(provider LearnedLatencyProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.provider = provider
}

// GetProvider returns the current learned latency provider.
func (r *LearnedLatencyRouter) GetProvider() LearnedLatencyProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.provider
}

// SetMaxLatency updates the maximum latency threshold.
func (r *LearnedLatencyRouter) SetMaxLatency(ms int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.MaxLatencyMs = ms
}

// SetMinSamples updates the minimum required samples.
func (r *LearnedLatencyRouter) SetMinSamples(samples int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.MinSamples = samples
}

// SetFallbackRouter sets the fallback router.
func (r *LearnedLatencyRouter) SetFallbackRouter(fallback Router) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = fallback
}
