// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
)

// RaftNode wraps a Raft instance and provides methods for cluster operations.
type RaftNode struct {
	config *Config
	raft   *raft.Raft
	fsm    *FSM
	logger *slog.Logger

	// For transport
	transport *raft.NetworkTransport

	// Lifecycle
	mu      sync.RWMutex
	running bool

	// Leader observers
	observersMu sync.RWMutex
	observers   []LeaderObserver
	stopCh      chan struct{}
}

// NewRaftNode creates a new RaftNode with the given configuration.
func NewRaftNode(config *Config, logger *slog.Logger) (*RaftNode, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	node := &RaftNode{
		config:    config,
		logger:    logger,
		observers: make([]LeaderObserver, 0),
		stopCh:    make(chan struct{}),
	}

	return node, nil
}

// Start initializes and starts the Raft node.
func (n *RaftNode) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.running {
		return nil
	}

	if err := n.setupRaft(); err != nil {
		return err
	}

	n.running = true

	// Start leader observer goroutine
	go n.watchLeadership()

	return nil
}

func (n *RaftNode) setupRaft() error {
	// Create data directory
	if err := os.MkdirAll(n.config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	// Create FSM
	n.fsm = NewFSM(n.logger)

	// Create Raft config
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(n.config.GetNodeID())
	raftConfig.HeartbeatTimeout = n.config.HeartbeatTimeout
	raftConfig.ElectionTimeout = n.config.ElectionTimeout
	raftConfig.SnapshotInterval = n.config.SnapshotInterval
	raftConfig.SnapshotThreshold = n.config.SnapshotThreshold
	raftConfig.Logger = NewHCLogAdapter(n.logger)

	// Create transport
	addr, err := net.ResolveTCPAddr("tcp", n.config.BindAddress)
	if err != nil {
		return fmt.Errorf("failed to resolve bind address: %w", err)
	}

	transport, err := raft.NewTCPTransport(n.config.BindAddress, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}
	n.transport = transport

	// Create stores
	storePath := filepath.Join(n.config.DataDir, "raft.db")
	boltStore, err := raftboltdb.NewBoltStore(storePath)
	if err != nil {
		return fmt.Errorf("failed to create bolt store: %w", err)
	}

	snapshotStore, err := raft.NewFileSnapshotStore(n.config.DataDir, 2, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to create snapshot store: %w", err)
	}

	// Create Raft instance
	r, err := raft.NewRaft(raftConfig, n.fsm, boltStore, boltStore, snapshotStore, transport)
	if err != nil {
		return fmt.Errorf("failed to create raft: %w", err)
	}
	n.raft = r

	// Bootstrap if configured
	if n.config.Bootstrap {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(n.config.GetNodeID()),
					Address: raft.ServerAddress(n.config.GetAdvertiseAddress()),
				},
			},
		}
		future := n.raft.BootstrapCluster(configuration)
		if err := future.Error(); err != nil && !errors.Is(err, raft.ErrCantBootstrap) {
			n.logger.Warn("bootstrap failed", "error", err)
		}
	}

	return nil
}

// watchLeadership monitors for leadership changes and notifies observers.
func (n *RaftNode) watchLeadership() {
	leaderCh := n.raft.LeaderCh()
	for {
		select {
		case <-n.stopCh:
			return
		case isLeader := <-leaderCh:
			n.notifyObservers(isLeader)
		}
	}
}

// notifyObservers notifies all registered leader observers.
func (n *RaftNode) notifyObservers(isLeader bool) {
	n.observersMu.RLock()
	defer n.observersMu.RUnlock()

	for _, observer := range n.observers {
		observer(isLeader)
	}
}

// RegisterLeaderObserver registers a callback for leadership changes.
func (n *RaftNode) RegisterLeaderObserver(observer LeaderObserver) {
	n.observersMu.Lock()
	defer n.observersMu.Unlock()
	n.observers = append(n.observers, observer)
}

// Stop gracefully stops the Raft node.
func (n *RaftNode) Stop(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.running {
		return nil
	}

	close(n.stopCh)
	n.running = false

	if n.raft != nil {
		future := n.raft.Shutdown()
		return future.Error()
	}
	return nil
}

// FSM returns the underlying FSM.
func (n *RaftNode) FSM() *FSM {
	return n.fsm
}

// Config returns the node configuration.
func (n *RaftNode) Config() *Config {
	return n.config
}

// NodeID returns this node's unique identifier.
func (n *RaftNode) NodeID() string {
	return n.config.GetNodeID()
}

// IsLeader returns true if this node is the Raft leader.
func (n *RaftNode) IsLeader() bool {
	if n.raft == nil {
		return false
	}
	return n.raft.State() == raft.Leader
}

// State returns the current cluster state.
func (n *RaftNode) State() State {
	if n.raft == nil {
		return StateShutdown
	}
	switch n.raft.State() {
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
func (n *RaftNode) Leader() (LeaderInfo, error) {
	if n.raft == nil {
		return LeaderInfo{}, ErrNotRunning
	}
	addr, id := n.raft.LeaderWithID()
	if addr == "" {
		return LeaderInfo{}, ErrNoLeader
	}
	return LeaderInfo{
		NodeID:  string(id),
		Address: string(addr),
	}, nil
}

// Nodes returns information about all nodes in the cluster.
func (n *RaftNode) Nodes() []NodeInfo {
	if n.raft == nil {
		return nil
	}

	future := n.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		n.logger.Error("failed to get configuration", "error", err)
		return nil
	}

	servers := future.Configuration().Servers
	nodes := make([]NodeInfo, 0, len(servers))

	for _, server := range servers {
		state := StateFollower
		if string(server.ID) == n.NodeID() {
			state = n.State()
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

// ApplyCommand applies a command to the Raft log.
func (n *RaftNode) ApplyCommand(ctx context.Context, cmdType CommandType, key string, value []byte) error {
	if n.raft == nil {
		return ErrNotRunning
	}
	if n.raft.State() != raft.Leader {
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

	timeout := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	future := n.raft.Apply(data, timeout)
	if err := future.Error(); err != nil {
		return err
	}

	// Check if FSM returned an error
	if resp := future.Response(); resp != nil {
		if err, ok := resp.(error); ok {
			return err
		}
	}

	return nil
}

// Barrier issues a barrier request to ensure all preceding operations are applied.
func (n *RaftNode) Barrier(timeout time.Duration) error {
	if n.raft == nil {
		return ErrNotRunning
	}
	future := n.raft.Barrier(timeout)
	return future.Error()
}

// WaitForLeader blocks until a leader is elected or context is canceled.
func (n *RaftNode) WaitForLeader(ctx context.Context) error {
	if n.raft == nil {
		return ErrNotRunning
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			addr, _ := n.raft.LeaderWithID()
			if addr != "" {
				return nil
			}
		}
	}
}

// AddVoter adds a new voting member to the cluster.
func (n *RaftNode) AddVoter(nodeID, address string) error {
	if n.raft == nil {
		return ErrNotRunning
	}
	future := n.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(address), 0, 10*time.Second)
	return future.Error()
}

// RemoveServer removes a server from the cluster.
func (n *RaftNode) RemoveServer(nodeID string) error {
	if n.raft == nil {
		return ErrNotRunning
	}
	future := n.raft.RemoveServer(raft.ServerID(nodeID), 0, 10*time.Second)
	return future.Error()
}

// Shutdown gracefully shuts down the Raft node.
func (n *RaftNode) Shutdown() error {
	return n.Stop(context.Background())
}

// LeaderCh returns a channel that signals leadership changes.
func (n *RaftNode) LeaderCh() <-chan bool {
	if n.raft == nil {
		ch := make(chan bool)
		close(ch)
		return ch
	}
	return n.raft.LeaderCh()
}

// GetConfiguration returns the current Raft configuration.
func (n *RaftNode) GetConfiguration() ([]raft.Server, error) {
	if n.raft == nil {
		return nil, ErrNotRunning
	}
	future := n.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, err
	}
	return future.Configuration().Servers, nil
}
