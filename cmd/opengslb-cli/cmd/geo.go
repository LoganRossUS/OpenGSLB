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

// GeoMappingResponse represents a custom mapping from the API.
type GeoMappingResponse struct {
	CIDR    string `json:"cidr"`
	Region  string `json:"region"`
	Comment string `json:"comment,omitempty"`
	Source  string `json:"source"`
}

// GeoMappingsResponse is the API response for /api/v1/geo/mappings.
type GeoMappingsResponse struct {
	Mappings    []GeoMappingResponse `json:"mappings"`
	GeneratedAt time.Time            `json:"generated_at"`
}

// GeoMappingRequest is the request body for adding a mapping.
type GeoMappingRequest struct {
	CIDR    string `json:"cidr"`
	Region  string `json:"region"`
	Comment string `json:"comment,omitempty"`
}

// GeoTestResponse is the API response for /api/v1/geo/test.
type GeoTestResponse struct {
	IP          string `json:"ip"`
	Region      string `json:"region"`
	MatchType   string `json:"match_type"`
	MatchedCIDR string `json:"matched_cidr,omitempty"`
	Comment     string `json:"comment,omitempty"`
	Country     string `json:"country,omitempty"`
	Continent   string `json:"continent,omitempty"`
}

var geoAddComment string

var geoCmd = &cobra.Command{
	Use:   "geo",
	Short: "Manage geolocation custom mappings",
	Long:  `View and manage custom CIDR-to-region mappings for geolocation routing.`,
}

var geoMappingsCmd = &cobra.Command{
	Use:   "mappings",
	Short: "List custom geolocation mappings",
	Long:  `Display all custom CIDR-to-region mappings.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewAPIClient()

		var response GeoMappingsResponse
		if err := client.Get("/api/v1/geo/mappings", &response); err != nil {
			return fmt.Errorf("failed to get mappings: %w", err)
		}

		if len(response.Mappings) == 0 {
			formatter.PrintMessage("No custom geolocation mappings configured.")
			return nil
		}

		if jsonOutput {
			return formatter.Print(response.Mappings)
		}

		// Human-readable table output
		headers := []string{"CIDR", "REGION", "SOURCE", "COMMENT"}
		rows := make([][]string, 0, len(response.Mappings))

		for _, m := range response.Mappings {
			rows = append(rows, []string{
				m.CIDR,
				m.Region,
				m.Source,
				m.Comment,
			})
		}

		formatter.PrintTable(headers, rows)
		return nil
	},
}

var geoAddCmd = &cobra.Command{
	Use:   "add <cidr> <region>",
	Short: "Add a custom geolocation mapping",
	Long: `Add a custom CIDR-to-region mapping.

Examples:
  opengslb-cli geo add 10.5.0.0/16 us-west-2 --comment "Seattle office"
  opengslb-cli geo add 192.168.1.0/24 eu-west-1`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cidr := args[0]
		region := args[1]

		client := NewAPIClient()

		req := GeoMappingRequest{
			CIDR:    cidr,
			Region:  region,
			Comment: geoAddComment,
		}

		var response GeoMappingResponse
		if err := client.Put("/api/v1/geo/mappings", req, &response); err != nil {
			return fmt.Errorf("failed to add mapping: %w", err)
		}

		if jsonOutput {
			return formatter.Print(response)
		}

		formatter.PrintMessage(fmt.Sprintf("Mapping added: %s -> %s", cidr, region))
		return nil
	},
}

var geoRemoveCmd = &cobra.Command{
	Use:   "remove <cidr>",
	Short: "Remove a custom geolocation mapping",
	Long: `Remove a custom CIDR-to-region mapping.

Examples:
  opengslb-cli geo remove 10.5.0.0/16`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cidr := args[0]

		client := NewAPIClient()

		path := fmt.Sprintf("/api/v1/geo/mappings/%s", URLEncode(cidr))
		if err := client.Delete(path); err != nil {
			return fmt.Errorf("failed to remove mapping: %w", err)
		}

		if jsonOutput {
			return formatter.Print(map[string]string{
				"message": fmt.Sprintf("Mapping removed: %s", cidr),
			})
		}

		formatter.PrintMessage(fmt.Sprintf("Mapping removed: %s", cidr))
		return nil
	},
}

var geoTestCmd = &cobra.Command{
	Use:   "test <ip>",
	Short: "Test which region an IP routes to",
	Long: `Test geolocation routing for a specific IP address.

Examples:
  opengslb-cli geo test 10.1.50.100
  opengslb-cli geo test 8.8.8.8`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := args[0]

		client := NewAPIClient()

		var response GeoTestResponse
		path := fmt.Sprintf("/api/v1/geo/test?ip=%s", URLEncode(ip))
		if err := client.Get(path, &response); err != nil {
			return fmt.Errorf("failed to test IP: %w", err)
		}

		if jsonOutput {
			return formatter.Print(response)
		}

		// Human-readable output
		pairs := []output.KVPair{
			{Key: "IP", Value: response.IP},
			{Key: "Region", Value: response.Region},
			{Key: "Match Type", Value: response.MatchType},
		}

		switch response.MatchType {
		case "custom_mapping":
			pairs = append(pairs, output.KVPair{Key: "Matched", Value: response.MatchedCIDR})
			if response.Comment != "" {
				pairs = append(pairs, output.KVPair{Key: "Comment", Value: response.Comment})
			}
		case "geoip":
			if response.Country != "" {
				pairs = append(pairs, output.KVPair{Key: "Country", Value: response.Country})
			}
			if response.Continent != "" {
				pairs = append(pairs, output.KVPair{Key: "Continent", Value: response.Continent})
			}
		case "default":
			pairs = append(pairs, output.KVPair{Key: "Note", Value: "Using default region"})
		}

		formatter.PrintKeyValue(pairs)
		return nil
	},
}

func init() {
	geoCmd.AddCommand(geoMappingsCmd)
	geoCmd.AddCommand(geoAddCmd)
	geoCmd.AddCommand(geoRemoveCmd)
	geoCmd.AddCommand(geoTestCmd)

	geoAddCmd.Flags().StringVar(&geoAddComment, "comment", "", "Optional comment for the mapping")
}
