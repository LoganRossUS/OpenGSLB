// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

func TestOverwatchDecisionMatrix(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name            string
		mode            string
		agentHealthy    bool
		externalHealthy bool
		expectVeto      bool
	}{
		// Permissive Mode
		{"Permissive: Agent Healthy, External Pass", "permissive", true, true, false},
		{"Permissive: Agent Unhealthy, External Pass", "permissive", false, true, false},
		{"Permissive: Agent Healthy, External Fail", "permissive", true, false, false},    // Should TRUST agent
		{"Permissive: Agent Unhealthy, External Fail", "permissive", false, false, false}, // Agreement

		// Balanced Mode
		{"Balanced: Agent Healthy, External Pass", "balanced", true, true, false},
		{"Balanced: Agent Healthy, External Fail", "balanced", true, false, true}, // Disagreement -> Veto

		// Strict Mode
		{"Strict: Agent Healthy, External Fail", "strict", true, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.OverwatchConfig{
				ExternalCheckInterval: 10 * time.Millisecond,
				VetoMode:              tt.mode,
			}
			o := NewOverwatch(cfg, nil, nil, nil, logger)

			// Simulate enough failures to trigger veto (threshold is 3 in code)
			addr := "1.2.3.4:80"

			// We need to call decideVeto multiple times to reach threshold
			// Or we can modify the test to check only after sufficient calls

			for i := 0; i < 5; i++ {
				o.decideVeto(addr, tt.agentHealthy, tt.externalHealthy)
			}

			isVetoed := !o.IsServeable(addr)
			if isVetoed != tt.expectVeto {
				t.Errorf("expected veto=%v, got %v", tt.expectVeto, isVetoed)
			}
		})
	}
}

func TestOverwatchVetoExpiry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.OverwatchConfig{
		ExternalCheckInterval: 50 * time.Millisecond,
		VetoMode:              "strict",
	}
	o := NewOverwatch(cfg, nil, nil, nil, logger)
	addr := "10.0.0.1:80"

	// Trigger veto
	for i := 0; i < 5; i++ {
		o.decideVeto(addr, true, false)
	}

	if o.IsServeable(addr) {
		t.Fatal("expected server to be vetoed")
	}

	// Wait for expiry (Code sets expiry to now + 2*Interval)
	// Interval is 50ms, so 100ms. Wait 150ms.
	time.Sleep(150 * time.Millisecond)

	// Force cleanup
	o.cleanupVetoes()

	if !o.IsServeable(addr) {
		t.Fatal("expected veto to expire")
	}
}
