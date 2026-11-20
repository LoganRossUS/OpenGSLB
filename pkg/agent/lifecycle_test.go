// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// A shutdown can arrive while the agent is still starting (a signal during
// startup). Stop and Stats must not read what Start is writing, and an agent
// that was stopped must not be left with a running heartbeat.
func TestAgent_StopAndStatsDuringStart(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for i := 0; i < 30; i++ {
		tmpDir := t.TempDir()
		cfg := &config.Config{
			Mode: config.ModeAgent,
			Agent: config.AgentConfig{
				Identity: config.AgentIdentityConfig{
					ServiceToken: "test-token-12345678",
					Region:       "us-east",
					CertPath:     filepath.Join(tmpDir, "agent.crt"),
					KeyPath:      filepath.Join(tmpDir, "agent.key"),
				},
				Backends: []config.AgentBackend{{
					Service: "webapp",
					Address: "127.0.0.1",
					Port:    8080,
					HealthCheck: config.HealthCheck{
						Type:     "tcp",
						Interval: time.Second,
						Timeout:  500 * time.Millisecond,
					},
				}},
				Heartbeat: config.HeartbeatConfig{Interval: time.Second},
			},
		}
		a, err := NewAgent(AgentConfig{Config: cfg, Logger: logger, Gossip: NewMockGossipSender()})
		if err != nil {
			t.Fatalf("NewAgent: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = a.Start(ctx)
		}()
		go func() {
			defer wg.Done()
			_ = a.Stats()
		}()
		go func() {
			defer wg.Done()
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer stopCancel()
			_ = a.Stop(stopCtx)
		}()
		wg.Wait()

		// Whatever the interleaving, a final Stop leaves nothing running.
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := a.Stop(stopCtx); err != nil {
			t.Fatalf("iteration %d: final Stop: %v", i, err)
		}
		stopCancel()
		cancel()
		if a.Stats().Running {
			t.Fatalf("iteration %d: agent still running after Stop", i)
		}
	}
}
