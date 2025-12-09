// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/memberlist"
)

func TestMessageEncodeDecode(t *testing.T) {
	tests := []struct {
		name    string
		msgType MessageType
		payload interface{}
	}{
		{
			name:    "health update",
			msgType: MsgHealthUpdate,
			payload: &HealthUpdate{
				ServerAddr: "10.0.1.10:80",
				Region:     "us-east-1",
				Healthy:    true,
				Latency:    50 * time.Millisecond,
				CheckType:  "http",
			},
		},
		{
			name:    "health update unhealthy",
			msgType: MsgHealthUpdate,
			payload: &HealthUpdate{
				ServerAddr: "10.0.1.11:80",
				Region:     "us-west-2",
				Healthy:    false,
				Error:      "connection refused",
				CheckType:  "tcp",
			},
		},
		{
			name:    "predictive signal",
			msgType: MsgPredictive,
			payload: &PredictiveSignal{
				NodeID:    "node-1",
				Signal:    "bleed",
				Reason:    "cpu_high",
				Value:     92.5,
				Threshold: 90.0,
			},
		},
		{
			name:    "override command",
			msgType: MsgOverride,
			payload: &OverrideCommand{
				TargetNode: "node-2",
				ServerAddr: "10.0.1.10:80",
				Action:     "force_unhealthy",
				Reason:     "manual override for maintenance",
				Expiry:     time.Now().Add(1 * time.Hour).Unix(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode payload
			payloadBytes, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			// Create message
			msg := &GossipMessage{
				Type:      tt.msgType,
				NodeID:    "test-node",
				Timestamp: time.Now(),
				Payload:   payloadBytes,
			}

			// Encode message
			data, err := msg.Encode()
			if err != nil {
				t.Fatalf("failed to encode message: %v", err)
			}

			// Decode message
			decoded, err := DecodeGossipMessage(data)
			if err != nil {
				t.Fatalf("failed to decode message: %v", err)
			}

			// Verify fields
			if decoded.Type != tt.msgType {
				t.Errorf("type mismatch: got %v, want %v", decoded.Type, tt.msgType)
			}
			if decoded.NodeID != "test-node" {
				t.Errorf("node_id mismatch: got %v, want test-node", decoded.NodeID)
			}
			if len(decoded.Payload) == 0 {
				t.Error("payload is empty")
			}
		})
	}
}

func TestHealthUpdateMessage(t *testing.T) {
	update := &HealthUpdate{
		ServerAddr: "10.0.1.10:80",
		Region:     "us-east-1",
		Healthy:    true,
		Latency:    45 * time.Millisecond,
		CheckType:  "http",
	}

	msg, err := NewHealthUpdateMessage("node-1", update)
	if err != nil {
		t.Fatalf("failed to create health update message: %v", err)
	}

	if msg.Type != MsgHealthUpdate {
		t.Errorf("wrong message type: got %v, want %v", msg.Type, MsgHealthUpdate)
	}

	if msg.NodeID != "node-1" {
		t.Errorf("wrong node ID: got %v, want node-1", msg.NodeID)
	}

	// Decode and verify payload
	decoded, err := DecodeHealthUpdate(msg.Payload)
	if err != nil {
		t.Fatalf("failed to decode health update: %v", err)
	}

	if decoded.ServerAddr != update.ServerAddr {
		t.Errorf("server addr mismatch: got %v, want %v", decoded.ServerAddr, update.ServerAddr)
	}
	if decoded.Region != update.Region {
		t.Errorf("region mismatch: got %v, want %v", decoded.Region, update.Region)
	}
	if decoded.Healthy != update.Healthy {
		t.Errorf("healthy mismatch: got %v, want %v", decoded.Healthy, update.Healthy)
	}
}

func TestPredictiveSignalMessage(t *testing.T) {
	signal := &PredictiveSignal{
		NodeID:    "node-1",
		Signal:    "drain",
		Reason:    "memory_pressure",
		Value:     88.5,
		Threshold: 85.0,
	}

	msg, err := NewPredictiveMessage("node-1", signal)
	if err != nil {
		t.Fatalf("failed to create predictive message: %v", err)
	}

	if msg.Type != MsgPredictive {
		t.Errorf("wrong message type: got %v, want %v", msg.Type, MsgPredictive)
	}

	// Decode and verify payload
	decoded, err := DecodePredictiveSignal(msg.Payload)
	if err != nil {
		t.Fatalf("failed to decode predictive signal: %v", err)
	}

	if decoded.Signal != signal.Signal {
		t.Errorf("signal mismatch: got %v, want %v", decoded.Signal, signal.Signal)
	}
	if decoded.Reason != signal.Reason {
		t.Errorf("reason mismatch: got %v, want %v", decoded.Reason, signal.Reason)
	}
}

func TestOverrideCommandMessage(t *testing.T) {
	override := &OverrideCommand{
		TargetNode: "node-2",
		ServerAddr: "10.0.1.10:80",
		Action:     "force_healthy",
		Reason:     "overwatch validation passed",
		Expiry:     0,
	}

	msg, err := NewOverrideMessage("overwatch-1", override)
	if err != nil {
		t.Fatalf("failed to create override message: %v", err)
	}

	if msg.Type != MsgOverride {
		t.Errorf("wrong message type: got %v, want %v", msg.Type, MsgOverride)
	}

	// Decode and verify payload
	decoded, err := DecodeOverrideCommand(msg.Payload)
	if err != nil {
		t.Fatalf("failed to decode override command: %v", err)
	}

	if decoded.Action != override.Action {
		t.Errorf("action mismatch: got %v, want %v", decoded.Action, override.Action)
	}
	if decoded.TargetNode != override.TargetNode {
		t.Errorf("target_node mismatch: got %v, want %v", decoded.TargetNode, override.TargetNode)
	}
}

func TestMessageTypeString(t *testing.T) {
	tests := []struct {
		msgType MessageType
		want    string
	}{
		{MsgHealthUpdate, "health_update"},
		{MsgPredictive, "predictive"},
		{MsgOverride, "override"},
		{MsgNodeState, "node_state"},
		{MessageType(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.msgType.String()
		if got != tt.want {
			t.Errorf("MessageType(%d).String() = %v, want %v", tt.msgType, got, tt.want)
		}
	}
}

func TestGossipManagerCreation(t *testing.T) {
	// Test with nil config
	_, err := NewGossipManager(nil, nil)
	if err == nil {
		t.Error("expected error with nil config (missing NodeID)")
	}

	// Test with valid config
	cfg := DefaultGossipConfig()
	cfg.NodeID = "test-node"
	cfg.BindAddr = "127.0.0.1"

	gm, err := NewGossipManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create gossip manager: %v", err)
	}

	if gm.IsRunning() {
		t.Error("gossip manager should not be running before Start()")
	}
}

func TestGossipManagerStartStop(t *testing.T) {
	cfg := DefaultGossipConfig()
	cfg.NodeID = "test-node-startstop"
	cfg.BindAddr = "127.0.0.1"
	cfg.BindPort = 17946 // Use non-default port to avoid conflicts

	gm, err := NewGossipManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create gossip manager: %v", err)
	}

	ctx := context.Background()

	// Start
	if err := gm.Start(ctx); err != nil {
		t.Fatalf("failed to start gossip manager: %v", err)
	}

	if !gm.IsRunning() {
		t.Error("gossip manager should be running after Start()")
	}

	// Verify we have at least ourselves as a member
	members := gm.Members()
	if len(members) == 0 {
		t.Error("expected at least one member (self)")
	}

	// Verify NumMembers
	if gm.NumMembers() < 1 {
		t.Errorf("NumMembers() = %d, want >= 1", gm.NumMembers())
	}

	// Verify LocalNode
	local := gm.LocalNode()
	if local == nil {
		t.Fatal("LocalNode() returned nil")
	}
	if local.Name != "test-node-startstop" {
		t.Errorf("LocalNode().Name = %v, want test-node-startstop", local.Name)
	}

	// Verify Uptime is positive
	if gm.Uptime() <= 0 {
		t.Error("Uptime() should be positive after Start()")
	}

	// Stop
	if err := gm.Stop(ctx); err != nil {
		t.Fatalf("failed to stop gossip manager: %v", err)
	}

	if gm.IsRunning() {
		t.Error("gossip manager should not be running after Stop()")
	}
}

func TestGossipManagerDoubleStart(t *testing.T) {
	cfg := DefaultGossipConfig()
	cfg.NodeID = "test-node-double"
	cfg.BindAddr = "127.0.0.1"
	cfg.BindPort = 17947

	gm, err := NewGossipManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create gossip manager: %v", err)
	}

	ctx := context.Background()
	defer gm.Stop(ctx)

	// First start should succeed
	if err := gm.Start(ctx); err != nil {
		t.Fatalf("first Start() failed: %v", err)
	}

	// Second start should fail
	if err := gm.Start(ctx); err == nil {
		t.Error("second Start() should have failed")
	}
}

func TestGossipManagerMessageHandlers(t *testing.T) {
	cfg := DefaultGossipConfig()
	cfg.NodeID = "test-node-handlers"
	cfg.BindAddr = "127.0.0.1"
	cfg.BindPort = 17948

	gm, err := NewGossipManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create gossip manager: %v", err)
	}

	ctx := context.Background()
	defer gm.Stop(ctx)

	var (
		mu               sync.Mutex
		receivedHealth   *HealthUpdate
		receivedPredict  *PredictiveSignal
		receivedOverride *OverrideCommand
		healthFrom       string
		predictFrom      string
		overrideFrom     string
	)

	// Set up handlers
	gm.OnHealthUpdate(func(update *HealthUpdate, from string) {
		mu.Lock()
		defer mu.Unlock()
		receivedHealth = update
		healthFrom = from
	})

	gm.OnPredictive(func(signal *PredictiveSignal, from string) {
		mu.Lock()
		defer mu.Unlock()
		receivedPredict = signal
		predictFrom = from
	})

	gm.OnOverride(func(override *OverrideCommand, from string) {
		mu.Lock()
		defer mu.Unlock()
		receivedOverride = override
		overrideFrom = from
	})

	if err := gm.Start(ctx); err != nil {
		t.Fatalf("failed to start gossip manager: %v", err)
	}

	// Test health update handler
	healthUpdate := &HealthUpdate{
		ServerAddr: "10.0.1.10:80",
		Region:     "us-east-1",
		Healthy:    true,
	}
	msg, _ := NewHealthUpdateMessage("sender-node", healthUpdate)
	if err := gm.HandleMessage(msg); err != nil {
		t.Errorf("HandleMessage(health) failed: %v", err)
	}

	mu.Lock()
	if receivedHealth == nil {
		t.Error("health update handler was not called")
	} else if receivedHealth.ServerAddr != healthUpdate.ServerAddr {
		t.Errorf("health update mismatch: got %v, want %v", receivedHealth.ServerAddr, healthUpdate.ServerAddr)
	}
	if healthFrom != "sender-node" {
		t.Errorf("health from mismatch: got %v, want sender-node", healthFrom)
	}
	mu.Unlock()

	// Test predictive handler
	predictSignal := &PredictiveSignal{
		NodeID: "node-1",
		Signal: "bleed",
		Reason: "cpu_high",
	}
	msg, _ = NewPredictiveMessage("sender-node", predictSignal)
	if err := gm.HandleMessage(msg); err != nil {
		t.Errorf("HandleMessage(predictive) failed: %v", err)
	}

	mu.Lock()
	if receivedPredict == nil {
		t.Error("predictive handler was not called")
	} else if receivedPredict.Signal != predictSignal.Signal {
		t.Errorf("predictive signal mismatch: got %v, want %v", receivedPredict.Signal, predictSignal.Signal)
	}
	if predictFrom != "sender-node" {
		t.Errorf("predict from mismatch: got %v, want sender-node", predictFrom)
	}
	mu.Unlock()

	// Test override handler
	overrideCmd := &OverrideCommand{
		TargetNode: "node-2",
		Action:     "force_unhealthy",
	}
	msg, _ = NewOverrideMessage("overwatch", overrideCmd)
	if err := gm.HandleMessage(msg); err != nil {
		t.Errorf("HandleMessage(override) failed: %v", err)
	}

	mu.Lock()
	if receivedOverride == nil {
		t.Error("override handler was not called")
	} else if receivedOverride.Action != overrideCmd.Action {
		t.Errorf("override action mismatch: got %v, want %v", receivedOverride.Action, overrideCmd.Action)
	}
	if overrideFrom != "overwatch" {
		t.Errorf("override from mismatch: got %v, want overwatch", overrideFrom)
	}
	mu.Unlock()
}

func TestGossipManagerBroadcastNotRunning(t *testing.T) {
	cfg := DefaultGossipConfig()
	cfg.NodeID = "test-node-norun"
	cfg.BindAddr = "127.0.0.1"

	gm, err := NewGossipManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create gossip manager: %v", err)
	}

	// Don't start the manager

	// Try to broadcast - should fail
	update := &HealthUpdate{ServerAddr: "10.0.1.10:80", Healthy: true}
	if err := gm.BroadcastHealthUpdate(update); err != ErrGossipNotRunning {
		t.Errorf("BroadcastHealthUpdate() error = %v, want %v", err, ErrGossipNotRunning)
	}

	signal := &PredictiveSignal{Signal: "bleed"}
	if err := gm.BroadcastPredictive(signal); err != ErrGossipNotRunning {
		t.Errorf("BroadcastPredictive() error = %v, want %v", err, ErrGossipNotRunning)
	}

	override := &OverrideCommand{Action: "force_healthy"}
	if err := gm.BroadcastOverride(override); err != ErrGossipNotRunning {
		t.Errorf("BroadcastOverride() error = %v, want %v", err, ErrGossipNotRunning)
	}
}

func TestGossipDelegateNotifyMsg(t *testing.T) {
	var received *GossipMessage

	handler := &mockMessageHandler{
		handleFunc: func(msg *GossipMessage) error {
			received = msg
			return nil
		},
	}

	delegate := NewGossipDelegate("test-node", handler, nil)

	// Create and encode a health update message
	update := &HealthUpdate{
		ServerAddr: "10.0.1.10:80",
		Healthy:    true,
	}
	msg, _ := NewHealthUpdateMessage("sender", update)
	data, _ := msg.Encode()

	// Notify the delegate
	delegate.NotifyMsg(data)

	// Verify message was received
	if received == nil {
		t.Fatal("message was not received")
	}
	if received.Type != MsgHealthUpdate {
		t.Errorf("message type mismatch: got %v, want %v", received.Type, MsgHealthUpdate)
	}
	if received.NodeID != "sender" {
		t.Errorf("node ID mismatch: got %v, want sender", received.NodeID)
	}
}

func TestGossipDelegateEmptyMessage(t *testing.T) {
	handler := &mockMessageHandler{
		handleFunc: func(msg *GossipMessage) error {
			t.Error("handler should not be called for empty message")
			return nil
		},
	}

	delegate := NewGossipDelegate("test-node", handler, nil)

	// Empty message should be ignored
	delegate.NotifyMsg(nil)
	delegate.NotifyMsg([]byte{})
}

func TestGossipEventDelegate(t *testing.T) {
	events := NewGossipEventDelegate(nil)
	events.OnJoin(func(_ *memberlist.Node) {
		// Handler registered
	})
	events.OnLeave(func(_ *memberlist.Node) {
		// Handler registered
	})
	events.OnUpdate(func(_ *memberlist.Node) {
		// Handler registered
	})

	// Note: We can't easily test the actual memberlist.Node callbacks
	// without creating a full memberlist. This tests the callback registration.
	if events.onJoin == nil {
		t.Error("OnJoin callback not set")
	}
	if events.onLeave == nil {
		t.Error("OnLeave callback not set")
	}
	if events.onUpdate == nil {
		t.Error("OnUpdate callback not set")
	}
}

func TestNodeStateUpdate(t *testing.T) {
	state := &NodeStateUpdate{
		NodeID:   "node-1",
		IsLeader: true,
		Uptime:   5 * time.Minute,
		HealthStates: []ServerHealthState{
			{
				ServerAddr:       "10.0.1.10:80",
				Region:           "us-east-1",
				Healthy:          true,
				LastCheck:        time.Now(),
				LastLatency:      45 * time.Millisecond,
				ConsecutiveFails: 0,
			},
			{
				ServerAddr:       "10.0.1.11:80",
				Region:           "us-east-1",
				Healthy:          false,
				LastCheck:        time.Now(),
				ConsecutiveFails: 3,
			},
		},
	}

	msg, err := NewNodeStateMessage("node-1", state)
	if err != nil {
		t.Fatalf("failed to create node state message: %v", err)
	}

	if msg.Type != MsgNodeState {
		t.Errorf("wrong message type: got %v, want %v", msg.Type, MsgNodeState)
	}

	// Decode and verify
	decoded, err := DecodeNodeStateUpdate(msg.Payload)
	if err != nil {
		t.Fatalf("failed to decode node state: %v", err)
	}

	if decoded.NodeID != state.NodeID {
		t.Errorf("node_id mismatch: got %v, want %v", decoded.NodeID, state.NodeID)
	}
	if decoded.IsLeader != state.IsLeader {
		t.Errorf("is_leader mismatch: got %v, want %v", decoded.IsLeader, state.IsLeader)
	}
	if len(decoded.HealthStates) != len(state.HealthStates) {
		t.Errorf("health_states length mismatch: got %v, want %v", len(decoded.HealthStates), len(state.HealthStates))
	}
}

// mockMessageHandler implements MessageHandler for testing.
type mockMessageHandler struct {
	handleFunc func(msg *GossipMessage) error
}

func (m *mockMessageHandler) HandleMessage(msg *GossipMessage) error {
	if m.handleFunc != nil {
		return m.handleFunc(msg)
	}
	return nil
}
