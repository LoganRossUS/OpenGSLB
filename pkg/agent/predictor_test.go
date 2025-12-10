// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// mockSystemMonitor implements SystemMonitor for testing.
type mockSystemMonitor struct {
	mu        sync.Mutex
	cpu       float64
	memory    float64
	errorRate float64
	cpuErr    error
	memErr    error
	errErr    error
}

func (m *mockSystemMonitor) CPUPercent() (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cpu, m.cpuErr
}

func (m *mockSystemMonitor) MemoryPercent() (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.memory, m.memErr
}

func (m *mockSystemMonitor) ErrorRate() (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.errorRate, m.errErr
}

func (m *mockSystemMonitor) setCPU(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cpu = v
}

func (m *mockSystemMonitor) setMemory(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.memory = v
}

func (m *mockSystemMonitor) setErrorRate(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorRate = v
}

func TestPredictor_NoBleeding(t *testing.T) {
	monitor := &mockSystemMonitor{
		cpu:       50.0,
		memory:    60.0,
		errorRate: 1.0,
	}

	cfg := PredictiveConfig{
		Enabled:            true,
		CPUThreshold:       85.0,
		MemoryThreshold:    90.0,
		ErrorRateThreshold: 5.0,
		CheckInterval:      50 * time.Millisecond,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	predictor := NewPredictor(cfg, monitor, logger)

	predictor.Start()
	defer predictor.Stop()

	// Wait for a check cycle
	time.Sleep(100 * time.Millisecond)

	if predictor.IsBleeding() {
		t.Error("expected not bleeding with metrics below threshold")
	}

	state := predictor.GetState()
	if state.Bleeding {
		t.Error("state should not be bleeding")
	}
}

func TestPredictor_CPUBleeding(t *testing.T) {
	monitor := &mockSystemMonitor{
		cpu:       90.0, // Above threshold
		memory:    60.0,
		errorRate: 1.0,
	}

	cfg := PredictiveConfig{
		Enabled:            true,
		CPUThreshold:       85.0,
		MemoryThreshold:    90.0,
		ErrorRateThreshold: 5.0,
		CheckInterval:      50 * time.Millisecond,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	predictor := NewPredictor(cfg, monitor, logger)

	predictor.Start()
	defer predictor.Stop()

	// Wait for a check cycle
	time.Sleep(100 * time.Millisecond)

	if !predictor.IsBleeding() {
		t.Error("expected bleeding with CPU above threshold")
	}

	state := predictor.GetState()
	if state.BleedReason != "cpu_threshold_exceeded" {
		t.Errorf("expected bleed reason 'cpu_threshold_exceeded', got %s", state.BleedReason)
	}
}

func TestPredictor_MemoryBleeding(t *testing.T) {
	monitor := &mockSystemMonitor{
		cpu:       50.0,
		memory:    95.0, // Above threshold
		errorRate: 1.0,
	}

	cfg := PredictiveConfig{
		Enabled:            true,
		CPUThreshold:       85.0,
		MemoryThreshold:    90.0,
		ErrorRateThreshold: 5.0,
		CheckInterval:      50 * time.Millisecond,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	predictor := NewPredictor(cfg, monitor, logger)

	predictor.Start()
	defer predictor.Stop()

	// Wait for a check cycle
	time.Sleep(100 * time.Millisecond)

	if !predictor.IsBleeding() {
		t.Error("expected bleeding with memory above threshold")
	}

	state := predictor.GetState()
	if state.BleedReason != "memory_threshold_exceeded" {
		t.Errorf("expected bleed reason 'memory_threshold_exceeded', got %s", state.BleedReason)
	}
}

func TestPredictor_Disabled(t *testing.T) {
	monitor := &mockSystemMonitor{
		cpu:       95.0,
		memory:    95.0,
		errorRate: 10.0,
	}

	cfg := PredictiveConfig{
		Enabled:            false, // Disabled
		CPUThreshold:       85.0,
		MemoryThreshold:    90.0,
		ErrorRateThreshold: 5.0,
		CheckInterval:      50 * time.Millisecond,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	predictor := NewPredictor(cfg, monitor, logger)

	predictor.Start()
	defer predictor.Stop()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Should not be bleeding when disabled
	if predictor.IsBleeding() {
		t.Error("predictor should not bleed when disabled")
	}
}
