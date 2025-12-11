// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// DomainOutput represents a domain for CLI output.
type DomainOutput struct {
	Name             string   `json:"name"`
	RoutingAlgorithm string   `json:"routing_algorithm"`
	Regions          []string `json:"regions"`
	TTL              int      `json:"ttl"`
}

var domainsConfigFile string

var domainsCmd = &cobra.Command{
	Use:   "domains",
	Short: "List configured domains",
	Long: `Display all configured GSLB domains and their routing configuration.

Note: This command reads from a local configuration file since domain
configuration is not exposed via the API.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Try to read from config file
		if domainsConfigFile == "" {
			// Check common locations
			locations := []string{
				"/etc/opengslb/overwatch.yaml",
				"/etc/opengslb/config.yaml",
				"./config.yaml",
				"./overwatch.yaml",
			}
			for _, loc := range locations {
				if _, err := os.Stat(loc); err == nil {
					domainsConfigFile = loc
					break
				}
			}
		}

		if domainsConfigFile == "" {
			return fmt.Errorf("no configuration file found. Use --config to specify the path")
		}

		data, err := os.ReadFile(domainsConfigFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		var cfg config.Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}

		if len(cfg.Domains) == 0 {
			formatter.PrintMessage("No domains configured.")
			return nil
		}

		if jsonOutput {
			domains := make([]DomainOutput, 0, len(cfg.Domains))
			for _, d := range cfg.Domains {
				domains = append(domains, DomainOutput{
					Name:             d.Name,
					RoutingAlgorithm: d.RoutingAlgorithm,
					Regions:          d.Regions,
					TTL:              d.TTL,
				})
			}
			return formatter.Print(domains)
		}

		// Human-readable table output
		headers := []string{"DOMAIN", "ALGORITHM", "REGIONS", "TTL"}
		rows := make([][]string, 0, len(cfg.Domains))

		for _, d := range cfg.Domains {
			algorithm := d.RoutingAlgorithm
			if algorithm == "" {
				algorithm = "round-robin"
			}

			ttl := d.TTL
			if ttl == 0 {
				ttl = cfg.DNS.DefaultTTL
			}

			rows = append(rows, []string{
				d.Name,
				algorithm,
				strings.Join(d.Regions, ", "),
				fmt.Sprintf("%d", ttl),
			})
		}

		formatter.PrintTable(headers, rows)
		return nil
	},
}

func init() {
	domainsCmd.Flags().StringVarP(&domainsConfigFile, "config", "c", "", "Path to configuration file")
}
