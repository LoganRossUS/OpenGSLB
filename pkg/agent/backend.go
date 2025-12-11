// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/health"
)

// BackendManager manages health checks for multiple backends on an agent.
// ADR-015: Each agent can monitor multiple services/backends.
type BackendManager struct {
	checker  health.Checker
	backends map[string]*BackendEntry
	mu       sync.RWMutex
	logger   *slog.Logger
	running  bool
	stopCh   chan struct{}
	wg       sync.WaitGroup

	// Callback for health state changes
	onChange func(BackendHealthUpdate)
}

// BackendEntry represents a single monitored backend.
type BackendEntry struct {
	Config BackendConfig
	Health *BackendHealth
	stopCh chan struct{}
}

// BackendConfig defines configuration for a single backend.
type BackendConfig struct {
	// Service name (used for DNS domain mapping)
	Service string

	// Address is the backend server IP or hostname
	Address string

	// Port is the backend server port
	Port int

	// Weight for routing decisions
	Weight int

	// Health check configuration
	HealthCheck HealthCheckConfig
}

// HealthCheckConfig defines how to check a backend's health.
type HealthCheckConfig struct {
	Type             string        // http, https, tcp
	Path             string        // For HTTP checks
	Host             string        // Host header for HTTP(S)
	Interval         time.Duration // Check interval
	Timeout          time.Duration // Per-check timeout
	FailureThreshold int           // Failures before unhealthy
	SuccessThreshold int           // Successes before healthy
}

// BackendHealth tracks health state for a single backend.
type BackendHealth struct {
	mu sync.RWMutex

	service           string
	address           string
	port              int
	weight            int
	healthy           bool
	lastCheck         time.Time
	lastHealthy       time.Time
	consecutiveFails  int
	consecutivePasses int
	lastError         error
	lastLatency       time.Duration

	failThreshold int
	passThreshold int
}

// BackendHealthUpdate is sent when a backend's health state changes.
type BackendHealthUpdate struct {
	Service         string
	Address         string
	Port            int
	Weight          int
	Healthy         bool
	PreviousHealthy bool
	Latency         time.Duration
	Error           error
	Timestamp       time.Time
}

// BackendHealthSnapshot is a point-in-time copy of backend health.
type BackendHealthSnapshot struct {
	Service           string        `json:"service"`
	Address           string        `json:"address"`
	Port              int           `json:"port"`
	Weight            int           `json:"weight"`
	Healthy           bool          `json:"healthy"`
	LastCheck         time.Time     `json:"last_check"`
	LastHealthy       time.Time     `json:"last_healthy"`
	ConsecutiveFails  int           `json:"consecutive_fails"`
	ConsecutivePasses int           `json:"consecutive_passes"`
	LastError         string        `json:"last_error,omitempty"`
	LastLatency       time.Duration `json:"last_latency_ns"`
}

// NewBackendManager creates a new backend manager.
func NewBackendManager(checker health.Checker, logger *slog.Logger) *BackendManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &BackendManager{
		checker:  checker,
		backends: make(map[string]*BackendEntry),
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// OnHealthChange sets a callback for health state changes.
func (m *BackendManager) OnHealthChange(fn func(BackendHealthUpdate)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

// AddBackend registers a backend for health checking.
func (m *BackendManager) AddBackend(cfg BackendConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := backendKey(cfg.Service, cfg.Address, cfg.Port)
	if _, exists := m.backends[key]; exists {
		return fmt.Errorf("backend %s already registered", key)
	}

	// Apply defaults
	if cfg.HealthCheck.Interval == 0 {
		cfg.HealthCheck.Interval = 30 * time.Second
	}
	if cfg.HealthCheck.Timeout == 0 {
		cfg.HealthCheck.Timeout = 5 * time.Second
	}
	if cfg.HealthCheck.FailureThreshold == 0 {
		cfg.HealthCheck.FailureThreshold = 3
	}
	if cfg.HealthCheck.SuccessThreshold == 0 {
		cfg.HealthCheck.SuccessThreshold = 2
	}
	if cfg.HealthCheck.Type == "" {
		cfg.HealthCheck.Type = "http"
	}
	if cfg.Weight == 0 {
		cfg.Weight = 100
	}

	entry := &BackendEntry{
		Config: cfg,
		Health: &BackendHealth{
			service:       cfg.Service,
			address:       cfg.Address,
			port:          cfg.Port,
			weight:        cfg.Weight,
			failThreshold: cfg.HealthCheck.FailureThreshold,
			passThreshold: cfg.HealthCheck.SuccessThreshold,
		},
		stopCh: make(chan struct{}),
	}
	m.backends[key] = entry

	// Start health check loop if manager is running
	if m.running {
		m.wg.Add(1)
		go m.checkLoop(entry)
	}

	m.logger.Info("backend registered",
		"service", cfg.Service,
		"address", cfg.Address,
		"port", cfg.Port,
		"check_type", cfg.HealthCheck.Type,
	)

	return nil
}

// RemoveBackend unregisters a backend from health checking.
func (m *BackendManager) RemoveBackend(service, address string, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := backendKey(service, address, port)
	entry, exists := m.backends[key]
	if !exists {
		return fmt.Errorf("backend %s not found", key)
	}

	close(entry.stopCh)
	delete(m.backends, key)

	m.logger.Info("backend removed",
		"service", service,
		"address", address,
		"port", port,
	)

	return nil
}

// Start begins health checking all registered backends.
func (m *BackendManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("backend manager already running")
	}

	m.running = true
	m.stopCh = make(chan struct{})

	for _, entry := range m.backends {
		m.wg.Add(1)
		go m.checkLoop(entry)
	}

	m.logger.Info("backend manager started", "backends", len(m.backends))
	return nil
}

// Stop halts all health checking.
func (m *BackendManager) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()

	m.wg.Wait()
	m.logger.Info("backend manager stopped")
	return nil
}

func (m *BackendManager) checkLoop(entry *BackendEntry) {
	defer m.wg.Done()

	ticker := time.NewTicker(entry.Config.HealthCheck.Interval)
	defer ticker.Stop()

	// Run immediate check
	m.performCheck(entry)

	for {
		select {
		case <-m.stopCh:
			return
		case <-entry.stopCh:
			return
		case <-ticker.C:
			m.performCheck(entry)
		}
	}
}

func (m *BackendManager) performCheck(entry *BackendEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), entry.Config.HealthCheck.Timeout)
	defer cancel()

	target := health.Target{
		Address: entry.Config.Address,
		Port:    entry.Config.Port,
		Path:    entry.Config.HealthCheck.Path,
		Scheme:  entry.Config.HealthCheck.Type,
		Host:    entry.Config.HealthCheck.Host,
		Timeout: entry.Config.HealthCheck.Timeout,
	}

	result := m.checker.Check(ctx, target)
	changed := entry.Health.RecordResult(result)

	if changed {
		m.mu.RLock()
		onChange := m.onChange
		m.mu.RUnlock()

		if onChange != nil {
			update := BackendHealthUpdate{
				Service:         entry.Config.Service,
				Address:         entry.Config.Address,
				Port:            entry.Config.Port,
				Weight:          entry.Config.Weight,
				Healthy:         entry.Health.IsHealthy(),
				PreviousHealthy: !entry.Health.IsHealthy(), // Inverted since it just changed
				Latency:         result.Latency,
				Error:           result.Error,
				Timestamp:       result.Timestamp,
			}
			go onChange(update)
		}

		if entry.Health.IsHealthy() {
			m.logger.Info("backend became healthy",
				"service", entry.Config.Service,
				"address", entry.Config.Address,
				"port", entry.Config.Port,
			)
		} else {
			m.logger.Warn("backend became unhealthy",
				"service", entry.Config.Service,
				"address", entry.Config.Address,
				"port", entry.Config.Port,
				"error", result.Error,
			)
		}
	}
}

// GetAllHealth returns health snapshots for all backends.
func (m *BackendManager) GetAllHealth() []BackendHealthSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshots := make([]BackendHealthSnapshot, 0, len(m.backends))
	for _, entry := range m.backends {
		snapshots = append(snapshots, entry.Health.Snapshot())
	}
	return snapshots
}

// GetHealth returns the health snapshot for a specific backend.
func (m *BackendManager) GetHealth(service, address string, port int) (BackendHealthSnapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := backendKey(service, address, port)
	entry, exists := m.backends[key]
	if !exists {
		return BackendHealthSnapshot{}, false
	}
	return entry.Health.Snapshot(), true
}

// BackendCount returns the number of registered backends.
func (m *BackendManager) BackendCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.backends)
}

// HealthyCount returns the number of healthy backends.
func (m *BackendManager) HealthyCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, entry := range m.backends {
		if entry.Health.IsHealthy() {
			count++
		}
	}
	return count
}

// RecordResult updates health state based on a check result.
// Returns true if the health status changed.
func (h *BackendHealth) RecordResult(result health.Result) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastCheck = result.Timestamp
	h.lastLatency = result.Latency
	previousHealthy := h.healthy

	if result.Healthy {
		h.consecutiveFails = 0
		h.consecutivePasses++
		h.lastError = nil
		h.lastHealthy = result.Timestamp

		if !h.healthy && h.consecutivePasses >= h.passThreshold {
			h.healthy = true
		}
	} else {
		h.consecutivePasses = 0
		h.consecutiveFails++
		h.lastError = result.Error

		if h.healthy && h.consecutiveFails >= h.failThreshold {
			h.healthy = false
		}
	}

	return h.healthy != previousHealthy
}

// IsHealthy returns true if the backend is currently healthy.
func (h *BackendHealth) IsHealthy() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.healthy
}

// Snapshot returns a point-in-time copy of the health state.
func (h *BackendHealth) Snapshot() BackendHealthSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var errStr string
	if h.lastError != nil {
		errStr = h.lastError.Error()
	}

	return BackendHealthSnapshot{
		Service:           h.service,
		Address:           h.address,
		Port:              h.port,
		Weight:            h.weight,
		Healthy:           h.healthy,
		LastCheck:         h.lastCheck,
		LastHealthy:       h.lastHealthy,
		ConsecutiveFails:  h.consecutiveFails,
		ConsecutivePasses: h.consecutivePasses,
		LastError:         errStr,
		LastLatency:       h.lastLatency,
	}
}

func backendKey(service, address string, port int) string {
	return fmt.Sprintf("%s:%s:%d", service, address, port)
}
