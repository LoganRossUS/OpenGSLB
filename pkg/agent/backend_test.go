// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/health"
)

// mockChecker implements health.Checker for testing.
type mockChecker struct {
	mu       sync.Mutex
	results  map[string]health.Result
	checkErr error
}

func newMockChecker() *mockChecker {
	return &mockChecker{
		results: make(map[string]health.Result),
	}
}

func (m *mockChecker) Check(ctx context.Context, target health.Target) health.Result {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := target.Address
	if result, ok := m.results[key]; ok {
		return result
	}

	// Default: healthy
	return health.Result{
		Healthy:   true,
		Latency:   10 * time.Millisecond,
		Timestamp: time.Now(),
	}
}

func (m *mockChecker) Type() string {
	return "mock"
}

func (m *mockChecker) SetResult(address string, result health.Result) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[address] = result
}

func TestBackendManager_AddBackend(t *testing.T) {
	checker := newMockChecker()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewBackendManager(checker, logger)

	err := manager.AddBackend(BackendConfig{
		Service: "webapp",
		Address: "127.0.0.1",
		Port:    8080,
		Weight:  100,
		HealthCheck: HealthCheckConfig{
			Type:     "http",
			Path:     "/health",
			Interval: 100 * time.Millisecond,
			Timeout:  50 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("AddBackend failed: %v", err)
	}

	if manager.BackendCount() != 1 {
		t.Errorf("expected 1 backend, got %d", manager.BackendCount())
	}

	// Adding duplicate should fail
	err = manager.AddBackend(BackendConfig{
		Service: "webapp",
		Address: "127.0.0.1",
		Port:    8080,
	})
	if err == nil {
		t.Error("expected error for duplicate backend")
	}
}

func TestBackendManager_RemoveBackend(t *testing.T) {
	checker := newMockChecker()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewBackendManager(checker, logger)

	manager.AddBackend(BackendConfig{
		Service: "webapp",
		Address: "127.0.0.1",
		Port:    8080,
	})

	err := manager.RemoveBackend("webapp", "127.0.0.1", 8080)
	if err != nil {
		t.Fatalf("RemoveBackend failed: %v", err)
	}

	if manager.BackendCount() != 0 {
		t.Errorf("expected 0 backends after remove, got %d", manager.BackendCount())
	}

	// Removing non-existent should fail
	err = manager.RemoveBackend("webapp", "127.0.0.1", 8080)
	if err == nil {
		t.Error("expected error for non-existent backend")
	}
}

func TestBackendManager_MultipleBackends(t *testing.T) {
	checker := newMockChecker()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewBackendManager(checker, logger)

	// Add multiple backends
	backends := []BackendConfig{
		{Service: "webapp", Address: "10.0.1.1", Port: 8080, Weight: 100},
		{Service: "webapp", Address: "10.0.1.2", Port: 8080, Weight: 100},
		{Service: "api", Address: "10.0.2.1", Port: 9000, Weight: 50},
	}

	for _, cfg := range backends {
		if err := manager.AddBackend(cfg); err != nil {
			t.Fatalf("AddBackend failed for %s: %v", cfg.Address, err)
		}
	}

	if manager.BackendCount() != 3 {
		t.Errorf("expected 3 backends, got %d", manager.BackendCount())
	}
}

func TestBackendManager_HealthChecking(t *testing.T) {
	checker := newMockChecker()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewBackendManager(checker, logger)

	// Configure mock to return healthy for this address
	checker.SetResult("10.0.1.1", health.Result{
		Healthy:   true,
		Latency:   5 * time.Millisecond,
		Timestamp: time.Now(),
	})

	err := manager.AddBackend(BackendConfig{
		Service: "webapp",
		Address: "10.0.1.1",
		Port:    8080,
		HealthCheck: HealthCheckConfig{
			Type:             "http",
			Interval:         50 * time.Millisecond,
			Timeout:          30 * time.Millisecond,
			FailureThreshold: 2,
			SuccessThreshold: 2,
		},
	})
	if err != nil {
		t.Fatalf("AddBackend failed: %v", err)
	}

	// Start manager
	if err := manager.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer manager.Stop()

	// Wait for health checks to run
	time.Sleep(150 * time.Millisecond)

	// Should be healthy after 2 successful checks
	snapshot, found := manager.GetHealth("webapp", "10.0.1.1", 8080)
	if !found {
		t.Fatal("backend not found")
	}

	if !snapshot.Healthy {
		t.Errorf("expected backend to be healthy, got unhealthy (passes: %d)",
			snapshot.ConsecutivePasses)
	}
}

func TestBackendManager_UnhealthyBackend(t *testing.T) {
	checker := newMockChecker()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewBackendManager(checker, logger)

	// Configure mock to return unhealthy
	checker.SetResult("10.0.1.1", health.Result{
		Healthy:   false,
		Error:     context.DeadlineExceeded,
		Timestamp: time.Now(),
	})

	err := manager.AddBackend(BackendConfig{
		Service: "webapp",
		Address: "10.0.1.1",
		Port:    8080,
		HealthCheck: HealthCheckConfig{
			Type:             "http",
			Interval:         50 * time.Millisecond,
			Timeout:          30 * time.Millisecond,
			FailureThreshold: 2,
			SuccessThreshold: 2,
		},
	})
	if err != nil {
		t.Fatalf("AddBackend failed: %v", err)
	}

	if err := manager.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer manager.Stop()

	// Wait for health checks
	time.Sleep(150 * time.Millisecond)

	snapshot, _ := manager.GetHealth("webapp", "10.0.1.1", 8080)
	if snapshot.Healthy {
		t.Error("expected backend to be unhealthy")
	}
	if snapshot.ConsecutiveFails < 2 {
		t.Errorf("expected at least 2 failures, got %d", snapshot.ConsecutiveFails)
	}
}

func TestBackendManager_OnHealthChange(t *testing.T) {
	checker := newMockChecker()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewBackendManager(checker, logger)

	var updates []BackendHealthUpdate
	var mu sync.Mutex

	manager.OnHealthChange(func(update BackendHealthUpdate) {
		mu.Lock()
		updates = append(updates, update)
		mu.Unlock()
	})

	// Start healthy
	checker.SetResult("10.0.1.1", health.Result{
		Healthy:   true,
		Timestamp: time.Now(),
	})

	err := manager.AddBackend(BackendConfig{
		Service: "webapp",
		Address: "10.0.1.1",
		Port:    8080,
		HealthCheck: HealthCheckConfig{
			Interval:         50 * time.Millisecond,
			Timeout:          30 * time.Millisecond,
			FailureThreshold: 2,
			SuccessThreshold: 2,
		},
	})
	if err != nil {
		t.Fatalf("AddBackend failed: %v", err)
	}

	if err := manager.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer manager.Stop()

	// Wait for it to become healthy
	time.Sleep(150 * time.Millisecond)

	// Now make it unhealthy
	checker.SetResult("10.0.1.1", health.Result{
		Healthy:   false,
		Error:     context.DeadlineExceeded,
		Timestamp: time.Now(),
	})

	// Wait for state change
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	updateCount := len(updates)
	mu.Unlock()

	// Should have received at least 2 updates (healthy -> established, healthy -> unhealthy)
	if updateCount < 2 {
		t.Errorf("expected at least 2 health change callbacks, got %d", updateCount)
	}
}

func TestBackendManager_GetAllHealth(t *testing.T) {
	checker := newMockChecker()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewBackendManager(checker, logger)

	manager.AddBackend(BackendConfig{Service: "webapp", Address: "10.0.1.1", Port: 8080})
	manager.AddBackend(BackendConfig{Service: "api", Address: "10.0.2.1", Port: 9000})

	snapshots := manager.GetAllHealth()
	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snapshots))
	}
}

func TestBackendManager_HealthyCount(t *testing.T) {
	checker := newMockChecker()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewBackendManager(checker, logger)

	// One healthy, one unhealthy
	checker.SetResult("10.0.1.1", health.Result{Healthy: true, Timestamp: time.Now()})
	checker.SetResult("10.0.1.2", health.Result{Healthy: false, Timestamp: time.Now()})

	manager.AddBackend(BackendConfig{
		Service: "webapp",
		Address: "10.0.1.1",
		Port:    8080,
		HealthCheck: HealthCheckConfig{
			Interval:         50 * time.Millisecond,
			FailureThreshold: 1,
			SuccessThreshold: 1,
		},
	})
	manager.AddBackend(BackendConfig{
		Service: "webapp",
		Address: "10.0.1.2",
		Port:    8080,
		HealthCheck: HealthCheckConfig{
			Interval:         50 * time.Millisecond,
			FailureThreshold: 1,
			SuccessThreshold: 1,
		},
	})

	if err := manager.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer manager.Stop()

	time.Sleep(100 * time.Millisecond)

	if manager.HealthyCount() != 1 {
		t.Errorf("expected 1 healthy backend, got %d", manager.HealthyCount())
	}
}

func TestBackendHealth_RecordResult(t *testing.T) {
	bh := &BackendHealth{
		service:       "test",
		address:       "127.0.0.1",
		port:          8080,
		weight:        100,
		failThreshold: 3,
		passThreshold: 2,
	}

	// Initially unknown/unhealthy
	if bh.IsHealthy() {
		t.Error("should not be healthy initially")
	}

	// One success - not enough
	changed := bh.RecordResult(health.Result{Healthy: true, Timestamp: time.Now()})
	if changed {
		t.Error("status should not change after 1 success")
	}

	// Second success - should become healthy
	changed = bh.RecordResult(health.Result{Healthy: true, Timestamp: time.Now()})
	if !changed {
		t.Error("status should change after 2 successes")
	}
	if !bh.IsHealthy() {
		t.Error("should be healthy now")
	}

	// One failure - not enough to become unhealthy
	changed = bh.RecordResult(health.Result{Healthy: false, Timestamp: time.Now()})
	if changed {
		t.Error("status should not change after 1 failure")
	}
	if !bh.IsHealthy() {
		t.Error("should still be healthy")
	}

	// Two more failures (total 3) - should become unhealthy
	bh.RecordResult(health.Result{Healthy: false, Timestamp: time.Now()})
	changed = bh.RecordResult(health.Result{Healthy: false, Timestamp: time.Now()})
	if !changed {
		t.Error("status should change after 3 failures")
	}
	if bh.IsHealthy() {
		t.Error("should be unhealthy now")
	}
}

func TestBackendHealth_Snapshot(t *testing.T) {
	bh := &BackendHealth{
		service:       "webapp",
		address:       "10.0.1.1",
		port:          8080,
		weight:        150,
		failThreshold: 3,
		passThreshold: 2,
	}

	bh.RecordResult(health.Result{
		Healthy:   true,
		Latency:   25 * time.Millisecond,
		Timestamp: time.Now(),
	})

	snap := bh.Snapshot()
	if snap.Service != "webapp" {
		t.Errorf("unexpected service: %s", snap.Service)
	}
	if snap.Address != "10.0.1.1" {
		t.Errorf("unexpected address: %s", snap.Address)
	}
	if snap.Port != 8080 {
		t.Errorf("unexpected port: %d", snap.Port)
	}
	if snap.Weight != 150 {
		t.Errorf("unexpected weight: %d", snap.Weight)
	}
	if snap.ConsecutivePasses != 1 {
		t.Errorf("unexpected consecutive passes: %d", snap.ConsecutivePasses)
	}
	if snap.LastLatency != 25*time.Millisecond {
		t.Errorf("unexpected latency: %v", snap.LastLatency)
	}
}

func TestBackendManager_StartStop(t *testing.T) {
	checker := newMockChecker()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewBackendManager(checker, logger)

	manager.AddBackend(BackendConfig{
		Service: "webapp",
		Address: "10.0.1.1",
		Port:    8080,
	})

	// Start
	if err := manager.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Starting again should fail
	if err := manager.Start(); err == nil {
		t.Error("expected error when starting already running manager")
	}

	// Stop
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Stopping again should be safe
	if err := manager.Stop(); err != nil {
		t.Errorf("second Stop returned error: %v", err)
	}
}
