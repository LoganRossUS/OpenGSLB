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

	"github.com/loganrossus/OpenGSLB/pkg/agent/latency"
	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/health"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
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

	// Latency learning (ADR-017)
	latencyCollector  latency.Collector
	latencyAggregator *latency.Aggregator

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

	// SendLatencyReport sends latency data to all Overwatch nodes (ADR-017).
	SendLatencyReport(agentID, region, backend string, subnets []overwatch.SubnetLatencyData) error

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

	// Start latency learning if enabled (ADR-017)
	if a.config.Agent.LatencyLearning.Enabled {
		if err := a.startLatencyCollection(ctx); err != nil {
			// Log warning but continue - latency learning is optional
			a.logger.Warn("latency learning disabled", "error", err)
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

	// Stop latency collection (ADR-017)
	if a.latencyCollector != nil {
		if err := a.latencyCollector.Close(); err != nil {
			a.logger.Error("error stopping latency collector", "error", err)
		}
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
	if err != nil {
		a.logger.Warn("failed to collect metrics for predictive health", "error", err)
	} else {
		predictiveState = &PredictiveState{
			CPUPercent: metrics.CPUPercent,
			MemPercent: metrics.MemoryPercent,
			ErrorRate:  metrics.ErrorRate,
			LastCheck:  metrics.Timestamp,
		}

		// Log metrics periodically for debugging
		a.logger.Debug("predictive health metrics",
			"cpu_percent", metrics.CPUPercent,
			"mem_percent", metrics.MemoryPercent,
			"error_rate", metrics.ErrorRate,
		)

		// Check thresholds
		cfg := a.config.Agent.Predictive
		if cfg.Enabled {
			if metrics.CPUPercent >= cfg.CPU.Threshold {
				predictiveState.Bleeding = true
				predictiveState.BleedReason = "cpu_threshold_exceeded"
				predictiveState.BleedingAt = time.Now()
				a.logger.Warn("CPU threshold exceeded, sending bleed signal",
					"cpu_percent", metrics.CPUPercent,
					"threshold", cfg.CPU.Threshold,
				)
			} else if metrics.MemoryPercent >= cfg.Memory.Threshold {
				predictiveState.Bleeding = true
				predictiveState.BleedReason = "memory_threshold_exceeded"
				predictiveState.BleedingAt = time.Now()
				a.logger.Warn("Memory threshold exceeded, sending bleed signal",
					"mem_percent", metrics.MemoryPercent,
					"threshold", cfg.Memory.Threshold,
				)
			} else if metrics.ErrorRate >= cfg.ErrorRate.Threshold {
				predictiveState.Bleeding = true
				predictiveState.BleedReason = "error_rate_threshold_exceeded"
				predictiveState.BleedingAt = time.Now()
				a.logger.Warn("Error rate threshold exceeded, sending bleed signal",
					"error_rate", metrics.ErrorRate,
					"threshold", cfg.ErrorRate.Threshold,
				)
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

// startLatencyCollection initializes and starts passive latency learning (ADR-017).
func (a *Agent) startLatencyCollection(ctx context.Context) error {
	cfg := a.config.Agent.LatencyLearning

	// Get ports from configured backends
	ports := a.getBackendPorts()
	if len(ports) == 0 {
		return fmt.Errorf("no backend ports configured for latency collection")
	}

	// Create collector with config
	collectorCfg := latency.CollectorConfig{
		Ports:            ports,
		PollInterval:     cfg.PollInterval,
		MinConnectionAge: cfg.MinConnectionAge,
	}

	// Apply defaults
	if collectorCfg.PollInterval == 0 {
		collectorCfg.PollInterval = 10 * time.Second
	}
	if collectorCfg.MinConnectionAge == 0 {
		collectorCfg.MinConnectionAge = 5 * time.Second
	}

	collector, err := latency.New(collectorCfg)
	if err != nil {
		return fmt.Errorf("failed to create latency collector: %w", err)
	}

	// Create aggregator with config
	aggregatorCfg := latency.AggregatorConfig{
		IPv4Prefix: cfg.IPv4Prefix,
		IPv6Prefix: cfg.IPv6Prefix,
		EWMAAlpha:  cfg.EWMAAlpha,
		MaxSubnets: cfg.MaxSubnets,
		SubnetTTL:  cfg.SubnetTTL,
		MinSamples: cfg.MinSamples,
	}

	aggregator := latency.NewAggregator(aggregatorCfg)

	// Start the collector
	if err := collector.Start(ctx); err != nil {
		return fmt.Errorf("failed to start latency collector: %w", err)
	}

	a.latencyCollector = collector
	a.latencyAggregator = aggregator

	// Start observation consumer goroutine
	go a.consumeLatencyObservations(ctx)

	// Start periodic reporting goroutine
	reportInterval := cfg.ReportInterval
	if reportInterval == 0 {
		reportInterval = 30 * time.Second
	}
	go a.reportLatencyData(ctx, reportInterval)

	a.logger.Info("latency learning started",
		"ports", ports,
		"poll_interval", collectorCfg.PollInterval,
		"report_interval", reportInterval,
	)

	return nil
}

// getBackendPorts returns the unique ports of all configured backends.
func (a *Agent) getBackendPorts() []uint16 {
	portSet := make(map[uint16]bool)
	for _, backend := range a.config.Agent.Backends {
		if backend.Port > 0 && backend.Port <= 65535 {
			portSet[uint16(backend.Port)] = true
		}
	}

	ports := make([]uint16, 0, len(portSet))
	for port := range portSet {
		ports = append(ports, port)
	}
	return ports
}

// consumeLatencyObservations reads observations from the collector and aggregates them.
func (a *Agent) consumeLatencyObservations(ctx context.Context) {
	if a.latencyCollector == nil || a.latencyAggregator == nil {
		return
	}

	observations := a.latencyCollector.Observations()
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case obs, ok := <-observations:
			if !ok {
				return
			}
			a.latencyAggregator.Record(obs)
			// Record RTT for metrics
			latency.RecordRTT(obs.RTT.Seconds())
		}
	}
}

// reportLatencyData periodically sends latency reports to Overwatch nodes.
func (a *Agent) reportLatencyData(ctx context.Context, interval time.Duration) {
	if a.latencyAggregator == nil || a.gossip == nil {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Also set up a pruning ticker (every hour)
	pruneTicker := time.NewTicker(1 * time.Hour)
	defer pruneTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.sendLatencyReports()
		case <-pruneTicker.C:
			a.latencyAggregator.Prune()
		}
	}
}

// sendLatencyReports sends latency data for each backend to Overwatch nodes.
func (a *Agent) sendLatencyReports() {
	if a.latencyAggregator == nil || a.gossip == nil {
		return
	}

	// Get reportable subnets
	stats := a.latencyAggregator.GetReportable()
	if len(stats) == 0 {
		return
	}

	// Convert to gossip format
	subnets := make([]overwatch.SubnetLatencyData, 0, len(stats))
	for _, s := range stats {
		subnets = append(subnets, overwatch.SubnetLatencyData{
			Subnet:      s.Subnet.String(),
			EWMA:        int64(s.EWMA),
			SampleCount: s.SampleCount,
			LastSeen:    s.LastUpdated,
		})
	}

	// Send a report for each backend service
	services := make(map[string]bool)
	for _, backend := range a.config.Agent.Backends {
		services[backend.Service] = true
	}

	for service := range services {
		if err := a.gossip.SendLatencyReport(
			a.identity.AgentID,
			a.identity.Region,
			service,
			subnets,
		); err != nil {
			a.logger.Error("failed to send latency report",
				"service", service,
				"error", err,
			)
		} else {
			latency.RecordReportSent()
			a.logger.Debug("sent latency report",
				"service", service,
				"subnets", len(subnets),
			)
		}
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
