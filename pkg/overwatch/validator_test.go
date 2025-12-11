// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/health"
)

// mockChecker implements health.Checker for testing.
type mockChecker struct {
	results map[string]health.Result
}

func newMockChecker() *mockChecker {
	return &mockChecker{
		results: make(map[string]health.Result),
	}
}

func (m *mockChecker) Check(ctx context.Context, target health.Target) health.Result {
	key := target.Address
	if result, ok := m.results[key]; ok {
		return result
	}
	return health.Result{Healthy: true, Latency: 10 * time.Millisecond}
}

func (m *mockChecker) Type() string {
	return "mock"
}

func (m *mockChecker) SetResult(address string, healthy bool, err error) {
	m.results[address] = health.Result{
		Healthy: healthy,
		Latency: 10 * time.Millisecond,
		Error:   err,
	}
}

func TestValidator_StartStop(t *testing.T) {
	registry := NewRegistry(RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}, nil)

	validator := NewValidator(ValidatorConfig{
		Enabled:       true,
		CheckInterval: 100 * time.Millisecond,
		CheckTimeout:  50 * time.Millisecond,
	}, registry, newMockChecker())

	// Start validator
	err := validator.Start()
	if err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}

	if !validator.IsRunning() {
		t.Error("validator should be running after Start")
	}

	// Stop validator
	err = validator.Stop()
	if err != nil {
		t.Fatalf("failed to stop validator: %v", err)
	}

	if validator.IsRunning() {
		t.Error("validator should not be running after Stop")
	}
}

func TestValidator_Disabled(t *testing.T) {
	registry := NewRegistry(RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}, nil)

	validator := NewValidator(ValidatorConfig{
		Enabled: false,
	}, registry, newMockChecker())

	// Start disabled validator should succeed
	err := validator.Start()
	if err != nil {
		t.Fatalf("failed to start disabled validator: %v", err)
	}

	if validator.IsRunning() {
		t.Error("disabled validator should not be running")
	}
}

func TestValidator_ValidatesBackends(t *testing.T) {
	registry := NewRegistry(RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}, nil)

	checker := newMockChecker()
	checker.SetResult("192.168.1.1", true, nil)
	checker.SetResult("192.168.1.2", false, nil)

	validator := NewValidator(ValidatorConfig{
		Enabled:       true,
		CheckInterval: 1 * time.Second,
		CheckTimeout:  100 * time.Millisecond,
	}, registry, checker)

	// Register backends
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	_ = registry.Register("agent-2", "us-east", "web", "192.168.1.2", 80, 100, true)

	// Trigger validation
	validator.ValidateNow()

	// Check validation results were applied
	backend1, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend1.ValidationHealthy == nil {
		t.Error("backend1 should have validation result")
	} else if !*backend1.ValidationHealthy {
		t.Error("backend1 validation should be healthy")
	}

	backend2, _ := registry.GetBackend("web", "192.168.1.2", 80)
	if backend2.ValidationHealthy == nil {
		t.Error("backend2 should have validation result")
	} else if *backend2.ValidationHealthy {
		t.Error("backend2 validation should be unhealthy")
	}

	// Check effective status reflects validation
	if backend2.EffectiveStatus != StatusUnhealthy {
		t.Errorf("backend2 should be unhealthy (validation overrides agent), got %s", backend2.EffectiveStatus)
	}
}

func TestValidator_ValidateSingleBackend(t *testing.T) {
	registry := NewRegistry(RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}, nil)

	checker := newMockChecker()
	checker.SetResult("192.168.1.1", false, nil)

	validator := NewValidator(ValidatorConfig{
		Enabled:       true,
		CheckInterval: 1 * time.Hour, // Long interval so periodic check doesn't interfere
		CheckTimeout:  100 * time.Millisecond,
	}, registry, checker)

	// Register backend
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)

	// Validate specific backend
	err := validator.ValidateBackend("web", "192.168.1.1", 80)
	if err != nil {
		t.Fatalf("failed to validate backend: %v", err)
	}

	// Check result
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.ValidationHealthy == nil || *backend.ValidationHealthy {
		t.Error("backend should be validated as unhealthy")
	}
}

func TestValidator_ValidationStats(t *testing.T) {
	registry := NewRegistry(RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}, nil)

	checker := newMockChecker()
	checker.SetResult("192.168.1.1", true, nil)
	checker.SetResult("192.168.1.2", false, nil)

	validator := NewValidator(ValidatorConfig{
		Enabled:       true,
		CheckInterval: 1 * time.Second,
		CheckTimeout:  100 * time.Millisecond,
	}, registry, checker)

	// Register backends with mixed health
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	_ = registry.Register("agent-2", "us-east", "web", "192.168.1.2", 80, 100, true) // Agent claims healthy

	// Trigger validation
	validator.ValidateNow()

	// Get stats
	stats := validator.GetValidationStats()

	if stats.TotalBackends != 2 {
		t.Errorf("expected 2 total backends, got %d", stats.TotalBackends)
	}
	if stats.ValidatedBackends != 2 {
		t.Errorf("expected 2 validated backends, got %d", stats.ValidatedBackends)
	}
	if stats.HealthyBackends != 1 {
		t.Errorf("expected 1 healthy backend, got %d", stats.HealthyBackends)
	}
	if stats.UnhealthyBackends != 1 {
		t.Errorf("expected 1 unhealthy backend, got %d", stats.UnhealthyBackends)
	}
	// backend 192.168.1.2: agent claims healthy, validation says unhealthy -> disagreement
	if stats.DisagreementCount != 1 {
		t.Errorf("expected 1 disagreement, got %d", stats.DisagreementCount)
	}
}
