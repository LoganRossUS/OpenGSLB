// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// ServerConfig holds API server configuration.
type ServerConfig struct {
	Address           string
	AllowedNetworks   []string
	TrustProxyHeaders bool
	Logger            *slog.Logger
	CORSConfig        *CORSConfig
	EnableCORS        bool
}

// OverwatchAPIHandlers interface for Overwatch-specific API endpoints.
type OverwatchAPIHandlers interface {
	HandleBackends(w http.ResponseWriter, r *http.Request)
	HandleBackendOverride(w http.ResponseWriter, r *http.Request)
	HandleStats(w http.ResponseWriter, r *http.Request)
	HandleValidate(w http.ResponseWriter, r *http.Request)
}

// Server provides the HTTP API for OpenGSLB.
type Server struct {
	config            ServerConfig
	httpServer        *http.Server
	logger            *slog.Logger
	handlers          *Handlers
	overwatchHandlers OverwatchAPIHandlers
	overrideHandlers  *OverrideHandlers
	dnssecHandlers    *DNSSECHandlers
	geoHandlers       *GeoHandlers

	// Dashboard/Overlord API handlers
	simpleHealthHandlers *SimpleHealthHandlers
	domainHandlers       *DomainHandlers
	serverHandlers       *ServerHandlers
	regionHandlers       *RegionHandlers
	nodeHandlers         *NodeHandlers
	gossipHandlers       *GossipHandlers
	auditHandlers        *AuditHandlers
	metricsHandlers      *MetricsHandlers
	configHandlers       *ConfigHandlers
	routingHandlers      *RoutingHandlers

	// Discovery handlers for walkable API
	discoveryHandlers *DiscoveryHandlers
}

// NewServer creates a new API server.
// ADR-015: Removed ClusterHandlers - cluster mode no longer exists.
func NewServer(cfg ServerConfig, handlers *Handlers) (*Server, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		config:   cfg,
		logger:   logger,
		handlers: handlers,
	}, nil
}

// SetOverwatchHandlers sets the Overwatch-specific API handlers.
func (s *Server) SetOverwatchHandlers(handlers OverwatchAPIHandlers) {
	s.overwatchHandlers = handlers
}

// SetOverrideHandlers sets the override handlers for override API endpoints.
func (s *Server) SetOverrideHandlers(oh *OverrideHandlers) {
	s.overrideHandlers = oh
}

// SetDNSSECHandlers sets the DNSSEC handlers for DNSSEC API endpoints.
func (s *Server) SetDNSSECHandlers(dh *DNSSECHandlers) {
	s.dnssecHandlers = dh
}

// SetGeoHandlers sets the geo handlers for geolocation API endpoints.
func (s *Server) SetGeoHandlers(gh *GeoHandlers) {
	s.geoHandlers = gh
}

// SetSimpleHealthHandlers sets the simple health handlers.
func (s *Server) SetSimpleHealthHandlers(h *SimpleHealthHandlers) {
	s.simpleHealthHandlers = h
}

// SetDomainHandlers sets the domain handlers.
func (s *Server) SetDomainHandlers(h *DomainHandlers) {
	s.domainHandlers = h
}

// SetServerHandlers sets the server handlers.
func (s *Server) SetServerHandlers(h *ServerHandlers) {
	s.serverHandlers = h
}

// SetRegionHandlers sets the region handlers.
func (s *Server) SetRegionHandlers(h *RegionHandlers) {
	s.regionHandlers = h
}

// SetNodeHandlers sets the node handlers.
func (s *Server) SetNodeHandlers(h *NodeHandlers) {
	s.nodeHandlers = h
}

// SetGossipHandlers sets the gossip handlers.
func (s *Server) SetGossipHandlers(h *GossipHandlers) {
	s.gossipHandlers = h
}

// SetAuditHandlers sets the audit handlers.
func (s *Server) SetAuditHandlers(h *AuditHandlers) {
	s.auditHandlers = h
}

// SetMetricsHandlers sets the metrics handlers.
func (s *Server) SetMetricsHandlers(h *MetricsHandlers) {
	s.metricsHandlers = h
}

// SetConfigHandlers sets the config handlers.
func (s *Server) SetConfigHandlers(h *ConfigHandlers) {
	s.configHandlers = h
}

// SetRoutingHandlers sets the routing handlers.
func (s *Server) SetRoutingHandlers(h *RoutingHandlers) {
	s.routingHandlers = h
}

// Start starts the API server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/api/v1/health/servers", s.withACL(s.handlers.HealthServers))
	mux.HandleFunc("/api/v1/health/regions", s.withACL(s.handlers.HealthRegions))
	mux.HandleFunc("/api/v1/ready", s.handlers.Ready)
	mux.HandleFunc("/api/v1/live", s.handlers.Live)

	// Overwatch-specific endpoints (Story 3)
	if s.overwatchHandlers != nil {
		mux.HandleFunc("/api/v1/overwatch/backends", s.withACL(s.overwatchHandlers.HandleBackends))
		mux.HandleFunc("/api/v1/overwatch/backends/", s.withACL(s.overwatchHandlers.HandleBackendOverride))
		mux.HandleFunc("/api/v1/overwatch/stats", s.withACL(s.overwatchHandlers.HandleStats))
		mux.HandleFunc("/api/v1/overwatch/validate", s.withACL(s.overwatchHandlers.HandleValidate))
	}

	// External override endpoints (Story 6)
	if s.overrideHandlers != nil {
		mux.HandleFunc("/api/v1/overrides/", s.withACL(s.overrideHandlers.HandleOverrides))
		mux.HandleFunc("/api/v1/overrides", s.withACL(s.overrideHandlers.HandleOverrides))
		s.logger.Debug("override API endpoints registered")
	}

	// DNSSEC endpoints (Stories 7 & 8)
	if s.dnssecHandlers != nil {
		mux.HandleFunc("/api/v1/dnssec/ds", s.withACL(s.dnssecHandlers.HandleDS))
		mux.HandleFunc("/api/v1/dnssec/keys", s.withACL(s.dnssecHandlers.HandleKeys))
		mux.HandleFunc("/api/v1/dnssec/status", s.withACL(s.dnssecHandlers.HandleStatus))
		mux.HandleFunc("/api/v1/dnssec/sync", s.withACL(s.dnssecHandlers.HandleSync))
		mux.HandleFunc("/api/v1/dnssec", s.withACL(s.dnssecHandlers.HandleDNSSEC))
		mux.HandleFunc("/api/v1/dnssec/", s.withACL(s.dnssecHandlers.HandleDNSSEC))
		s.logger.Debug("DNSSEC API endpoints registered")
	}

	// Geolocation endpoints (Sprint 6)
	if s.geoHandlers != nil {
		mux.HandleFunc("/api/v1/geo/mappings", s.withACL(s.geoHandlers.HandleMappings))
		mux.HandleFunc("/api/v1/geo/mappings/", s.withACL(s.geoHandlers.DeleteMapping))
		mux.HandleFunc("/api/v1/geo/test", s.withACL(s.geoHandlers.TestIP))
		s.logger.Debug("geolocation API endpoints registered")
	}

	// Simple health endpoint (for Overlord dashboard)
	if s.simpleHealthHandlers != nil {
		mux.HandleFunc("/api/health", s.simpleHealthHandlers.HandleHealth)
		s.logger.Debug("simple health API endpoint registered")
	}

	// Domain management endpoints
	if s.domainHandlers != nil {
		mux.HandleFunc("/api/v1/domains", s.withACL(s.domainHandlers.HandleDomains))
		mux.HandleFunc("/api/v1/domains/", s.withACL(s.domainHandlers.HandleDomains))
		s.logger.Debug("domain API endpoints registered")
	}

	// Server management endpoints
	if s.serverHandlers != nil {
		mux.HandleFunc("/api/v1/servers", s.withACL(s.serverHandlers.HandleServers))
		mux.HandleFunc("/api/v1/servers/", s.withACL(s.serverHandlers.HandleServers))
		s.logger.Debug("server API endpoints registered")
	}

	// Region management endpoints
	if s.regionHandlers != nil {
		mux.HandleFunc("/api/v1/regions", s.withACL(s.regionHandlers.HandleRegions))
		mux.HandleFunc("/api/v1/regions/", s.withACL(s.regionHandlers.HandleRegions))
		s.logger.Debug("region API endpoints registered")
	}

	// Node management endpoints (Overwatch and Agent nodes)
	if s.nodeHandlers != nil {
		mux.HandleFunc("/api/v1/nodes/", s.withACL(s.nodeHandlers.HandleNodes))
		s.logger.Debug("node API endpoints registered")
	}

	// Gossip protocol endpoints
	if s.gossipHandlers != nil {
		mux.HandleFunc("/api/v1/gossip/", s.withACL(s.gossipHandlers.HandleGossip))
		s.logger.Debug("gossip API endpoints registered")
	}

	// Audit log endpoints
	if s.auditHandlers != nil {
		mux.HandleFunc("/api/v1/audit-logs", s.withACL(s.auditHandlers.HandleAuditLogs))
		mux.HandleFunc("/api/v1/audit-logs/", s.withACL(s.auditHandlers.HandleAuditLogs))
		s.logger.Debug("audit log API endpoints registered")
	}

	// Metrics endpoints
	if s.metricsHandlers != nil {
		mux.HandleFunc("/api/v1/metrics", s.withACL(s.metricsHandlers.HandleMetrics))
		mux.HandleFunc("/api/v1/metrics/", s.withACL(s.metricsHandlers.HandleMetrics))
		s.logger.Debug("metrics API endpoints registered")
	}

	// Config and preferences endpoints
	if s.configHandlers != nil {
		mux.HandleFunc("/api/v1/preferences", s.withACL(s.configHandlers.HandlePreferences))
		mux.HandleFunc("/api/v1/config", s.withACL(s.configHandlers.HandleConfig))
		mux.HandleFunc("/api/v1/config/", s.withACL(s.configHandlers.HandleConfig))
		s.logger.Debug("config API endpoints registered")
	}

	// Routing endpoints
	if s.routingHandlers != nil {
		mux.HandleFunc("/api/v1/routing/", s.withACL(s.routingHandlers.HandleRouting))
		s.logger.Debug("routing API endpoints registered")
	}

	// Create discovery handlers for walkable API
	s.discoveryHandlers = NewDiscoveryHandlers()

	// Discovery endpoints for walkable API (no ACL - publicly accessible)
	mux.HandleFunc("/api/v1/health", s.discoveryHandlers.HandleHealthRoot)
	mux.HandleFunc("/api/v1/health/", s.discoveryHandlers.HandleHealthRoot)
	mux.HandleFunc("/api/v1/geo", s.discoveryHandlers.HandleGeoRoot)
	mux.HandleFunc("/api/v1/geo/", s.discoveryHandlers.HandleGeoRoot)
	mux.HandleFunc("/api/v1/overwatch", s.discoveryHandlers.HandleOverwatchRoot)
	mux.HandleFunc("/api/v1/overwatch/", s.discoveryHandlers.HandleOverwatchRoot)
	mux.HandleFunc("/api/v1/version", s.discoveryHandlers.HandleVersion)
	mux.HandleFunc("/api/v1", s.discoveryHandlers.HandleV1Root)
	mux.HandleFunc("/api/v1/", s.discoveryHandlers.HandleV1Root)
	mux.HandleFunc("/api", s.discoveryHandlers.HandleAPIRoot)
	mux.HandleFunc("/api/", s.discoveryHandlers.HandleAPIRoot)
	s.logger.Debug("API discovery endpoints registered")

	// ADR-015: Cluster endpoints removed
	// The following endpoints no longer exist:
	// - /api/v1/cluster/status
	// - /api/v1/cluster/join
	// - /api/v1/cluster/remove

	// Apply CORS middleware if enabled
	var handler http.Handler = mux
	if s.config.EnableCORS {
		corsConfig := DefaultCORSConfig()
		if s.config.CORSConfig != nil {
			corsConfig = *s.config.CORSConfig
		}
		handler = CORSMiddleware(corsConfig, mux)
		s.logger.Debug("CORS middleware enabled")
	}

	s.httpServer = &http.Server{
		Addr:         s.config.Address,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("starting API server",
		"address", s.config.Address,
		"override_endpoints", s.overrideHandlers != nil,
		"dnssec_endpoints", s.dnssecHandlers != nil,
	)

	// Start server in a way that respects context cancellation
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return s.httpServer.Shutdown(context.Background())
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// Shutdown gracefully stops the API server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	s.logger.Info("stopping API server")
	return s.httpServer.Shutdown(ctx)
}

// withACL wraps a handler with IP-based access control.
func (s *Server) withACL(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAllowed(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// isAllowed checks if the request IP is in the allowed networks.
func (s *Server) isAllowed(r *http.Request) bool {
	// Get client IP, respecting proxy headers if configured
	clientIP := s.getClientIP(r)

	if len(s.config.AllowedNetworks) == 0 {
		// No ACL configured - only localhost allowed by default
		ip := net.ParseIP(clientIP)
		return ip != nil && ip.IsLoopback()
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	for _, network := range s.config.AllowedNetworks {
		_, cidr, err := net.ParseCIDR(network)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// getClientIP extracts the client IP from the request.
func (s *Server) getClientIP(r *http.Request) string {
	// If we trust proxy headers, check X-Forwarded-For first
	if s.config.TrustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For can contain multiple IPs; use the first one
			if idx := len(xff); idx > 0 {
				for i, c := range xff {
					if c == ',' {
						return xff[:i]
					}
				}
				return xff
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return xri
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
