//go:build integration

// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial
//
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/cluster"
	"github.com/miekg/dns"
)

// =============================================================================
// Cluster Test Configuration
// =============================================================================
const (
	// Base ports for cluster nodes (each node uses base + nodeIndex)
	clusterBaseRaftPort   = 17000
	clusterBaseDNSPort    = 17100
	clusterBaseAPIPort    = 17200
	clusterBaseGossipPort = 17300

	// Timeouts
	clusterFormationTimeout = 30 * time.Second
	leaderElectionTimeout   = 10 * time.Second
	gossipPropagationTime   = 5 * time.Second
)

// testNode represents a cluster node for testing.
type testNode struct {
	NodeID      string
	RaftAddr    string
	DNSAddr     string
	APIAddr     string
	GossipAddr  string
	DataDir     string
	RaftNode    *cluster.RaftNode
	GossipMgr   *cluster.GossipManager
	IsBootstrap bool
}

// =============================================================================
// Test: 3-Node Cluster Formation
// =============================================================================
func TestClusterFormation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cluster integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), clusterFormationTimeout)
	defer cancel()

	// Create temporary directories for each node
	tmpDir, err := os.MkdirTemp("", "opengslb-cluster-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	nodes := make([]*testNode, 3)

	// Initialize nodes
	for i := 0; i < 3; i++ {
		nodes[i] = &testNode{
			NodeID:      fmt.Sprintf("node-%d", i+1),
			RaftAddr:    fmt.Sprintf("127.0.0.1:%d", clusterBaseRaftPort+i),
			DNSAddr:     fmt.Sprintf("127.0.0.1:%d", clusterBaseDNSPort+i),
			APIAddr:     fmt.Sprintf("127.0.0.1:%d", clusterBaseAPIPort+i),
			GossipAddr:  fmt.Sprintf("127.0.0.1:%d", clusterBaseGossipPort+i),
			DataDir:     filepath.Join(tmpDir, fmt.Sprintf("node-%d", i+1)),
			IsBootstrap: i == 0,
		}
		if err := os.MkdirAll(nodes[i].DataDir, 0755); err != nil {
			t.Fatalf("failed to create node data dir: %v", err)
		}
	}

	// Start bootstrap node first
	t.Log("Starting bootstrap node (node-1)...")
	if err := startRaftNode(ctx, nodes[0], nil); err != nil {
		t.Fatalf("failed to start bootstrap node: %v", err)
	}
	defer nodes[0].RaftNode.Shutdown()

	// Wait for bootstrap node to become leader
	t.Log("Waiting for bootstrap node to become leader...")
	if err := waitForLeader(ctx, nodes[0].RaftNode, leaderElectionTimeout); err != nil {
		t.Fatalf("bootstrap node failed to become leader: %v", err)
	}
	t.Logf("Bootstrap node is leader: %v", nodes[0].RaftNode.IsLeader())

	// Start remaining nodes and join cluster
	for i := 1; i < 3; i++ {
		t.Logf("Starting node-%d and joining cluster...", i+1)
		if err := startRaftNode(ctx, nodes[i], []string{nodes[0].RaftAddr}); err != nil {
			t.Fatalf("failed to start node-%d: %v", i+1, err)
		}
		defer nodes[i].RaftNode.Shutdown()

		// Add voter to cluster (must be done from leader)
		if err := nodes[0].RaftNode.AddVoter(nodes[i].NodeID, nodes[i].RaftAddr); err != nil {
			t.Fatalf("failed to add node-%d as voter: %v", i+1, err)
		}
		t.Logf("Node-%d added to cluster", i+1)
	}

	// Verify cluster has 3 members
	time.Sleep(2 * time.Second) // Allow cluster to stabilize
	servers, err := nodes[0].RaftNode.GetConfiguration()
	if err != nil {
		t.Fatalf("failed to get cluster configuration: %v", err)
	}
	if len(servers) != 3 {
		t.Errorf("expected 3 cluster members, got %d", len(servers))
	}
	t.Logf("Cluster formed successfully with %d nodes", len(servers))
	for _, srv := range servers {
		t.Logf(" - %s at %s", srv.ID, srv.Address)
	}

	// Verify exactly one leader
	leaderCount := 0
	for _, node := range nodes {
		if node.RaftNode.IsLeader() {
			leaderCount++
			t.Logf("Leader: %s", node.NodeID)
		}
	}
	if leaderCount != 1 {
		t.Errorf("expected exactly 1 leader, got %d", leaderCount)
	}
}

// ... [All other tests remain unchanged until the helper] ...

// =============================================================================
// Helper Functions
// =============================================================================
func startRaftNode(ctx context.Context, node *testNode, join []string) error {
	cfg := cluster.DefaultConfig()
	cfg.NodeID = node.NodeID
	cfg.NodeName = node.NodeID
	cfg.BindAddress = node.RaftAddr
	cfg.DataDir = node.DataDir
	cfg.Bootstrap = node.IsBootstrap
	cfg.Join = join

	// === FIXED: Fast + valid Raft timeouts for CI ===
	hb := 200 * time.Millisecond
	cfg.HeartbeatTimeout = hb
	cfg.LeaderLeaseTimeout = hb / 2 // Critical: must be <= HeartbeatTimeout
	cfg.ElectionTimeout = hb * 10   // 2s — gives election time in CI
	cfg.CommitTimeout = hb          // Improves commit speed (optional but good)

	raftNode, err := cluster.NewRaftNode(cfg, nil)
	if err != nil {
		return fmt.Errorf("failed to create raft node: %w", err)
	}
	if err := raftNode.Start(ctx); err != nil {
		return fmt.Errorf("failed to start raft node: %w", err)
	}

	node.RaftNode = raftNode
	return nil
}

func waitForLeader(ctx context.Context, raftNode *cluster.RaftNode, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return raftNode.WaitForLeader(ctx)
}
