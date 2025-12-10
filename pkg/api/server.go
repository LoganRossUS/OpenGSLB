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
}

// Server provides the HTTP API for OpenGSLB.
type Server struct {
	config     ServerConfig
	httpServer *http.Server
	logger     *slog.Logger
	handlers   *Handlers
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

// Start starts the API server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/api/v1/health/servers", s.withACL(s.handlers.HealthServers))
	mux.HandleFunc("/api/v1/health/regions", s.withACL(s.handlers.HealthRegions))
	mux.HandleFunc("/api/v1/ready", s.handlers.Ready)
	mux.HandleFunc("/api/v1/live", s.handlers.Live)

	// ADR-015: Cluster endpoints removed
	// The following endpoints no longer exist:
	// - /api/v1/cluster/status
	// - /api/v1/cluster/join
	// - /api/v1/cluster/remove

	s.httpServer = &http.Server{
		Addr:         s.config.Address,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("starting API server", "address", s.config.Address)

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
