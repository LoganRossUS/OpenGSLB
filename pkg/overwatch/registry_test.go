// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"testing"
	"time"
)

func TestRegistry_Register(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Register a backend
	err := registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	if err != nil {
		t.Fatalf("failed to register backend: %v", err)
	}

	// Verify registration
	backend, exists := registry.GetBackend("web", "192.168.1.1", 80)
	if !exists {
		t.Fatal("backend not found after registration")
	}

	if backend.Service != "web" {
		t.Errorf("expected service 'web', got %q", backend.Service)
	}
	if backend.Address != "192.168.1.1" {
		t.Errorf("expected address '192.168.1.1', got %q", backend.Address)
	}
	if backend.Port != 80 {
		t.Errorf("expected port 80, got %d", backend.Port)
	}
	if backend.AgentID != "agent-1" {
		t.Errorf("expected agent_id 'agent-1', got %q", backend.AgentID)
	}
	if backend.Region != "us-east" {
		t.Errorf("expected region 'us-east', got %q", backend.Region)
	}
	if !backend.AgentHealthy {
		t.Error("expected backend to be healthy")
	}
	if backend.EffectiveStatus != StatusHealthy {
		t.Errorf("expected status healthy, got %s", backend.EffectiveStatus)
	}
}

func TestRegistry_UpdateExisting(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Register initial backend
	err := registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	if err != nil {
		t.Fatalf("failed to register backend: %v", err)
	}

	// Update with new health status
	err = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 200, false)
	if err != nil {
		t.Fatalf("failed to update backend: %v", err)
	}

	// Verify update
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.Weight != 200 {
		t.Errorf("expected weight 200, got %d", backend.Weight)
	}
	if backend.AgentHealthy {
		t.Error("expected backend to be unhealthy after update")
	}
	if backend.EffectiveStatus != StatusUnhealthy {
		t.Errorf("expected status unhealthy, got %s", backend.EffectiveStatus)
	}
}

func TestRegistry_Deregister(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Register and then deregister
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)

	err := registry.Deregister("web", "192.168.1.1", 80)
	if err != nil {
		t.Fatalf("failed to deregister backend: %v", err)
	}

	// Verify removal
	_, exists := registry.GetBackend("web", "192.168.1.1", 80)
	if exists {
		t.Error("backend should not exist after deregistration")
	}
}

func TestRegistry_ValidationOverridesAgent(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Register healthy backend
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)

	// External validation says unhealthy
	err := registry.UpdateValidation("web", "192.168.1.1", 80, false, "connection refused")
	if err != nil {
		t.Fatalf("failed to update validation: %v", err)
	}

	// Verify validation overrides agent claim
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.AgentHealthy != true {
		t.Error("agent_healthy should still be true")
	}
	if backend.ValidationHealthy == nil || *backend.ValidationHealthy != false {
		t.Error("validation_healthy should be false")
	}
	if backend.EffectiveStatus != StatusUnhealthy {
		t.Errorf("expected effective status unhealthy (validation wins), got %s", backend.EffectiveStatus)
	}
}

func TestRegistry_ManualOverride(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Register unhealthy backend
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, false)

	// Set manual override to healthy
	err := registry.SetOverride("web", "192.168.1.1", 80, true, "maintenance bypass", "admin")
	if err != nil {
		t.Fatalf("failed to set override: %v", err)
	}

	// Verify override takes effect
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.OverrideStatus == nil || *backend.OverrideStatus != true {
		t.Error("override should be set to true")
	}
	if backend.OverrideReason != "maintenance bypass" {
		t.Errorf("expected override reason 'maintenance bypass', got %q", backend.OverrideReason)
	}
	if backend.EffectiveStatus != StatusHealthy {
		t.Errorf("expected effective status healthy (override), got %s", backend.EffectiveStatus)
	}

	// Clear override
	err = registry.ClearOverride("web", "192.168.1.1", 80)
	if err != nil {
		t.Fatalf("failed to clear override: %v", err)
	}

	// Verify override cleared and agent status takes effect
	backend, _ = registry.GetBackend("web", "192.168.1.1", 80)
	if backend.OverrideStatus != nil {
		t.Error("override should be cleared")
	}
	if backend.EffectiveStatus != StatusUnhealthy {
		t.Errorf("expected effective status unhealthy (agent claim), got %s", backend.EffectiveStatus)
	}
}

func TestRegistry_GetBackends(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Register multiple backends for the same service
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	_ = registry.Register("agent-2", "us-east", "web", "192.168.1.2", 80, 100, true)
	_ = registry.Register("agent-3", "us-west", "api", "192.168.2.1", 8080, 100, true)

	// Get all backends for 'web' service
	webBackends := registry.GetBackends("web")
	if len(webBackends) != 2 {
		t.Errorf("expected 2 web backends, got %d", len(webBackends))
	}

	// Get all backends
	allBackends := registry.GetAllBackends()
	if len(allBackends) != 3 {
		t.Errorf("expected 3 total backends, got %d", len(allBackends))
	}
}

func TestRegistry_GetHealthyBackends(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Register mixed health backends
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	_ = registry.Register("agent-2", "us-east", "web", "192.168.1.2", 80, 100, false)
	_ = registry.Register("agent-3", "us-east", "web", "192.168.1.3", 80, 100, true)

	// Get healthy backends
	healthy := registry.GetHealthyBackends("web")
	if len(healthy) != 2 {
		t.Errorf("expected 2 healthy backends, got %d", len(healthy))
	}
}

func TestRegistry_IsHealthy(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	_ = registry.Register("agent-2", "us-east", "web", "192.168.1.2", 80, 100, false)

	if !registry.IsHealthy("192.168.1.1", 80) {
		t.Error("192.168.1.1:80 should be healthy")
	}
	if registry.IsHealthy("192.168.1.2", 80) {
		t.Error("192.168.1.2:80 should be unhealthy")
	}
	if registry.IsHealthy("192.168.1.3", 80) {
		t.Error("192.168.1.3:80 should not exist")
	}
}

func TestRegistry_StatusChangeCallback(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	var callbackCalled bool
	var callbackBackend *Backend
	var callbackOld, callbackNew BackendStatus

	registry.OnStatusChange(func(backend *Backend, old, new BackendStatus) {
		callbackCalled = true
		callbackBackend = backend
		callbackOld = old
		callbackNew = new
	})

	// Register healthy backend
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)

	// Update to unhealthy - should trigger callback
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, false)

	if !callbackCalled {
		t.Error("status change callback was not called")
	}
	if callbackBackend == nil || callbackBackend.Address != "192.168.1.1" {
		t.Error("callback received wrong backend")
	}
	if callbackOld != StatusHealthy {
		t.Errorf("expected old status healthy, got %s", callbackOld)
	}
	if callbackNew != StatusUnhealthy {
		t.Errorf("expected new status unhealthy, got %s", callbackNew)
	}
}

func TestRegistry_BackendCount(t *testing.T) {
	cfg := RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	if registry.BackendCount() != 0 {
		t.Error("expected 0 backends initially")
	}

	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	_ = registry.Register("agent-2", "us-east", "web", "192.168.1.2", 80, 100, true)

	if registry.BackendCount() != 2 {
		t.Errorf("expected 2 backends, got %d", registry.BackendCount())
	}

	_ = registry.Deregister("web", "192.168.1.1", 80)

	if registry.BackendCount() != 1 {
		t.Errorf("expected 1 backend after deregister, got %d", registry.BackendCount())
	}
}

func TestRegistry_ValidationRecoversStaleBackend(t *testing.T) {
	// Use a very short stale threshold for testing
	cfg := RegistryConfig{
		StaleThreshold: 50 * time.Millisecond,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Start registry to enable stale detection background loop
	_ = registry.Start()
	defer registry.Stop()

	// Register backend
	err := registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	if err != nil {
		t.Fatalf("failed to register backend: %v", err)
	}

	// Wait for backend to become stale (stale detection runs every 25ms)
	time.Sleep(100 * time.Millisecond)

	// Verify backend is stale before validation
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusStale {
		t.Errorf("expected backend to be stale before validation, got %s", backend.EffectiveStatus)
	}

	// External validation says healthy - should recover stale backend
	err = registry.UpdateValidation("web", "192.168.1.1", 80, true, "")
	if err != nil {
		t.Fatalf("failed to update validation: %v", err)
	}

	// Verify validation recovered the stale backend
	backend, _ = registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusHealthy {
		t.Errorf("expected stale backend to be recovered to healthy by validation, got %s", backend.EffectiveStatus)
	}
}

func TestRegistry_StaleWithUnhealthyValidation(t *testing.T) {
	// Use a very short stale threshold for testing
	cfg := RegistryConfig{
		StaleThreshold: 50 * time.Millisecond,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Start registry to enable stale detection background loop
	_ = registry.Start()
	defer registry.Stop()

	// Register backend
	err := registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	if err != nil {
		t.Fatalf("failed to register backend: %v", err)
	}

	// Wait for backend to become stale
	time.Sleep(100 * time.Millisecond)

	// External validation says unhealthy
	err = registry.UpdateValidation("web", "192.168.1.1", 80, false, "connection refused")
	if err != nil {
		t.Fatalf("failed to update validation: %v", err)
	}

	// Verify stale backend with unhealthy validation stays unhealthy
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusUnhealthy {
		t.Errorf("expected stale backend with unhealthy validation to be unhealthy, got %s", backend.EffectiveStatus)
	}
}

func TestRegistry_StaleWithoutValidation(t *testing.T) {
	// Use a very short stale threshold for testing
	cfg := RegistryConfig{
		StaleThreshold: 50 * time.Millisecond,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Start registry to enable stale detection background loop
	_ = registry.Start()
	defer registry.Stop()

	// Register backend
	err := registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	if err != nil {
		t.Fatalf("failed to register backend: %v", err)
	}

	// Wait for backend to become stale (stale detection runs every 25ms)
	time.Sleep(100 * time.Millisecond)

	// Verify backend is stale without validation
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.ValidationHealthy != nil {
		t.Error("validation should not be set")
	}
	if backend.EffectiveStatus != StatusStale {
		t.Errorf("expected stale backend without validation to be stale, got %s", backend.EffectiveStatus)
	}
}

func TestRegistry_OverrideWinsOverStale(t *testing.T) {
	// Use a very short stale threshold for testing
	cfg := RegistryConfig{
		StaleThreshold: 50 * time.Millisecond,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Start registry to enable stale detection background loop
	_ = registry.Start()
	defer registry.Stop()

	// Register backend
	err := registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)
	if err != nil {
		t.Fatalf("failed to register backend: %v", err)
	}

	// Wait for backend to become stale (stale detection runs every 25ms)
	time.Sleep(100 * time.Millisecond)

	// Verify backend is stale
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusStale {
		t.Errorf("expected backend to be stale, got %s", backend.EffectiveStatus)
	}

	// Set manual override to healthy - should override stale status
	err = registry.SetOverride("web", "192.168.1.1", 80, true, "maintenance bypass", "admin")
	if err != nil {
		t.Fatalf("failed to set override: %v", err)
	}

	// Verify override wins over stale status
	backend, _ = registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusHealthy {
		t.Errorf("expected override to win over stale, got %s", backend.EffectiveStatus)
	}
}

func TestRegistry_HealthAuthorityHierarchy(t *testing.T) {
	// Test the full health authority hierarchy:
	// 1. Override (highest)
	// 2. Validation
	// 3. Staleness
	// 4. Agent claim (lowest)

	cfg := RegistryConfig{
		StaleThreshold: 50 * time.Millisecond,
		RemoveAfter:    5 * time.Minute,
	}
	registry := NewRegistry(cfg, nil)

	// Start registry to enable stale detection background loop
	_ = registry.Start()
	defer registry.Stop()

	// Register backend with healthy agent claim
	_ = registry.Register("agent-1", "us-east", "web", "192.168.1.1", 80, 100, true)

	// Step 1: Agent says healthy → effective is healthy
	backend, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusHealthy {
		t.Errorf("step 1: expected healthy from agent claim, got %s", backend.EffectiveStatus)
	}

	// Step 2: Validation says unhealthy → validation wins over agent
	_ = registry.UpdateValidation("web", "192.168.1.1", 80, false, "")
	backend, _ = registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusUnhealthy {
		t.Errorf("step 2: expected unhealthy from validation (wins over agent), got %s", backend.EffectiveStatus)
	}

	// Step 3: Override says healthy → override wins over validation
	_ = registry.SetOverride("web", "192.168.1.1", 80, true, "test", "test")
	backend, _ = registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusHealthy {
		t.Errorf("step 3: expected healthy from override (wins over validation), got %s", backend.EffectiveStatus)
	}

	// Step 4: Clear override, validation still present → back to validation status
	_ = registry.ClearOverride("web", "192.168.1.1", 80)
	backend, _ = registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusUnhealthy {
		t.Errorf("step 4: expected unhealthy from validation after clearing override, got %s", backend.EffectiveStatus)
	}

	// Step 5: Wait for stale, but validation still present → validation wins over staleness
	time.Sleep(100 * time.Millisecond)
	backend, _ = registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusUnhealthy {
		t.Errorf("step 5: expected unhealthy from validation (wins over staleness), got %s", backend.EffectiveStatus)
	}

	// Step 6: Update validation to healthy → recovered from stale
	_ = registry.UpdateValidation("web", "192.168.1.1", 80, true, "")
	backend, _ = registry.GetBackend("web", "192.168.1.1", 80)
	if backend.EffectiveStatus != StatusHealthy {
		t.Errorf("step 6: expected healthy from validation (recovered from stale), got %s", backend.EffectiveStatus)
	}
}
