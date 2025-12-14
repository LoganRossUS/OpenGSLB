// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig holds CORS middleware configuration.
type CORSConfig struct {
	// AllowedOrigins is a list of allowed origins. Use "*" to allow all origins.
	AllowedOrigins []string
	// AllowedMethods is a list of allowed HTTP methods.
	AllowedMethods []string
	// AllowedHeaders is a list of allowed request headers.
	AllowedHeaders []string
	// ExposedHeaders is a list of headers that browsers are allowed to access.
	ExposedHeaders []string
	// AllowCredentials indicates whether the request can include user credentials.
	AllowCredentials bool
	// MaxAge indicates how long (in seconds) the results of a preflight request can be cached.
	MaxAge int
}

// DefaultCORSConfig returns a default CORS configuration suitable for development.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-Requested-With",
			"X-Request-ID",
		},
		ExposedHeaders: []string{
			"X-Request-ID",
			"X-Total-Count",
		},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// CORSMiddleware creates a CORS middleware handler.
func CORSMiddleware(config CORSConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowedOrigin := ""
		if len(config.AllowedOrigins) > 0 {
			for _, o := range config.AllowedOrigins {
				if o == "*" {
					allowedOrigin = "*"
					break
				}
				if o == origin {
					allowedOrigin = origin
					break
				}
			}
		}

		// Set CORS headers if origin is allowed
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)

			if config.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if len(config.ExposedHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
			}
		}

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			if len(config.AllowedMethods) > 0 {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
			}

			if len(config.AllowedHeaders) > 0 {
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
			}

			if config.MaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
			}

			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CORSHandler wraps a handler function with CORS middleware.
func CORSHandler(config CORSConfig, next http.HandlerFunc) http.HandlerFunc {
	handler := CORSMiddleware(config, http.HandlerFunc(next))
	return handler.ServeHTTP
}
