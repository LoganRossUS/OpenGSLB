// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"net/http"
	"time"
)

// SimpleHealthResponse is the response for GET /api/health.
// This provides a simple health check with version and uptime information.
type SimpleHealthResponse struct {
	Status    string    `json:"status"` // healthy, degraded, unhealthy
	Version   string    `json:"version"`
	Uptime    int64     `json:"uptime_seconds"`
	Timestamp time.Time `json:"timestamp"`
}

// SimpleHealthProvider provides version and uptime information.
type SimpleHealthProvider interface {
	// GetVersion returns the application version.
	GetVersion() string
	// GetStartTime returns when the application started.
	GetStartTime() time.Time
	// IsHealthy returns whether the application is healthy.
	IsHealthy() bool
}

// SimpleHealthHandlers provides HTTP handlers for the simple health endpoint.
type SimpleHealthHandlers struct {
	provider SimpleHealthProvider
}

// NewSimpleHealthHandlers creates a new SimpleHealthHandlers instance.
func NewSimpleHealthHandlers(provider SimpleHealthProvider) *SimpleHealthHandlers {
	return &SimpleHealthHandlers{
		provider: provider,
	}
}

// HandleHealth handles GET /api/health requests.
func (h *SimpleHealthHandlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := "healthy"
	if h.provider != nil && !h.provider.IsHealthy() {
		status = "unhealthy"
	}

	version := "unknown"
	var uptime int64

	if h.provider != nil {
		version = h.provider.GetVersion()
		startTime := h.provider.GetStartTime()
		if !startTime.IsZero() {
			uptime = int64(time.Since(startTime).Seconds())
		}
	}

	resp := SimpleHealthResponse{
		Status:    status,
		Version:   version,
		Uptime:    uptime,
		Timestamp: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}
