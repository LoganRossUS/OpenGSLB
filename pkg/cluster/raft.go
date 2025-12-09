// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
)

// RaftNode implements the Manager interface using hashicorp/raft.
type RaftNode struct {
	config *Config
	logger *slog.Logger

	raft      *raft.Raft
	fsm       *FSM
	transport *raft.NetworkTransport
	logStore  raft.LogStore
	stable    raft.StableStore
	snaps     raft.SnapshotStore

	mu        sync.RWMutex
	observers []LeaderObserver
	leaderCh  <-chan bool
}

// NewRaftNode creates a new RaftNode with the given configuration.
func NewRaftNode(cfg *Config, logger *slog.Logger) (*RaftNode, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &RaftNode{
		config:    cfg,
		logger:    logger,
		observers: make([]LeaderObserver, 0),
	}, nil
}

// Start initializes and starts the Raft node.
func (r *RaftNode) Start(ctx context.Context) error {
	r.logger.Info("starting Raft node",
		"node_id", r.config.GetNodeID(),
		"bind_address", r.config.BindAddress,
		"data_dir", r.config.DataDir,
		"bootstrap", r.config.Bootstrap,
	)

	// Create data directory
	if err := os.MkdirAll(r.config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Set up Raft configuration
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(r.config.GetNodeID())
	raftConfig.HeartbeatTimeout = r.config.HeartbeatTimeout
	raftConfig.ElectionTimeout = r.config.ElectionTimeout
	raftConfig.SnapshotInterval = r.config.SnapshotInterval
	raftConfig.SnapshotThreshold = r.config.SnapshotThreshold
	raftConfig.Logger = NewHCLogAdapter(r.logger.With("component", "raft"))

	// Create FSM
	r.fsm = NewFSM(r.logger.With("component", "fsm"))

	// Set up transport
	addr, err := net.ResolveTCPAddr("tcp", r.config.BindAddress)
	if err != nil {
		return fmt.Errorf("failed to resolve bind address: %w", err)
	}

	transport, err := raft.NewTCPTransport(
		r.config.BindAddress,
		addr,
		3,              // maxPool
		10*time.Second, // timeout
		os.Stderr,      // logOutput
	)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}
	r.transport = transport

	// Set up stores
	boltPath := filepath.Join(r.config.DataDir, "raft.db")
	boltStore, err := raftboltdb.NewBoltStore(boltPath)
	if err != nil {
		r.cleanup()
		return fmt.Errorf("failed to create bolt store: %w", err)
	}
	r.logStore = boltStore
	r.stable = boltStore

	// Snapshot store
	snapshotStore, err := raft.NewFileSnapshotStore(r.config.DataDir, 2, os.Stderr)
	if err != nil {
		r.cleanup()
		return fmt.Errorf("failed to create snapshot store: %w", err)
	}
	r.snaps = snapshotStore

	// Create Raft instance
	ra, err := raft.NewRaft(raftConfig, r.fsm, r.logStore, r.stable, r.snaps, r.transport)
	if err != nil {
		r.cleanup()
		return fmt.Errorf("failed to create raft: %w", err)
	}
	r.raft = ra

	// Store leader channel for observer notifications
	r.leaderCh = ra.LeaderCh()

	// Start leader observer goroutine
	go r.watchLeadership(ctx)

	// Bootstrap if requested and this is a fresh cluster
	if r.config.Bootstrap {
		hasState, err := raft.HasExistingState(r.logStore, r.stable, r.snaps)
		if err != nil {
			r.cleanup()
			return fmt.Errorf("failed to check existing state: %w", err)
		}

		if !hasState {
			r.logger.Info("bootstrapping new cluster")
			configuration := raft.Configuration{
				Servers: []raft.Server{
					{
						ID:      raft.ServerID(r.config.GetNodeID()),
						Address: raft.ServerAddress(r.config.GetAdvertiseAddress()),
					},
				},
			}
			future := ra.BootstrapCluster(configuration)
			if err := future.Error(); err != nil {
				r.cleanup()
				return fmt.Errorf("failed to bootstrap cluster: %w", err)
			}
			r.logger.Info("cluster bootstrapped successfully")
		} else {
			r.logger.Info("existing state found, skipping bootstrap")
		}
	}

	// If join addresses provided, attempt to join
	if len(r.config.Join) > 0 && !r.config.Bootstrap {
		if err := r.joinCluster(ctx); err != nil {
			r.logger.Warn("failed to join cluster on startup, will retry",
				"error", err,
			)
			// Don't fail startup - we'll retry joining
		}
	}

	r.logger.Info("Raft node started",
		"state", r.State().String(),
	)

	return nil
}

// joinCluster attempts to join an existing cluster via the join API.
func (r *RaftNode) joinCluster(ctx context.Context) error {
	req := JoinRequest{
		NodeID:      r.config.GetNodeID(),
		NodeName:    r.config.NodeName,
		RaftAddress: r.config.GetAdvertiseAddress(),
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal join request: %w", err)
	}

	// Try each join address
	for _, addr := range r.config.Join {
		r.logger.Info("attempting to join cluster", "target", addr)

		if err := r.tryJoin(ctx, addr, reqBody); err != nil {
			r.logger.Warn("join attempt failed",
				"target", addr,
				"error", err,
			)
			continue
		}

		r.logger.Info("joined cluster successfully", "via", addr)
		return nil
	}

	return fmt.Errorf("failed to join any cluster node")
}

// tryJoin attempts to join via a single address.
func (r *RaftNode) tryJoin(ctx context.Context, addr string, reqBody []byte) error {
	// Import needed: "bytes", "net/http", "io"
	url := fmt.Sprintf("http://%s/api/v1/cluster/join", addr)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var joinResp JoinResponse
	if err := json.Unmarshal(body, &joinResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Handle redirect to leader
	if resp.StatusCode == http.StatusTemporaryRedirect && joinResp.LeaderAddress != "" {
		r.logger.Info("redirected to leader", "leader", joinResp.LeaderAddress)
		return r.tryJoin(ctx, joinResp.LeaderAddress, reqBody)
	}

	if !joinResp.Success {
		return fmt.Errorf("join failed: %s", joinResp.Message)
	}

	return nil
}

// watchLeadership monitors the leader channel and notifies observers.
func (r *RaftNode) watchLeadership(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case isLeader, ok := <-r.leaderCh:
			if !ok {
				return
			}
			r.logger.Info("leadership changed", "is_leader", isLeader)
			r.notifyObservers(isLeader)
		}
	}
}

// notifyObservers calls all registered leader observers.
func (r *RaftNode) notifyObservers(isLeader bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, observer := range r.observers {
		go observer(isLeader)
	}
}

// cleanup releases resources on startup failure.
func (r *RaftNode) cleanup() {
	if r.transport != nil {
		r.transport.Close()
	}
	if closer, ok := r.logStore.(interface{ Close() error }); ok {
		closer.Close()
	}
}

// Stop gracefully shuts down the Raft node.
func (r *RaftNode) Stop(ctx context.Context) error {
	if r.raft == nil {
		return nil
	}

	r.logger.Info("stopping Raft node")

	// If we're the leader, try to transfer leadership
	if r.IsLeader() {
		r.logger.Info("transferring leadership before shutdown")
		future := r.raft.LeadershipTransfer()
		if err := future.Error(); err != nil {
			r.logger.Warn("leadership transfer failed", "error", err)
		}
	}

	// Shutdown Raft
	future := r.raft.Shutdown()
	if err := future.Error(); err != nil {
		return fmt.Errorf("raft shutdown failed: %w", err)
	}

	r.cleanup()

	r.logger.Info("Raft node stopped")
	return nil
}

// IsLeader returns true if this node is the current leader.
func (r *RaftNode) IsLeader() bool {
	if r.raft == nil {
		return false
	}
	return r.raft.State() == raft.Leader
}

// State returns the current Raft state.
func (r *RaftNode) State() State {
	if r.raft == nil {
		return StateShutdown
	}
	switch r.raft.State() {
	case raft.Follower:
		return StateFollower
	case raft.Candidate:
		return StateCandidate
	case raft.Leader:
		return StateLeader
	case raft.Shutdown:
		return StateShutdown
	default:
		return StateFollower
	}
}

// Leader returns information about the current leader.
func (r *RaftNode) Leader() (LeaderInfo, error) {
	if r.raft == nil {
		return LeaderInfo{}, ErrNotRunning
	}

	addr, id := r.raft.LeaderWithID()
	if addr == "" {
		return LeaderInfo{}, ErrNoLeader
	}

	return LeaderInfo{
		NodeID:  string(id),
		Address: string(addr),
	}, nil
}

// NodeID returns this node's identifier.
func (r *RaftNode) NodeID() string {
	return r.config.GetNodeID()
}

// Nodes returns information about all known cluster nodes.
func (r *RaftNode) Nodes() []NodeInfo {
	if r.raft == nil {
		return nil
	}

	future := r.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		r.logger.Error("failed to get configuration", "error", err)
		return nil
	}

	servers := future.Configuration().Servers
	nodes := make([]NodeInfo, 0, len(servers))

	leaderAddr, _ := r.raft.LeaderWithID()

	for _, server := range servers {
		state := StateFollower
		if string(server.Address) == string(leaderAddr) {
			state = StateLeader
		}
		if string(server.ID) == r.config.GetNodeID() {
			state = r.State()
		}

		nodes = append(nodes, NodeInfo{
			ID:      string(server.ID),
			Address: string(server.Address),
			State:   state,
			IsVoter: server.Suffrage == raft.Voter,
		})
	}

	return nodes
}

// AddVoter adds a new voting member to the cluster.
func (r *RaftNode) AddVoter(id, address string) error {
	if r.raft == nil {
		return ErrNotRunning
	}
	if !r.IsLeader() {
		return ErrNotLeader
	}

	r.logger.Info("adding voter to cluster",
		"id", id,
		"address", address,
	)

	future := r.raft.AddVoter(
		raft.ServerID(id),
		raft.ServerAddress(address),
		0, // prevIndex - use 0 for new additions
		30*time.Second,
	)

	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to add voter: %w", err)
	}

	r.logger.Info("voter added successfully", "id", id)
	return nil
}

// FSM returns the underlying FSM.
func (r *RaftNode) FSM() *FSM {
	return r.fsm
}

// ApplyCommand applies a command to the FSM via Raft.
func (r *RaftNode) ApplyCommand(ctx context.Context, cmdType CommandType, key string, value []byte) error {
	if !r.IsLeader() {
		return ErrNotLeader
	}

	cmd := Command{
		Type:  cmdType,
		Key:   key,
		Value: value,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	future := r.raft.Apply(data, 10*time.Second) // TODO: Make timeout configurable?
	if err := future.Error(); err != nil {
		return err
	}
	// Check response from Apply
	resp := future.Response()
	if resp != nil {
		if err, ok := resp.(error); ok {
			return err
		}
	}
	return nil
}

// RemoveServer removes a server from the cluster.
func (r *RaftNode) RemoveServer(id string) error {
	if r.raft == nil {
		return ErrNotRunning
	}
	if !r.IsLeader() {
		return ErrNotLeader
	}

	r.logger.Info("removing server from cluster", "id", id)

	future := r.raft.RemoveServer(
		raft.ServerID(id),
		0, // prevIndex
		30*time.Second,
	)

	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to remove server: %w", err)
	}

	r.logger.Info("server removed successfully", "id", id)
	return nil
}

// RegisterLeaderObserver registers a callback for leadership changes.
func (r *RaftNode) RegisterLeaderObserver(observer LeaderObserver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observers = append(r.observers, observer)
}

// WaitForLeader blocks until a leader is elected or context is canceled.
func (r *RaftNode) WaitForLeader(ctx context.Context) error {
	if r.raft == nil {
		return ErrNotRunning
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			addr, _ := r.raft.LeaderWithID()
			if addr != "" {
				return nil
			}
		}
	}
}

// Barrier ensures all preceding operations are applied.
func (r *RaftNode) Barrier(timeout time.Duration) error {
	if r.raft == nil {
		return ErrNotRunning
	}
	if !r.IsLeader() {
		return ErrNotLeader
	}

	future := r.raft.Barrier(timeout)
	return future.Error()
}
