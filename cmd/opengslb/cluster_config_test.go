// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"strings"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

func TestValidateClusterFlags_Standalone(t *testing.T) {
	tests := []struct {
		name          string
		cfg           *config.Config
		bootstrapFlag bool
		joinAddresses string
		wantErr       bool
		errContains   string
	}{
		{
			name: "standalone with no flags",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{Mode: config.ModeStandalone},
			},
			wantErr: false,
		},
		{
			name: "standalone with empty mode (defaults to standalone)",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{Mode: ""},
			},
			wantErr: false,
		},
		{
			name: "standalone with bootstrap flag errors",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{Mode: config.ModeStandalone},
			},
			bootstrapFlag: true,
			wantErr:       true,
			errContains:   "--bootstrap flag requires --mode=cluster",
		},
		{
			name: "standalone with join flag errors",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{Mode: config.ModeStandalone},
			},
			joinAddresses: "10.0.1.10:7946",
			wantErr:       true,
			errContains:   "--join flag requires --mode=cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global flags
			oldBootstrap := bootstrapFlag
			oldJoin := joinAddresses
			defer func() {
				bootstrapFlag = oldBootstrap
				joinAddresses = oldJoin
			}()

			bootstrapFlag = tt.bootstrapFlag
			joinAddresses = tt.joinAddresses

			err := validateClusterFlags(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateClusterFlags_ClusterMode(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantErr     bool
		errContains string
	}{
		{
			name: "cluster with bootstrap",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{
					Mode:        config.ModeCluster,
					BindAddress: "10.0.1.10:7946",
					Bootstrap:   true,
				},
			},
			wantErr: false,
		},
		{
			name: "cluster with join",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{
					Mode:        config.ModeCluster,
					BindAddress: "10.0.1.10:7946",
					Join:        []string{"10.0.1.11:7946"},
				},
			},
			wantErr: false,
		},
		{
			name: "cluster with both bootstrap and join errors",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{
					Mode:        config.ModeCluster,
					BindAddress: "10.0.1.10:7946",
					Bootstrap:   true,
					Join:        []string{"10.0.1.11:7946"},
				},
			},
			wantErr:     true,
			errContains: "mutually exclusive",
		},
		{
			name: "cluster with neither bootstrap nor join errors",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{
					Mode:        config.ModeCluster,
					BindAddress: "10.0.1.10:7946",
				},
			},
			wantErr:     true,
			errContains: "requires either --bootstrap or --join",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global flags
			oldBootstrap := bootstrapFlag
			oldJoin := joinAddresses
			defer func() {
				bootstrapFlag = oldBootstrap
				joinAddresses = oldJoin
			}()
			bootstrapFlag = false
			joinAddresses = ""

			err := validateClusterFlags(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestApplyClusterOverrides(t *testing.T) {
	tests := []struct {
		name           string
		cfg            *config.Config
		modeFlag       string
		bootstrapFlagV bool
		joinFlag       string
		wantMode       config.RuntimeMode
		wantBootstrap  bool
		wantJoin       []string
	}{
		{
			name:     "defaults to standalone when nothing set",
			cfg:      &config.Config{},
			wantMode: config.ModeStandalone,
		},
		{
			name:     "mode flag overrides config",
			cfg:      &config.Config{Cluster: config.ClusterConfig{Mode: config.ModeStandalone}},
			modeFlag: "cluster",
			wantMode: config.ModeCluster,
		},
		{
			name:           "bootstrap flag overrides config",
			cfg:            &config.Config{Cluster: config.ClusterConfig{Bootstrap: false}},
			bootstrapFlagV: true,
			wantMode:       config.ModeStandalone,
			wantBootstrap:  true,
		},
		{
			name:     "join flag overrides config",
			cfg:      &config.Config{Cluster: config.ClusterConfig{Join: []string{"old:7946"}}},
			joinFlag: "new1:7946,new2:7946",
			wantMode: config.ModeStandalone,
			wantJoin: []string{"new1:7946", "new2:7946"},
		},
		{
			name: "config values preserved when no flags",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{
					Mode:      config.ModeCluster,
					Bootstrap: true,
					Join:      []string{"existing:7946"},
				},
			},
			wantMode:      config.ModeCluster,
			wantBootstrap: true,
			wantJoin:      []string{"existing:7946"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global flags
			oldMode := runtimeMode
			oldBootstrap := bootstrapFlag
			oldJoin := joinAddresses
			defer func() {
				runtimeMode = oldMode
				bootstrapFlag = oldBootstrap
				joinAddresses = oldJoin
			}()

			runtimeMode = tt.modeFlag
			bootstrapFlag = tt.bootstrapFlagV
			joinAddresses = tt.joinFlag

			applyClusterOverrides(tt.cfg, nil)

			if tt.cfg.Cluster.Mode != tt.wantMode {
				t.Errorf("mode = %v, want %v", tt.cfg.Cluster.Mode, tt.wantMode)
			}
			if tt.cfg.Cluster.Bootstrap != tt.wantBootstrap {
				t.Errorf("bootstrap = %v, want %v", tt.cfg.Cluster.Bootstrap, tt.wantBootstrap)
			}
			if tt.wantJoin != nil {
				if len(tt.cfg.Cluster.Join) != len(tt.wantJoin) {
					t.Errorf("join = %v, want %v", tt.cfg.Cluster.Join, tt.wantJoin)
				} else {
					for i, v := range tt.wantJoin {
						if tt.cfg.Cluster.Join[i] != v {
							t.Errorf("join[%d] = %v, want %v", i, tt.cfg.Cluster.Join[i], v)
						}
					}
				}
			}
		})
	}
}

func TestClusterConfig_ModeHelpers(t *testing.T) {
	tests := []struct {
		name           string
		mode           config.RuntimeMode
		wantCluster    bool
		wantStandalone bool
	}{
		{"cluster mode", config.ModeCluster, true, false},
		{"standalone mode", config.ModeStandalone, false, true},
		{"empty mode defaults to standalone", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ClusterConfig{Mode: tt.mode}

			if got := cfg.IsClusterMode(); got != tt.wantCluster {
				t.Errorf("IsClusterMode() = %v, want %v", got, tt.wantCluster)
			}
			if got := cfg.IsStandaloneMode(); got != tt.wantStandalone {
				t.Errorf("IsStandaloneMode() = %v, want %v", got, tt.wantStandalone)
			}
		})
	}
}
