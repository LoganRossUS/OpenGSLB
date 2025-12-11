// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cmd

import (
	"fmt"

	"github.com/loganrossus/OpenGSLB/cmd/opengslb-cli/output"
	"github.com/spf13/cobra"
)

// DSRecord represents a DS record from the API.
type DSRecord struct {
	Zone         string `json:"zone"`
	KeyTag       uint16 `json:"key_tag"`
	Algorithm    uint8  `json:"algorithm"`
	DigestType   uint8  `json:"digest_type"`
	Digest       string `json:"digest"`
	DSRecordText string `json:"ds_record"`
	CreatedAt    string `json:"created_at"`
}

// DSResponse is the API response for /api/v1/dnssec/ds.
type DSResponse struct {
	Enabled   bool       `json:"enabled"`
	Message   string     `json:"message,omitempty"`
	DSRecords []DSRecord `json:"ds_records,omitempty"`
}

var dnssecZone string

var dnssecCmd = &cobra.Command{
	Use:   "dnssec",
	Short: "DNSSEC management commands",
	Long:  `Commands for managing and viewing DNSSEC information.`,
}

var dnssecDSCmd = &cobra.Command{
	Use:   "ds",
	Short: "Show DS records for zones",
	Long: `Display DNSSEC DS records that should be added to the parent zone.

Examples:
  opengslb-cli dnssec ds
  opengslb-cli dnssec ds --zone gslb.example.com`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewAPIClient()

		path := "/api/v1/dnssec/ds"
		if dnssecZone != "" {
			path = fmt.Sprintf("%s?zone=%s", path, URLEncode(dnssecZone))
		}

		var response DSResponse
		if err := client.Get(path, &response); err != nil {
			return fmt.Errorf("failed to get DS records: %w", err)
		}

		if !response.Enabled {
			if jsonOutput {
				return formatter.Print(response)
			}
			formatter.PrintMessage("DNSSEC is disabled.")
			return nil
		}

		if len(response.DSRecords) == 0 {
			if jsonOutput {
				return formatter.Print(response)
			}
			formatter.PrintMessage("No DS records available.")
			return nil
		}

		if jsonOutput {
			return formatter.Print(response)
		}

		// Human-readable output
		for _, ds := range response.DSRecords {
			fmt.Printf("Zone: %s\n", ds.Zone)
			fmt.Printf("DS Record: %s\n\n", ds.DSRecordText)

			pairs := []output.KVPair{
				{Key: "Key Tag", Value: fmt.Sprintf("%d", ds.KeyTag)},
				{Key: "Algorithm", Value: fmt.Sprintf("%d", ds.Algorithm)},
				{Key: "Digest Type", Value: fmt.Sprintf("%d", ds.DigestType)},
				{Key: "Created At", Value: ds.CreatedAt},
			}
			formatter.PrintKeyValue(pairs)
			fmt.Println("\nAdd the DS Record above to your parent zone.")
			fmt.Println()
		}

		return nil
	},
}

var dnssecStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show DNSSEC status",
	Long:  `Display the current DNSSEC status including key information and sync status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewAPIClient()

		var response map[string]interface{}
		if err := client.Get("/api/v1/dnssec/status", &response); err != nil {
			return fmt.Errorf("failed to get DNSSEC status: %w", err)
		}

		if jsonOutput {
			return formatter.Print(response)
		}

		// Human-readable output
		enabled, _ := response["enabled"].(bool)
		if !enabled {
			formatter.PrintMessage("DNSSEC is disabled.")
			return nil
		}

		fmt.Println("DNSSEC Status: Enabled")

		if keys, ok := response["keys"].([]interface{}); ok && len(keys) > 0 {
			fmt.Printf("\nKeys: %d\n", len(keys))
			for i, k := range keys {
				if keyMap, ok := k.(map[string]interface{}); ok {
					zone, _ := keyMap["zone"].(string)
					keyTag, _ := keyMap["key_tag"].(float64)
					fmt.Printf("  %d. Zone: %s, Key Tag: %.0f\n", i+1, zone, keyTag)
				}
			}
		}

		if sync, ok := response["sync"].(map[string]interface{}); ok {
			fmt.Println("\nSync Status:")
			if lastSync, ok := sync["last_sync"].(string); ok && lastSync != "" {
				fmt.Printf("  Last Sync: %s\n", lastSync)
			}
			if peers, ok := sync["peers"].([]interface{}); ok {
				fmt.Printf("  Peers: %d\n", len(peers))
			}
		}

		return nil
	},
}

func init() {
	dnssecCmd.AddCommand(dnssecDSCmd)
	dnssecCmd.AddCommand(dnssecStatusCmd)

	dnssecDSCmd.Flags().StringVar(&dnssecZone, "zone", "", "Filter by zone name")
}
