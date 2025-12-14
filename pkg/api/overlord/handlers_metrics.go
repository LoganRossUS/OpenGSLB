// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overlord

import (
	"net/http"
	"time"
)

// handleMetricsOverview handles GET /api/metrics/overview
func (h *Handlers) handleMetricsOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	stats := SystemStats{}

	cfg := h.dataProvider.GetConfig()
	if cfg != nil {
		stats.TotalDomains = len(cfg.Domains)
		stats.TotalRegions = len(cfg.Regions)

		// Count overwatches (for now, just this node)
		stats.TotalOverwatches = 1
		stats.ActiveOverwatches = 1
	}

	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		backends := registry.GetAllBackends()
		agentMap := make(map[string]bool)
		var totalLatency float64
		var latencyCount int

		for _, b := range backends {
			stats.TotalBackends++

			switch string(b.EffectiveStatus) {
			case "healthy":
				stats.HealthyBackends++
			case "unhealthy":
				stats.UnhealthyBackends++
			case "stale":
				stats.StaleBackends++
			}

			if b.OverrideStatus != nil {
				stats.ActiveOverrides++
			}

			if b.AgentID != "" {
				agentMap[b.AgentID] = b.EffectiveStatus != "stale"
			}

			if b.SmoothedLatency > 0 {
				totalLatency += float64(b.SmoothedLatency.Milliseconds())
				latencyCount++
			}
		}

		stats.TotalAgents = len(agentMap)
		for _, active := range agentMap {
			if active {
				stats.ActiveAgents++
			}
		}

		if latencyCount > 0 {
			stats.AvgLatencyMs = totalLatency / float64(latencyCount)
		}
	}

	// Placeholder DNS queries (would come from actual metrics)
	stats.DNSQueriesLast24h = 1250000

	writeJSON(w, http.StatusOK, MetricsOverviewResponse{SystemStats: stats})
}

// handleMetricsHistory handles GET /api/metrics/history
func (h *Handlers) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Parse query parameters (interval reserved for future use)
	_ = r.URL.Query().Get("interval")

	// Generate sample historical data
	// In production, this would query actual metrics storage
	metrics := make([]MetricDataPoint, 0, 24)
	now := time.Now()

	for i := 23; i >= 0; i-- {
		timestamp := now.Add(-time.Duration(i) * time.Hour)
		metrics = append(metrics, MetricDataPoint{
			Timestamp:              timestamp,
			Hour:                   timestamp.Format("15:00"),
			Queries:                int64(40000 + (i * 2000)),
			LatencyP50:             float64(20 + (i % 5)),
			LatencyP95:             float64(50 + (i % 10)),
			LatencyP99:             float64(100 + (i % 20)),
			HealthCheckSuccessRate: 99.0 + float64(i%100)/100,
			RoutingDecisions: map[string]int{
				"geolocation": 15000 + (i * 500),
				"latency":     12000 + (i * 400),
				"failover":    500 + (i * 20),
				"round-robin": 8000 + (i * 300),
				"weighted":    5000 + (i * 200),
			},
		})
	}

	writeJSON(w, http.StatusOK, MetricsHistoryResponse{Metrics: metrics})
}

// handleMetricsPerNode handles GET /api/metrics/per-node
func (h *Handlers) handleMetricsPerNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Generate sample per-node metrics
	metrics := make([]PerNodeMetrics, 0, 24)
	now := time.Now()
	nodeID := cfg.Overwatch.Identity.NodeID

	for i := 23; i >= 0; i-- {
		timestamp := now.Add(-time.Duration(i) * time.Hour)
		metrics = append(metrics, PerNodeMetrics{
			Timestamp: timestamp,
			PerNode: map[string]NodeMetrics{
				nodeID: {
					Queries:    int64(45000 + (i * 1500)),
					LatencyAvg: float64(25 + (i % 10)),
					Healthy:    true,
				},
			},
		})
	}

	writeJSON(w, http.StatusOK, MetricsPerNodeResponse{Metrics: metrics})
}

// handleMetricsPerRegion handles GET /api/metrics/per-region
func (h *Handlers) handleMetricsPerRegion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	cfg := h.dataProvider.GetConfig()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "configuration not available", "CONFIG_UNAVAILABLE")
		return
	}

	// Generate sample per-region metrics
	metrics := make([]PerRegionMetrics, 0, 24)
	now := time.Now()

	for i := 23; i >= 0; i-- {
		timestamp := now.Add(-time.Duration(i) * time.Hour)
		perRegion := make(map[string]RegionMetrics)

		for _, region := range cfg.Regions {
			perRegion[region.Name] = RegionMetrics{
				Queries:       int64(10000 + (i * 500)),
				LatencyAvg:    float64(30 + (i % 15)),
				HealthPercent: 95.0 + float64(i%5),
			}
		}

		metrics = append(metrics, PerRegionMetrics{
			Timestamp: timestamp,
			PerRegion: perRegion,
		})
	}

	writeJSON(w, http.StatusOK, MetricsPerRegionResponse{Metrics: metrics})
}

// handleMetricsHealthSummary handles GET /api/metrics/health-summary
func (h *Handlers) handleMetricsHealthSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	regions := make([]HealthSummaryItem, 0)

	cfg := h.dataProvider.GetConfig()
	registry := h.dataProvider.GetBackendRegistry()

	if cfg != nil {
		for _, cr := range cfg.Regions {
			item := HealthSummaryItem{
				Region:          cr.Name,
				TotalBackends:   len(cr.Servers),
				HealthyBackends: len(cr.Servers), // Default to all healthy
			}

			if registry != nil {
				item.TotalBackends = 0
				item.HealthyBackends = 0
				var totalLatency float64
				var latencyCount int

				backends := registry.GetAllBackends()
				for _, b := range backends {
					if b.Region == cr.Name {
						item.TotalBackends++
						if b.EffectiveStatus == "healthy" {
							item.HealthyBackends++
						}
						if b.SmoothedLatency > 0 {
							totalLatency += float64(b.SmoothedLatency.Milliseconds())
							latencyCount++
						}
					}
				}

				if latencyCount > 0 {
					item.AvgLatency = totalLatency / float64(latencyCount)
				}
			}

			if item.TotalBackends > 0 {
				item.HealthPercent = float64(item.HealthyBackends) / float64(item.TotalBackends) * 100
			}

			regions = append(regions, item)
		}
	}

	writeJSON(w, http.StatusOK, MetricsHealthSummaryResponse{Regions: regions})
}

// handleMetricsRoutingDistribution handles GET /api/metrics/routing-distribution
func (h *Handlers) handleMetricsRoutingDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Count routing algorithms from configured domains
	distribution := []RoutingDistributionItem{
		{Name: "Geolocation", Value: 0, Color: "#3B82F6"},
		{Name: "Latency", Value: 0, Color: "#10B981"},
		{Name: "Failover", Value: 0, Color: "#F59E0B"},
		{Name: "Weighted", Value: 0, Color: "#8B5CF6"},
		{Name: "Round Robin", Value: 0, Color: "#EC4899"},
	}

	cfg := h.dataProvider.GetConfig()
	if cfg != nil {
		for _, domain := range cfg.Domains {
			switch domain.RoutingAlgorithm {
			case "geolocation", "geo":
				distribution[0].Value++
			case "latency":
				distribution[1].Value++
			case "failover":
				distribution[2].Value++
			case "weighted":
				distribution[3].Value++
			case "round-robin":
				distribution[4].Value++
			}
		}
	}

	// If no domains, add sample data
	if len(cfg.Domains) == 0 {
		distribution[0].Value = 45
		distribution[1].Value = 25
		distribution[2].Value = 10
		distribution[3].Value = 15
		distribution[4].Value = 5
	}

	writeJSON(w, http.StatusOK, MetricsRoutingDistributionResponse{Distribution: distribution})
}

// handleMetricsRoutingFlows handles GET /api/metrics/routing-flows
func (h *Handlers) handleMetricsRoutingFlows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	flows := make([]RoutingFlow, 0)

	cfg := h.dataProvider.GetConfig()
	if cfg != nil && len(cfg.Regions) > 1 {
		// Generate sample flows between regions
		for i, sourceRegion := range cfg.Regions {
			for j, destRegion := range cfg.Regions {
				if i == j {
					// Same region (local)
					flows = append(flows, RoutingFlow{
						Source:        sourceRegion.Name,
						Destination:   destRegion.Name,
						Value:         10000 + (i * 1000),
						IsCrossRegion: false,
					})
				} else {
					// Cross-region
					flows = append(flows, RoutingFlow{
						Source:        sourceRegion.Name,
						Destination:   destRegion.Name,
						Value:         500 + (i * 100),
						IsCrossRegion: true,
					})
				}
			}
		}
	}

	// Add sample data if no regions
	if len(flows) == 0 {
		flows = []RoutingFlow{
			{Source: "us-east-1", Destination: "us-east-1", Value: 15000, IsCrossRegion: false},
			{Source: "us-east-1", Destination: "eu-west-1", Value: 1500, IsCrossRegion: true},
			{Source: "eu-west-1", Destination: "eu-west-1", Value: 12000, IsCrossRegion: false},
			{Source: "eu-west-1", Destination: "us-east-1", Value: 800, IsCrossRegion: true},
		}
	}

	writeJSON(w, http.StatusOK, MetricsRoutingFlowsResponse{Flows: flows})
}

// handleMetricsRoutingDecisions handles GET /api/metrics/routing-decisions
func (h *Handlers) handleMetricsRoutingDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	decisions := make([]RoutingDecision, 0)

	cfg := h.dataProvider.GetConfig()
	if cfg != nil && len(cfg.Regions) > 0 {
		for _, region := range cfg.Regions {
			decision := RoutingDecision{
				SourceRegion:  region.Name,
				TotalRequests: 15000,
				Destinations:  make([]RoutingDestination, 0),
			}

			// Add local destination
			decision.Destinations = append(decision.Destinations, RoutingDestination{
				Region:        region.Name,
				Count:         12000,
				Percentage:    80.0,
				IsCrossRegion: false,
			})

			// Add cross-region destinations
			for _, otherRegion := range cfg.Regions {
				if otherRegion.Name != region.Name {
					decision.Destinations = append(decision.Destinations, RoutingDestination{
						Region:        otherRegion.Name,
						Count:         500,
						Percentage:    3.33,
						IsCrossRegion: true,
					})
				}
			}

			decisions = append(decisions, decision)
		}
	}

	// Add sample data if no regions
	if len(decisions) == 0 {
		decisions = []RoutingDecision{
			{
				SourceRegion:  "us-east-1",
				TotalRequests: 15000,
				Destinations: []RoutingDestination{
					{Region: "us-east-1", Count: 12000, Percentage: 80.0, IsCrossRegion: false},
					{Region: "eu-west-1", Count: 2000, Percentage: 13.3, IsCrossRegion: true},
					{Region: "ap-south-1", Count: 1000, Percentage: 6.7, IsCrossRegion: true},
				},
			},
		}
	}

	writeJSON(w, http.StatusOK, MetricsRoutingDecisionsResponse{Decisions: decisions})
}
