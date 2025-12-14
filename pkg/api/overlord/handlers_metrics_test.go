// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overlord

import (
	"net/http"
	"testing"
)

func TestHandleMetricsOverview_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsOverview, http.MethodGet, "/api/metrics/overview", "")
	assertStatus(t, rr, http.StatusOK)

	var resp MetricsOverviewResponse
	decodeJSON(t, rr, &resp)

	if resp.SystemStats.TotalDomains != 2 {
		t.Errorf("expected 2 total domains, got %d", resp.SystemStats.TotalDomains)
	}
	if resp.SystemStats.TotalRegions != 2 {
		t.Errorf("expected 2 total regions, got %d", resp.SystemStats.TotalRegions)
	}
}

func TestHandleMetricsOverview_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsOverview, http.MethodPost, "/api/metrics/overview", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleMetricsHistory_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsHistory, http.MethodGet, "/api/metrics/history", "")
	assertStatus(t, rr, http.StatusOK)

	var resp MetricsHistoryResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Metrics) != 24 {
		t.Errorf("expected 24 data points, got %d", len(resp.Metrics))
	}
}

func TestHandleMetricsHistory_GET_WithInterval(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsHistory, http.MethodGet, "/api/metrics/history?interval=15m", "")
	assertStatus(t, rr, http.StatusOK)
}

func TestHandleMetricsHistory_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsHistory, http.MethodPost, "/api/metrics/history", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleMetricsPerNode_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsPerNode, http.MethodGet, "/api/metrics/per-node", "")
	assertStatus(t, rr, http.StatusOK)

	var resp MetricsPerNodeResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Metrics) != 24 {
		t.Errorf("expected 24 data points, got %d", len(resp.Metrics))
	}
}

func TestHandleMetricsPerNode_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsPerNode, http.MethodPost, "/api/metrics/per-node", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleMetricsPerRegion_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsPerRegion, http.MethodGet, "/api/metrics/per-region", "")
	assertStatus(t, rr, http.StatusOK)

	var resp MetricsPerRegionResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Metrics) != 24 {
		t.Errorf("expected 24 data points, got %d", len(resp.Metrics))
	}
}

func TestHandleMetricsPerRegion_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsPerRegion, http.MethodPost, "/api/metrics/per-region", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleMetricsHealthSummary_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsHealthSummary, http.MethodGet, "/api/metrics/health-summary", "")
	assertStatus(t, rr, http.StatusOK)

	var resp MetricsHealthSummaryResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Regions) != 2 {
		t.Errorf("expected 2 regions, got %d", len(resp.Regions))
	}
}

func TestHandleMetricsHealthSummary_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsHealthSummary, http.MethodPost, "/api/metrics/health-summary", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleMetricsRoutingDistribution_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsRoutingDistribution, http.MethodGet, "/api/metrics/routing-distribution", "")
	assertStatus(t, rr, http.StatusOK)

	var resp MetricsRoutingDistributionResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Distribution) != 5 {
		t.Errorf("expected 5 distribution items, got %d", len(resp.Distribution))
	}
}

func TestHandleMetricsRoutingDistribution_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsRoutingDistribution, http.MethodPost, "/api/metrics/routing-distribution", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleMetricsRoutingFlows_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsRoutingFlows, http.MethodGet, "/api/metrics/routing-flows", "")
	assertStatus(t, rr, http.StatusOK)

	var resp MetricsRoutingFlowsResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Flows) == 0 {
		t.Error("expected non-empty flows")
	}
}

func TestHandleMetricsRoutingFlows_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsRoutingFlows, http.MethodPost, "/api/metrics/routing-flows", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleMetricsRoutingDecisions_GET(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsRoutingDecisions, http.MethodGet, "/api/metrics/routing-decisions", "")
	assertStatus(t, rr, http.StatusOK)

	var resp MetricsRoutingDecisionsResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Decisions) == 0 {
		t.Error("expected non-empty decisions")
	}
}

func TestHandleMetricsRoutingDecisions_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleMetricsRoutingDecisions, http.MethodPost, "/api/metrics/routing-decisions", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}
