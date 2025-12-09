// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package cluster provides distributed coordination using Raft consensus.
package cluster

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/raft"
)

// Common errors.
var (
	ErrNotLeader     = errors.New("not the cluster leader")
	ErrBootstrapJoin = errors.New("cannot specify both bootstrap and join")
	ErrNotFound      = errors.New("key not found")
	ErrTimeout       = errors.New("operation timed out")
)

// Manager defines the interface for cluster coordination operations.
type Manager interface {
	// Start initializes and starts the cluster manager.
	Start(ctx context.Context) error

	// Shutdown gracefully stops the cluster manager.
	Shutdown(ctx context.Context) error

	// IsLeader returns true if this node is the cluster leader.
	IsLeader() bool

	// NodeID returns this node's unique identifier.
	NodeID() string

	// State returns the current Raft state as a string.
	State() string

	// Leader returns the address of the current leader.
	Leader() raft.ServerAddress

	// LeaderWithID returns both the address and ID of the current leader.
	LeaderWithID() (raft.ServerAddress, raft.ServerID)

	// AddVoter adds a new voting member to the cluster.
	// Only the leader can add voters.
	AddVoter(nodeID string, address string) error

	// RemoveServer removes a server from the cluster.
	// Only the leader can remove servers.
	RemoveServer(nodeID string) error

	// GetConfiguration returns the current Raft configuration.
	GetConfiguration() []raft.Server

	// LeaderCh returns a channel that signals leader state changes.
	// Sends true when becoming leader, false when losing leadership.
	LeaderCh() <-chan bool

	// Barrier ensures all preceding operations are applied.
	// Useful for read-after-write consistency.
	Barrier(timeout time.Duration) error

	// WaitForLeader blocks until a leader is elected or timeout.
	WaitForLeader(timeout time.Duration) error
}

// Config holds the configuration for a cluster Manager.
type Config struct {
	// NodeID is a unique identifier for this node.
	// If empty, defaults to NodeName.
	NodeID string

	// NodeName is a human-readable name for this node.
	NodeName string

	// BindAddress is the address for Raft communication (ip:port).
	BindAddress string

	// AdvertiseAddress is the address advertised to other nodes.
	// Defaults to BindAddress if empty.
	AdvertiseAddress string

	// APIAddress is the address of this node's API server.
	// Used by join client to know where to send join requests.
	// If empty, defaults to the metrics/API listen address.
	APIAddress string

	// DataDir is the directory for Raft state and logs.
	DataDir string

	// Bootstrap indicates this node should initialize a new cluster.
	Bootstrap bool

	// Join specifies API addresses of existing cluster nodes to join.
	// Format: "host:port" or "http://host:port"
	Join []string

	// HeartbeatTimeout is the time between heartbeats.
	HeartbeatTimeout time.Duration

	// ElectionTimeout is the time before a new election starts.
	ElectionTimeout time.Duration

	// SnapshotInterval is the minimum time between snapshots.
	SnapshotInterval time.Duration

	// SnapshotThreshold is the number of log entries before snapshot.
	SnapshotThreshold uint64
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		HeartbeatTimeout:  1000 * time.Millisecond,
		ElectionTimeout:   1000 * time.Millisecond,
		SnapshotInterval:  120 * time.Second,
		SnapshotThreshold: 8192,
	}
}

// GetNodeID returns NodeID if set, otherwise falls back to NodeName.
func (c *Config) GetNodeID() string {
	if c.NodeID != "" {
		return c.NodeID
	}
	return c.NodeName
}

// GetAdvertiseAddress returns AdvertiseAddress if set, otherwise BindAddress.
func (c *Config) GetAdvertiseAddress() string {
	if c.AdvertiseAddress != "" {
		return c.AdvertiseAddress
	}
	return c.BindAddress
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.NodeID == "" && c.NodeName == "" {
		return errors.New("either NodeID or NodeName must be set")
	}
	if c.BindAddress == "" {
		return errors.New("BindAddress is required")
	}
	if c.DataDir == "" {
		return errors.New("DataDir is required")
	}
	if c.Bootstrap && len(c.Join) > 0 {
		return ErrBootstrapJoin
	}
	return nil
}
