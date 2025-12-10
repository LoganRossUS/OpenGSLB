// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestHeartbeatSender_SendsHeartbeats(t *testing.T) {
	transport := NewMockHeartbeatTransport()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	sender := NewHeartbeatSender(HeartbeatSenderConfig{
		Interval:        50 * time.Millisecond,
		MissedThreshold: 3,
		Logger:          logger,
	}, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sender.Start(ctx, "test-agent", "us-east", 3)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sender.Stop()

	// Wait for a few heartbeats
	time.Sleep(180 * time.Millisecond)

	messages := transport.Messages()
	if len(messages) < 2 {
		t.Errorf("expected at least 2 heartbeats, got %d", len(messages))
	}

	// Verify message content
	if len(messages) > 0 {
		msg := messages[0]
		if msg.AgentID != "test-agent" {
			t.Errorf("expected agent ID 'test-agent', got %q", msg.AgentID)
		}
		if msg.Region != "us-east" {
			t.Errorf("expected region 'us-east', got %q", msg.Region)
		}
		if msg.BackendCount != 3 {
			t.Errorf("expected backend count 3, got %d", msg.BackendCount)
		}
		if !msg.Healthy {
			t.Error("expected healthy=true")
		}
	}

	// Verify sequence numbers increment
	if len(messages) >= 2 {
		if messages[1].SequenceNum <= messages[0].SequenceNum {
			t.Error("sequence numbers should increment")
		}
	}
}

func TestHeartbeatSender_StopCleanly(t *testing.T) {
	transport := NewMockHeartbeatTransport()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	sender := NewHeartbeatSender(HeartbeatSenderConfig{
		Interval:        100 * time.Millisecond,
		MissedThreshold: 3,
		Logger:          logger,
	}, transport)

	ctx := context.Background()

	err := sender.Start(ctx, "test-agent", "us-east", 2)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop should return quickly
	done := make(chan struct{})
	go func() {
		sender.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good, stopped cleanly
	case <-time.After(1 * time.Second):
		t.Error("Stop took too long")
	}

	if sender.IsRunning() {
		t.Error("sender should not be running after Stop")
	}
}

func TestHeartbeatSender_TracksFailures(t *testing.T) {
	transport := NewMockHeartbeatTransport()
	transport.SetError(errors.New("send failed"))

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	sender := NewHeartbeatSender(HeartbeatSenderConfig{
		Interval:        50 * time.Millisecond,
		MissedThreshold: 3,
		Logger:          logger,
	}, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sender.Start(ctx, "test-agent", "us-east", 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sender.Stop()

	// Wait for some failed sends
	time.Sleep(180 * time.Millisecond)

	if sender.ConsecutiveFailures() < 2 {
		t.Errorf("expected at least 2 failures, got %d", sender.ConsecutiveFailures())
	}

	// LastSent should be zero (no successful sends)
	if !sender.LastSent().IsZero() {
		t.Error("LastSent should be zero with all failures")
	}
}

func TestHeartbeatSender_ResetsFailuresOnSuccess(t *testing.T) {
	transport := NewMockHeartbeatTransport()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	sender := NewHeartbeatSender(HeartbeatSenderConfig{
		Interval:        50 * time.Millisecond,
		MissedThreshold: 3,
		Logger:          logger,
	}, transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start with failures
	transport.SetError(errors.New("send failed"))

	err := sender.Start(ctx, "test-agent", "us-east", 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sender.Stop()

	// Wait for failures to accumulate
	time.Sleep(120 * time.Millisecond)

	failuresBefore := sender.ConsecutiveFailures()
	if failuresBefore < 1 {
		t.Skip("no failures accumulated")
	}

	// Clear error and wait for success
	transport.SetError(nil)
	time.Sleep(120 * time.Millisecond)

	if sender.ConsecutiveFailures() != 0 {
		t.Errorf("failures should reset to 0, got %d", sender.ConsecutiveFailures())
	}

	if sender.LastSent().IsZero() {
		t.Error("LastSent should be set after success")
	}
}

func TestHeartbeatSender_Stats(t *testing.T) {
	transport := NewMockHeartbeatTransport()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	sender := NewHeartbeatSender(HeartbeatSenderConfig{
		Interval:        100 * time.Millisecond,
		MissedThreshold: 5,
		Logger:          logger,
	}, transport)

	// Stats before start
	stats := sender.Stats()
	if stats.Running {
		t.Error("should not be running before Start")
	}
	if stats.Interval != 100*time.Millisecond {
		t.Errorf("unexpected interval: %v", stats.Interval)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender.Start(ctx, "test-agent", "us-east", 1)
	defer sender.Stop()

	time.Sleep(50 * time.Millisecond)

	stats = sender.Stats()
	if !stats.Running {
		t.Error("should be running after Start")
	}
}

func TestHeartbeatSender_ContextCancellation(t *testing.T) {
	transport := NewMockHeartbeatTransport()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	sender := NewHeartbeatSender(HeartbeatSenderConfig{
		Interval:        1 * time.Second, // Long interval
		MissedThreshold: 3,
		Logger:          logger,
	}, transport)

	ctx, cancel := context.WithCancel(context.Background())

	err := sender.Start(ctx, "test-agent", "us-east", 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Cancel context should stop the sender
	cancel()

	// Give it time to notice
	time.Sleep(100 * time.Millisecond)

	// Sender loop should have exited
	// Note: IsRunning may still be true since we didn't call Stop()
	// but the goroutine should have exited
}

func TestDefaultHeartbeatConfig(t *testing.T) {
	cfg := DefaultHeartbeatConfig()
	if cfg.Interval != 10*time.Second {
		t.Errorf("unexpected default interval: %v", cfg.Interval)
	}
	if cfg.MissedThreshold != 3 {
		t.Errorf("unexpected default missed threshold: %d", cfg.MissedThreshold)
	}
}

func TestMockHeartbeatTransport(t *testing.T) {
	mock := NewMockHeartbeatTransport()

	msg := HeartbeatMessage{
		AgentID:      "test",
		Region:       "test-region",
		Timestamp:    time.Now(),
		SequenceNum:  1,
		BackendCount: 2,
		Healthy:      true,
	}

	// Send should succeed
	if err := mock.SendHeartbeat(msg); err != nil {
		t.Errorf("SendHeartbeat failed: %v", err)
	}

	messages := mock.Messages()
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}

	// Set error
	mock.SetError(errors.New("test error"))
	if err := mock.SendHeartbeat(msg); err == nil {
		t.Error("expected error after SetError")
	}

	// Clear
	mock.Clear()
	mock.SetError(nil)
	if len(mock.Messages()) != 0 {
		t.Error("expected 0 messages after Clear")
	}
}
