// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/cluster"
	"github.com/loganrossus/OpenGSLB/pkg/config"
)

func createTestMonitor(t *testing.T, cpuPercent, memPercent float64) *Monitor {
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")
	meminfoPath := filepath.Join(dir, "meminfo")

	// CPU file
	statContent := `cpu  1000 100 200 8000 50 10 5 0 0 0
`
	if err := os.WriteFile(statPath, []byte(statContent), 0644); err != nil {
		t.Fatalf("failed to write stat file: %v", err)
	}

	// Memory file - calculate values to get desired percentage
	// percent = (total - available) / total * 100
	// available = total * (1 - percent/100)
	total := uint64(16384000)
	available := uint64(float64(total) * (1 - memPercent/100))
	meminfoContent := "MemTotal:       16384000 kB\n"
	meminfoContent += "MemAvailable:   " + string(rune(available)) + " kB\n"

	// Use specific values for predictable results
	if memPercent >= 90 {
		meminfoContent = `MemTotal:       16384000 kB
MemAvailable:    1638400 kB
`
	} else if memPercent >= 50 {
		meminfoContent = `MemTotal:       16384000 kB
MemAvailable:    8192000 kB
`
	} else {
		meminfoContent = `MemTotal:       16384000 kB
MemAvailable:   12288000 kB
`
	}

	if err := os.WriteFile(meminfoPath, []byte(meminfoContent), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths(statPath, meminfoPath)

	// Initial read to establish baseline
	_, _ = m.cpuPercent()

	return m
}

func TestNewPredictor(t *testing.T) {
	cfg := config.PredictiveHealthConfig{
		Enabled: true,
		CPU:     config.PredictiveMetricConfig{Threshold: 90, BleedDuration: 30 * time.Second},
	}
	m := NewMonitor(nil, time.Minute)
	p := NewPredictor(cfg, "test-node", m, nil)

	if p == nil {
		t.Fatal("NewPredictor returned nil")
	}
	if p.nodeID != "test-node" {
		t.Errorf("expected nodeID 'test-node', got %s", p.nodeID)
	}
}

func TestPredictor_IsBleeding_Initially(t *testing.T) {
	cfg := config.PredictiveHealthConfig{Enabled: true}
	m := NewMonitor(nil, time.Minute)
	p := NewPredictor(cfg, "test-node", m, nil)

	if p.IsBleeding() {
		t.Error("expected not bleeding initially")
	}
	if p.BleedReason() != "" {
		t.Errorf("expected empty bleed reason, got %s", p.BleedReason())
	}
}

func TestPredictor_MemoryThresholdExceeded(t *testing.T) {
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")
	meminfoPath := filepath.Join(dir, "meminfo")

	statContent := `cpu  1000 100 200 8000 50 10 5 0 0 0
`
	// 90% memory usage
	meminfoContent := `MemTotal:       16384000 kB
MemAvailable:    1638400 kB
`
	if err := os.WriteFile(statPath, []byte(statContent), 0644); err != nil {
		t.Fatalf("failed to write stat file: %v", err)
	}
	if err := os.WriteFile(meminfoPath, []byte(meminfoContent), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths(statPath, meminfoPath)
	_, _ = m.cpuPercent() // establish baseline

	cfg := config.PredictiveHealthConfig{
		Enabled: true,
		CPU:     config.PredictiveMetricConfig{Threshold: 95, BleedDuration: 30 * time.Second},
		Memory:  config.PredictiveMetricConfig{Threshold: 85, BleedDuration: 30 * time.Second},
		ErrorRate: config.PredictiveErrorRateConfig{
			Threshold:     100,
			Window:        time.Minute,
			BleedDuration: time.Minute,
		},
	}

	var receivedSignal *cluster.PredictiveSignal
	p := NewPredictor(cfg, "test-node", m, nil)
	p.OnSignal(func(s *cluster.PredictiveSignal) {
		receivedSignal = s
	})

	signal, err := p.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if signal == nil {
		t.Fatal("expected signal for high memory, got nil")
	}

	if signal.Signal != SignalBleed {
		t.Errorf("expected signal 'bleed', got %s", signal.Signal)
	}
	if signal.Reason != ReasonMemoryHigh {
		t.Errorf("expected reason 'memory_high', got %s", signal.Reason)
	}
	if signal.NodeID != "test-node" {
		t.Errorf("expected nodeID 'test-node', got %s", signal.NodeID)
	}
	if !p.IsBleeding() {
		t.Error("expected predictor to be bleeding")
	}

	// Callback should not have been called (we returned the signal)
	if receivedSignal != nil {
		t.Error("callback should not be invoked from Evaluate()")
	}
}

func TestPredictor_ErrorRateThresholdExceeded(t *testing.T) {
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")
	meminfoPath := filepath.Join(dir, "meminfo")

	statContent := `cpu  1000 100 200 8000 50 10 5 0 0 0
`
	// Low memory usage
	meminfoContent := `MemTotal:       16384000 kB
MemAvailable:   14745600 kB
`
	if err := os.WriteFile(statPath, []byte(statContent), 0644); err != nil {
		t.Fatalf("failed to write stat file: %v", err)
	}
	if err := os.WriteFile(meminfoPath, []byte(meminfoContent), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths(statPath, meminfoPath)
	_, _ = m.cpuPercent()

	// Record many errors to exceed threshold
	for i := 0; i < 15; i++ {
		m.RecordError()
	}

	cfg := config.PredictiveHealthConfig{
		Enabled: true,
		CPU:     config.PredictiveMetricConfig{Threshold: 95, BleedDuration: 30 * time.Second},
		Memory:  config.PredictiveMetricConfig{Threshold: 95, BleedDuration: 30 * time.Second},
		ErrorRate: config.PredictiveErrorRateConfig{
			Threshold:     10,
			Window:        time.Minute,
			BleedDuration: time.Minute,
		},
	}

	p := NewPredictor(cfg, "test-node", m, nil)
	signal, err := p.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if signal == nil {
		t.Fatal("expected signal for high error rate, got nil")
	}

	if signal.Reason != ReasonErrorRateHigh {
		t.Errorf("expected reason 'error_rate_high', got %s", signal.Reason)
	}
}

func TestPredictor_SignalCleared(t *testing.T) {
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")
	meminfoPath := filepath.Join(dir, "meminfo")

	statContent := `cpu  1000 100 200 8000 50 10 5 0 0 0
`
	// High memory initially
	highMemContent := `MemTotal:       16384000 kB
MemAvailable:    1638400 kB
`
	if err := os.WriteFile(statPath, []byte(statContent), 0644); err != nil {
		t.Fatalf("failed to write stat file: %v", err)
	}
	if err := os.WriteFile(meminfoPath, []byte(highMemContent), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths(statPath, meminfoPath)
	_, _ = m.cpuPercent()

	cfg := config.PredictiveHealthConfig{
		Enabled: true,
		CPU:     config.PredictiveMetricConfig{Threshold: 95, BleedDuration: 30 * time.Second},
		Memory:  config.PredictiveMetricConfig{Threshold: 85, BleedDuration: 30 * time.Second},
		ErrorRate: config.PredictiveErrorRateConfig{
			Threshold:     100,
			Window:        time.Minute,
			BleedDuration: time.Minute,
		},
	}

	p := NewPredictor(cfg, "test-node", m, nil)

	// First evaluation - should trigger bleed
	signal1, _ := p.Evaluate()
	if signal1 == nil || signal1.Signal != SignalBleed {
		t.Fatal("expected bleed signal")
	}

	// Now "recover" - write low memory file
	lowMemContent := `MemTotal:       16384000 kB
MemAvailable:   14745600 kB
`
	if err := os.WriteFile(meminfoPath, []byte(lowMemContent), 0644); err != nil {
		t.Fatalf("failed to write recovery meminfo file: %v", err)
	}

	// Second evaluation - should clear
	signal2, err := p.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if signal2 == nil {
		t.Fatal("expected clear signal, got nil")
	}

	if signal2.Signal != SignalClear {
		t.Errorf("expected signal 'clear', got %s", signal2.Signal)
	}
	if signal2.Reason != ReasonRecovered {
		t.Errorf("expected reason 'recovered', got %s", signal2.Reason)
	}
	if p.IsBleeding() {
		t.Error("expected predictor to not be bleeding after recovery")
	}
}

func TestPredictor_NoSignalWhenNotExceeded(t *testing.T) {
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")
	meminfoPath := filepath.Join(dir, "meminfo")

	statContent := `cpu  1000 100 200 8000 50 10 5 0 0 0
`
	// Low usage
	meminfoContent := `MemTotal:       16384000 kB
MemAvailable:   14745600 kB
`
	if err := os.WriteFile(statPath, []byte(statContent), 0644); err != nil {
		t.Fatalf("failed to write stat file: %v", err)
	}
	if err := os.WriteFile(meminfoPath, []byte(meminfoContent), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths(statPath, meminfoPath)
	_, _ = m.cpuPercent()

	cfg := config.PredictiveHealthConfig{
		Enabled: true,
		CPU:     config.PredictiveMetricConfig{Threshold: 90, BleedDuration: 30 * time.Second},
		Memory:  config.PredictiveMetricConfig{Threshold: 85, BleedDuration: 30 * time.Second},
		ErrorRate: config.PredictiveErrorRateConfig{
			Threshold:     10,
			Window:        time.Minute,
			BleedDuration: time.Minute,
		},
	}

	p := NewPredictor(cfg, "test-node", m, nil)
	signal, err := p.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if signal != nil {
		t.Errorf("expected nil signal when not exceeded, got %+v", signal)
	}
}

func TestPredictor_Disabled(t *testing.T) {
	m := NewMonitor(nil, time.Minute)
	cfg := config.PredictiveHealthConfig{Enabled: false}

	p := NewPredictor(cfg, "test-node", m, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start should return immediately when disabled
	err := p.Start(ctx)
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPredictor_StartLoop(t *testing.T) {
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")
	meminfoPath := filepath.Join(dir, "meminfo")

	statContent := `cpu  1000 100 200 8000 50 10 5 0 0 0
`
	// High memory to trigger signal
	meminfoContent := `MemTotal:       16384000 kB
MemAvailable:    1638400 kB
`
	if err := os.WriteFile(statPath, []byte(statContent), 0644); err != nil {
		t.Fatalf("failed to write stat file: %v", err)
	}
	if err := os.WriteFile(meminfoPath, []byte(meminfoContent), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths(statPath, meminfoPath)
	_, _ = m.cpuPercent()

	cfg := config.PredictiveHealthConfig{
		Enabled: true,
		CPU:     config.PredictiveMetricConfig{Threshold: 95, BleedDuration: 30 * time.Second},
		Memory:  config.PredictiveMetricConfig{Threshold: 85, BleedDuration: 30 * time.Second},
		ErrorRate: config.PredictiveErrorRateConfig{
			Threshold:     100,
			Window:        time.Minute,
			BleedDuration: time.Minute,
		},
	}

	var mu sync.Mutex
	var signals []*cluster.PredictiveSignal

	p := NewPredictor(cfg, "test-node", m, nil)
	p.SetInterval(50 * time.Millisecond) // Fast for testing
	p.OnSignal(func(s *cluster.PredictiveSignal) {
		mu.Lock()
		signals = append(signals, s)
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start in goroutine
	go func() {
		_ = p.Start(ctx)
	}()

	// Wait for at least one evaluation
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	signalCount := len(signals)
	mu.Unlock()

	// Should have received at least one signal
	if signalCount < 1 {
		t.Error("expected at least one signal from the loop")
	}
}

func TestPredictor_GetMetrics(t *testing.T) {
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")
	meminfoPath := filepath.Join(dir, "meminfo")

	statContent := `cpu  1000 100 200 8000 50 10 5 0 0 0
`
	meminfoContent := `MemTotal:       16384000 kB
MemAvailable:    8192000 kB
`
	if err := os.WriteFile(statPath, []byte(statContent), 0644); err != nil {
		t.Fatalf("failed to write stat file: %v", err)
	}
	if err := os.WriteFile(meminfoPath, []byte(meminfoContent), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths(statPath, meminfoPath)
	_, _ = m.cpuPercent()

	cfg := config.PredictiveHealthConfig{Enabled: true}
	p := NewPredictor(cfg, "test-node", m, nil)

	metrics, bleeding, reason := p.GetMetrics()
	if metrics == nil {
		t.Fatal("GetMetrics returned nil metrics")
	}
	if bleeding {
		t.Error("expected not bleeding")
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %s", reason)
	}
}
