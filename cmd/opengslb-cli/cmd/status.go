// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cmd

import (
	"fmt"
	"time"

	"github.com/loganrossus/OpenGSLB/cmd/opengslb-cli/output"
	"github.com/spf13/cobra"
)

// ReadyResponse is the API response for /api/v1/ready.
type ReadyResponse struct {
	Ready       bool   `json:"ready"`
	Message     string `json:"message,omitempty"`
	DNSReady    bool   `json:"dns_ready"`
	HealthReady bool   `json:"health_ready"`
}

// OverwatchStatsResponse is the API response for /api/v1/overwatch/stats.
type OverwatchStatsResponse struct {
	Mode          string    `json:"mode"`
	StartedAt     time.Time `json:"started_at"`
	AgentCount    int       `json:"agent_count"`
	BackendCount  int       `json:"backend_count"`
	HealthyCount  int       `json:"healthy_count"`
	UnhealthyCount int      `json:"unhealthy_count"`
	StaleCount    int       `json:"stale_count"`
	DomainCount   int       `json:"domain_count"`
	DNSSECEnabled bool      `json:"dnssec_enabled"`
	Zones         []string  `json:"zones"`
}

// StatusOutput is the combined status output.
type StatusOutput struct {
	Status        string    `json:"status"`
	Mode          string    `json:"mode"`
	Uptime        string    `json:"uptime"`
	AgentCount    int       `json:"agent_count"`
	BackendCount  int       `json:"backend_count"`
	HealthyCount  int       `json:"healthy_count"`
	UnhealthyCount int      `json:"unhealthy_count"`
	StaleCount    int       `json:"stale_count"`
	DomainCount   int       `json:"domain_count"`
	DNSSECEnabled bool      `json:"dnssec_enabled"`
	Zones         []string  `json:"zones"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show overall Overwatch status",
	Long:  `Display the overall health and status of the OpenGSLB Overwatch instance.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewAPIClient()

		// Get readiness status
		var ready ReadyResponse
		if err := client.Get("/api/v1/ready", &ready); err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}

		// Try to get Overwatch stats (may not be available in agent mode)
		var stats OverwatchStatsResponse
		statsErr := client.Get("/api/v1/overwatch/stats", &stats)

		// Determine overall status
		statusStr := "Healthy"
		if !ready.Ready {
			statusStr = "Degraded"
		}
		if statsErr != nil && !ready.Ready {
			statusStr = "Unhealthy"
		}

		// Calculate uptime
		var uptime string
		if stats.StartedAt.IsZero() {
			uptime = "unknown"
		} else {
			uptime = formatDuration(time.Since(stats.StartedAt))
		}

		if jsonOutput {
			output := StatusOutput{
				Status:        statusStr,
				Mode:          stats.Mode,
				Uptime:        uptime,
				AgentCount:    stats.AgentCount,
				BackendCount:  stats.BackendCount,
				HealthyCount:  stats.HealthyCount,
				UnhealthyCount: stats.UnhealthyCount,
				StaleCount:    stats.StaleCount,
				DomainCount:   stats.DomainCount,
				DNSSECEnabled: stats.DNSSECEnabled,
				Zones:         stats.Zones,
			}
			return formatter.Print(output)
		}

		// Human-readable output
		fmt.Printf("OpenGSLB Overwatch Status: %s\n", statusStr)

		pairs := []output.KVPair{
			{Key: "Mode", Value: coalesce(stats.Mode, "unknown")},
			{Key: "Uptime", Value: uptime},
		}

		if statsErr == nil {
			pairs = append(pairs,
				output.KVPair{Key: "Agents", Value: fmt.Sprintf("%d connected", stats.AgentCount)},
				output.KVPair{Key: "Backends", Value: fmt.Sprintf("%d total, %d healthy, %d unhealthy, %d stale",
					stats.BackendCount, stats.HealthyCount, stats.UnhealthyCount, stats.StaleCount)},
				output.KVPair{Key: "Domains", Value: fmt.Sprintf("%d configured", stats.DomainCount)},
			)

			dnssecStatus := "Enabled"
			if !stats.DNSSECEnabled {
				dnssecStatus = "Disabled"
			}
			if len(stats.Zones) > 0 {
				dnssecStatus += fmt.Sprintf(" (zones: %v)", stats.Zones)
			}
			pairs = append(pairs, output.KVPair{Key: "DNSSEC", Value: dnssecStatus})
		}

		formatter.PrintKeyValue(pairs)

		if !ready.Ready && ready.Message != "" {
			fmt.Printf("\nWarning: %s\n", ready.Message)
		}

		return nil
	},
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// coalesce returns the first non-empty string.
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
