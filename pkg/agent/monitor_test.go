// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewMonitor(t *testing.T) {
	m := NewMonitor(nil, 0)
	if m == nil {
		t.Fatal("NewMonitor returned nil")
	}
	if m.errorWindow != 60*time.Second {
		t.Errorf("expected default error window of 60s, got %v", m.errorWindow)
	}
}

func TestNewMonitor_CustomWindow(t *testing.T) {
	m := NewMonitor(nil, 30*time.Second)
	if m.errorWindow != 30*time.Second {
		t.Errorf("expected error window of 30s, got %v", m.errorWindow)
	}
}

func TestMonitor_CPUPercent(t *testing.T) {
	// Create temporary /proc/stat file
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")

	// Write initial CPU stats
	initialContent := `cpu  1000 100 200 8000 50 10 5 0 0 0
cpu0 500 50 100 4000 25 5 2 0 0 0
`
	if err := os.WriteFile(statPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial stat file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths(statPath, "/dev/null")

	// First read - establishes baseline
	cpu1, err := m.cpuPercent()
	if err != nil {
		t.Fatalf("first cpuPercent failed: %v", err)
	}
	if cpu1 != 0 {
		t.Errorf("expected 0 for first reading, got %v", cpu1)
	}

	// Simulate 50% CPU usage: 500 more used time, 500 more total
	updatedContent := `cpu  1200 150 300 8500 60 15 8 0 0 0
cpu0 600 75 150 4250 30 7 4 0 0 0
`
	if err := os.WriteFile(statPath, []byte(updatedContent), 0644); err != nil {
		t.Fatalf("failed to write updated stat file: %v", err)
	}

	// Second read
	cpu2, err := m.cpuPercent()
	if err != nil {
		t.Fatalf("second cpuPercent failed: %v", err)
	}

	// Calculate expected: total delta = 933, idle delta = 510
	// Used = 933 - 510 = 423, percent = 423/933 * 100 = ~45.3%
	if cpu2 < 40 || cpu2 > 50 {
		t.Errorf("expected CPU around 45%%, got %.2f%%", cpu2)
	}
}

func TestMonitor_MemoryPercent(t *testing.T) {
	dir := t.TempDir()
	meminfoPath := filepath.Join(dir, "meminfo")

	// 16GB total, 4GB available = 75% used
	content := `MemTotal:       16384000 kB
MemFree:          512000 kB
MemAvailable:    4096000 kB
Buffers:          256000 kB
Cached:          2048000 kB
`
	if err := os.WriteFile(meminfoPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths("/dev/null", meminfoPath)

	mem, err := m.memoryPercent()
	if err != nil {
		t.Fatalf("memoryPercent failed: %v", err)
	}

	// Expected: (16384000 - 4096000) / 16384000 * 100 = 75%
	if mem < 74 || mem > 76 {
		t.Errorf("expected memory around 75%%, got %.2f%%", mem)
	}
}

func TestMonitor_MemoryPercent_HighUsage(t *testing.T) {
	dir := t.TempDir()
	meminfoPath := filepath.Join(dir, "meminfo")

	// 16GB total, 1.6GB available = 90% used
	content := `MemTotal:       16384000 kB
MemFree:          100000 kB
MemAvailable:    1638400 kB
`
	if err := os.WriteFile(meminfoPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths("/dev/null", meminfoPath)

	mem, err := m.memoryPercent()
	if err != nil {
		t.Fatalf("memoryPercent failed: %v", err)
	}

	if mem < 89 || mem > 91 {
		t.Errorf("expected memory around 90%%, got %.2f%%", mem)
	}
}

func TestMonitor_ErrorRateTracking(t *testing.T) {
	m := NewMonitor(nil, time.Minute)

	// Record 5 errors
	for i := 0; i < 5; i++ {
		m.RecordError()
	}

	count := m.ErrorCount()
	if count != 5 {
		t.Errorf("expected 5 errors, got %d", count)
	}

	rate := m.errorRate()
	// 5 errors in 1 minute window = 5/minute
	if rate < 4.9 || rate > 5.1 {
		t.Errorf("expected error rate around 5/min, got %.2f", rate)
	}
}

func TestMonitor_ErrorRateWindowExpiry(t *testing.T) {
	// Use a 100ms window for fast testing
	m := NewMonitor(nil, 100*time.Millisecond)

	// Record errors
	m.RecordError()
	m.RecordError()

	count1 := m.ErrorCount()
	if count1 != 2 {
		t.Errorf("expected 2 errors immediately, got %d", count1)
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	count2 := m.ErrorCount()
	if count2 != 0 {
		t.Errorf("expected 0 errors after window expiry, got %d", count2)
	}
}

func TestMonitor_Collect(t *testing.T) {
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

	// Record some errors
	m.RecordError()
	m.RecordError()
	m.RecordError()

	metrics, err := m.Collect()
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if metrics == nil {
		t.Fatal("Collect returned nil metrics")
	}

	// Memory should be around 50%
	if metrics.MemoryPercent < 49 || metrics.MemoryPercent > 51 {
		t.Errorf("expected memory around 50%%, got %.2f%%", metrics.MemoryPercent)
	}

	// Error rate should be 3/minute
	if metrics.ErrorRate < 2.9 || metrics.ErrorRate > 3.1 {
		t.Errorf("expected error rate around 3/min, got %.2f", metrics.ErrorRate)
	}

	if metrics.Timestamp.IsZero() {
		t.Error("timestamp should not be zero")
	}
}

func TestMonitor_CPUPercent_MissingFile(t *testing.T) {
	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths("/nonexistent/stat", "/dev/null")

	_, err := m.cpuPercent()
	if err == nil {
		t.Error("expected error for missing stat file")
	}
}

func TestMonitor_MemoryPercent_MissingFile(t *testing.T) {
	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths("/dev/null", "/nonexistent/meminfo")

	_, err := m.memoryPercent()
	if err == nil {
		t.Error("expected error for missing meminfo file")
	}
}

func TestMonitor_MemoryPercent_MissingFields(t *testing.T) {
	dir := t.TempDir()
	meminfoPath := filepath.Join(dir, "meminfo")

	// Missing MemAvailable
	content := `MemTotal:       16384000 kB
MemFree:          512000 kB
`
	if err := os.WriteFile(meminfoPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write meminfo file: %v", err)
	}

	m := NewMonitor(nil, time.Minute)
	m.SetProcPaths("/dev/null", meminfoPath)

	_, err := m.memoryPercent()
	if err == nil {
		t.Error("expected error for missing MemAvailable")
	}
}

func TestParseCPULine(t *testing.T) {
	m := NewMonitor(nil, time.Minute)

	tests := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{
			name:    "valid line",
			line:    "cpu  1000 100 200 8000 50 10 5 0 0 0",
			wantErr: false,
		},
		{
			name:    "too few fields",
			line:    "cpu  1000 100 200",
			wantErr: true,
		},
		{
			name:    "invalid number",
			line:    "cpu  abc 100 200 8000 50 10 5 0 0 0",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, err := m.parseCPULine(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if stats.user != 1000 {
					t.Errorf("expected user=1000, got %d", stats.user)
				}
			}
		})
	}
}
