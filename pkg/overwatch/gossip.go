// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"log/slog"
	"time"
)

// GossipMessage represents a message received from an agent.
type GossipMessage struct {
	// Type is the message type.
	Type GossipMessageType `json:"type"`
	// AgentID is the sending agent's ID.
	AgentID string `json:"agent_id"`
	// Region is the agent's region.
	Region string `json:"region"`
	// Timestamp is when the message was sent.
	Timestamp time.Time `json:"timestamp"`
	// Payload is the message-specific data.
	Payload any `json:"payload"`
}

// GossipMessageType defines the types of gossip messages.
type GossipMessageType string

const (
	// MessageHeartbeat is a periodic heartbeat with backend status.
	MessageHeartbeat GossipMessageType = "heartbeat"
	// MessageRegister is a backend registration message.
	MessageRegister GossipMessageType = "register"
	// MessageDeregister is a backend deregistration message.
	MessageDeregister GossipMessageType = "deregister"
)

// HeartbeatPayload is the payload for heartbeat messages.
type HeartbeatPayload struct {
	// Backends contains the current health status of all backends.
	Backends []BackendHeartbeat `json:"backends"`
	// Fingerprint is the agent's certificate fingerprint for authentication.
	Fingerprint string `json:"fingerprint"`
}

// BackendHeartbeat is the health status of a single backend in a heartbeat.
type BackendHeartbeat struct {
	Service string `json:"service"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight"`
	Healthy bool   `json:"healthy"`
}

// RegisterPayload is the payload for registration messages.
type RegisterPayload struct {
	Service string `json:"service"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight"`
}

// DeregisterPayload is the payload for deregistration messages.
type DeregisterPayload struct {
	Service string `json:"service"`
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// GossipReceiver receives gossip messages from agents.
// Story 4 will provide the actual implementation using memberlist.
type GossipReceiver interface {
	// Start begins receiving gossip messages.
	Start(ctx context.Context) error
	// Stop halts the receiver.
	Stop() error
	// MessageChan returns the channel for received messages.
	MessageChan() <-chan GossipMessage
}

// GossipReceiverConfig configures the gossip receiver.
type GossipReceiverConfig struct {
	// BindAddress is the address to listen for gossip (host:port).
	BindAddress string
	// EncryptionKey is the gossip encryption key.
	EncryptionKey string
	// ProbeInterval is the interval between failure probes.
	ProbeInterval time.Duration
	// ProbeTimeout is the timeout for a single probe.
	ProbeTimeout time.Duration
	// GossipInterval is the interval between gossip messages.
	GossipInterval time.Duration
	// Logger for gossip operations.
	Logger *slog.Logger
}

// GossipHandler processes gossip messages and updates the registry.
type GossipHandler struct {
	registry *Registry
	logger   *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewGossipHandler creates a new gossip message handler.
func NewGossipHandler(registry *Registry, logger *slog.Logger) *GossipHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &GossipHandler{
		registry: registry,
		logger:   logger,
	}
}

// Start begins processing gossip messages from the receiver.
func (h *GossipHandler) Start(receiver GossipReceiver) error {
	h.ctx, h.cancel = context.WithCancel(context.Background())

	go h.processMessages(receiver.MessageChan())

	h.logger.Info("gossip handler started")
	return nil
}

// Stop halts the gossip handler.
func (h *GossipHandler) Stop() error {
	if h.cancel != nil {
		h.cancel()
	}
	h.logger.Info("gossip handler stopped")
	return nil
}

// processMessages processes incoming gossip messages.
func (h *GossipHandler) processMessages(messages <-chan GossipMessage) {
	for {
		select {
		case <-h.ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}
			h.handleMessage(msg)
		}
	}
}

// handleMessage processes a single gossip message.
func (h *GossipHandler) handleMessage(msg GossipMessage) {
	switch msg.Type {
	case MessageHeartbeat:
		h.handleHeartbeat(msg)
	case MessageRegister:
		h.handleRegister(msg)
	case MessageDeregister:
		h.handleDeregister(msg)
	default:
		h.logger.Warn("unknown message type", "type", msg.Type, "agent_id", msg.AgentID)
	}

	// Record metrics
	RecordGossipMessage(msg.AgentID, string(msg.Type))
}

// handleHeartbeat processes a heartbeat message.
func (h *GossipHandler) handleHeartbeat(msg GossipMessage) {
	payload, ok := msg.Payload.(HeartbeatPayload)
	if !ok {
		// Try to handle map[string]interface{} from JSON unmarshaling
		if m, ok := msg.Payload.(map[string]interface{}); ok {
			payload = h.parseHeartbeatPayload(m)
		} else {
			h.logger.Warn("invalid heartbeat payload", "agent_id", msg.AgentID)
			return
		}
	}

	for _, backend := range payload.Backends {
		if err := h.registry.Register(
			msg.AgentID,
			msg.Region,
			backend.Service,
			backend.Address,
			backend.Port,
			backend.Weight,
			backend.Healthy,
		); err != nil {
			h.logger.Warn("failed to register backend from heartbeat",
				"agent_id", msg.AgentID,
				"service", backend.Service,
				"address", backend.Address,
				"error", err,
			)
		}
	}

	h.logger.Debug("processed heartbeat",
		"agent_id", msg.AgentID,
		"backends", len(payload.Backends),
	)
}

// handleRegister processes a registration message.
func (h *GossipHandler) handleRegister(msg GossipMessage) {
	payload, ok := msg.Payload.(RegisterPayload)
	if !ok {
		if m, ok := msg.Payload.(map[string]interface{}); ok {
			payload = h.parseRegisterPayload(m)
		} else {
			h.logger.Warn("invalid register payload", "agent_id", msg.AgentID)
			return
		}
	}

	if err := h.registry.Register(
		msg.AgentID,
		msg.Region,
		payload.Service,
		payload.Address,
		payload.Port,
		payload.Weight,
		true, // Assume healthy on initial registration
	); err != nil {
		h.logger.Warn("failed to register backend",
			"agent_id", msg.AgentID,
			"service", payload.Service,
			"error", err,
		)
	}
}

// handleDeregister processes a deregistration message.
func (h *GossipHandler) handleDeregister(msg GossipMessage) {
	payload, ok := msg.Payload.(DeregisterPayload)
	if !ok {
		if m, ok := msg.Payload.(map[string]interface{}); ok {
			payload = h.parseDeregisterPayload(m)
		} else {
			h.logger.Warn("invalid deregister payload", "agent_id", msg.AgentID)
			return
		}
	}

	if err := h.registry.Deregister(
		payload.Service,
		payload.Address,
		payload.Port,
	); err != nil {
		h.logger.Warn("failed to deregister backend",
			"agent_id", msg.AgentID,
			"service", payload.Service,
			"error", err,
		)
	}
}

// parseHeartbeatPayload parses a heartbeat payload from a map.
func (h *GossipHandler) parseHeartbeatPayload(m map[string]interface{}) HeartbeatPayload {
	payload := HeartbeatPayload{}

	if fingerprint, ok := m["fingerprint"].(string); ok {
		payload.Fingerprint = fingerprint
	}

	if backends, ok := m["backends"].([]interface{}); ok {
		for _, b := range backends {
			if bm, ok := b.(map[string]interface{}); ok {
				backend := BackendHeartbeat{}
				if s, ok := bm["service"].(string); ok {
					backend.Service = s
				}
				if a, ok := bm["address"].(string); ok {
					backend.Address = a
				}
				if p, ok := bm["port"].(float64); ok {
					backend.Port = int(p)
				}
				if w, ok := bm["weight"].(float64); ok {
					backend.Weight = int(w)
				}
				if healthy, ok := bm["healthy"].(bool); ok {
					backend.Healthy = healthy
				}
				payload.Backends = append(payload.Backends, backend)
			}
		}
	}

	return payload
}

// parseRegisterPayload parses a register payload from a map.
func (h *GossipHandler) parseRegisterPayload(m map[string]interface{}) RegisterPayload {
	payload := RegisterPayload{}

	if s, ok := m["service"].(string); ok {
		payload.Service = s
	}
	if a, ok := m["address"].(string); ok {
		payload.Address = a
	}
	if p, ok := m["port"].(float64); ok {
		payload.Port = int(p)
	}
	if w, ok := m["weight"].(float64); ok {
		payload.Weight = int(w)
	}

	return payload
}

// parseDeregisterPayload parses a deregister payload from a map.
func (h *GossipHandler) parseDeregisterPayload(m map[string]interface{}) DeregisterPayload {
	payload := DeregisterPayload{}

	if s, ok := m["service"].(string); ok {
		payload.Service = s
	}
	if a, ok := m["address"].(string); ok {
		payload.Address = a
	}
	if p, ok := m["port"].(float64); ok {
		payload.Port = int(p)
	}

	return payload
}

// NoOpGossipReceiver is a no-op gossip receiver for use before Story 4.
type NoOpGossipReceiver struct {
	msgChan chan GossipMessage
}

// NewNoOpGossipReceiver creates a no-op gossip receiver.
func NewNoOpGossipReceiver() *NoOpGossipReceiver {
	return &NoOpGossipReceiver{
		msgChan: make(chan GossipMessage),
	}
}

// Start implements GossipReceiver.
func (r *NoOpGossipReceiver) Start(ctx context.Context) error {
	return nil
}

// Stop implements GossipReceiver.
func (r *NoOpGossipReceiver) Stop() error {
	close(r.msgChan)
	return nil
}

// MessageChan implements GossipReceiver.
func (r *NoOpGossipReceiver) MessageChan() <-chan GossipMessage {
	return r.msgChan
}
