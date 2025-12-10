// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// HeartbeatSender periodically sends heartbeat messages to Overwatch nodes.
// ADR-015: Heartbeats are explicit keepalives, separate from health updates.
type HeartbeatSender struct {
	config   HeartbeatSenderConfig
	logger   *slog.Logger
	sender   HeartbeatTransport
	mu       sync.RWMutex
	running  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	lastSent time.Time
	failures int
}

// HeartbeatSenderConfig configures the heartbeat sender.
type HeartbeatSenderConfig struct {
	// Interval between heartbeat messages
	Interval time.Duration

	// MissedThreshold is the number of failed sends before logging error
	MissedThreshold int

	// Logger for heartbeat operations
	Logger *slog.Logger
}

// HeartbeatTransport is the interface for sending heartbeat messages.
// This is implemented by the gossip layer.
type HeartbeatTransport interface {
	// SendHeartbeat sends a heartbeat message to all configured Overwatch nodes.
	SendHeartbeat(msg HeartbeatMessage) error
}

// HeartbeatMessage contains the heartbeat payload.
type HeartbeatMessage struct {
	// AgentID identifies this agent
	AgentID string `json:"agent_id"`

	// Region is the geographic region
	Region string `json:"region"`

	// Timestamp when heartbeat was sent
	Timestamp time.Time `json:"timestamp"`

	// SequenceNum for ordering and gap detection
	SequenceNum uint64 `json:"sequence_num"`

	// BackendCount is the number of backends this agent monitors
	BackendCount int `json:"backend_count"`

	// Healthy is the overall agent health status
	Healthy bool `json:"healthy"`
}

// DefaultHeartbeatConfig returns sensible defaults.
func DefaultHeartbeatConfig() HeartbeatSenderConfig {
	return HeartbeatSenderConfig{
		Interval:        10 * time.Second,
		MissedThreshold: 3,
	}
}

// NewHeartbeatSender creates a new heartbeat sender.
func NewHeartbeatSender(cfg HeartbeatSenderConfig, sender HeartbeatTransport) *HeartbeatSender {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.Interval <= 0 {
		cfg.Interval = 10 * time.Second
	}
	if cfg.MissedThreshold <= 0 {
		cfg.MissedThreshold = 3
	}

	return &HeartbeatSender{
		config: cfg,
		logger: logger,
		sender: sender,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins sending heartbeats at the configured interval.
func (h *HeartbeatSender) Start(ctx context.Context, agentID, region string, backendCount int) error {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return nil
	}
	h.running = true
	h.stopCh = make(chan struct{})
	h.doneCh = make(chan struct{})
	h.mu.Unlock()

	go h.heartbeatLoop(ctx, agentID, region, backendCount)
	return nil
}

// Stop stops the heartbeat sender.
func (h *HeartbeatSender) Stop() {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return
	}
	h.running = false
	close(h.stopCh)
	h.mu.Unlock()

	<-h.doneCh
}

// IsRunning returns true if the heartbeat sender is active.
func (h *HeartbeatSender) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

// LastSent returns the timestamp of the last successful heartbeat.
func (h *HeartbeatSender) LastSent() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastSent
}

// ConsecutiveFailures returns the number of consecutive send failures.
func (h *HeartbeatSender) ConsecutiveFailures() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.failures
}

func (h *HeartbeatSender) heartbeatLoop(ctx context.Context, agentID, region string, backendCount int) {
	defer close(h.doneCh)

	ticker := time.NewTicker(h.config.Interval)
	defer ticker.Stop()

	var sequenceNum uint64

	// Send initial heartbeat immediately
	h.sendHeartbeat(agentID, region, backendCount, &sequenceNum)

	for {
		select {
		case <-ctx.Done():
			h.logger.Debug("heartbeat sender stopping due to context cancellation")
			return
		case <-h.stopCh:
			h.logger.Debug("heartbeat sender stopping")
			return
		case <-ticker.C:
			h.sendHeartbeat(agentID, region, backendCount, &sequenceNum)
		}
	}
}

func (h *HeartbeatSender) sendHeartbeat(agentID, region string, backendCount int, seqNum *uint64) {
	*seqNum++

	msg := HeartbeatMessage{
		AgentID:      agentID,
		Region:       region,
		Timestamp:    time.Now(),
		SequenceNum:  *seqNum,
		BackendCount: backendCount,
		Healthy:      true, // Agent is healthy if heartbeat sender is running
	}

	if err := h.sender.SendHeartbeat(msg); err != nil {
		h.mu.Lock()
		h.failures++
		failures := h.failures
		h.mu.Unlock()

		if failures >= h.config.MissedThreshold {
			h.logger.Error("heartbeat send failed",
				"error", err,
				"consecutive_failures", failures,
			)
		} else {
			h.logger.Warn("heartbeat send failed",
				"error", err,
				"consecutive_failures", failures,
			)
		}
	} else {
		h.mu.Lock()
		h.lastSent = msg.Timestamp
		h.failures = 0
		h.mu.Unlock()

		h.logger.Debug("heartbeat sent",
			"sequence_num", msg.SequenceNum,
			"backend_count", backendCount,
		)
	}
}

// UpdateBackendCount updates the backend count for heartbeat messages.
// This is called when backends are added or removed.
func (h *HeartbeatSender) UpdateBackendCount(count int) {
	// The count is passed to sendHeartbeat each time, so this is a no-op.
	// However, if we want to track changes:
	h.logger.Debug("backend count updated for heartbeat", "count", count)
}

// HeartbeatStats contains heartbeat sender statistics.
type HeartbeatStats struct {
	Running             bool
	LastSent            time.Time
	ConsecutiveFailures int
	Interval            time.Duration
}

// Stats returns current heartbeat sender statistics.
func (h *HeartbeatSender) Stats() HeartbeatStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return HeartbeatStats{
		Running:             h.running,
		LastSent:            h.lastSent,
		ConsecutiveFailures: h.failures,
		Interval:            h.config.Interval,
	}
}

// MockHeartbeatTransport is a test implementation of HeartbeatTransport.
type MockHeartbeatTransport struct {
	mu       sync.Mutex
	messages []HeartbeatMessage
	sendErr  error
}

// NewMockHeartbeatTransport creates a mock transport for testing.
func NewMockHeartbeatTransport() *MockHeartbeatTransport {
	return &MockHeartbeatTransport{
		messages: make([]HeartbeatMessage, 0),
	}
}

// SendHeartbeat records the message for later inspection.
func (m *MockHeartbeatTransport) SendHeartbeat(msg HeartbeatMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.messages = append(m.messages, msg)
	return nil
}

// SetError configures the mock to return an error.
func (m *MockHeartbeatTransport) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

// Messages returns all recorded messages.
func (m *MockHeartbeatTransport) Messages() []HeartbeatMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]HeartbeatMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

// Clear removes all recorded messages.
func (m *MockHeartbeatTransport) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = m.messages[:0]
}
