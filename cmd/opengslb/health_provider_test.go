// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// newTestBackendRegistry returns a backend registry without a store whose own
// log output is discarded.
func newTestBackendRegistry(t *testing.T) *overwatch.Registry {
	t.Helper()
	return overwatch.NewRegistry(overwatch.RegistryConfig{
		StaleThreshold: 15 * time.Second,
		RemoveAfter:    60 * time.Second,
		Logger:         slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	}, nil)
}

// TestCombinedHealthProvider_ExclusionsAreNotLoggedAtInfo covers finding
// 1B #8: IsHealthy runs for every candidate server of every query, so
// excluding an unhealthy or draining backend must not write to the log at
// INFO. The exclusion stays visible at DEBUG.
func TestCombinedHealthProvider_ExclusionsAreNotLoggedAtInfo(t *testing.T) {
	const service = "app.gslb.example.com"

	registry := newTestBackendRegistry(t)
	// An unhealthy backend (failed validation) and a draining one (agent
	// reported predictive-health draining).
	if err := registry.RegisterStatic(service, "10.0.1.10", 80, 100, "eu", "http"); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateValidation(service, "10.0.1.10", 80, false, "connection refused"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("agent-1", "eu", service, "10.0.1.11", 80, 100, true); err != nil {
		t.Fatal(err)
	}
	registry.UpdateDraining("agent-1", true, "cpu_threshold_exceeded", time.Now(), 97, 40, 0)

	for _, tc := range []struct {
		level     slog.Level
		wantLines bool
	}{
		{slog.LevelInfo, false},
		{slog.LevelDebug, true},
	} {
		t.Run(tc.level.String(), func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: tc.level}))
			provider := &combinedHealthProvider{backendRegistry: registry, logger: logger}

			if provider.IsHealthy("10.0.1.10", 80) {
				t.Error("backend that failed validation is reported healthy")
			}
			if provider.IsHealthy("10.0.1.11", 80) {
				t.Error("draining backend is reported healthy")
			}

			out := buf.String()
			if !tc.wantLines {
				if out != "" {
					t.Errorf("IsHealthy logged at INFO or above on the query path:\n%s", out)
				}
				return
			}
			if strings.Contains(out, "level=INFO") {
				t.Errorf("exclusion still logged at INFO:\n%s", out)
			}
			for _, want := range []string{"10.0.1.10", "10.0.1.11", "level=DEBUG"} {
				if !strings.Contains(out, want) {
					t.Errorf("debug log does not mention %q:\n%s", want, out)
				}
			}
		})
	}
}
