// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
	"github.com/loganrossus/OpenGSLB/pkg/version"
)

// ServerConfig holds Dashboard API server configuration.
type ServerConfig struct {
	// Address is the address to listen on (e.g., ":3001")
	Address string

	// AllowedOrigins is a list of allowed CORS origins
	AllowedOrigins []string

	// AllowedNetworks is a list of CIDR networks allowed to access the API
	AllowedNetworks []string

	// TrustProxyHeaders enables X-Forwarded-For header parsing
	TrustProxyHeaders bool

	// Logger for API operations
	Logger *slog.Logger
}

// DefaultServerConfig returns sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Address:        ":3001",
		AllowedOrigins: []string{"*"},
		Logger:         slog.Default(),
	}
}

// DataProvider is the interface for accessing OpenGSLB data.
type DataProvider interface {
	// Configuration access
	GetConfig() *config.Config
	UpdateConfig(cfg *config.Config) error

	// Backend registry access (overwatch mode)
	GetBackendRegistry() *overwatch.Registry

	// Validator access (overwatch mode)
	GetValidator() *overwatch.Validator
}

// Server is the Dashboard API server.
type Server struct {
	config     ServerConfig
	httpServer *http.Server
	mu         sync.Mutex
	logger     *slog.Logger
	handlers   *Handlers
	startTime  time.Time
}

// Handlers contains all Dashboard API handlers.
type Handlers struct {
	dataProvider DataProvider
	logger       *slog.Logger
	auditLogger  *AuditLogger
}

// NewServer creates a new Dashboard API server.
func NewServer(cfg ServerConfig, dataProvider DataProvider) (*Server, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	handlers := &Handlers{
		dataProvider: dataProvider,
		logger:       logger,
		auditLogger:  NewAuditLogger(logger),
	}

	return &Server{
		config:    cfg,
		logger:    logger,
		handlers:  handlers,
		startTime: time.Now(),
	}, nil
}

// Start starts the Dashboard API server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register all routes
	s.registerRoutes(mux)

	// Wrap with CORS middleware
	handler := s.corsMiddleware(mux)

	// Wrap with ACL middleware
	handler = s.aclMiddleware(handler)

	httpServer := &http.Server{
		Addr:         s.config.Address,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.mu.Lock()
	s.httpServer = httpServer
	s.mu.Unlock()

	s.logger.Info("starting Dashboard API server",
		"address", s.config.Address,
		"allowed_origins", s.config.AllowedOrigins,
	)

	// Start server in a way that respects context cancellation
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return httpServer.Shutdown(context.Background())
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// Shutdown gracefully stops the Dashboard API server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	httpServer := s.httpServer
	s.mu.Unlock()

	if httpServer == nil {
		return nil
	}
	s.logger.Info("stopping Dashboard API server")
	return httpServer.Shutdown(ctx)
}

// registerRoutes registers all API routes.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health & System
	mux.HandleFunc("/api/health", s.handlers.handleHealth)

	// Domains
	mux.HandleFunc("/api/domains", s.handlers.handleDomains)
	mux.HandleFunc("/api/domains/", s.handlers.handleDomainByName)

	// Servers/Backends
	mux.HandleFunc("/api/servers", s.handlers.handleServers)
	mux.HandleFunc("/api/servers/", s.handlers.handleServerByID)

	// Regions
	mux.HandleFunc("/api/regions", s.handlers.handleRegions)
	mux.HandleFunc("/api/regions/", s.handlers.handleRegionByName)

	// Overwatch Nodes
	mux.HandleFunc("/api/nodes/overwatch", s.handlers.handleOverwatchNodes)
	mux.HandleFunc("/api/nodes/overwatch/", s.handlers.handleOverwatchNodeByID)

	// Agent Nodes
	mux.HandleFunc("/api/nodes/agent", s.handlers.handleAgentNodes)
	mux.HandleFunc("/api/nodes/agent/", s.handlers.handleAgentNodeByID)

	// Overrides
	mux.HandleFunc("/api/overrides", s.handlers.handleOverrides)
	mux.HandleFunc("/api/overrides/", s.handlers.handleOverrideByID)

	// Health Validation
	mux.HandleFunc("/api/health/validate", s.handlers.handleHealthValidate)
	mux.HandleFunc("/api/health/validation/", s.handlers.handleValidationStatus)
	mux.HandleFunc("/api/health/status", s.handlers.handleHealthStatus)

	// Metrics
	mux.HandleFunc("/api/metrics/overview", s.handlers.handleMetricsOverview)
	mux.HandleFunc("/api/metrics/history", s.handlers.handleMetricsHistory)
	mux.HandleFunc("/api/metrics/per-node", s.handlers.handleMetricsPerNode)
	mux.HandleFunc("/api/metrics/per-region", s.handlers.handleMetricsPerRegion)
	mux.HandleFunc("/api/metrics/health-summary", s.handlers.handleMetricsHealthSummary)
	mux.HandleFunc("/api/metrics/routing-distribution", s.handlers.handleMetricsRoutingDistribution)
	mux.HandleFunc("/api/metrics/routing-flows", s.handlers.handleMetricsRoutingFlows)
	mux.HandleFunc("/api/metrics/routing-decisions", s.handlers.handleMetricsRoutingDecisions)

	// Gossip
	mux.HandleFunc("/api/gossip/nodes", s.handlers.handleGossipNodes)
	mux.HandleFunc("/api/gossip/nodes/", s.handlers.handleGossipNodeByID)
	mux.HandleFunc("/api/gossip/config", s.handlers.handleGossipConfig)
	mux.HandleFunc("/api/gossip/config/generate-key", s.handlers.handleGossipGenerateKey)

	// Geo Mappings
	mux.HandleFunc("/api/geo-mappings", s.handlers.handleGeoMappings)
	mux.HandleFunc("/api/geo-mappings/", s.handlers.handleGeoMappingByID)
	mux.HandleFunc("/api/geolocation/config", s.handlers.handleGeolocationConfig)
	mux.HandleFunc("/api/geolocation/lookup", s.handlers.handleGeolocationLookup)

	// DNSSEC
	mux.HandleFunc("/api/dnssec/status", s.handlers.handleDNSSECStatus)
	mux.HandleFunc("/api/dnssec/keys", s.handlers.handleDNSSECKeys)
	mux.HandleFunc("/api/dnssec/keys/generate", s.handlers.handleDNSSECKeysGenerate)
	mux.HandleFunc("/api/dnssec/keys/", s.handlers.handleDNSSECKeyByID)
	mux.HandleFunc("/api/dnssec/sync", s.handlers.handleDNSSECSync)
	mux.HandleFunc("/api/dnssec/sync/status", s.handlers.handleDNSSECSyncStatus)

	// Audit Logs
	mux.HandleFunc("/api/audit-logs", s.handlers.handleAuditLogs)
	mux.HandleFunc("/api/audit-logs/stats", s.handlers.handleAuditLogsStats)
	mux.HandleFunc("/api/audit-logs/export", s.handlers.handleAuditLogsExport)
	mux.HandleFunc("/api/audit-logs/", s.handlers.handleAuditLogByID)

	// Configuration
	mux.HandleFunc("/api/preferences", s.handlers.handlePreferences)
	mux.HandleFunc("/api/config/api-settings", s.handlers.handleAPISettings)
	mux.HandleFunc("/api/config/validation", s.handlers.handleValidationConfig)
	mux.HandleFunc("/api/config/stale-handling", s.handlers.handleStaleConfig)

	// Routing
	mux.HandleFunc("/api/routing/algorithms", s.handlers.handleRoutingAlgorithms)
	mux.HandleFunc("/api/routing/test", s.handlers.handleRoutingTest)
	mux.HandleFunc("/api/routing/decisions", s.handlers.handleRoutingDecisions)
	mux.HandleFunc("/api/routing/flows", s.handlers.handleRoutingFlows)

	s.logger.Debug("Dashboard API routes registered")
}

// corsMiddleware adds CORS headers for development.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range s.config.AllowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// aclMiddleware checks if the request is from an allowed network.
func (s *Server) aclMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAllowed(r) {
			writeError(w, http.StatusForbidden, "Forbidden", "ACCESS_DENIED")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isAllowed checks if the request IP is in the allowed networks.
func (s *Server) isAllowed(r *http.Request) bool {
	if len(s.config.AllowedNetworks) == 0 {
		return true // No ACL configured - allow all
	}

	clientIP := s.getClientIP(r)
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
	if s.config.TrustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			for i, c := range xff {
				if c == ',' {
					return strings.TrimSpace(xff[:i])
				}
			}
			return strings.TrimSpace(xff)
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return xri
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ============================================================================
// Health Handler
// ============================================================================

// handleHealth handles GET /api/health
func (h *Handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Calculate uptime (approximation - would need actual start time)
	uptime := int64(time.Since(time.Now().Add(-24 * time.Hour)).Seconds())

	writeJSON(w, http.StatusOK, HealthCheckResponse{
		Status:  "ok",
		Version: version.Version,
		Uptime:  uptime,
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Can't do much here, response already started
		return
	}
}

func writeError(w http.ResponseWriter, status int, message string, code string) {
	writeJSON(w, status, ErrorResponse{
		Error:   true,
		Message: message,
		Code:    code,
	})
}

// getUser extracts the user from request headers.
func getUser(r *http.Request) string {
	user := r.Header.Get("X-User")
	if user == "" {
		user = "api"
	}
	return user
}

// parsePathParam extracts a parameter from URL path.
// For path /api/domains/example.com, with prefix /api/domains/, returns "example.com"
func parsePathParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	param := strings.TrimPrefix(path, prefix)
	// Remove trailing slash if present
	param = strings.TrimSuffix(param, "/")
	// Remove any additional path segments
	if idx := strings.Index(param, "/"); idx != -1 {
		param = param[:idx]
	}
	return param
}

// parseSubPath extracts a sub-path after the ID.
// For path /api/domains/example.com/backends, with prefix /api/domains/,
// returns id="example.com", subPath="backends"
func parseSubPath(path, prefix string) (id string, subPath string) {
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	remaining := strings.TrimPrefix(path, prefix)
	remaining = strings.TrimSuffix(remaining, "/")
	parts := strings.SplitN(remaining, "/", 2)
	if len(parts) == 0 {
		return "", ""
	}
	id = parts[0]
	if len(parts) > 1 {
		subPath = parts[1]
	}
	return id, subPath
}
