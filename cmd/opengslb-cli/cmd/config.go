// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cmd

import (
	"fmt"
	"os"

	"github.com/loganrossus/OpenGSLB/cmd/opengslb-cli/output"
	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ConfigValidationResult represents the result of config validation.
type ConfigValidationResult struct {
	Valid       bool     `json:"valid"`
	Mode        string   `json:"mode"`
	Zones       []string `json:"zones,omitempty"`
	RegionCount int      `json:"region_count,omitempty"`
	DomainCount int      `json:"domain_count,omitempty"`
	TokenCount  int      `json:"token_count,omitempty"`
	Errors      []string `json:"errors,omitempty"`
}

var configFile string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
	Long:  `Commands for validating and working with configuration files.`,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a configuration file",
	Long: `Validate an OpenGSLB configuration file for syntax and semantic errors.

Examples:
  opengslb-cli config validate --config /etc/opengslb/overwatch.yaml
  opengslb-cli config validate -c ./config.yaml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if configFile == "" {
			return fmt.Errorf("--config flag is required")
		}

		data, err := os.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		var cfg config.Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			result := ConfigValidationResult{
				Valid:  false,
				Errors: []string{fmt.Sprintf("YAML parse error: %v", err)},
			}
			if jsonOutput {
				return formatter.Print(result)
			}
			formatter.PrintMessage(fmt.Sprintf("Configuration invalid: %v", err))
			return fmt.Errorf("configuration invalid")
		}

		// Validate the configuration
		if err := cfg.Validate(); err != nil {
			result := ConfigValidationResult{
				Valid:  false,
				Mode:   string(cfg.Mode),
				Errors: []string{err.Error()},
			}
			if jsonOutput {
				return formatter.Print(result)
			}
			formatter.PrintMessage(fmt.Sprintf("Configuration invalid: %v", err))
			return fmt.Errorf("configuration invalid")
		}

		// Build result
		result := ConfigValidationResult{
			Valid:       true,
			Mode:        string(cfg.GetEffectiveMode()),
			Zones:       cfg.DNS.Zones,
			RegionCount: len(cfg.Regions),
			DomainCount: len(cfg.Domains),
		}

		if cfg.IsOverwatchMode() && cfg.Overwatch.AgentTokens != nil {
			result.TokenCount = len(cfg.Overwatch.AgentTokens)
		}

		if jsonOutput {
			return formatter.Print(result)
		}

		// Human-readable output
		fmt.Println("Configuration valid.")
		pairs := []output.KVPair{
			{Key: "Mode", Value: result.Mode},
		}

		if result.Mode == "overwatch" || result.Mode == "" {
			if len(result.Zones) > 0 {
				pairs = append(pairs, output.KVPair{Key: "Zones", Value: fmt.Sprintf("%v", result.Zones)})
			}
			pairs = append(pairs,
				output.KVPair{Key: "Regions", Value: fmt.Sprintf("%d", result.RegionCount)},
				output.KVPair{Key: "Domains", Value: fmt.Sprintf("%d", result.DomainCount)},
			)
			if result.TokenCount > 0 {
				pairs = append(pairs, output.KVPair{Key: "Agent tokens", Value: fmt.Sprintf("%d", result.TokenCount)})
			}
		}

		if result.Mode == "agent" {
			pairs = append(pairs, output.KVPair{Key: "Backends", Value: fmt.Sprintf("%d", len(cfg.Agent.Backends))})
		}

		formatter.PrintKeyValue(pairs)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configValidateCmd)
	configValidateCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file (required)")
	configValidateCmd.MarkFlagRequired("config")
}
