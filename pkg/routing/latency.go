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
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
)

// LatencyInfo contains latency information for a backend server.
// This mirrors dns.LatencyInfo to avoid circular imports.
type LatencyInfo struct {
	// SmoothedLatency is the EMA of validation latency measurements.
	SmoothedLatency time.Duration
	// Samples is the number of latency samples collected.
	Samples int
	// LastLatency is the most recent raw latency measurement.
	LastLatency time.Duration
	// HasData indicates whether latency data is available.
	HasData bool
}

// LatencyProvider provides latency data for servers.
type LatencyProvider interface {
	GetLatency(address string, port int) LatencyInfo
}

// LatencyRouterConfig contains configuration for the LatencyRouter.
type LatencyRouterConfig struct {
	// Provider is the source of latency data for servers.
	Provider LatencyProvider

	// MaxLatencyMs excludes servers above this threshold (0 = no limit).
	// Default: 500
	MaxLatencyMs int

	// MinSamples is the minimum number of samples before using latency data.
	// Servers with fewer samples fall back to round-robin.
	// Default: 3
	MinSamples int

	// Logger for routing decisions.
	Logger *slog.Logger
}

// DefaultLatencyRouterConfig returns sensible defaults.
func DefaultLatencyRouterConfig() LatencyRouterConfig {
	return LatencyRouterConfig{
		MaxLatencyMs: 500,
		MinSamples:   3,
		Logger:       slog.Default(),
	}
}

// LatencyRouter implements latency-based server selection.
// It selects the server with the lowest smoothed (EMA) latency.
type LatencyRouter struct {
	mu       sync.RWMutex
	provider LatencyProvider
	config   LatencyRouterConfig
	fallback Router
	logger   *slog.Logger
}

// NewLatencyRouter creates a new latency-based router.
func NewLatencyRouter(cfg LatencyRouterConfig) *LatencyRouter {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Apply defaults
	if cfg.MaxLatencyMs == 0 {
		cfg.MaxLatencyMs = 500
	}
	if cfg.MinSamples == 0 {
		cfg.MinSamples = 3
	}

	return &LatencyRouter{
		provider: cfg.Provider,
		config:   cfg,
		fallback: NewRoundRobinRouter(),
		logger:   logger,
	}
}

// serverLatency pairs a server with its latency info for sorting.
type serverLatency struct {
	server  *Server
	latency LatencyInfo
}

// Route selects the server with the lowest smoothed latency.
// Falls back to round-robin when:
// - No latency provider is configured
// - No servers have sufficient latency samples
// - All servers are above the maximum latency threshold
func (r *LatencyRouter) Route(ctx context.Context, pool ServerPool) (*Server, error) {
	servers := pool.Servers()
	if len(servers) == 0 {
		return nil, ErrNoHealthyServers
	}

	// Get domain from context for metrics
	domain := GetDomain(ctx)

	r.mu.RLock()
	provider := r.provider
	maxLatency := time.Duration(r.config.MaxLatencyMs) * time.Millisecond
	minSamples := r.config.MinSamples
	r.mu.RUnlock()

	// If no provider, fall back to round-robin
	if provider == nil {
		r.logger.Debug("no latency provider configured, using round-robin fallback")
		if domain != "" {
			metrics.RecordLatencyFallback(domain, "no_provider")
		}
		return r.fallback.Route(ctx, pool)
	}

	// Collect latency data for all servers
	var withLatency []serverLatency
	for _, server := range servers {
		info := provider.GetLatency(server.Address, server.Port)
		if info.HasData && info.Samples >= minSamples {
			withLatency = append(withLatency, serverLatency{
				server:  server,
				latency: info,
			})
		} else if domain != "" {
			// Record servers rejected due to insufficient data
			serverAddr := fmt.Sprintf("%s:%d", server.Address, server.Port)
			metrics.RecordLatencyRejection(domain, serverAddr, "no_data")
		}
	}

	// If no servers have latency data, fall back to round-robin
	if len(withLatency) == 0 {
		r.logger.Debug("no servers with latency data, using round-robin fallback",
			"total_servers", len(servers),
			"min_samples_required", minSamples,
		)
		if domain != "" {
			metrics.RecordLatencyFallback(domain, "no_latency_data")
		}
		return r.fallback.Route(ctx, pool)
	}

	// Filter by max latency threshold (if configured)
	var withinThreshold []serverLatency
	if maxLatency > 0 {
		for _, sl := range withLatency {
			if sl.latency.SmoothedLatency <= maxLatency {
				withinThreshold = append(withinThreshold, sl)
			} else if domain != "" {
				// Record servers rejected due to latency threshold
				serverAddr := fmt.Sprintf("%s:%d", sl.server.Address, sl.server.Port)
				metrics.RecordLatencyRejection(domain, serverAddr, "above_threshold")
			}
		}

		// If all servers are above threshold, use all servers anyway
		// but log a warning
		if len(withinThreshold) == 0 {
			r.logger.Warn("all servers above latency threshold, using lowest latency",
				"threshold_ms", r.config.MaxLatencyMs,
				"servers_above_threshold", len(withLatency),
			)
			withinThreshold = withLatency
		}
	} else {
		withinThreshold = withLatency
	}

	// Select server with lowest smoothed latency
	selected := r.selectLowestLatency(withinThreshold)

	r.logger.Debug("latency-based routing decision",
		"selected_address", selected.server.Address,
		"selected_latency_ms", selected.latency.SmoothedLatency.Milliseconds(),
		"candidates", len(withinThreshold),
		"total_servers", len(servers),
	)

	// Record the selected server latency
	if domain != "" {
		serverAddr := fmt.Sprintf("%s:%d", selected.server.Address, selected.server.Port)
		metrics.RecordLatencyRoutingDecision(domain, serverAddr, float64(selected.latency.SmoothedLatency.Milliseconds()))
	}

	return selected.server, nil
}

// selectLowestLatency returns the server with the lowest smoothed latency.
func (r *LatencyRouter) selectLowestLatency(servers []serverLatency) serverLatency {
	if len(servers) == 0 {
		return serverLatency{}
	}

	lowest := servers[0]
	for _, sl := range servers[1:] {
		if sl.latency.SmoothedLatency < lowest.latency.SmoothedLatency {
			lowest = sl
		}
	}
	return lowest
}

// Algorithm returns the algorithm name.
func (r *LatencyRouter) Algorithm() string {
	return AlgorithmLatency
}

// SetProvider sets or updates the latency provider.
func (r *LatencyRouter) SetProvider(provider LatencyProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.provider = provider
}

// GetProvider returns the current latency provider.
func (r *LatencyRouter) GetProvider() LatencyProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.provider
}

// SetMaxLatency updates the maximum latency threshold.
func (r *LatencyRouter) SetMaxLatency(ms int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.MaxLatencyMs = ms
}

// SetMinSamples updates the minimum required samples.
func (r *LatencyRouter) SetMinSamples(samples int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.MinSamples = samples
}
