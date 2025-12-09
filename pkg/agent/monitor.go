// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package agent provides local system monitoring for predictive health signals.
// It implements the "predictive from the inside" philosophy where agent nodes
// monitor their own health and signal when they're likely to fail.
package agent

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Metrics contains current system metrics collected by the Monitor.
type Metrics struct {
	// CPUPercent is the CPU utilization percentage (0-100).
	CPUPercent float64

	// MemoryPercent is the memory utilization percentage (0-100).
	MemoryPercent float64

	// ErrorRate is the number of health check errors per minute.
	ErrorRate float64

	// Timestamp is when the metrics were collected.
	Timestamp time.Time
}

// cpuStats holds CPU time values from /proc/stat.
type cpuStats struct {
	user    uint64
	nice    uint64
	system  uint64
	idle    uint64
	iowait  uint64
	irq     uint64
	softirq uint64
	steal   uint64
}

// total returns the sum of all CPU time values.
func (c cpuStats) total() uint64 {
	return c.user + c.nice + c.system + c.idle + c.iowait + c.irq + c.softirq + c.steal
}

// idle returns the idle CPU time.
func (c cpuStats) idleTime() uint64 {
	return c.idle + c.iowait
}

// Monitor collects local system metrics for predictive health evaluation.
// It reads from /proc/stat and /proc/meminfo on Linux systems.
type Monitor struct {
	logger *slog.Logger

	// CPU calculation state
	lastCPUStats cpuStats
	lastCPUTime  time.Time
	cpuMu        sync.Mutex

	// Error rate tracking
	errorWindow     time.Duration
	errorTimestamps []time.Time
	errorMu         sync.Mutex

	// Paths for /proc files (can be overridden for testing)
	procStatPath    string
	procMeminfoPath string
}

// NewMonitor creates a new Monitor for collecting system metrics.
// The errorWindow parameter specifies the time window over which
// error rate is calculated.
func NewMonitor(logger *slog.Logger, errorWindow time.Duration) *Monitor {
	if logger == nil {
		logger = slog.Default()
	}
	if errorWindow <= 0 {
		errorWindow = 60 * time.Second
	}

	return &Monitor{
		logger:          logger,
		errorWindow:     errorWindow,
		errorTimestamps: make([]time.Time, 0),
		procStatPath:    "/proc/stat",
		procMeminfoPath: "/proc/meminfo",
	}
}

// Collect gathers current system metrics.
// Returns an error if metrics cannot be read (e.g., on non-Linux systems).
func (m *Monitor) Collect() (*Metrics, error) {
	cpu, err := m.cpuPercent()
	if err != nil {
		return nil, fmt.Errorf("failed to collect CPU metrics: %w", err)
	}

	mem, err := m.memoryPercent()
	if err != nil {
		return nil, fmt.Errorf("failed to collect memory metrics: %w", err)
	}

	return &Metrics{
		CPUPercent:    cpu,
		MemoryPercent: mem,
		ErrorRate:     m.errorRate(),
		Timestamp:     time.Now(),
	}, nil
}

// RecordError records a health check error for error rate calculation.
// Call this each time a health check fails.
func (m *Monitor) RecordError() {
	m.errorMu.Lock()
	defer m.errorMu.Unlock()

	m.errorTimestamps = append(m.errorTimestamps, time.Now())
}

// cpuPercent returns the current CPU utilization percentage.
// It calculates the percentage based on the delta between two readings.
func (m *Monitor) cpuPercent() (float64, error) {
	stats, err := m.readCPUStats()
	if err != nil {
		return 0, err
	}

	m.cpuMu.Lock()
	defer m.cpuMu.Unlock()

	now := time.Now()

	// If this is the first reading or too much time has passed, just save the stats
	if m.lastCPUTime.IsZero() || now.Sub(m.lastCPUTime) > 10*time.Minute {
		m.lastCPUStats = stats
		m.lastCPUTime = now
		// Return 0 for the first reading
		return 0, nil
	}

	// Calculate delta
	totalDelta := stats.total() - m.lastCPUStats.total()
	idleDelta := stats.idleTime() - m.lastCPUStats.idleTime()

	// Save current stats for next calculation
	m.lastCPUStats = stats
	m.lastCPUTime = now

	if totalDelta == 0 {
		return 0, nil
	}

	// CPU usage = (total - idle) / total * 100
	usedDelta := totalDelta - idleDelta
	percent := float64(usedDelta) / float64(totalDelta) * 100

	return percent, nil
}

// readCPUStats parses /proc/stat to get CPU time values.
func (m *Monitor) readCPUStats() (cpuStats, error) {
	file, err := os.Open(m.procStatPath)
	if err != nil {
		return cpuStats{}, fmt.Errorf("failed to open %s: %w", m.procStatPath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			return m.parseCPULine(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return cpuStats{}, fmt.Errorf("failed to read %s: %w", m.procStatPath, err)
	}

	return cpuStats{}, fmt.Errorf("cpu line not found in %s", m.procStatPath)
}

// parseCPULine parses a cpu line from /proc/stat.
// Format: cpu  user nice system idle iowait irq softirq steal guest guest_nice
func (m *Monitor) parseCPULine(line string) (cpuStats, error) {
	fields := strings.Fields(line)
	if len(fields) < 9 {
		return cpuStats{}, fmt.Errorf("unexpected format in cpu line: %s", line)
	}

	var stats cpuStats
	var err error

	stats.user, err = strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return cpuStats{}, fmt.Errorf("failed to parse user: %w", err)
	}

	stats.nice, err = strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return cpuStats{}, fmt.Errorf("failed to parse nice: %w", err)
	}

	stats.system, err = strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		return cpuStats{}, fmt.Errorf("failed to parse system: %w", err)
	}

	stats.idle, err = strconv.ParseUint(fields[4], 10, 64)
	if err != nil {
		return cpuStats{}, fmt.Errorf("failed to parse idle: %w", err)
	}

	stats.iowait, err = strconv.ParseUint(fields[5], 10, 64)
	if err != nil {
		return cpuStats{}, fmt.Errorf("failed to parse iowait: %w", err)
	}

	stats.irq, err = strconv.ParseUint(fields[6], 10, 64)
	if err != nil {
		return cpuStats{}, fmt.Errorf("failed to parse irq: %w", err)
	}

	stats.softirq, err = strconv.ParseUint(fields[7], 10, 64)
	if err != nil {
		return cpuStats{}, fmt.Errorf("failed to parse softirq: %w", err)
	}

	stats.steal, err = strconv.ParseUint(fields[8], 10, 64)
	if err != nil {
		return cpuStats{}, fmt.Errorf("failed to parse steal: %w", err)
	}

	return stats, nil
}

// memoryPercent returns the current memory utilization percentage.
func (m *Monitor) memoryPercent() (float64, error) {
	file, err := os.Open(m.procMeminfoPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open %s: %w", m.procMeminfoPath, err)
	}
	defer file.Close()

	var memTotal, memAvailable uint64
	var foundTotal, foundAvailable bool

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "MemTotal:") {
			val, err := parseMemValue(line)
			if err != nil {
				return 0, fmt.Errorf("failed to parse MemTotal: %w", err)
			}
			memTotal = val
			foundTotal = true
		} else if strings.HasPrefix(line, "MemAvailable:") {
			val, err := parseMemValue(line)
			if err != nil {
				return 0, fmt.Errorf("failed to parse MemAvailable: %w", err)
			}
			memAvailable = val
			foundAvailable = true
		}

		if foundTotal && foundAvailable {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("failed to read %s: %w", m.procMeminfoPath, err)
	}

	if !foundTotal {
		return 0, fmt.Errorf("MemTotal not found in %s", m.procMeminfoPath)
	}
	if !foundAvailable {
		return 0, fmt.Errorf("MemAvailable not found in %s", m.procMeminfoPath)
	}

	if memTotal == 0 {
		return 0, nil
	}

	// Memory usage = (total - available) / total * 100
	used := memTotal - memAvailable
	percent := float64(used) / float64(memTotal) * 100

	return percent, nil
}

// parseMemValue parses a line from /proc/meminfo.
// Format: MemTotal:       16384000 kB
func parseMemValue(line string) (uint64, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected format: %s", line)
	}

	// Parse the numeric value (in kB)
	val, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse value: %w", err)
	}

	return val, nil
}

// errorRate returns the current error rate (errors per minute).
// It counts errors within the configured time window and extrapolates to per-minute.
func (m *Monitor) errorRate() float64 {
	m.errorMu.Lock()
	defer m.errorMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-m.errorWindow)

	// Remove old timestamps
	validStart := 0
	for i, ts := range m.errorTimestamps {
		if ts.After(cutoff) {
			validStart = i
			break
		}
		if i == len(m.errorTimestamps)-1 {
			// All timestamps are old
			m.errorTimestamps = m.errorTimestamps[:0]
			return 0
		}
	}
	m.errorTimestamps = m.errorTimestamps[validStart:]

	// Calculate errors per minute
	errorCount := len(m.errorTimestamps)
	if errorCount == 0 {
		return 0
	}

	// Extrapolate to per-minute rate
	windowMinutes := m.errorWindow.Minutes()
	if windowMinutes <= 0 {
		return float64(errorCount)
	}

	return float64(errorCount) / windowMinutes * 1.0 // errors per minute
}

// SetProcPaths sets custom paths for /proc files (for testing).
func (m *Monitor) SetProcPaths(statPath, meminfoPath string) {
	m.procStatPath = statPath
	m.procMeminfoPath = meminfoPath
}

// ErrorCount returns the number of errors in the current window.
func (m *Monitor) ErrorCount() int {
	m.errorMu.Lock()
	defer m.errorMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-m.errorWindow)

	count := 0
	for _, ts := range m.errorTimestamps {
		if ts.After(cutoff) {
			count++
		}
	}
	return count
}
