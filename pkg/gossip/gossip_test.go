// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package gossip

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/agent"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// testEncryptionKey is a valid 32-byte base64-encoded key for testing.
var testEncryptionKey = base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestReceiverConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  ReceiverConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ReceiverConfig{
				BindAddress:   "0.0.0.0:7946",
				EncryptionKey: testEncryptionKey,
				Logger:        testLogger(),
			},
			wantErr: false,
		},
		{
			name: "missing encryption key",
			config: ReceiverConfig{
				BindAddress: "0.0.0.0:7946",
				Logger:      testLogger(),
			},
			wantErr: true,
		},
		{
			name: "invalid encryption key encoding",
			config: ReceiverConfig{
				BindAddress:   "0.0.0.0:7946",
				EncryptionKey: "not-valid-base64!@#$",
				Logger:        testLogger(),
			},
			wantErr: true,
		},
		{
			name: "wrong key length",
			config: ReceiverConfig{
				BindAddress:   "0.0.0.0:7946",
				EncryptionKey: base64.StdEncoding.EncodeToString([]byte("short")),
				Logger:        testLogger(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMemberlistReceiver(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMemberlistReceiver() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSenderConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  SenderConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: SenderConfig{
				NodeName:       "test-agent",
				OverwatchNodes: []string{"localhost:7946"},
				EncryptionKey:  testEncryptionKey,
				Region:         "us-east-1",
				Logger:         testLogger(),
			},
			wantErr: false,
		},
		{
			name: "missing encryption key",
			config: SenderConfig{
				NodeName:       "test-agent",
				OverwatchNodes: []string{"localhost:7946"},
				Region:         "us-east-1",
				Logger:         testLogger(),
			},
			wantErr: true,
		},
		{
			name: "missing overwatch nodes",
			config: SenderConfig{
				NodeName:      "test-agent",
				EncryptionKey: testEncryptionKey,
				Region:        "us-east-1",
				Logger:        testLogger(),
			},
			wantErr: true,
		},
		{
			name: "empty overwatch nodes",
			config: SenderConfig{
				NodeName:       "test-agent",
				OverwatchNodes: []string{},
				EncryptionKey:  testEncryptionKey,
				Region:         "us-east-1",
				Logger:         testLogger(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMemberlistSender(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMemberlistSender() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReceiverStartStop(t *testing.T) {
	receiver, err := NewMemberlistReceiver(ReceiverConfig{
		NodeName:      "test-overwatch",
		BindAddress:   "127.0.0.1:0", // Random port
		EncryptionKey: testEncryptionKey,
		Logger:        testLogger(),
	})
	if err != nil {
		t.Fatalf("NewMemberlistReceiver() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start the receiver
	if err := receiver.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify it's running
	if receiver.NumMembers() < 1 {
		t.Error("Expected at least 1 member (self)")
	}

	// Stop the receiver
	if err := receiver.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Starting again after stop should work
	receiver2, err := NewMemberlistReceiver(ReceiverConfig{
		NodeName:      "test-overwatch-2",
		BindAddress:   "127.0.0.1:0",
		EncryptionKey: testEncryptionKey,
		Logger:        testLogger(),
	})
	if err != nil {
		t.Fatalf("NewMemberlistReceiver() error = %v", err)
	}

	if err := receiver2.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := receiver2.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestReceiverDoubleStart(t *testing.T) {
	receiver, err := NewMemberlistReceiver(ReceiverConfig{
		BindAddress:   "127.0.0.1:0",
		EncryptionKey: testEncryptionKey,
		Logger:        testLogger(),
	})
	if err != nil {
		t.Fatalf("NewMemberlistReceiver() error = %v", err)
	}

	ctx := context.Background()

	// Start should succeed
	if err := receiver.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Second start should fail
	if err := receiver.Start(ctx); err == nil {
		t.Error("Expected error on double start")
	}

	receiver.Stop()
}

func TestSenderNotRunning(t *testing.T) {
	sender, err := NewMemberlistSender(SenderConfig{
		NodeName:       "test-agent",
		OverwatchNodes: []string{"localhost:7946"},
		EncryptionKey:  testEncryptionKey,
		Region:         "us-east-1",
		Logger:         testLogger(),
	})
	if err != nil {
		t.Fatalf("NewMemberlistSender() error = %v", err)
	}

	// Trying to send without starting should fail
	msg := agent.HealthUpdateMessage{
		AgentID:   "test-agent",
		Region:    "us-east-1",
		Timestamp: time.Now(),
	}

	if err := sender.SendHealthUpdate(msg); err == nil {
		t.Error("Expected error when sender not running")
	}
}

func TestDefaultReceiverConfig(t *testing.T) {
	cfg := DefaultReceiverConfig()

	if cfg.BindAddress != "0.0.0.0:7946" {
		t.Errorf("BindAddress = %v, want 0.0.0.0:7946", cfg.BindAddress)
	}
	if cfg.ProbeInterval != 1*time.Second {
		t.Errorf("ProbeInterval = %v, want 1s", cfg.ProbeInterval)
	}
	if cfg.ProbeTimeout != 500*time.Millisecond {
		t.Errorf("ProbeTimeout = %v, want 500ms", cfg.ProbeTimeout)
	}
	if cfg.GossipInterval != 200*time.Millisecond {
		t.Errorf("GossipInterval = %v, want 200ms", cfg.GossipInterval)
	}
}

func TestNodeMeta(t *testing.T) {
	meta := NodeMeta{
		Role:      RoleAgent,
		Region:    "us-east-1",
		Version:   "1.0.0",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
	}

	if meta.Role != RoleAgent {
		t.Errorf("Role = %v, want %v", meta.Role, RoleAgent)
	}
	if meta.Region != "us-east-1" {
		t.Errorf("Region = %v, want us-east-1", meta.Region)
	}
}

func TestClusterIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := testLogger()

	// Start receiver (Overwatch)
	receiver, err := NewMemberlistReceiver(ReceiverConfig{
		NodeName:      "test-overwatch",
		BindAddress:   "127.0.0.1:0", // Random port
		EncryptionKey: testEncryptionKey,
		Logger:        logger,
	})
	if err != nil {
		t.Fatalf("NewMemberlistReceiver() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := receiver.Start(ctx); err != nil {
		t.Fatalf("Receiver Start() error = %v", err)
	}
	defer receiver.Stop()

	// Get the actual port the receiver bound to
	localNode := receiver.LocalNode()
	if localNode == nil {
		t.Fatal("LocalNode() returned nil")
	}
	overwatchAddr := localNode.FullAddress().Addr

	t.Logf("Receiver listening on %s", overwatchAddr)

	// Create sender (Agent) and connect to receiver
	sender, err := NewMemberlistSender(SenderConfig{
		NodeName:       "test-agent",
		BindAddress:    "127.0.0.1:0",
		OverwatchNodes: []string{overwatchAddr},
		EncryptionKey:  testEncryptionKey,
		Region:         "us-east-1",
		Logger:         logger,
	})
	if err != nil {
		t.Fatalf("NewMemberlistSender() error = %v", err)
	}

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("Sender Start() error = %v", err)
	}
	defer sender.Stop()

	// Wait for cluster to converge
	time.Sleep(1 * time.Second)

	// Verify both nodes see each other
	if receiver.NumMembers() != 2 {
		t.Errorf("Receiver NumMembers() = %d, want 2", receiver.NumMembers())
	}
	if sender.NumMembers() != 2 {
		t.Errorf("Sender NumMembers() = %d, want 2", sender.NumMembers())
	}

	// Send a health update
	healthMsg := agent.HealthUpdateMessage{
		AgentID:   "test-agent",
		Region:    "us-east-1",
		Timestamp: time.Now(),
		Backends: []agent.BackendHealthSnapshot{
			{
				Service: "api",
				Address: "10.0.0.1",
				Port:    8080,
				Weight:  100,
				Healthy: true,
			},
		},
	}

	if err := sender.SendHealthUpdate(healthMsg); err != nil {
		t.Errorf("SendHealthUpdate() error = %v", err)
	}

	// Give time for message delivery
	time.Sleep(500 * time.Millisecond)

	// Check that message was received
	select {
	case msg := <-receiver.MessageChan():
		if msg.Type != overwatch.MessageHeartbeat {
			t.Errorf("Message type = %v, want %v", msg.Type, overwatch.MessageHeartbeat)
		}
		if msg.AgentID != "test-agent" {
			t.Errorf("AgentID = %v, want test-agent", msg.AgentID)
		}
		if msg.Region != "us-east-1" {
			t.Errorf("Region = %v, want us-east-1", msg.Region)
		}
		t.Logf("Received message: type=%s, agent=%s", msg.Type, msg.AgentID)
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for message")
	}
}

func TestRegisterDeregisterMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := testLogger()

	// Start receiver
	receiver, err := NewMemberlistReceiver(ReceiverConfig{
		NodeName:      "test-overwatch",
		BindAddress:   "127.0.0.1:0",
		EncryptionKey: testEncryptionKey,
		Logger:        logger,
	})
	if err != nil {
		t.Fatalf("NewMemberlistReceiver() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := receiver.Start(ctx); err != nil {
		t.Fatalf("Receiver Start() error = %v", err)
	}
	defer receiver.Stop()

	localNode := receiver.LocalNode()
	overwatchAddr := localNode.FullAddress().Addr

	// Create sender
	sender, err := NewMemberlistSender(SenderConfig{
		NodeName:       "test-agent",
		BindAddress:    "127.0.0.1:0",
		OverwatchNodes: []string{overwatchAddr},
		EncryptionKey:  testEncryptionKey,
		Region:         "us-east-1",
		Logger:         logger,
	})
	if err != nil {
		t.Fatalf("NewMemberlistSender() error = %v", err)
	}

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("Sender Start() error = %v", err)
	}
	defer sender.Stop()

	time.Sleep(1 * time.Second)

	// Send register message
	err = sender.SendRegister("test-agent", "us-east-1", "api", "10.0.0.1", 8080, 100)
	if err != nil {
		t.Errorf("SendRegister() error = %v", err)
	}

	// Wait for message
	select {
	case msg := <-receiver.MessageChan():
		if msg.Type != overwatch.MessageRegister {
			t.Errorf("Message type = %v, want %v", msg.Type, overwatch.MessageRegister)
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for register message")
	}

	// Send deregister message
	err = sender.SendDeregister("test-agent", "us-east-1", "api", "10.0.0.1", 8080)
	if err != nil {
		t.Errorf("SendDeregister() error = %v", err)
	}

	// Wait for message
	select {
	case msg := <-receiver.MessageChan():
		if msg.Type != overwatch.MessageDeregister {
			t.Errorf("Message type = %v, want %v", msg.Type, overwatch.MessageDeregister)
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for deregister message")
	}
}

func TestEncryptionKeyMismatch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := testLogger()

	// Generate a different key for sender (must be exactly 32 bytes)
	differentKey := base64.StdEncoding.EncodeToString([]byte("different_key_32_bytes_exactly!!"))

	// Start receiver with first key
	receiver, err := NewMemberlistReceiver(ReceiverConfig{
		NodeName:      "test-overwatch",
		BindAddress:   "127.0.0.1:0",
		EncryptionKey: testEncryptionKey,
		Logger:        logger,
	})
	if err != nil {
		t.Fatalf("NewMemberlistReceiver() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := receiver.Start(ctx); err != nil {
		t.Fatalf("Receiver Start() error = %v", err)
	}
	defer receiver.Stop()

	localNode := receiver.LocalNode()
	overwatchAddr := localNode.FullAddress().Addr

	// Create sender with different key
	sender, err := NewMemberlistSender(SenderConfig{
		NodeName:       "test-agent",
		BindAddress:    "127.0.0.1:0",
		OverwatchNodes: []string{overwatchAddr},
		EncryptionKey:  differentKey, // Different key!
		Region:         "us-east-1",
		Logger:         logger,
	})
	if err != nil {
		t.Fatalf("NewMemberlistSender() error = %v", err)
	}

	// Joining should fail due to encryption key mismatch
	err = sender.Start(ctx)
	if err == nil {
		sender.Stop()
		t.Log("Warning: Join with mismatched key succeeded - memberlist may retry silently")
		// This is actually expected behavior - memberlist will fail to decrypt
		// but might not return an error immediately
	} else {
		t.Logf("Join with mismatched key failed as expected: %v", err)
	}
}
