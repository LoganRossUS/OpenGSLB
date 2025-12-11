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

// Override represents an active override.
type Override struct {
	Service   string    `json:"service"`
	Address   string    `json:"address"`
	Healthy   bool      `json:"healthy"`
	Reason    string    `json:"reason"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	Authority string    `json:"authority,omitempty"`
}

// OverridesListResponse is the API response for /api/v1/overrides.
type OverridesListResponse struct {
	Overrides []Override `json:"overrides"`
}

// OverrideSetRequest is the request body for setting an override.
type OverrideSetRequest struct {
	Healthy bool   `json:"healthy"`
	Reason  string `json:"reason"`
	Source  string `json:"source"`
}

var (
	overrideHealthy bool
	overrideReason  string
)

var overridesCmd = &cobra.Command{
	Use:   "overrides",
	Short: "Manage health overrides",
	Long:  `View and manage health overrides for backend servers.`,
}

var overridesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active overrides",
	Long:  `Display all active health overrides.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewAPIClient()

		var response OverridesListResponse
		if err := client.Get("/api/v1/overrides", &response); err != nil {
			return fmt.Errorf("failed to get overrides: %w", err)
		}

		if len(response.Overrides) == 0 {
			formatter.PrintMessage("No active overrides.")
			return nil
		}

		if jsonOutput {
			return formatter.Print(response.Overrides)
		}

		// Human-readable table output
		headers := []string{"SERVICE", "ADDRESS", "HEALTHY", "REASON", "CREATED"}
		rows := make([][]string, 0, len(response.Overrides))

		for _, o := range response.Overrides {
			healthy := "false"
			if o.Healthy {
				healthy = "true"
			}

			created := o.CreatedAt.Format(time.RFC3339)

			rows = append(rows, []string{
				o.Service,
				o.Address,
				healthy,
				o.Reason,
				created,
			})
		}

		formatter.PrintTable(headers, rows)
		return nil
	},
}

var overridesSetCmd = &cobra.Command{
	Use:   "set <service> <address>",
	Short: "Set a health override",
	Long: `Set a health override for a specific backend server.

Examples:
  opengslb-cli overrides set myapp 10.0.1.10:8080 --healthy=false --reason="Maintenance"
  opengslb-cli overrides set myapp 10.0.1.10:8080 --healthy=true --reason="Restored"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		address := args[1]

		client := NewAPIClient()

		req := OverrideSetRequest{
			Healthy: overrideHealthy,
			Reason:  overrideReason,
			Source:  "cli",
		}

		path := fmt.Sprintf("/api/v1/overrides/%s/%s", URLEncode(service), URLEncode(address))
		var response Override
		if err := client.Put(path, req, &response); err != nil {
			return fmt.Errorf("failed to set override: %w", err)
		}

		if jsonOutput {
			return formatter.Print(response)
		}

		healthyStr := "unhealthy"
		if response.Healthy {
			healthyStr = "healthy"
		}
		formatter.PrintMessage(fmt.Sprintf("Override set for %s/%s: %s", service, address, healthyStr))
		return nil
	},
}

var overridesClearCmd = &cobra.Command{
	Use:   "clear <service> <address>",
	Short: "Clear a health override",
	Long: `Remove a health override for a specific backend server.

Examples:
  opengslb-cli overrides clear myapp 10.0.1.10:8080`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		address := args[1]

		client := NewAPIClient()

		path := fmt.Sprintf("/api/v1/overrides/%s/%s", URLEncode(service), URLEncode(address))
		if err := client.Delete(path); err != nil {
			return fmt.Errorf("failed to clear override: %w", err)
		}

		if jsonOutput {
			return formatter.Print(map[string]string{
				"message": fmt.Sprintf("Override cleared for %s/%s", service, address),
			})
		}

		formatter.PrintMessage(fmt.Sprintf("Override cleared for %s/%s", service, address))
		return nil
	},
}

func init() {
	overridesCmd.AddCommand(overridesListCmd)
	overridesCmd.AddCommand(overridesSetCmd)
	overridesCmd.AddCommand(overridesClearCmd)

	overridesSetCmd.Flags().BoolVar(&overrideHealthy, "healthy", false, "Set health status (true or false)")
	overridesSetCmd.Flags().StringVar(&overrideReason, "reason", "", "Reason for the override")
	overridesSetCmd.MarkFlagRequired("reason")
}
