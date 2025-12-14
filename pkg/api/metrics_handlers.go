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

// MetricsProvider defines the interface for metrics operations.
type MetricsProvider interface {
	// GetOverview returns the system metrics overview.
	GetOverview() (*MetricsOverview, error)
	// GetHistory returns historical metrics data.
	GetHistory(filter MetricsHistoryFilter) ([]MetricsDataPoint, error)
	// GetNodeMetrics returns metrics for a specific node.
	GetNodeMetrics(nodeID string) (*NodeMetrics, error)
	// GetRegionMetrics returns metrics for a specific region.
	GetRegionMetrics(regionID string) (*RegionMetrics, error)
	// GetRoutingStats returns routing decision statistics.
	GetRoutingStats() (*RoutingStats, error)
}

// MetricsOverview represents the system metrics overview.
type MetricsOverview struct {
	Timestamp          time.Time         `json:"timestamp"`
	Uptime             int64             `json:"uptime_seconds"`
	QueriesTotal       int64             `json:"queries_total"`
	QueriesPerSec      float64           `json:"queries_per_sec"`
	HealthChecksTotal  int64             `json:"health_checks_total"`
	HealthChecksPerSec float64           `json:"health_checks_per_sec"`
	ActiveDomains      int               `json:"active_domains"`
	ActiveServers      int               `json:"active_servers"`
	HealthyServers     int               `json:"healthy_servers"`
	UnhealthyServers   int               `json:"unhealthy_servers"`
	ActiveRegions      int               `json:"active_regions"`
	OverwatchNodes     int               `json:"overwatch_nodes"`
	AgentNodes         int               `json:"agent_nodes"`
	DNSSECEnabled      bool              `json:"dnssec_enabled"`
	GossipEnabled      bool              `json:"gossip_enabled"`
	ResponseTimes      ResponseTimeStats `json:"response_times"`
	ErrorRate          float64           `json:"error_rate"`
	CacheHitRate       float64           `json:"cache_hit_rate"`
	Memory             MemoryStats       `json:"memory"`
	CPU                CPUStats          `json:"cpu"`
}

// ResponseTimeStats contains response time statistics.
type ResponseTimeStats struct {
	Avg float64 `json:"avg_ms"`
	P50 float64 `json:"p50_ms"`
	P95 float64 `json:"p95_ms"`
	P99 float64 `json:"p99_ms"`
	Max float64 `json:"max_ms"`
}

// MemoryStats contains memory usage statistics.
type MemoryStats struct {
	Used      int64   `json:"used_bytes"`
	Available int64   `json:"available_bytes"`
	Total     int64   `json:"total_bytes"`
	Percent   float64 `json:"percent"`
}

// CPUStats contains CPU usage statistics.
type CPUStats struct {
	Used   float64 `json:"used_percent"`
	System float64 `json:"system_percent"`
	User   float64 `json:"user_percent"`
	Idle   float64 `json:"idle_percent"`
	Cores  int     `json:"cores"`
}

// MetricsHistoryFilter contains parameters for filtering historical metrics.
type MetricsHistoryFilter struct {
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Metrics    []string  `json:"metrics"`
	Resolution string    `json:"resolution"` // 1m, 5m, 1h, 1d
	NodeID     string    `json:"node_id,omitempty"`
	RegionID   string    `json:"region_id,omitempty"`
}

// MetricsDataPoint represents a single metrics data point.
type MetricsDataPoint struct {
	Timestamp time.Time          `json:"timestamp"`
	Values    map[string]float64 `json:"values"`
	Labels    map[string]string  `json:"labels,omitempty"`
}

// NodeMetrics contains metrics for a specific node.
type NodeMetrics struct {
	NodeID            string            `json:"node_id"`
	NodeType          string            `json:"node_type"` // overwatch, agent
	Timestamp         time.Time         `json:"timestamp"`
	Status            string            `json:"status"`
	Uptime            int64             `json:"uptime_seconds"`
	QueriesTotal      int64             `json:"queries_total,omitempty"`
	QueriesPerSec     float64           `json:"queries_per_sec,omitempty"`
	ChecksTotal       int64             `json:"checks_total,omitempty"`
	ChecksPerSec      float64           `json:"checks_per_sec,omitempty"`
	ResponseTimes     ResponseTimeStats `json:"response_times"`
	ErrorRate         float64           `json:"error_rate"`
	Memory            MemoryStats       `json:"memory"`
	CPU               CPUStats          `json:"cpu"`
	ConnectionsActive int               `json:"connections_active"`
}

// RegionMetrics contains metrics for a specific region.
type RegionMetrics struct {
	RegionID       string            `json:"region_id"`
	Timestamp      time.Time         `json:"timestamp"`
	TotalServers   int               `json:"total_servers"`
	HealthyServers int               `json:"healthy_servers"`
	QueriesTotal   int64             `json:"queries_total"`
	QueriesPerSec  float64           `json:"queries_per_sec"`
	ResponseTimes  ResponseTimeStats `json:"response_times"`
	ErrorRate      float64           `json:"error_rate"`
	TrafficPercent float64           `json:"traffic_percent"`
}

// RoutingStats contains routing decision statistics.
type RoutingStats struct {
	Timestamp       time.Time        `json:"timestamp"`
	TotalDecisions  int64            `json:"total_decisions"`
	ByAlgorithm     map[string]int64 `json:"by_algorithm"`
	ByRegion        map[string]int64 `json:"by_region"`
	ByOutcome       map[string]int64 `json:"by_outcome"`
	GeoRoutingHits  int64            `json:"geo_routing_hits"`
	FailoverEvents  int64            `json:"failover_events"`
	AvgDecisionTime float64          `json:"avg_decision_time_us"`
}

// MetricsOverviewResponse is the response for GET /api/v1/metrics/overview.
type MetricsOverviewResponse struct {
	Overview MetricsOverview `json:"overview"`
}

// MetricsHistoryResponse is the response for GET /api/v1/metrics/history.
type MetricsHistoryResponse struct {
	DataPoints  []MetricsDataPoint `json:"data_points"`
	Total       int                `json:"total"`
	GeneratedAt time.Time          `json:"generated_at"`
}

// NodeMetricsResponse is the response for GET /api/v1/metrics/nodes/{id}.
type NodeMetricsResponse struct {
	Metrics NodeMetrics `json:"metrics"`
}

// RegionMetricsResponse is the response for GET /api/v1/metrics/regions/{id}.
type RegionMetricsResponse struct {
	Metrics RegionMetrics `json:"metrics"`
}

// RoutingStatsResponse is the response for GET /api/v1/metrics/routing.
type RoutingStatsResponse struct {
	Stats RoutingStats `json:"stats"`
}

// MetricsHandlers provides HTTP handlers for metrics API endpoints.
type MetricsHandlers struct {
	provider MetricsProvider
	logger   *slog.Logger
}

// NewMetricsHandlers creates a new MetricsHandlers instance.
func NewMetricsHandlers(provider MetricsProvider, logger *slog.Logger) *MetricsHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &MetricsHandlers{
		provider: provider,
		logger:   logger,
	}
}

// HandleMetrics routes /api/v1/metrics requests based on HTTP method and path.
func (h *MetricsHandlers) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse the path to determine which handler to use
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/metrics")
	path = strings.TrimPrefix(path, "/")

	parts := strings.SplitN(path, "/", 2)

	// Handle /api/v1/metrics or /api/v1/metrics/overview
	if parts[0] == "" || parts[0] == "overview" {
		h.handleOverview(w, r)
		return
	}

	// Handle sub-resources
	var subID string
	if len(parts) > 1 {
		subID = parts[1]
	}

	switch parts[0] {
	case "history":
		h.handleHistory(w, r)
	case "nodes":
		h.handleNodeMetrics(w, r, subID)
	case "regions":
		h.handleRegionMetrics(w, r, subID)
	case "routing":
		h.handleRoutingStats(w, r)
	default:
		h.writeError(w, http.StatusNotFound, "unknown endpoint: "+parts[0])
	}
}

// handleOverview handles GET /api/v1/metrics/overview.
func (h *MetricsHandlers) handleOverview(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "metrics provider not configured")
		return
	}

	overview, err := h.provider.GetOverview()
	if err != nil {
		h.logger.Error("failed to get metrics overview", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to get metrics overview: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, MetricsOverviewResponse{Overview: *overview})
}

// handleHistory handles GET /api/v1/metrics/history.
func (h *MetricsHandlers) handleHistory(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "metrics provider not configured")
		return
	}

	filter := h.parseHistoryFilter(r)

	dataPoints, err := h.provider.GetHistory(filter)
	if err != nil {
		h.logger.Error("failed to get metrics history", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to get metrics history: "+err.Error())
		return
	}

	resp := MetricsHistoryResponse{
		DataPoints:  dataPoints,
		Total:       len(dataPoints),
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// handleNodeMetrics handles GET /api/v1/metrics/nodes/{id}.
func (h *MetricsHandlers) handleNodeMetrics(w http.ResponseWriter, r *http.Request, nodeID string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "metrics provider not configured")
		return
	}

	if nodeID == "" {
		h.writeError(w, http.StatusBadRequest, "node ID is required")
		return
	}

	metrics, err := h.provider.GetNodeMetrics(nodeID)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "node metrics not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, NodeMetricsResponse{Metrics: *metrics})
}

// handleRegionMetrics handles GET /api/v1/metrics/regions/{id}.
func (h *MetricsHandlers) handleRegionMetrics(w http.ResponseWriter, r *http.Request, regionID string) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "metrics provider not configured")
		return
	}

	if regionID == "" {
		h.writeError(w, http.StatusBadRequest, "region ID is required")
		return
	}

	metrics, err := h.provider.GetRegionMetrics(regionID)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "region metrics not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, RegionMetricsResponse{Metrics: *metrics})
}

// handleRoutingStats handles GET /api/v1/metrics/routing.
func (h *MetricsHandlers) handleRoutingStats(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "metrics provider not configured")
		return
	}

	stats, err := h.provider.GetRoutingStats()
	if err != nil {
		h.logger.Error("failed to get routing stats", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to get routing stats: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, RoutingStatsResponse{Stats: *stats})
}

// parseHistoryFilter parses query parameters into a MetricsHistoryFilter.
func (h *MetricsHandlers) parseHistoryFilter(r *http.Request) MetricsHistoryFilter {
	// Default to last hour
	endTime := time.Now().UTC()
	startTime := endTime.Add(-1 * time.Hour)

	filter := MetricsHistoryFilter{
		StartTime:  startTime,
		EndTime:    endTime,
		Resolution: "1m",
	}

	// Parse time range
	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartTime = t
		}
	}
	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndTime = t
		}
	}

	// Parse duration shorthand (e.g., "1h", "24h", "7d")
	if duration := r.URL.Query().Get("duration"); duration != "" {
		if d, err := parseDuration(duration); err == nil {
			filter.StartTime = filter.EndTime.Add(-d)
		}
	}

	// Parse metrics list
	if metrics := r.URL.Query().Get("metrics"); metrics != "" {
		filter.Metrics = strings.Split(metrics, ",")
	}

	// Parse resolution
	if resolution := r.URL.Query().Get("resolution"); resolution != "" {
		filter.Resolution = resolution
	}

	// Parse optional node/region filters
	filter.NodeID = r.URL.Query().Get("node_id")
	filter.RegionID = r.URL.Query().Get("region_id")

	return filter
}

// parseDuration parses duration strings like "1h", "24h", "7d".
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, nil
	}

	unit := s[len(s)-1]
	valueStr := s[:len(s)-1]
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, err
	}

	switch unit {
	case 'm':
		return time.Duration(value) * time.Minute, nil
	case 'h':
		return time.Duration(value) * time.Hour, nil
	case 'd':
		return time.Duration(value) * 24 * time.Hour, nil
	default:
		return time.Duration(value) * time.Second, nil
	}
}

// writeJSON writes a JSON response with the given status code.
func (h *MetricsHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *MetricsHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
