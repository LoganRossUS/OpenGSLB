// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package agent provides the agent mode implementation for OpenGSLB.
// ADR-015: Agents run on application servers, monitor local health,
// and gossip state to Overwatch nodes.
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/health"
)

// Agent is the main orchestrator for agent mode.
// It coordinates identity, health checking, predictive monitoring,
// heartbeat, and gossip communication.
type Agent struct {
	config   *config.Config
	identity *Identity
	backends *BackendManager
	monitor  *Monitor
	// predictor is initialized from the monitor
	heartbeat *HeartbeatSender
	gossip    GossipSender
	logger    *slog.Logger

	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
	doneCh    chan struct{}
	startTime time.Time
}

// GossipSender is the interface for sending gossip messages.
// This will be implemented by the gossip package in Story 4.
type GossipSender interface {
	// SendHealthUpdate sends a health update to all Overwatch nodes.
	SendHealthUpdate(msg HealthUpdateMessage) error

	// SendHeartbeat sends a heartbeat message to all Overwatch nodes.
	SendHeartbeat(msg HeartbeatMessage) error

	// Start connects to Overwatch nodes and begins gossip.
	Start(ctx context.Context) error

	// Stop disconnects from Overwatch nodes.
	Stop() error
}

// HealthUpdateMessage contains health state for all backends.
type HealthUpdateMessage struct {
	AgentID     string                  `json:"agent_id"`
	Region      string                  `json:"region"`
	Timestamp   time.Time               `json:"timestamp"`
	Backends    []BackendHealthSnapshot `json:"backends"`
	Predictive  *PredictiveState        `json:"predictive,omitempty"`
	Certificate []byte                  `json:"certificate,omitempty"` // For TOFU
}

// AgentConfig holds agent initialization configuration.
type AgentConfig struct {
	Config *config.Config
	Logger *slog.Logger
	Gossip GossipSender // Optional, can be nil for testing
}

// NewAgent creates a new agent instance.
func NewAgent(cfg AgentConfig) (*Agent, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Initialize identity
	identity, err := NewIdentity(IdentityConfig{
		ServiceToken: cfg.Config.Agent.Identity.ServiceToken,
		Region:       cfg.Config.Agent.Identity.Region,
		CertPath:     cfg.Config.Agent.Identity.CertPath,
		KeyPath:      cfg.Config.Agent.Identity.KeyPath,
		Logger:       logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize identity: %w", err)
	}

	// Initialize health checker
	checker := health.NewCompositeChecker()
	checker.Register("http", health.NewHTTPChecker())
	checker.Register("tcp", health.NewTCPChecker())

	// Initialize backend manager
	backends := NewBackendManager(checker, logger)

	// Initialize system monitor for predictive health
	errorWindow := 60 * time.Second
	if cfg.Config.Agent.Predictive.ErrorRate.Window > 0 {
		errorWindow = cfg.Config.Agent.Predictive.ErrorRate.Window
	}
	monitor := NewMonitor(logger, errorWindow)

	agent := &Agent{
		config:   cfg.Config,
		identity: identity,
		backends: backends,
		monitor:  monitor,
		gossip:   cfg.Gossip,
		logger:   logger,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}

	// Register configured backends
	for _, backend := range cfg.Config.Agent.Backends {
		bcfg := BackendConfig{
			Service: backend.Service,
			Address: backend.Address,
			Port:    backend.Port,
			Weight:  backend.Weight,
			HealthCheck: HealthCheckConfig{
				Type:             backend.HealthCheck.Type,
				Path:             backend.HealthCheck.Path,
				Host:             backend.HealthCheck.Host,
				Interval:         backend.HealthCheck.Interval,
				Timeout:          backend.HealthCheck.Timeout,
				FailureThreshold: backend.HealthCheck.FailureThreshold,
				SuccessThreshold: backend.HealthCheck.SuccessThreshold,
			},
		}
		if err := backends.AddBackend(bcfg); err != nil {
			return nil, fmt.Errorf("failed to add backend %s: %w", backend.Service, err)
		}
	}

	// Set up health change callback to send gossip updates
	backends.OnHealthChange(func(update BackendHealthUpdate) {
		agent.onHealthChange(update)
	})

	return agent, nil
}

// Start begins agent operations.
func (a *Agent) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("agent already running")
	}
	a.running = true
	a.startTime = time.Now()
	a.stopCh = make(chan struct{})
	a.doneCh = make(chan struct{})
	a.mu.Unlock()

	a.logger.Info("starting agent",
		"agent_id", a.identity.AgentID,
		"region", a.identity.Region,
		"backends", a.backends.BackendCount(),
	)

	// Start backend health checking
	if err := a.backends.Start(); err != nil {
		return fmt.Errorf("failed to start backend manager: %w", err)
	}

	// Start gossip if configured
	if a.gossip != nil {
		if err := a.gossip.Start(ctx); err != nil {
			if stopErr := a.backends.Stop(); stopErr != nil {
				a.logger.Error("error stopping backends", "error", stopErr)
			}
			return fmt.Errorf("failed to start gossip: %w", err)
		}
	}

	// Initialize and start heartbeat
	if a.gossip != nil {
		a.heartbeat = NewHeartbeatSender(
			HeartbeatSenderConfig{
				Interval:        a.config.Agent.Heartbeat.Interval,
				MissedThreshold: a.config.Agent.Heartbeat.MissedThreshold,
				Logger:          a.logger,
			},
			a.gossip,
		)
		if err := a.heartbeat.Start(ctx, a.identity.AgentID, a.identity.Region, a.backends.BackendCount()); err != nil {
			if stopErr := a.gossip.Stop(); stopErr != nil {
				a.logger.Error("error stopping gossip", "error", stopErr)
			}
			if stopErr := a.backends.Stop(); stopErr != nil {
				a.logger.Error("error stopping backends", "error", stopErr)
			}
			return fmt.Errorf("failed to start heartbeat: %w", err)
		}
	}

	// Start main agent loop
	go a.runLoop(ctx)

	a.logger.Info("agent started successfully")
	return nil
}

// Stop gracefully shuts down the agent.
func (a *Agent) Stop(ctx context.Context) error {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil
	}
	a.running = false
	close(a.stopCh)
	a.mu.Unlock()

	a.logger.Info("stopping agent")

	// Send deregistration message before stopping
	if a.gossip != nil {
		a.sendDeregistration()
	}

	// Stop heartbeat
	if a.heartbeat != nil {
		a.heartbeat.Stop()
	}

	// Stop gossip
	if a.gossip != nil {
		if err := a.gossip.Stop(); err != nil {
			a.logger.Error("error stopping gossip", "error", err)
		}
	}

	// Stop backend health checks
	if err := a.backends.Stop(); err != nil {
		a.logger.Error("error stopping backends", "error", err)
	}

	// Wait for main loop to exit
	select {
	case <-a.doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}

	a.logger.Info("agent stopped")
	return nil
}

func (a *Agent) runLoop(ctx context.Context) {
	defer close(a.doneCh)

	// Periodic tasks ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.periodicTasks()
		}
	}
}

func (a *Agent) periodicTasks() {
	// Check if certificate needs rotation
	if a.identity.NeedsRotation(30 * 24 * time.Hour) { // Rotate 30 days before expiry
		certPath := a.config.Agent.Identity.CertPath
		keyPath := a.config.Agent.Identity.KeyPath
		if certPath == "" || keyPath == "" {
			certPath, keyPath = DefaultIdentityPaths()
		}
		if err := a.identity.RotateCertificate(certPath, keyPath); err != nil {
			a.logger.Error("certificate rotation failed", "error", err)
		}
	}

	// Send periodic health update (in addition to change-triggered updates)
	a.sendHealthUpdate()
}

func (a *Agent) onHealthChange(update BackendHealthUpdate) {
	a.logger.Debug("backend health changed",
		"service", update.Service,
		"address", update.Address,
		"healthy", update.Healthy,
	)

	// Record error for predictive monitoring if unhealthy
	if !update.Healthy {
		a.monitor.RecordError()
	}

	// Send health update via gossip
	a.sendHealthUpdate()
}

func (a *Agent) sendHealthUpdate() {
	if a.gossip == nil {
		return
	}

	// Collect predictive state from monitor
	metrics, err := a.monitor.Collect()
	var predictiveState *PredictiveState
	if err == nil {
		predictiveState = &PredictiveState{
			CPUPercent: metrics.CPUPercent,
			MemPercent: metrics.MemoryPercent,
			ErrorRate:  metrics.ErrorRate,
			LastCheck:  metrics.Timestamp,
		}

		// Check thresholds
		cfg := a.config.Agent.Predictive
		if cfg.Enabled {
			if metrics.CPUPercent >= cfg.CPU.Threshold {
				predictiveState.Bleeding = true
				predictiveState.BleedReason = "cpu_threshold_exceeded"
				predictiveState.BleedingAt = time.Now()
			} else if metrics.MemoryPercent >= cfg.Memory.Threshold {
				predictiveState.Bleeding = true
				predictiveState.BleedReason = "memory_threshold_exceeded"
				predictiveState.BleedingAt = time.Now()
			} else if metrics.ErrorRate >= cfg.ErrorRate.Threshold {
				predictiveState.Bleeding = true
				predictiveState.BleedReason = "error_rate_threshold_exceeded"
				predictiveState.BleedingAt = time.Now()
			}
		}
	}

	msg := HealthUpdateMessage{
		AgentID:    a.identity.AgentID,
		Region:     a.identity.Region,
		Timestamp:  time.Now(),
		Backends:   a.backends.GetAllHealth(),
		Predictive: predictiveState,
	}

	if err := a.gossip.SendHealthUpdate(msg); err != nil {
		a.logger.Error("failed to send health update", "error", err)
	}
}

func (a *Agent) sendDeregistration() {
	// Send a final health update with all backends marked as leaving
	// The Overwatch will interpret an empty backends list as deregistration
	msg := HealthUpdateMessage{
		AgentID:   a.identity.AgentID,
		Region:    a.identity.Region,
		Timestamp: time.Now(),
		Backends:  nil, // Empty indicates deregistration
	}

	if err := a.gossip.SendHealthUpdate(msg); err != nil {
		a.logger.Error("failed to send deregistration", "error", err)
	}
}

// GetIdentity returns the agent's identity.
func (a *Agent) GetIdentity() *Identity {
	return a.identity
}

// GetBackendManager returns the backend manager.
func (a *Agent) GetBackendManager() *BackendManager {
	return a.backends
}

// Stats returns agent statistics.
type AgentStats struct {
	AgentID       string
	Region        string
	Running       bool
	StartTime     time.Time
	Uptime        time.Duration
	BackendCount  int
	HealthyCount  int
	HeartbeatSent time.Time
}

// Stats returns current agent statistics.
func (a *Agent) Stats() AgentStats {
	a.mu.RLock()
	running := a.running
	startTime := a.startTime
	a.mu.RUnlock()

	var heartbeatSent time.Time
	if a.heartbeat != nil {
		heartbeatSent = a.heartbeat.LastSent()
	}

	var uptime time.Duration
	if running {
		uptime = time.Since(startTime)
	}

	return AgentStats{
		AgentID:       a.identity.AgentID,
		Region:        a.identity.Region,
		Running:       running,
		StartTime:     startTime,
		Uptime:        uptime,
		BackendCount:  a.backends.BackendCount(),
		HealthyCount:  a.backends.HealthyCount(),
		HeartbeatSent: heartbeatSent,
	}
}

// MockGossipSender is a test implementation of GossipSender.
type MockGossipSender struct {
	mu            sync.Mutex
	healthUpdates []HealthUpdateMessage
	heartbeats    []HeartbeatMessage
	startErr      error
	sendHealthErr error
	sendHBErr     error
	started       bool
}

// NewMockGossipSender creates a mock gossip sender for testing.
func NewMockGossipSender() *MockGossipSender {
	return &MockGossipSender{
		healthUpdates: make([]HealthUpdateMessage, 0),
		heartbeats:    make([]HeartbeatMessage, 0),
	}
}

func (m *MockGossipSender) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}

func (m *MockGossipSender) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = false
	return nil
}

func (m *MockGossipSender) SendHealthUpdate(msg HealthUpdateMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendHealthErr != nil {
		return m.sendHealthErr
	}
	m.healthUpdates = append(m.healthUpdates, msg)
	return nil
}

func (m *MockGossipSender) SendHeartbeat(msg HeartbeatMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendHBErr != nil {
		return m.sendHBErr
	}
	m.heartbeats = append(m.heartbeats, msg)
	return nil
}

// HealthUpdates returns recorded health updates.
func (m *MockGossipSender) HealthUpdates() []HealthUpdateMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]HealthUpdateMessage, len(m.healthUpdates))
	copy(result, m.healthUpdates)
	return result
}

// Heartbeats returns recorded heartbeats.
func (m *MockGossipSender) Heartbeats() []HeartbeatMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]HeartbeatMessage, len(m.heartbeats))
	copy(result, m.heartbeats)
	return result
}

// SetError configures errors for testing.
func (m *MockGossipSender) SetError(startErr, sendHealthErr, sendHBErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startErr = startErr
	m.sendHealthErr = sendHealthErr
	m.sendHBErr = sendHBErr
}

// Clear removes all recorded messages.
func (m *MockGossipSender) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthUpdates = m.healthUpdates[:0]
	m.heartbeats = m.heartbeats[:0]
}
