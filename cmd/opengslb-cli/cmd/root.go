// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package cmd implements CLI commands for opengslb-cli.
package cmd

import (
	"fmt"
	"os"

	"github.com/loganrossus/OpenGSLB/cmd/opengslb-cli/output"
	"github.com/loganrossus/OpenGSLB/pkg/version"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	apiEndpoint string
	timeout     int
	jsonOutput  bool

	// Formatter for output
	formatter output.Formatter
)

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:   "opengslb-cli",
	Short: "CLI for managing OpenGSLB Overwatch",
	Long: `opengslb-cli is a command-line tool for managing and debugging OpenGSLB Overwatch.

It provides commands to:
  - View overall Overwatch status
  - List and manage backend servers
  - View configured domains
  - Manage health overrides
  - Manage geolocation custom mappings
  - Validate configuration files
  - View DNSSEC DS records

Use --api to specify the Overwatch API endpoint (default: http://localhost:8080).`,
	Version: version.Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		formatter = output.GetFormatter(jsonOutput)
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiEndpoint, "api", getEnvOrDefault("OPENGSLB_API", "http://localhost:8080"), "Overwatch API endpoint")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 10, "API request timeout in seconds")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	// Add subcommands
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(serversCmd)
	rootCmd.AddCommand(domainsCmd)
	rootCmd.AddCommand(overridesCmd)
	rootCmd.AddCommand(geoCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(dnssecCmd)
	rootCmd.AddCommand(completionCmd)

	// Version template
	rootCmd.SetVersionTemplate(fmt.Sprintf("opengslb-cli version %s\n", version.Version))
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
