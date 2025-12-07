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

package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ServerConfig configures the API server.
type ServerConfig struct {
	Address           string
	AllowedNetworks   []string
	TrustProxyHeaders bool
	Logger            *slog.Logger
}

// Server is the HTTP API server.
type Server struct {
	config     ServerConfig
	httpServer *http.Server
	handlers   *Handlers
	logger     *slog.Logger
}

// NewServer creates a new API server.
func NewServer(cfg ServerConfig, handlers *Handlers) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Default to localhost only if no networks specified
	if len(cfg.AllowedNetworks) == 0 {
		cfg.AllowedNetworks = []string{"127.0.0.1/32", "::1/128"}
	}

	return &Server{
		config:   cfg,
		handlers: handlers,
		logger:   cfg.Logger,
	}, nil
}

// Start begins serving the API.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/api/v1/health/servers", s.handlers.HealthServers)
	mux.HandleFunc("/api/v1/health/regions", s.handlers.HealthRegions)
	mux.HandleFunc("/api/v1/ready", s.handlers.Ready)
	mux.HandleFunc("/api/v1/live", s.handlers.Live)

	// Build middleware chain
	var handler http.Handler = mux

	// Add logging middleware
	loggingMw := NewLoggingMiddleware(s.logger)
	handler = loggingMw.Wrap(handler)

	// Add ACL middleware (outermost, checks first)
	aclMw, err := NewACLMiddleware(s.config.AllowedNetworks, s.config.TrustProxyHeaders, s.logger)
	if err != nil {
		return fmt.Errorf("failed to create ACL middleware: %w", err)
	}
	handler = aclMw.Wrap(handler)

	s.httpServer = &http.Server{
		Addr:         s.config.Address,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("starting API server",
		"address", s.config.Address,
		"allowed_networks", s.config.AllowedNetworks,
		"trust_proxy_headers", s.config.TrustProxyHeaders,
	)

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("API server error: %w", err)
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	}
}

// Shutdown gracefully stops the API server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	s.logger.Info("shutting down API server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(shutdownCtx)
}
