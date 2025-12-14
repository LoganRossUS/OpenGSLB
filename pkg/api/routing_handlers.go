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

// RoutingProvider defines the interface for routing operations.
type RoutingProvider interface {
	// ListAlgorithms returns available routing algorithms.
	ListAlgorithms() []RoutingAlgorithm
	// GetAlgorithm returns a specific routing algorithm by ID.
	GetAlgorithm(id string) (*RoutingAlgorithm, error)

	// TestRouting simulates routing for a given request.
	TestRouting(request RoutingTestRequest) (*RoutingTestResult, error)

	// GetDecisions returns recent routing decisions.
	GetDecisions(filter RoutingDecisionFilter) ([]RoutingDecision, int, error)

	// GetFlows returns traffic flow information.
	GetFlows(filter FlowFilter) ([]TrafficFlow, error)
}

// RoutingAlgorithm represents a routing algorithm configuration.
type RoutingAlgorithm struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        string            `json:"type"` // weighted, round-robin, least-connections, geo, failover, latency
	Parameters  map[string]string `json:"parameters,omitempty"`
	Enabled     bool              `json:"enabled"`
	Default     bool              `json:"default"`
}

// RoutingTestRequest is the request body for testing routing.
type RoutingTestRequest struct {
	Domain    string `json:"domain"`
	ClientIP  string `json:"client_ip"`
	QueryType string `json:"query_type"` // A, AAAA, CNAME, etc.
	EDNS      *EDNS  `json:"edns,omitempty"`
}

// EDNS contains EDNS client subnet information.
type EDNS struct {
	ClientSubnet string `json:"client_subnet,omitempty"`
	SourceScope  int    `json:"source_scope,omitempty"`
}

// RoutingTestResult is the result of a routing test.
type RoutingTestResult struct {
	Domain          string            `json:"domain"`
	ClientIP        string            `json:"client_ip"`
	ClientRegion    string            `json:"client_region,omitempty"`
	ClientCountry   string            `json:"client_country,omitempty"`
	Algorithm       string            `json:"algorithm"`
	SelectedBackend *SelectedBackend  `json:"selected_backend,omitempty"`
	Alternatives    []SelectedBackend `json:"alternatives,omitempty"`
	Factors         []RoutingFactor   `json:"factors"`
	Decision        string            `json:"decision"` // success, no_healthy_backend, domain_not_found
	DecisionTime    int64             `json:"decision_time_us"`
	TTL             int               `json:"ttl"`
	Timestamp       time.Time         `json:"timestamp"`
}

// SelectedBackend represents a selected backend server.
type SelectedBackend struct {
	Address  string  `json:"address"`
	Port     int     `json:"port"`
	Region   string  `json:"region"`
	Weight   int     `json:"weight"`
	Priority int     `json:"priority"`
	Healthy  bool    `json:"healthy"`
	Score    float64 `json:"score,omitempty"`
}

// RoutingFactor represents a factor that influenced routing.
type RoutingFactor struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"` // geo, health, weight, latency, custom
	Value  string  `json:"value"`
	Weight float64 `json:"weight"`
	Impact string  `json:"impact"` // positive, negative, neutral
}

// RoutingDecision represents a recorded routing decision.
type RoutingDecision struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Domain         string    `json:"domain"`
	ClientIP       string    `json:"client_ip"`
	ClientRegion   string    `json:"client_region,omitempty"`
	Algorithm      string    `json:"algorithm"`
	SelectedServer string    `json:"selected_server"`
	SelectedRegion string    `json:"selected_region"`
	DecisionTime   int64     `json:"decision_time_us"`
	Outcome        string    `json:"outcome"` // success, failover, fallback
}

// RoutingDecisionFilter contains parameters for filtering routing decisions.
type RoutingDecisionFilter struct {
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Domain    string     `json:"domain,omitempty"`
	Algorithm string     `json:"algorithm,omitempty"`
	Region    string     `json:"region,omitempty"`
	Outcome   string     `json:"outcome,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}

// TrafficFlow represents traffic flow between regions.
type TrafficFlow struct {
	SourceRegion      string    `json:"source_region"`
	DestinationRegion string    `json:"destination_region"`
	RequestCount      int64     `json:"request_count"`
	BytesTransferred  int64     `json:"bytes_transferred"`
	AvgLatency        float64   `json:"avg_latency_ms"`
	ErrorRate         float64   `json:"error_rate"`
	Timestamp         time.Time `json:"timestamp"`
}

// FlowFilter contains parameters for filtering traffic flows.
type FlowFilter struct {
	StartTime    *time.Time `json:"start_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	SourceRegion string     `json:"source_region,omitempty"`
	DestRegion   string     `json:"dest_region,omitempty"`
}

// AlgorithmListResponse is the response for GET /api/v1/routing/algorithms.
type AlgorithmListResponse struct {
	Algorithms  []RoutingAlgorithm `json:"algorithms"`
	Total       int                `json:"total"`
	GeneratedAt time.Time          `json:"generated_at"`
}

// AlgorithmResponse is the response for single algorithm operations.
type AlgorithmResponse struct {
	Algorithm RoutingAlgorithm `json:"algorithm"`
}

// RoutingTestResponse is the response for POST /api/v1/routing/test.
type RoutingTestResponse struct {
	Result RoutingTestResult `json:"result"`
}

// RoutingDecisionListResponse is the response for GET /api/v1/routing/decisions.
type RoutingDecisionListResponse struct {
	Decisions   []RoutingDecision `json:"decisions"`
	Total       int               `json:"total"`
	Limit       int               `json:"limit"`
	Offset      int               `json:"offset"`
	GeneratedAt time.Time         `json:"generated_at"`
}

// FlowListResponse is the response for GET /api/v1/routing/flows.
type FlowListResponse struct {
	Flows       []TrafficFlow `json:"flows"`
	Total       int           `json:"total"`
	GeneratedAt time.Time     `json:"generated_at"`
}

// RoutingHandlers provides HTTP handlers for routing API endpoints.
type RoutingHandlers struct {
	provider RoutingProvider
	logger   *slog.Logger
}

// NewRoutingHandlers creates a new RoutingHandlers instance.
func NewRoutingHandlers(provider RoutingProvider, logger *slog.Logger) *RoutingHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &RoutingHandlers{
		provider: provider,
		logger:   logger,
	}
}

// HandleRouting routes /api/v1/routing requests based on HTTP method and path.
func (h *RoutingHandlers) HandleRouting(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/routing")
	path = strings.TrimPrefix(path, "/")

	parts := strings.SplitN(path, "/", 2)

	if parts[0] == "" {
		h.writeError(w, http.StatusNotFound, "specify endpoint: algorithms, test, decisions, or flows")
		return
	}

	var subID string
	if len(parts) > 1 {
		subID = parts[1]
	}

	switch parts[0] {
	case "algorithms":
		h.handleAlgorithms(w, r, subID)
	case "test":
		h.handleTest(w, r)
	case "decisions":
		h.handleDecisions(w, r)
	case "flows":
		h.handleFlows(w, r)
	default:
		h.writeError(w, http.StatusNotFound, "unknown endpoint: "+parts[0])
	}
}

// handleAlgorithms handles /api/v1/routing/algorithms requests.
func (h *RoutingHandlers) handleAlgorithms(w http.ResponseWriter, r *http.Request, algorithmID string) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "routing provider not configured")
		return
	}

	// List all algorithms
	if algorithmID == "" {
		algorithms := h.provider.ListAlgorithms()
		resp := AlgorithmListResponse{
			Algorithms:  algorithms,
			Total:       len(algorithms),
			GeneratedAt: time.Now().UTC(),
		}
		h.writeJSON(w, http.StatusOK, resp)
		return
	}

	// Get single algorithm
	algorithm, err := h.provider.GetAlgorithm(algorithmID)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "algorithm not found: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, AlgorithmResponse{Algorithm: *algorithm})
}

// handleTest handles POST /api/v1/routing/test requests.
func (h *RoutingHandlers) handleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "routing provider not configured")
		return
	}

	var req RoutingTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.Domain == "" {
		h.writeError(w, http.StatusBadRequest, "domain is required")
		return
	}
	if req.ClientIP == "" {
		h.writeError(w, http.StatusBadRequest, "client_ip is required")
		return
	}

	// Set defaults
	if req.QueryType == "" {
		req.QueryType = "A"
	}

	result, err := h.provider.TestRouting(req)
	if err != nil {
		h.logger.Error("failed to test routing", "domain", req.Domain, "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to test routing: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, RoutingTestResponse{Result: *result})
}

// handleDecisions handles GET /api/v1/routing/decisions requests.
func (h *RoutingHandlers) handleDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "routing provider not configured")
		return
	}

	filter := h.parseDecisionFilter(r)

	decisions, total, err := h.provider.GetDecisions(filter)
	if err != nil {
		h.logger.Error("failed to get routing decisions", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to get routing decisions: "+err.Error())
		return
	}

	resp := RoutingDecisionListResponse{
		Decisions:   decisions,
		Total:       total,
		Limit:       filter.Limit,
		Offset:      filter.Offset,
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// handleFlows handles GET /api/v1/routing/flows requests.
func (h *RoutingHandlers) handleFlows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "routing provider not configured")
		return
	}

	filter := h.parseFlowFilter(r)

	flows, err := h.provider.GetFlows(filter)
	if err != nil {
		h.logger.Error("failed to get traffic flows", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to get traffic flows: "+err.Error())
		return
	}

	resp := FlowListResponse{
		Flows:       flows,
		Total:       len(flows),
		GeneratedAt: time.Now().UTC(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// parseDecisionFilter parses query parameters into a RoutingDecisionFilter.
func (h *RoutingHandlers) parseDecisionFilter(r *http.Request) RoutingDecisionFilter {
	filter := RoutingDecisionFilter{
		Limit:  100,
		Offset: 0,
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

	// Parse filters
	filter.Domain = r.URL.Query().Get("domain")
	filter.Algorithm = r.URL.Query().Get("algorithm")
	filter.Region = r.URL.Query().Get("region")
	filter.Outcome = r.URL.Query().Get("outcome")

	// Parse pagination
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := parseInt(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := parseInt(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	return filter
}

// parseFlowFilter parses query parameters into a FlowFilter.
func (h *RoutingHandlers) parseFlowFilter(r *http.Request) FlowFilter {
	filter := FlowFilter{}

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

	// Parse region filters
	filter.SourceRegion = r.URL.Query().Get("source_region")
	filter.DestRegion = r.URL.Query().Get("dest_region")

	return filter
}

// parseInt parses a string to int.
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

// writeJSON writes a JSON response with the given status code.
func (h *RoutingHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *RoutingHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
