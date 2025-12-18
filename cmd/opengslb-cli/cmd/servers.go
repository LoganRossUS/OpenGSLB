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
	Service             string        `json:"service"`
	Address             string        `json:"address"`
	Port                int           `json:"port"`
	Weight              int           `json:"weight"`
	Region              string        `json:"region"`
	AgentID             string        `json:"agent_id"`
	AgentHealthy        bool          `json:"agent_healthy"`
	AgentLastSeen       time.Time     `json:"agent_last_seen"`
	ValidationHealthy   *bool         `json:"validation_healthy,omitempty"`
	ValidationLastCheck time.Time     `json:"validation_last_check,omitempty"`
	ValidationError     string        `json:"validation_error,omitempty"`
	OverrideStatus      *bool         `json:"override_status,omitempty"`
	OverrideReason      string        `json:"override_reason,omitempty"`
	OverrideBy          string        `json:"override_by,omitempty"`
	EffectiveStatus     string        `json:"effective_status"`
	SmoothedLatency     time.Duration `json:"smoothed_latency,omitempty"`
	LatencySamples      int           `json:"latency_samples,omitempty"`
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
	Short: "Manage backend servers",
	Long:  `Manage backend servers via the OpenGSLB API. List, create, update, and delete servers.`,
}

var serversListCmd = &cobra.Command{
	Use:   "list",
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

var (
	createServerName     string
	createServerAddress  string
	createServerPort     int
	createServerWeight   int
	createServerRegion   string
	createServerService  string
	createServerProtocol string
)

var serversCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new backend server",
	Long:  `Register a new backend server via the API.`,
	Example: `  opengslb-cli servers create --address 10.0.1.50 --port 8080 --service app.example.com --region us-east --weight 100
  opengslb-cli servers create --address 10.0.2.50 --port 80 --service webapp.example.com --region eu-west`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if createServerAddress == "" {
			return fmt.Errorf("--address is required")
		}
		if createServerPort == 0 {
			return fmt.Errorf("--port is required")
		}
		if createServerService == "" {
			return fmt.Errorf("--service is required")
		}
		if createServerRegion == "" {
			return fmt.Errorf("--region is required")
		}

		client := NewAPIClient()

		// Build request body
		reqBody := map[string]interface{}{
			"address": createServerAddress,
			"port":    createServerPort,
			"name":    createServerService,
			"region":  createServerRegion,
			"weight":  createServerWeight,
			"enabled": true,
			"metadata": map[string]string{
				"service": createServerService,
			},
		}

		if createServerName != "" {
			reqBody["name"] = createServerName
		}
		if createServerProtocol != "" {
			reqBody["protocol"] = createServerProtocol
		}

		var response map[string]interface{}
		if err := client.Post("/api/v1/servers", reqBody, &response); err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		if jsonOutput {
			return formatter.Print(response)
		}

		fmt.Printf("✓ Server created successfully\n")
		fmt.Printf("  Service: %s\n", createServerService)
		fmt.Printf("  Address: %s:%d\n", createServerAddress, createServerPort)
		fmt.Printf("  Region:  %s\n", createServerRegion)
		fmt.Printf("  Weight:  %d\n", createServerWeight)
		return nil
	},
}

var (
	deleteServerID string
)

var serversDeleteCmd = &cobra.Command{
	Use:   "delete SERVER_ID",
	Short: "Delete a backend server",
	Long:  `Remove a backend server from the registry. Server ID format: service:address:port`,
	Example: `  opengslb-cli servers delete app.example.com:10.0.1.50:8080
  opengslb-cli servers delete webapp.example.com:172.28.1.10:80`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serverID := args[0]

		client := NewAPIClient()

		if err := client.Delete("/api/v1/servers/" + serverID); err != nil {
			return fmt.Errorf("failed to delete server: %w", err)
		}

		if jsonOutput {
			return formatter.Print(map[string]string{"status": "deleted", "id": serverID})
		}

		fmt.Printf("✓ Server deleted successfully: %s\n", serverID)
		return nil
	},
}

var (
	updateServerID     string
	updateServerWeight int
	updateServerRegion string
)

var serversUpdateCmd = &cobra.Command{
	Use:   "update SERVER_ID",
	Short: "Update a backend server",
	Long:  `Update server properties like weight or region. Server ID format: service:address:port`,
	Example: `  opengslb-cli servers update app.example.com:10.0.1.50:8080 --weight 200
  opengslb-cli servers update webapp.example.com:172.28.1.10:80 --region us-west`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serverID := args[0]

		client := NewAPIClient()

		// Build update request
		reqBody := make(map[string]interface{})
		if updateServerWeight > 0 {
			reqBody["weight"] = updateServerWeight
		}
		if updateServerRegion != "" {
			reqBody["region"] = updateServerRegion
		}

		if len(reqBody) == 0 {
			return fmt.Errorf("no updates specified (use --weight or --region)")
		}

		var response map[string]interface{}
		if err := client.Patch("/api/v1/servers/"+serverID, reqBody, &response); err != nil {
			return fmt.Errorf("failed to update server: %w", err)
		}

		if jsonOutput {
			return formatter.Print(response)
		}

		fmt.Printf("✓ Server updated successfully: %s\n", serverID)
		if updateServerWeight > 0 {
			fmt.Printf("  Weight: %d\n", updateServerWeight)
		}
		if updateServerRegion != "" {
			fmt.Printf("  Region: %s\n", updateServerRegion)
		}
		return nil
	},
}

func init() {
	// Add subcommands
	serversCmd.AddCommand(serversListCmd)
	serversCmd.AddCommand(serversCreateCmd)
	serversCmd.AddCommand(serversUpdateCmd)
	serversCmd.AddCommand(serversDeleteCmd)

	// List command flags
	serversListCmd.Flags().StringVar(&serversFilterService, "service", "", "Filter by service name")
	serversListCmd.Flags().StringVar(&serversFilterRegion, "region", "", "Filter by region")
	serversListCmd.Flags().StringVar(&serversFilterStatus, "status", "", "Filter by status (healthy, unhealthy, stale)")

	// Create command flags
	serversCreateCmd.Flags().StringVar(&createServerName, "name", "", "Server name (optional)")
	serversCreateCmd.Flags().StringVar(&createServerAddress, "address", "", "Server IP address (required)")
	serversCreateCmd.Flags().IntVar(&createServerPort, "port", 0, "Server port (required)")
	serversCreateCmd.Flags().IntVar(&createServerWeight, "weight", 100, "Server weight (default: 100)")
	serversCreateCmd.Flags().StringVar(&createServerRegion, "region", "", "Server region (required)")
	serversCreateCmd.Flags().StringVar(&createServerService, "service", "", "Service/domain name (required)")
	serversCreateCmd.Flags().StringVar(&createServerProtocol, "protocol", "tcp", "Protocol (default: tcp)")
	serversCreateCmd.MarkFlagRequired("address")
	serversCreateCmd.MarkFlagRequired("port")
	serversCreateCmd.MarkFlagRequired("service")
	serversCreateCmd.MarkFlagRequired("region")

	// Update command flags
	serversUpdateCmd.Flags().IntVar(&updateServerWeight, "weight", 0, "New server weight")
	serversUpdateCmd.Flags().StringVar(&updateServerRegion, "region", "", "New server region")
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
