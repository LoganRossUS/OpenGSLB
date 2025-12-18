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
	// MessageAgentAuth is an agent authentication/registration message (TOFU).
	MessageAgentAuth GossipMessageType = "agent_auth"
)

// HeartbeatPayload is the payload for heartbeat messages.
type HeartbeatPayload struct {
	// Backends contains the current health status of all backends.
	Backends []BackendHeartbeat `json:"backends"`
	// Fingerprint is the agent's certificate fingerprint for authentication.
	Fingerprint string `json:"fingerprint"`
	// Predictive contains predictive health state from the agent.
	Predictive *PredictiveHeartbeat `json:"predictive,omitempty"`
}

// PredictiveHeartbeat contains predictive health information from an agent.
type PredictiveHeartbeat struct {
	// Bleeding indicates the agent is requesting traffic drain due to resource pressure.
	Bleeding bool `json:"bleeding"`
	// BleedReason is the reason for bleeding (cpu_threshold_exceeded, memory_threshold_exceeded, error_rate_threshold_exceeded).
	BleedReason string `json:"bleed_reason,omitempty"`
	// BleedingAt is when bleeding started.
	BleedingAt time.Time `json:"bleeding_at,omitempty"`
	// CPUPercent is the current CPU utilization.
	CPUPercent float64 `json:"cpu_percent"`
	// MemPercent is the current memory utilization.
	MemPercent float64 `json:"mem_percent"`
	// ErrorRate is the current error rate.
	ErrorRate float64 `json:"error_rate"`
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

// AgentAuthPayload is the payload for agent authentication messages (TOFU).
type AgentAuthPayload struct {
	// CertificatePEM is the agent's PEM-encoded certificate.
	CertificatePEM []byte `json:"certificate_pem"`
	// ServiceToken is the pre-shared token for first-time registration.
	ServiceToken string `json:"service_token,omitempty"`
	// Fingerprint is the certificate fingerprint for subsequent auth.
	Fingerprint string `json:"fingerprint"`
}

// AgentAuthResponse is the response to an agent authentication request.
type AgentAuthResponse struct {
	// Success indicates if authentication was successful.
	Success bool `json:"success"`
	// Message provides details about the result.
	Message string `json:"message,omitempty"`
	// Error provides error details if authentication failed.
	Error string `json:"error,omitempty"`
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

// DNSRegistry defines the interface for dynamic DNS server registration.
// v1.1.0: Allows agent-registered servers to be added to DNS responses.
type DNSRegistry interface {
	RegisterServer(service string, address string, port int, weight int, region string) error
	DeregisterServer(service string, address string, port int) error
}

// GossipHandler processes gossip messages and updates the registry.
type GossipHandler struct {
	registry    *Registry
	dnsRegistry DNSRegistry // v1.1.0: For dynamic DNS registration
	auth        *AgentAuth
	logger      *slog.Logger
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewGossipHandler creates a new gossip message handler.
// v1.1.0: Now accepts optional DNS registry for dynamic server registration.
func NewGossipHandler(registry *Registry, dnsRegistry DNSRegistry, logger *slog.Logger) *GossipHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &GossipHandler{
		registry:    registry,
		dnsRegistry: dnsRegistry,
		logger:      logger,
	}
}

// SetAuth sets the agent authenticator for TOFU authentication.
func (h *GossipHandler) SetAuth(auth *AgentAuth) {
	h.auth = auth
}

// SetDNSRegistry sets the DNS registry for dynamic server registration.
// v1.1.0: Called after DNS registry is initialized.
func (h *GossipHandler) SetDNSRegistry(dnsRegistry DNSRegistry) {
	h.dnsRegistry = dnsRegistry
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
	case MessageAgentAuth:
		h.handleAgentAuth(msg)
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

// handleAgentAuth processes an agent authentication message.
func (h *GossipHandler) handleAgentAuth(msg GossipMessage) {
	if h.auth == nil {
		h.logger.Warn("agent auth not configured, rejecting auth request", "agent_id", msg.AgentID)
		return
	}

	payload, ok := msg.Payload.(AgentAuthPayload)
	if !ok {
		if m, ok := msg.Payload.(map[string]interface{}); ok {
			payload = h.parseAgentAuthPayload(m)
		} else {
			h.logger.Warn("invalid agent auth payload", "agent_id", msg.AgentID)
			return
		}
	}

	err := h.auth.AuthenticateAgent(h.ctx, msg.AgentID, payload.CertificatePEM, payload.ServiceToken)
	if err != nil {
		h.logger.Warn("agent authentication failed",
			"agent_id", msg.AgentID,
			"error", err,
		)
		return
	}

	h.logger.Info("agent authenticated successfully", "agent_id", msg.AgentID)
}

// parseAgentAuthPayload parses an agent auth payload from a map.
func (h *GossipHandler) parseAgentAuthPayload(m map[string]interface{}) AgentAuthPayload {
	payload := AgentAuthPayload{}

	if certPEM, ok := m["certificate_pem"].(string); ok {
		payload.CertificatePEM = []byte(certPEM)
	}
	if serviceToken, ok := m["service_token"].(string); ok {
		payload.ServiceToken = serviceToken
	}
	if fingerprint, ok := m["fingerprint"].(string); ok {
		payload.Fingerprint = fingerprint
	}

	return payload
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
		// Register in backend registry (for health tracking and validation)
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
			continue // Skip DNS registration if backend registry fails
		}

		// v1.1.0: Also register in DNS registry (for DNS responses)
		if h.dnsRegistry != nil {
			if err := h.dnsRegistry.RegisterServer(
				backend.Service,
				backend.Address,
				backend.Port,
				backend.Weight,
				msg.Region,
			); err != nil {
				h.logger.Warn("failed to register backend in DNS registry",
					"agent_id", msg.AgentID,
					"service", backend.Service,
					"address", backend.Address,
					"error", err,
				)
			} else {
				h.logger.Debug("registered backend in DNS registry",
					"agent_id", msg.AgentID,
					"service", backend.Service,
					"address", backend.Address,
					"port", backend.Port,
					"region", msg.Region,
				)
			}
		}
	}

	// Process predictive health state if present
	if payload.Predictive != nil {
		h.registry.UpdateDraining(
			msg.AgentID,
			payload.Predictive.Bleeding,
			payload.Predictive.BleedReason,
			payload.Predictive.BleedingAt,
			payload.Predictive.CPUPercent,
			payload.Predictive.MemPercent,
			payload.Predictive.ErrorRate,
		)

		if payload.Predictive.Bleeding {
			h.logger.Debug("agent reporting bleed signal",
				"agent_id", msg.AgentID,
				"reason", payload.Predictive.BleedReason,
				"cpu_percent", payload.Predictive.CPUPercent,
				"mem_percent", payload.Predictive.MemPercent,
				"error_rate", payload.Predictive.ErrorRate,
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

	// Parse predictive health state
	if pred, ok := m["predictive"].(map[string]interface{}); ok {
		payload.Predictive = h.parsePredictiveHeartbeat(pred)
	}

	return payload
}

// parsePredictiveHeartbeat parses a predictive heartbeat from a map.
func (h *GossipHandler) parsePredictiveHeartbeat(m map[string]interface{}) *PredictiveHeartbeat {
	pred := &PredictiveHeartbeat{}

	if bleeding, ok := m["bleeding"].(bool); ok {
		pred.Bleeding = bleeding
	}
	if reason, ok := m["bleed_reason"].(string); ok {
		pred.BleedReason = reason
	}
	if bleedingAt, ok := m["bleeding_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, bleedingAt); err == nil {
			pred.BleedingAt = t
		}
	}
	if cpu, ok := m["cpu_percent"].(float64); ok {
		pred.CPUPercent = cpu
	}
	if mem, ok := m["mem_percent"].(float64); ok {
		pred.MemPercent = mem
	}
	if errRate, ok := m["error_rate"].(float64); ok {
		pred.ErrorRate = errRate
	}

	return pred
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
