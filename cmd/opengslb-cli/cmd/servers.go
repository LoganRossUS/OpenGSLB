// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// BackendResponse represents a backend from the API.
type BackendResponse struct {
	Service          string        `json:"service"`
	Address          string        `json:"address"`
	Port             int           `json:"port"`
	Weight           int           `json:"weight"`
	Region           string        `json:"region"`
	AgentID          string        `json:"agent_id"`
	AgentHealthy     bool          `json:"agent_healthy"`
	AgentLastSeen    time.Time     `json:"agent_last_seen"`
	ValidationHealthy *bool        `json:"validation_healthy,omitempty"`
	ValidationLastCheck time.Time  `json:"validation_last_check,omitempty"`
	ValidationError  string        `json:"validation_error,omitempty"`
	OverrideStatus   *bool         `json:"override_status,omitempty"`
	OverrideReason   string        `json:"override_reason,omitempty"`
	OverrideBy       string        `json:"override_by,omitempty"`
	EffectiveStatus  string        `json:"effective_status"`
	SmoothedLatency  time.Duration `json:"smoothed_latency,omitempty"`
	LatencySamples   int           `json:"latency_samples,omitempty"`
}

// BackendsListResponse is the API response for /api/v1/overwatch/backends.
type BackendsListResponse struct {
	Backends    []BackendResponse `json:"backends"`
	GeneratedAt time.Time         `json:"generated_at"`
}

var (
	serversFilterService string
	serversFilterRegion  string
	serversFilterStatus  string
)

var serversCmd = &cobra.Command{
	Use:   "servers",
	Short: "List backend servers with health status",
	Long:  `Display all registered backend servers with their health status and metadata.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewAPIClient()

		var response BackendsListResponse
		if err := client.Get("/api/v1/overwatch/backends", &response); err != nil {
			return fmt.Errorf("failed to get backends: %w", err)
		}

		// Filter backends
		backends := filterBackends(response.Backends)

		if jsonOutput {
			return formatter.Print(backends)
		}

		// Human-readable table output
		headers := []string{"SERVICE", "ADDRESS", "REGION", "STATUS", "LATENCY", "AUTHORITY"}
		rows := make([][]string, 0, len(backends))

		for _, b := range backends {
			address := fmt.Sprintf("%s:%d", b.Address, b.Port)

			// Format latency
			latency := "-"
			if b.LatencySamples > 0 {
				latency = fmt.Sprintf("%dms", b.SmoothedLatency.Milliseconds())
			}

			// Determine authority
			authority := getAuthority(b)

			rows = append(rows, []string{
				b.Service,
				address,
				b.Region,
				b.EffectiveStatus,
				latency,
				authority,
			})
		}

		formatter.PrintTable(headers, rows)
		return nil
	},
}

func init() {
	serversCmd.Flags().StringVar(&serversFilterService, "service", "", "Filter by service name")
	serversCmd.Flags().StringVar(&serversFilterRegion, "region", "", "Filter by region")
	serversCmd.Flags().StringVar(&serversFilterStatus, "status", "", "Filter by status (healthy, unhealthy, stale)")
}

// filterBackends filters backends based on command flags.
func filterBackends(backends []BackendResponse) []BackendResponse {
	if serversFilterService == "" && serversFilterRegion == "" && serversFilterStatus == "" {
		return backends
	}

	result := make([]BackendResponse, 0)
	for _, b := range backends {
		if serversFilterService != "" && b.Service != serversFilterService {
			continue
		}
		if serversFilterRegion != "" && b.Region != serversFilterRegion {
			continue
		}
		if serversFilterStatus != "" && b.EffectiveStatus != serversFilterStatus {
			continue
		}
		result = append(result, b)
	}
	return result
}

// getAuthority returns the source of the health determination.
func getAuthority(b BackendResponse) string {
	if b.OverrideStatus != nil {
		return "override"
	}
	if b.ValidationHealthy != nil {
		if *b.ValidationHealthy {
			return "overwatch"
		}
		return "overwatch (veto)"
	}
	if b.EffectiveStatus == "stale" {
		return "stale"
	}
	return "agent"
}
