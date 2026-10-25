// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// gaugeValue reads the current value of a gauge without pulling in the
// prometheus testutil module.
func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		t.Fatalf("read gauge: %v", err)
	}
	return m.GetGauge().GetValue()
}

// Standalone overwatch: static backends from config, external validation on,
// no agents ever report. These tests reproduce the Azure lab drill of
// 2026-10-24 (primary VIP stopped, then restarted) at the registry and
// validator layer.

const (
	labService = "www.app.lab.internal"
	labPrimary = "10.107.0.10"
	labStandby = "10.110.0.10"
)

// newStandaloneRegistry builds a registry with the lab's stale settings and two
// static backends, primary first, exactly as RegisterStatic is called from
// cmd/opengslb for config-defined servers.
func newStandaloneRegistry(t *testing.T, logger *slog.Logger, cfg RegistryConfig) *Registry {
	t.Helper()
	if cfg.StaleThreshold == 0 {
		cfg.StaleThreshold = 15 * time.Second
	}
	if cfg.RemoveAfter == 0 {
		cfg.RemoveAfter = 60 * time.Second
	}
	cfg.Logger = logger
	registry := NewRegistry(cfg, nil)
	if err := registry.RegisterStatic(labService, labPrimary, 80, 100, "ch7", "http"); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	if err := registry.RegisterStatic(labService, labStandby, 80, 100, "dc10", "http"); err != nil {
		t.Fatalf("register standby: %v", err)
	}
	return registry
}

func effectiveStatus(t *testing.T, registry *Registry, address string) BackendStatus {
	t.Helper()
	backend, ok := registry.GetBackend(labService, address, 80)
	if !ok {
		t.Fatalf("backend %s not found", address)
	}
	return backend.EffectiveStatus
}

// TestValidator_StandaloneFailbackWithoutAgent reproduces the drill: the
// primary fails validation, then passes again. With no agent registered,
// validation must be the sole authority and the primary must return to
// healthy on the first passing cycle. It also asserts that the validator does
// not log "validation disagrees with agent" for a backend that has no agent.
func TestValidator_StandaloneFailbackWithoutAgent(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	registry := newStandaloneRegistry(t, logger, RegistryConfig{})
	checker := newMockChecker()
	validator := NewValidator(ValidatorConfig{
		Enabled:       true,
		CheckInterval: 5 * time.Second,
		CheckTimeout:  2 * time.Second,
		Logger:        logger,
	}, registry, checker)

	// Steady state: both VIPs answer, both healthy.
	validator.ValidateNow()
	if got := effectiveStatus(t, registry, labPrimary); got != StatusHealthy {
		t.Fatalf("steady state: primary expected healthy, got %s", got)
	}
	if got := effectiveStatus(t, registry, labStandby); got != StatusHealthy {
		t.Fatalf("steady state: standby expected healthy, got %s", got)
	}

	// NGINX stopped on the primary VIP: one failing cycle takes it out.
	checker.SetResult(labPrimary, false, errors.New("connection refused"))
	validator.ValidateNow()
	if got := effectiveStatus(t, registry, labPrimary); got != StatusUnhealthy {
		t.Fatalf("after failure: primary expected unhealthy, got %s", got)
	}

	// NGINX restarted: the first passing cycle must bring it back.
	checker.SetResult(labPrimary, true, nil)
	validator.ValidateNow()
	if got := effectiveStatus(t, registry, labPrimary); got != StatusHealthy {
		t.Fatalf("after recovery: primary expected healthy after one passing validation, got %s", got)
	}

	// Nothing above involved an agent, so nothing may be reported as a
	// disagreement with one.
	if strings.Contains(logBuf.String(), "validation disagrees with agent") {
		t.Errorf("validator logged an agent disagreement for agentless backends:\n%s", logBuf.String())
	}
}

// TestValidator_DisagreementLoggedOnceForAgentBackend keeps the disagreement
// log for agent-registered backends but only at the transition, not on every
// validation cycle.
func TestValidator_DisagreementLoggedOnceForAgentBackend(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	registry := NewRegistry(RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
		Logger:         logger,
	}, nil)
	if err := registry.Register("agent-1", "ch7", labService, labPrimary, 80, 100, true); err != nil {
		t.Fatalf("register agent backend: %v", err)
	}

	checker := newMockChecker()
	checker.SetResult(labPrimary, false, errors.New("connection refused"))
	validator := NewValidator(ValidatorConfig{
		Enabled:       true,
		CheckInterval: 5 * time.Second,
		CheckTimeout:  2 * time.Second,
		Logger:        logger,
	}, registry, checker)

	for i := 0; i < 5; i++ {
		validator.ValidateNow()
	}

	if got := strings.Count(logBuf.String(), "validation disagrees with agent"); got != 1 {
		t.Errorf("expected exactly one disagreement log at the transition, got %d:\n%s", got, logBuf.String())
	}
}

// TestUpdateRegistryMetrics_StandaloneAuthority reproduces the lab metric
// observation: with no agent ever reporting, backends must not be attributed
// to the "agent" authority.
func TestUpdateRegistryMetrics_StandaloneAuthority(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	registry := newStandaloneRegistry(t, logger, RegistryConfig{})

	UpdateRegistryMetrics(registry)
	if got := gaugeValue(t, metrics.OverwatchBackendsByAuthority.WithLabelValues("agent")); got != 0 {
		t.Errorf("static backends attributed to agent authority before validation: %v", got)
	}

	if err := registry.UpdateValidation(labService, labPrimary, 80, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateValidation(labService, labStandby, 80, false, "connection refused"); err != nil {
		t.Fatal(err)
	}
	UpdateRegistryMetrics(registry)
	if got := gaugeValue(t, metrics.OverwatchBackendsByAuthority.WithLabelValues("agent")); got != 0 {
		t.Errorf("validated static backends attributed to agent authority: %v", got)
	}
}
