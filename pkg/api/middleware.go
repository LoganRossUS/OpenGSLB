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
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// ACLMiddleware enforces IP-based access control.
type ACLMiddleware struct {
	allowedNetworks   []*net.IPNet
	trustProxyHeaders bool
	logger            *slog.Logger
}

// NewACLMiddleware creates a new ACL middleware.
func NewACLMiddleware(networks []string, trustProxy bool, logger *slog.Logger) (*ACLMiddleware, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	var parsed []*net.IPNet
	for _, cidr := range networks {
		// Handle single IPs without CIDR notation
		if !strings.Contains(cidr, "/") {
			if strings.Contains(cidr, ":") {
				cidr = cidr + "/128" // IPv6
			} else {
				cidr = cidr + "/32" // IPv4
			}
		}

		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, ipnet)
	}

	return &ACLMiddleware{
		allowedNetworks:   parsed,
		trustProxyHeaders: trustProxy,
		logger:            logger,
	}, nil
}

// Wrap returns an http.Handler that enforces the ACL before calling the next handler.
func (m *ACLMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := m.extractClientIP(r)
		if clientIP == nil {
			m.logger.Warn("could not parse client IP",
				"remote_addr", r.RemoteAddr,
				"path", r.URL.Path,
			)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if !m.isAllowed(clientIP) {
			m.logger.Warn("access denied by ACL",
				"client_ip", clientIP.String(),
				"path", r.URL.Path,
			)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractClientIP gets the client IP from the request.
// If trustProxyHeaders is enabled, checks X-Forwarded-For first.
func (m *ACLMiddleware) extractClientIP(r *http.Request) net.IP {
	if m.trustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For can be comma-separated; first is the original client
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				ipStr := strings.TrimSpace(parts[0])
				if ip := net.ParseIP(ipStr); ip != nil {
					return ip
				}
			}
		}

		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if ip := net.ParseIP(xri); ip != nil {
				return ip
			}
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have a port
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}

// isAllowed checks if the IP is in any of the allowed networks.
func (m *ACLMiddleware) isAllowed(ip net.IP) bool {
	// If no networks configured, deny all (fail-closed)
	if len(m.allowedNetworks) == 0 {
		return false
	}

	for _, network := range m.allowedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// LoggingMiddleware logs API requests.
type LoggingMiddleware struct {
	logger *slog.Logger
}

// NewLoggingMiddleware creates a new logging middleware.
func NewLoggingMiddleware(logger *slog.Logger) *LoggingMiddleware {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &LoggingMiddleware{logger: logger}
}

// Wrap returns an http.Handler that logs requests.
func (m *LoggingMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		m.logger.Debug("api request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
