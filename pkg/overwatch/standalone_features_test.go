// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
)

// TestRegistry_ValidationThresholds covers the optional consecutive-result
// thresholds. The default of 1 keeps today's behavior (first result wins).
func TestRegistry_ValidationThresholds(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	registry := newStandaloneRegistry(t, logger, RegistryConfig{
		ValidationFailureThreshold: 2,
		ValidationSuccessThreshold: 2,
	})

	set := func(healthy bool) {
		errMsg := ""
		if !healthy {
			errMsg = "connection refused"
		}
		if err := registry.UpdateValidation(labService, labPrimary, 80, healthy, errMsg); err != nil {
			t.Fatalf("update validation: %v", err)
		}
	}

	// Two passes establish healthy.
	set(true)
	set(true)
	if got := effectiveStatus(t, registry, labPrimary); got != StatusHealthy {
		t.Fatalf("expected healthy after two passes, got %s", got)
	}

	// One failure is not enough, two are.
	set(false)
	if got := effectiveStatus(t, registry, labPrimary); got != StatusHealthy {
		t.Fatalf("one failure below threshold must not change status, got %s", got)
	}
	set(false)
	if got := effectiveStatus(t, registry, labPrimary); got != StatusUnhealthy {
		t.Fatalf("expected unhealthy after two failures, got %s", got)
	}

	// A single pass does not recover; a pass, a failure and a pass does not either.
	set(true)
	if got := effectiveStatus(t, registry, labPrimary); got != StatusUnhealthy {
		t.Fatalf("one pass below threshold must not recover, got %s", got)
	}
	set(false)
	set(true)
	if got := effectiveStatus(t, registry, labPrimary); got != StatusUnhealthy {
		t.Fatalf("non-consecutive passes must not recover, got %s", got)
	}
	set(true)
	if got := effectiveStatus(t, registry, labPrimary); got != StatusHealthy {
		t.Fatalf("expected healthy after two consecutive passes, got %s", got)
	}
}

// TestRegistryMetrics_AuthorityLabelsAndBackendGauge checks the authority
// attribution for config-defined backends and the per-backend health gauge.
func TestRegistryMetrics_AuthorityLabelsAndBackendGauge(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	registry := newStandaloneRegistry(t, logger, RegistryConfig{})

	// Before any validation the static backends are healthy by default.
	UpdateRegistryMetrics(registry)
	if got := gaugeValue(t, metrics.OverwatchBackendsByAuthority.WithLabelValues(string(AuthorityStatic))); got != 2 {
		t.Errorf("expected 2 backends under static authority, got %v", got)
	}

	// After validation they are decided by validation.
	if err := registry.UpdateValidation(labService, labPrimary, 80, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateValidation(labService, labStandby, 80, false, "connection refused"); err != nil {
		t.Fatal(err)
	}
	UpdateRegistryMetrics(registry)
	if got := gaugeValue(t, metrics.OverwatchBackendsByAuthority.WithLabelValues(string(AuthorityValidation))); got != 2 {
		t.Errorf("expected 2 backends under validation authority, got %v", got)
	}
	if got := gaugeValue(t, metrics.OverwatchBackendsByAuthority.WithLabelValues(string(AuthorityStatic))); got != 0 {
		t.Errorf("expected 0 backends left under static authority, got %v", got)
	}

	// Per-backend health gauge with region and address labels.
	primary := gaugeValue(t, metrics.BackendHealthy.WithLabelValues(labService, "ch7", labPrimary+":80"))
	standby := gaugeValue(t, metrics.BackendHealthy.WithLabelValues(labService, "dc10", labStandby+":80"))
	if primary != 1 || standby != 0 {
		t.Errorf("per-backend gauge: primary=%v standby=%v, want 1 and 0", primary, standby)
	}

	// Deregistration removes the series.
	if err := registry.Deregister(labService, labStandby, 80); err != nil {
		t.Fatal(err)
	}
	if n := metrics.BackendHealthy.DeleteLabelValues(labService, "dc10", labStandby+":80"); n {
		t.Errorf("per-backend gauge series for the deregistered backend was still present")
	}
}
