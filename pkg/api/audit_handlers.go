// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AuditProvider defines the interface for audit log operations.
type AuditProvider interface {
	// ListAuditLogs returns audit log entries with pagination and filtering.
	ListAuditLogs(filter AuditFilter) ([]AuditEntry, int, error)
	// GetAuditEntry returns a single audit entry by ID.
	GetAuditEntry(id string) (*AuditEntry, error)
	// GetAuditStats returns audit log statistics.
	GetAuditStats() (*AuditStats, error)
	// ExportAuditLogs exports audit logs in the specified format.
	ExportAuditLogs(filter AuditFilter, format string) ([]byte, error)
}

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	ID         string                 `json:"id"`
	Timestamp  time.Time              `json:"timestamp"`
	Action     string                 `json:"action"`
	Resource   string                 `json:"resource"`
	ResourceID string                 `json:"resource_id,omitempty"`
	Actor      string                 `json:"actor"`
	ActorType  string                 `json:"actor_type"` // user, api, system
	ActorIP    string                 `json:"actor_ip,omitempty"`
	Status     string                 `json:"status"` // success, failure
	Details    map[string]interface{} `json:"details,omitempty"`
	Before     map[string]interface{} `json:"before,omitempty"`
	After      map[string]interface{} `json:"after,omitempty"`
	Duration   int64                  `json:"duration_ms,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
}

// AuditFilter contains parameters for filtering audit logs.
type AuditFilter struct {
	StartTime  *time.Time `json:"start_time,omitempty"`
	EndTime    *time.Time `json:"end_time,omitempty"`
	Actions    []string   `json:"actions,omitempty"`
	Resources  []string   `json:"resources,omitempty"`
	Actors     []string   `json:"actors,omitempty"`
	ActorTypes []string   `json:"actor_types,omitempty"`
	Status     string     `json:"status,omitempty"`
	Search     string     `json:"search,omitempty"`
	Limit      int        `json:"limit,omitempty"`
	Offset     int        `json:"offset,omitempty"`
	SortBy     string     `json:"sort_by,omitempty"`
	SortOrder  string     `json:"sort_order,omitempty"` // asc, desc
}

// AuditStats contains audit log statistics.
type AuditStats struct {
	TotalEntries   int64                `json:"total_entries"`
	EntriesLast24h int64                `json:"entries_last_24h"`
	EntriesLast7d  int64                `json:"entries_last_7d"`
	ByAction       map[string]int64     `json:"by_action"`
	ByResource     map[string]int64     `json:"by_resource"`
	ByStatus       map[string]int64     `json:"by_status"`
	ByActorType    map[string]int64     `json:"by_actor_type"`
	TopActors      []ActorActivityCount `json:"top_actors"`
	RetentionDays  int                  `json:"retention_days"`
	OldestEntry    *time.Time           `json:"oldest_entry,omitempty"`
	NewestEntry    *time.Time           `json:"newest_entry,omitempty"`
}

// ActorActivityCount represents activity count for an actor.
type ActorActivityCount struct {
	Actor string `json:"actor"`
	Count int64  `json:"count"`
}

// AuditListResponse is the response for GET /api/v1/audit-logs.
type AuditListResponse struct {
	Entries     []AuditEntry `json:"entries"`
	Total       int          `json:"total"`
	Limit       int          `json:"limit"`
	Offset      int          `json:"offset"`
	GeneratedAt time.Time    `json:"generated_at"`
}

// AuditEntryResponse is the response for single audit entry operations.
type AuditEntryResponse struct {
	Entry AuditEntry `json:"entry"`
}

// AuditStatsResponse is the response for GET /api/v1/audit-logs/stats.
type AuditStatsResponse struct {
	Stats       AuditStats `json:"stats"`
	GeneratedAt time.Time  `json:"generated_at"`
}

// AuditHandlers provides HTTP handlers for audit log API endpoints.
type AuditHandlers struct {
	provider AuditProvider
	logger   *slog.Logger
}

// NewAuditHandlers creates a new AuditHandlers instance.
func NewAuditHandlers(provider AuditProvider, logger *slog.Logger) *AuditHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditHandlers{
		provider: provider,
		logger:   logger,
	}
}

// HandleAuditLogs routes /api/v1/audit-logs requests based on HTTP method and path.
func (h *AuditHandlers) HandleAuditLogs(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/audit-logs")
	path = strings.TrimPrefix(path, "/")

	parts := strings.SplitN(path, "/", 2)

	// Handle /api/v1/audit-logs
	if parts[0] == "" {
		if r.Method != http.MethodGet {
			h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.listAuditLogs(w, r)
		return
	}

	// Handle sub-resources
	switch parts[0] {
	case "stats":
		h.handleStats(w, r)
	case "export":
		h.handleExport(w, r)
	default:
		// Treat as entry ID
		h.getAuditEntry(w, r, parts[0])
	}
}

// listAuditLogs handles GET /api/v1/audit-logs.
func (h *AuditHandlers) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "audit provider not configured")
		return
	}

	// Parse query parameters into filter
	filter := h.parseAuditFilter(r)

	entries, total, err := h.provider.ListAuditLogs(filter)
	if err != nil {
		h.logger.Error("failed to list audit logs", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to list audit logs: "+err.Error())
		return
	}

	resp := AuditListResponse{
		Entries:     entries,
		Total:       total,
		Limit:       filter.Limit,
		Offset:      filter.Offset,
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// getAuditEntry handles GET /api/v1/audit-logs/{id}.
func (h *AuditHandlers) getAuditEntry(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "audit provider not configured")
		return
	}

	entry, err := h.provider.GetAuditEntry(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "audit entry not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, AuditEntryResponse{Entry: *entry})
}

// handleStats handles GET /api/v1/audit-logs/stats.
func (h *AuditHandlers) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "audit provider not configured")
		return
	}

	stats, err := h.provider.GetAuditStats()
	if err != nil {
		h.logger.Error("failed to get audit stats", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to get audit stats: "+err.Error())
		return
	}

	resp := AuditStatsResponse{
		Stats:       *stats,
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// handleExport handles GET /api/v1/audit-logs/export.
func (h *AuditHandlers) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "audit provider not configured")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	filter := h.parseAuditFilter(r)

	data, err := h.provider.ExportAuditLogs(filter, format)
	if err != nil {
		h.logger.Error("failed to export audit logs", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to export audit logs: "+err.Error())
		return
	}

	// Set appropriate content type based on format
	var contentType string
	var filename string
	switch format {
	case "csv":
		contentType = "text/csv"
		filename = "audit-logs.csv"
	case "json":
		contentType = "application/json"
		filename = "audit-logs.json"
	default:
		contentType = "application/octet-stream"
		filename = "audit-logs." + format
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// parseAuditFilter parses query parameters into an AuditFilter.
func (h *AuditHandlers) parseAuditFilter(r *http.Request) AuditFilter {
	filter := AuditFilter{
		Limit:     100, // Default limit
		Offset:    0,
		SortBy:    "timestamp",
		SortOrder: "desc",
	}

	// Parse time range
	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartTime = &t
		}
	}
	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndTime = &t
		}
	}

	// Parse array filters
	if actions := r.URL.Query().Get("actions"); actions != "" {
		filter.Actions = strings.Split(actions, ",")
	}
	if resources := r.URL.Query().Get("resources"); resources != "" {
		filter.Resources = strings.Split(resources, ",")
	}
	if actors := r.URL.Query().Get("actors"); actors != "" {
		filter.Actors = strings.Split(actors, ",")
	}
	if actorTypes := r.URL.Query().Get("actor_types"); actorTypes != "" {
		filter.ActorTypes = strings.Split(actorTypes, ",")
	}

	// Parse simple filters
	filter.Status = r.URL.Query().Get("status")
	filter.Search = r.URL.Query().Get("search")

	// Parse pagination
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	// Parse sorting
	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		filter.SortBy = sortBy
	}
	if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
		filter.SortOrder = sortOrder
	}

	return filter
}

// writeJSON writes a JSON response with the given status code.
func (h *AuditHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *AuditHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
