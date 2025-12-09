// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"encoding/json"
	"time"
)

// MessageType identifies the type of gossip message.
type MessageType uint8

const (
	// MsgHealthUpdate is sent when a server's health status changes.
	MsgHealthUpdate MessageType = iota

	// MsgPredictive is sent when an agent predicts an impending failure.
	MsgPredictive

	// MsgOverride is sent when an overwatch node overrides health status.
	MsgOverride

	// MsgNodeState is sent periodically to share full node state.
	MsgNodeState
)

// String returns the string representation of a MessageType.
func (m MessageType) String() string {
	switch m {
	case MsgHealthUpdate:
		return "health_update"
	case MsgPredictive:
		return "predictive"
	case MsgOverride:
		return "override"
	case MsgNodeState:
		return "node_state"
	default:
		return "unknown"
	}
}

// GossipMessage is the envelope for all gossip protocol messages.
type GossipMessage struct {
	Type      MessageType `json:"type"`
	NodeID    string      `json:"node_id"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   []byte      `json:"payload"`
}

// Encode serializes the GossipMessage to bytes.
func (m *GossipMessage) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeGossipMessage deserializes bytes into a GossipMessage.
func DecodeGossipMessage(data []byte) (*GossipMessage, error) {
	var msg GossipMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// HealthUpdate is sent when a server's health status changes.
type HealthUpdate struct {
	ServerAddr string        `json:"server_addr"`
	Region     string        `json:"region"`
	Healthy    bool          `json:"healthy"`
	Latency    time.Duration `json:"latency"`
	Error      string        `json:"error,omitempty"`
	CheckType  string        `json:"check_type"`
}

// Encode serializes the HealthUpdate to bytes.
func (h *HealthUpdate) Encode() ([]byte, error) {
	return json.Marshal(h)
}

// DecodeHealthUpdate deserializes bytes into a HealthUpdate.
func DecodeHealthUpdate(data []byte) (*HealthUpdate, error) {
	var hu HealthUpdate
	if err := json.Unmarshal(data, &hu); err != nil {
		return nil, err
	}
	return &hu, nil
}

// PredictiveSignal is sent when an agent predicts an impending failure.
// This implements the "predictive from the inside" philosophy.
type PredictiveSignal struct {
	NodeID    string  `json:"node_id"`
	Signal    string  `json:"signal"`    // "bleed", "drain", "critical"
	Reason    string  `json:"reason"`    // "cpu_high", "memory_pressure", "error_rate"
	Value     float64 `json:"value"`     // Current metric value
	Threshold float64 `json:"threshold"` // Threshold that triggered signal
}

// Encode serializes the PredictiveSignal to bytes.
func (p *PredictiveSignal) Encode() ([]byte, error) {
	return json.Marshal(p)
}

// DecodePredictiveSignal deserializes bytes into a PredictiveSignal.
func DecodePredictiveSignal(data []byte) (*PredictiveSignal, error) {
	var ps PredictiveSignal
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, err
	}
	return &ps, nil
}

// OverrideCommand is sent when an overwatch node overrides health status.
// This implements the "reactive from the outside" philosophy.
type OverrideCommand struct {
	TargetNode string `json:"target_node"` // Node being overridden
	ServerAddr string `json:"server_addr"` // Server address
	Action     string `json:"action"`      // "force_healthy", "force_unhealthy", "clear"
	Reason     string `json:"reason"`      // Human-readable reason
	Expiry     int64  `json:"expiry"`      // Unix timestamp when override expires (0 = never)
}

// Encode serializes the OverrideCommand to bytes.
func (o *OverrideCommand) Encode() ([]byte, error) {
	return json.Marshal(o)
}

// DecodeOverrideCommand deserializes bytes into an OverrideCommand.
func DecodeOverrideCommand(data []byte) (*OverrideCommand, error) {
	var oc OverrideCommand
	if err := json.Unmarshal(data, &oc); err != nil {
		return nil, err
	}
	return &oc, nil
}

// NodeStateUpdate contains the full state of a node for synchronization.
type NodeStateUpdate struct {
	NodeID       string              `json:"node_id"`
	HealthStates []ServerHealthState `json:"health_states"`
	IsLeader     bool                `json:"is_leader"`
	Uptime       time.Duration       `json:"uptime"`
}

// ServerHealthState represents the health state of a single server.
type ServerHealthState struct {
	ServerAddr       string        `json:"server_addr"`
	Region           string        `json:"region"`
	Healthy          bool          `json:"healthy"`
	LastCheck        time.Time     `json:"last_check"`
	LastLatency      time.Duration `json:"last_latency"`
	ConsecutiveFails int           `json:"consecutive_fails"`
}

// Encode serializes the NodeStateUpdate to bytes.
func (n *NodeStateUpdate) Encode() ([]byte, error) {
	return json.Marshal(n)
}

// DecodeNodeStateUpdate deserializes bytes into a NodeStateUpdate.
func DecodeNodeStateUpdate(data []byte) (*NodeStateUpdate, error) {
	var nsu NodeStateUpdate
	if err := json.Unmarshal(data, &nsu); err != nil {
		return nil, err
	}
	return &nsu, nil
}

// NewHealthUpdateMessage creates a GossipMessage containing a HealthUpdate.
func NewHealthUpdateMessage(nodeID string, update *HealthUpdate) (*GossipMessage, error) {
	payload, err := update.Encode()
	if err != nil {
		return nil, err
	}
	return &GossipMessage{
		Type:      MsgHealthUpdate,
		NodeID:    nodeID,
		Timestamp: time.Now(),
		Payload:   payload,
	}, nil
}

// NewPredictiveMessage creates a GossipMessage containing a PredictiveSignal.
func NewPredictiveMessage(nodeID string, signal *PredictiveSignal) (*GossipMessage, error) {
	payload, err := signal.Encode()
	if err != nil {
		return nil, err
	}
	return &GossipMessage{
		Type:      MsgPredictive,
		NodeID:    nodeID,
		Timestamp: time.Now(),
		Payload:   payload,
	}, nil
}

// NewOverrideMessage creates a GossipMessage containing an OverrideCommand.
func NewOverrideMessage(nodeID string, override *OverrideCommand) (*GossipMessage, error) {
	payload, err := override.Encode()
	if err != nil {
		return nil, err
	}
	return &GossipMessage{
		Type:      MsgOverride,
		NodeID:    nodeID,
		Timestamp: time.Now(),
		Payload:   payload,
	}, nil
}

// NewNodeStateMessage creates a GossipMessage containing a NodeStateUpdate.
func NewNodeStateMessage(nodeID string, state *NodeStateUpdate) (*GossipMessage, error) {
	payload, err := state.Encode()
	if err != nil {
		return nil, err
	}
	return &GossipMessage{
		Type:      MsgNodeState,
		NodeID:    nodeID,
		Timestamp: time.Now(),
		Payload:   payload,
	}, nil
}
