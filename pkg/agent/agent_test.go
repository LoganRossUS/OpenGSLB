// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

func TestNewAgent(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Mode: config.ModeAgent,
		Agent: config.AgentConfig{
			Identity: config.AgentIdentityConfig{
				ServiceToken: "test-token-12345678",
				Region:       "us-east",
				CertPath:     certPath,
				KeyPath:      keyPath,
			},
			Backends: []config.AgentBackend{
				{
					Service: "webapp",
					Address: "127.0.0.1",
					Port:    8080,
					Weight:  100,
					HealthCheck: config.HealthCheck{
						Type:             "http",
						Path:             "/health",
						Interval:         5 * time.Second,
						Timeout:          2 * time.Second,
						FailureThreshold: 3,
						SuccessThreshold: 2,
					},
				},
			},
			Predictive: config.PredictiveHealthConfig{
				Enabled:       true,
				CheckInterval: 10 * time.Second,
				CPU:           config.PredictiveMetricConfig{Threshold: 85},
				Memory:        config.PredictiveMetricConfig{Threshold: 90},
				ErrorRate:     config.PredictiveErrorRateConfig{Threshold: 5, Window: 60 * time.Second},
			},
			Heartbeat: config.HeartbeatConfig{
				Interval:        10 * time.Second,
				MissedThreshold: 3,
			},
		},
	}

	agent, err := NewAgent(AgentConfig{
		Config: cfg,
		Logger: logger,
		Gossip: nil, // No gossip for this test
	})
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Verify identity was created
	if agent.identity == nil {
		t.Fatal("identity should not be nil")
	}
	if agent.identity.AgentID == "" {
		t.Error("agent ID should not be empty")
	}
	if agent.identity.Region != "us-east" {
		t.Errorf("expected region 'us-east', got %q", agent.identity.Region)
	}

	// Verify backend was registered
	if agent.backends.BackendCount() != 1 {
		t.Errorf("expected 1 backend, got %d", agent.backends.BackendCount())
	}
}

func TestAgent_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Mode: config.ModeAgent,
		Agent: config.AgentConfig{
			Identity: config.AgentIdentityConfig{
				ServiceToken: "test-token-12345678",
				Region:       "us-east",
				CertPath:     certPath,
				KeyPath:      keyPath,
			},
			Backends: []config.AgentBackend{
				{
					Service: "webapp",
					Address: "127.0.0.1",
					Port:    8080,
					HealthCheck: config.HealthCheck{
						Type:     "http",
						Interval: 1 * time.Second,
						Timeout:  500 * time.Millisecond,
					},
				},
			},
			Heartbeat: config.HeartbeatConfig{
				Interval: 1 * time.Second,
			},
		},
	}

	agent, err := NewAgent(AgentConfig{
		Config: cfg,
		Logger: logger,
		Gossip: nil,
	})
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify running
	stats := agent.Stats()
	if !stats.Running {
		t.Error("agent should be running after Start")
	}
	if stats.AgentID == "" {
		t.Error("agent ID should be set")
	}

	// Starting again should fail
	if err := agent.Start(ctx); err == nil {
		t.Error("expected error when starting already running agent")
	}

	// Stop
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := agent.Stop(stopCtx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify stopped
	stats = agent.Stats()
	if stats.Running {
		t.Error("agent should not be running after Stop")
	}
}

func TestAgent_WithMockGossip(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockGossip := NewMockGossipSender()

	cfg := &config.Config{
		Mode: config.ModeAgent,
		Agent: config.AgentConfig{
			Identity: config.AgentIdentityConfig{
				ServiceToken: "test-token-12345678",
				Region:       "eu-west",
				CertPath:     certPath,
				KeyPath:      keyPath,
			},
			Backends: []config.AgentBackend{
				{
					Service: "api",
					Address: "10.0.1.1",
					Port:    9000,
					Weight:  100,
					HealthCheck: config.HealthCheck{
						Type:             "http",
						Path:             "/health",
						Interval:         100 * time.Millisecond,
						Timeout:          50 * time.Millisecond,
						FailureThreshold: 2,
						SuccessThreshold: 2,
					},
				},
			},
			Heartbeat: config.HeartbeatConfig{
				Interval:        100 * time.Millisecond,
				MissedThreshold: 3,
			},
			Predictive: config.PredictiveHealthConfig{
				Enabled: false,
			},
		},
	}

	agent, err := NewAgent(AgentConfig{
		Config: cfg,
		Logger: logger,
		Gossip: mockGossip,
	})
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for heartbeats and health updates
	time.Sleep(350 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	agent.Stop(stopCtx)

	// Verify heartbeats were sent
	heartbeats := mockGossip.Heartbeats()
	if len(heartbeats) < 2 {
		t.Errorf("expected at least 2 heartbeats, got %d", len(heartbeats))
	}

	// Verify heartbeat content
	if len(heartbeats) > 0 {
		hb := heartbeats[0]
		if hb.Region != "eu-west" {
			t.Errorf("expected region 'eu-west', got %q", hb.Region)
		}
		if hb.BackendCount != 1 {
			t.Errorf("expected backend count 1, got %d", hb.BackendCount)
		}
	}
}

func TestAgent_MultipleBackends(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Mode: config.ModeAgent,
		Agent: config.AgentConfig{
			Identity: config.AgentIdentityConfig{
				ServiceToken: "test-token-12345678",
				Region:       "ap-south",
				CertPath:     certPath,
				KeyPath:      keyPath,
			},
			Backends: []config.AgentBackend{
				{Service: "webapp", Address: "127.0.0.1", Port: 8080, Weight: 100},
				{Service: "api", Address: "127.0.0.1", Port: 9000, Weight: 100},
				{Service: "dbproxy", Address: "127.0.0.1", Port: 5432, Weight: 50},
			},
			Heartbeat: config.HeartbeatConfig{
				Interval: 10 * time.Second,
			},
		},
	}

	agent, err := NewAgent(AgentConfig{
		Config: cfg,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Verify all backends registered
	if agent.backends.BackendCount() != 3 {
		t.Errorf("expected 3 backends, got %d", agent.backends.BackendCount())
	}

	// Verify stats
	stats := agent.Stats()
	if stats.BackendCount != 3 {
		t.Errorf("expected backend count 3, got %d", stats.BackendCount)
	}
}

func TestAgent_GetIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Mode: config.ModeAgent,
		Agent: config.AgentConfig{
			Identity: config.AgentIdentityConfig{
				ServiceToken: "test-token-12345678",
				Region:       "us-west",
				CertPath:     certPath,
				KeyPath:      keyPath,
			},
			Backends: []config.AgentBackend{
				{Service: "webapp", Address: "127.0.0.1", Port: 8080},
			},
		},
	}

	agent, err := NewAgent(AgentConfig{
		Config: cfg,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	identity := agent.GetIdentity()
	if identity == nil {
		t.Fatal("GetIdentity returned nil")
	}
	if identity.Region != "us-west" {
		t.Errorf("expected region 'us-west', got %q", identity.Region)
	}
}

func TestAgent_GetBackendManager(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Mode: config.ModeAgent,
		Agent: config.AgentConfig{
			Identity: config.AgentIdentityConfig{
				ServiceToken: "test-token-12345678",
				Region:       "us-east",
				CertPath:     certPath,
				KeyPath:      keyPath,
			},
			Backends: []config.AgentBackend{
				{Service: "webapp", Address: "127.0.0.1", Port: 8080},
			},
		},
	}

	agent, err := NewAgent(AgentConfig{
		Config: cfg,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	bm := agent.GetBackendManager()
	if bm == nil {
		t.Fatal("GetBackendManager returned nil")
	}
	if bm.BackendCount() != 1 {
		t.Errorf("expected 1 backend, got %d", bm.BackendCount())
	}
}

func TestMockGossipSender(t *testing.T) {
	mock := NewMockGossipSender()

	// Test Start
	ctx := context.Background()
	if err := mock.Start(ctx); err != nil {
		t.Errorf("Start failed: %v", err)
	}

	// Test SendHealthUpdate
	msg := HealthUpdateMessage{
		AgentID:   "test-agent",
		Region:    "test-region",
		Timestamp: time.Now(),
	}
	if err := mock.SendHealthUpdate(msg); err != nil {
		t.Errorf("SendHealthUpdate failed: %v", err)
	}

	updates := mock.HealthUpdates()
	if len(updates) != 1 {
		t.Errorf("expected 1 health update, got %d", len(updates))
	}

	// Test SendHeartbeat
	hb := HeartbeatMessage{
		AgentID:      "test-agent",
		Region:       "test-region",
		Timestamp:    time.Now(),
		SequenceNum:  1,
		BackendCount: 2,
		Healthy:      true,
	}
	if err := mock.SendHeartbeat(hb); err != nil {
		t.Errorf("SendHeartbeat failed: %v", err)
	}

	heartbeats := mock.Heartbeats()
	if len(heartbeats) != 1 {
		t.Errorf("expected 1 heartbeat, got %d", len(heartbeats))
	}

	// Test Stop
	if err := mock.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Test Clear
	mock.Clear()
	if len(mock.HealthUpdates()) != 0 {
		t.Error("health updates should be empty after Clear")
	}
	if len(mock.Heartbeats()) != 0 {
		t.Error("heartbeats should be empty after Clear")
	}
}
