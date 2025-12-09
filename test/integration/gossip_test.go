//go:build integration

// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/loganrossus/OpenGSLB/pkg/cluster"
)

// TestGossipClusterFormation tests that nodes can form a gossip cluster.
func TestGossipClusterFormation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create 3 gossip nodes
	nodes := make([]*cluster.GossipManager, 3)
	basePort := 17950

	for i := 0; i < 3; i++ {
		cfg := cluster.DefaultGossipConfig()
		cfg.NodeID = fmt.Sprintf("test-node-%d", i)
		cfg.BindAddr = "127.0.0.1"
		cfg.BindPort = basePort + i

		// First node has no seeds; subsequent nodes seed from first node
		if i > 0 {
			cfg.Seeds = []string{fmt.Sprintf("127.0.0.1:%d", basePort)}
		}

		gm, err := cluster.NewGossipManager(cfg, nil)
		if err != nil {
			t.Fatalf("failed to create gossip manager %d: %v", i, err)
		}
		nodes[i] = gm
	}

	// Start all nodes
	for i, gm := range nodes {
		if err := gm.Start(ctx); err != nil {
			t.Fatalf("failed to start gossip manager %d: %v", i, err)
		}
		defer gm.Stop(ctx)
	}

	// Wait for cluster to form
	time.Sleep(2 * time.Second)

	// Verify all nodes see each other
	for i, gm := range nodes {
		members := gm.NumMembers()
		if members != 3 {
			t.Errorf("node %d sees %d members, want 3", i, members)
		}
	}
}

// TestGossipHealthUpdatePropagation tests that health updates propagate to all nodes.
func TestGossipHealthUpdatePropagation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create 3 gossip nodes
	nodes := make([]*cluster.GossipManager, 3)
	basePort := 17960

	// Track received updates per node
	var mu sync.Mutex
	receivedUpdates := make(map[string][]*cluster.HealthUpdate)

	for i := 0; i < 3; i++ {
		cfg := cluster.DefaultGossipConfig()
		cfg.NodeID = fmt.Sprintf("health-node-%d", i)
		cfg.BindAddr = "127.0.0.1"
		cfg.BindPort = basePort + i

		if i > 0 {
			cfg.Seeds = []string{fmt.Sprintf("127.0.0.1:%d", basePort)}
		}

		gm, err := cluster.NewGossipManager(cfg, nil)
		if err != nil {
			t.Fatalf("failed to create gossip manager %d: %v", i, err)
		}

		// Set up handler to track received updates
		nodeID := cfg.NodeID
		gm.OnHealthUpdate(func(update *cluster.HealthUpdate, from string) {
			mu.Lock()
			defer mu.Unlock()
			receivedUpdates[nodeID] = append(receivedUpdates[nodeID], update)
		})

		nodes[i] = gm
	}

	// Start all nodes
	for i, gm := range nodes {
		if err := gm.Start(ctx); err != nil {
			t.Fatalf("failed to start gossip manager %d: %v", i, err)
		}
		defer gm.Stop(ctx)
	}

	// Wait for cluster to form
	time.Sleep(2 * time.Second)

	// Broadcast a health update from node 0
	update := &cluster.HealthUpdate{
		ServerAddr: "10.0.1.10:80",
		Region:     "us-east-1",
		Healthy:    false,
		Error:      "connection refused",
		CheckType:  "http",
	}

	if err := nodes[0].BroadcastHealthUpdate(update); err != nil {
		t.Fatalf("failed to broadcast health update: %v", err)
	}

	// Wait for propagation (should be <500ms, we give it 1s)
	time.Sleep(1 * time.Second)

	// Verify nodes 1 and 2 received the update
	mu.Lock()
	defer mu.Unlock()

	for i := 1; i < 3; i++ {
		nodeID := fmt.Sprintf("health-node-%d", i)
		updates := receivedUpdates[nodeID]
		if len(updates) == 0 {
			t.Errorf("node %s did not receive health update", nodeID)
			continue
		}

		received := updates[0]
		if received.ServerAddr != update.ServerAddr {
			t.Errorf("node %s: server_addr mismatch: got %v, want %v",
				nodeID, received.ServerAddr, update.ServerAddr)
		}
		if received.Healthy != update.Healthy {
			t.Errorf("node %s: healthy mismatch: got %v, want %v",
				nodeID, received.Healthy, update.Healthy)
		}
	}
}

// TestGossipPredictiveSignalPropagation tests predictive signal propagation.
func TestGossipPredictiveSignalPropagation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create 2 gossip nodes
	nodes := make([]*cluster.GossipManager, 2)
	basePort := 17970

	var mu sync.Mutex
	var receivedSignal *cluster.PredictiveSignal
	var receivedFrom string

	for i := 0; i < 2; i++ {
		cfg := cluster.DefaultGossipConfig()
		cfg.NodeID = fmt.Sprintf("predict-node-%d", i)
		cfg.BindAddr = "127.0.0.1"
		cfg.BindPort = basePort + i

		if i > 0 {
			cfg.Seeds = []string{fmt.Sprintf("127.0.0.1:%d", basePort)}
		}

		gm, err := cluster.NewGossipManager(cfg, nil)
		if err != nil {
			t.Fatalf("failed to create gossip manager %d: %v", i, err)
		}

		// Only node 1 receives
		if i == 1 {
			gm.OnPredictive(func(signal *cluster.PredictiveSignal, from string) {
				mu.Lock()
				defer mu.Unlock()
				receivedSignal = signal
				receivedFrom = from
			})
		}

		nodes[i] = gm
	}

	// Start all nodes
	for i, gm := range nodes {
		if err := gm.Start(ctx); err != nil {
			t.Fatalf("failed to start gossip manager %d: %v", i, err)
		}
		defer gm.Stop(ctx)
	}

	// Wait for cluster to form
	time.Sleep(2 * time.Second)

	// Send predictive signal from node 0
	signal := &cluster.PredictiveSignal{
		NodeID:    "predict-node-0",
		Signal:    "bleed",
		Reason:    "cpu_high",
		Value:     92.5,
		Threshold: 90.0,
	}

	if err := nodes[0].BroadcastPredictive(signal); err != nil {
		t.Fatalf("failed to broadcast predictive signal: %v", err)
	}

	// Wait for propagation
	time.Sleep(1 * time.Second)

	// Verify node 1 received the signal
	mu.Lock()
	defer mu.Unlock()

	if receivedSignal == nil {
		t.Fatal("node 1 did not receive predictive signal")
	}
	if receivedSignal.Signal != signal.Signal {
		t.Errorf("signal mismatch: got %v, want %v", receivedSignal.Signal, signal.Signal)
	}
	if receivedSignal.Reason != signal.Reason {
		t.Errorf("reason mismatch: got %v, want %v", receivedSignal.Reason, signal.Reason)
	}
	if receivedFrom != "predict-node-0" {
		t.Errorf("from mismatch: got %v, want predict-node-0", receivedFrom)
	}
}

// TestGossipNodeJoinLeave tests node join and leave detection.
func TestGossipNodeJoinLeave(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	basePort := 17980

	// Create first node
	cfg1 := cluster.DefaultGossipConfig()
	cfg1.NodeID = "join-leave-node-1"
	cfg1.BindAddr = "127.0.0.1"
	cfg1.BindPort = basePort

	var mu sync.Mutex
	var joinCount int
	var leaveCount int

	gm1, err := cluster.NewGossipManager(cfg1, nil)
	if err != nil {
		t.Fatalf("failed to create gossip manager 1: %v", err)
	}

	gm1.OnNodeJoin(func(_ *memberlist.Node) {
		mu.Lock()
		defer mu.Unlock()
		joinCount++
	})

	gm1.OnNodeLeave(func(_ *memberlist.Node) {
		mu.Lock()
		defer mu.Unlock()
		leaveCount++
	})

	if err := gm1.Start(ctx); err != nil {
		t.Fatalf("failed to start gossip manager 1: %v", err)
	}
	defer gm1.Stop(ctx)

	// Create and start second node
	cfg2 := cluster.DefaultGossipConfig()
	cfg2.NodeID = "join-leave-node-2"
	cfg2.BindAddr = "127.0.0.1"
	cfg2.BindPort = basePort + 1
	cfg2.Seeds = []string{fmt.Sprintf("127.0.0.1:%d", basePort)}

	gm2, err := cluster.NewGossipManager(cfg2, nil)
	if err != nil {
		t.Fatalf("failed to create gossip manager 2: %v", err)
	}

	if err := gm2.Start(ctx); err != nil {
		t.Fatalf("failed to start gossip manager 2: %v", err)
	}

	// Wait for join detection
	time.Sleep(2 * time.Second)

	// Verify node 1 sees 2 members
	if gm1.NumMembers() != 2 {
		t.Errorf("node 1 sees %d members, want 2", gm1.NumMembers())
	}

	// Stop node 2
	if err := gm2.Stop(ctx); err != nil {
		t.Fatalf("failed to stop gossip manager 2: %v", err)
	}

	// Wait for leave detection
	time.Sleep(2 * time.Second)

	// Verify node 1 sees 1 member again
	if gm1.NumMembers() != 1 {
		t.Errorf("node 1 sees %d members after leave, want 1", gm1.NumMembers())
	}
}

// TestGossipPropagationLatency measures the latency of health update propagation.
func TestGossipPropagationLatency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create 3 nodes
	nodes := make([]*cluster.GossipManager, 3)
	basePort := 17990

	received := make(chan time.Time, 10)

	for i := 0; i < 3; i++ {
		cfg := cluster.DefaultGossipConfig()
		cfg.NodeID = fmt.Sprintf("latency-node-%d", i)
		cfg.BindAddr = "127.0.0.1"
		cfg.BindPort = basePort + i
		cfg.GossipInterval = 100 * time.Millisecond // Faster gossip for test

		if i > 0 {
			cfg.Seeds = []string{fmt.Sprintf("127.0.0.1:%d", basePort)}
		}

		gm, err := cluster.NewGossipManager(cfg, nil)
		if err != nil {
			t.Fatalf("failed to create gossip manager %d: %v", i, err)
		}

		// Track when other nodes receive
		if i > 0 {
			gm.OnHealthUpdate(func(update *cluster.HealthUpdate, from string) {
				received <- time.Now()
			})
		}

		nodes[i] = gm
	}

	// Start all nodes
	for i, gm := range nodes {
		if err := gm.Start(ctx); err != nil {
			t.Fatalf("failed to start gossip manager %d: %v", i, err)
		}
		defer gm.Stop(ctx)
	}

	// Wait for cluster to form
	time.Sleep(2 * time.Second)

	// Send health update and measure propagation time
	sendTime := time.Now()
	update := &cluster.HealthUpdate{
		ServerAddr: "10.0.1.10:80",
		Region:     "us-east-1",
		Healthy:    true,
	}

	if err := nodes[0].BroadcastHealthUpdate(update); err != nil {
		t.Fatalf("failed to broadcast: %v", err)
	}

	// Wait for 2 receipts (from nodes 1 and 2)
	var latencies []time.Duration
	timeout := time.After(5 * time.Second)

	for len(latencies) < 2 {
		select {
		case recvTime := <-received:
			latencies = append(latencies, recvTime.Sub(sendTime))
		case <-timeout:
			t.Fatalf("timeout waiting for propagation, received %d/2", len(latencies))
		}
	}

	// Calculate max latency
	var maxLatency time.Duration
	for _, l := range latencies {
		if l > maxLatency {
			maxLatency = l
		}
	}

	t.Logf("Propagation latencies: %v", latencies)
	t.Logf("Max propagation latency: %v", maxLatency)

	// Verify propagation within 500ms
	if maxLatency > 500*time.Millisecond {
		t.Errorf("propagation latency %v exceeds 500ms threshold", maxLatency)
	}
}
