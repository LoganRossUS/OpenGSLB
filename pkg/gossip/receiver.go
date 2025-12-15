// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package gossip implements the memberlist-based gossip protocol for OpenGSLB.
// ADR-015: Agents gossip state to Overwatch nodes using encrypted memberlist.
package gossip

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// MemberlistReceiver implements overwatch.GossipReceiver using hashicorp/memberlist.
// It listens for incoming gossip messages from agents and forwards them to the handler.
type MemberlistReceiver struct {
	config   ReceiverConfig
	list     *memberlist.Memberlist
	delegate *receiverDelegate
	msgChan  chan overwatch.GossipMessage
	logger   *slog.Logger

	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// ReceiverConfig configures the MemberlistReceiver.
type ReceiverConfig struct {
	// NodeName is the unique name for this Overwatch node in the cluster.
	// Defaults to hostname if empty.
	NodeName string

	// BindAddress is the address to listen for gossip (host:port).
	// Default: "0.0.0.0:7946"
	BindAddress string

	// EncryptionKey is the 32-byte base64-encoded encryption key.
	// REQUIRED - memberlist will refuse to start without encryption.
	EncryptionKey string

	// ProbeInterval is the interval between failure probes.
	// Default: 1s
	ProbeInterval time.Duration

	// ProbeTimeout is the timeout for a single probe.
	// Default: 500ms
	ProbeTimeout time.Duration

	// GossipInterval is the interval between gossip messages.
	// Default: 200ms
	GossipInterval time.Duration

	// Logger for gossip operations.
	Logger *slog.Logger
}

// DefaultReceiverConfig returns sensible defaults for Overwatch receivers.
func DefaultReceiverConfig() ReceiverConfig {
	return ReceiverConfig{
		BindAddress:    "0.0.0.0:7946",
		ProbeInterval:  1 * time.Second,
		ProbeTimeout:   500 * time.Millisecond,
		GossipInterval: 200 * time.Millisecond,
	}
}

// NewMemberlistReceiver creates a new memberlist-based gossip receiver for Overwatch.
func NewMemberlistReceiver(cfg ReceiverConfig) (*MemberlistReceiver, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Validate required encryption key
	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("encryption key is required (ADR-015 mandates encryption)")
	}

	// Decode encryption key
	keyBytes, err := base64.StdEncoding.DecodeString(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("invalid encryption key encoding: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(keyBytes))
	}

	// Apply defaults
	if cfg.BindAddress == "" {
		cfg.BindAddress = "0.0.0.0:7946"
	}
	if cfg.ProbeInterval == 0 {
		cfg.ProbeInterval = 1 * time.Second
	}
	if cfg.ProbeTimeout == 0 {
		cfg.ProbeTimeout = 500 * time.Millisecond
	}
	if cfg.GossipInterval == 0 {
		cfg.GossipInterval = 200 * time.Millisecond
	}

	msgChan := make(chan overwatch.GossipMessage, 1000) // Buffer for incoming messages

	r := &MemberlistReceiver{
		config:  cfg,
		msgChan: msgChan,
		logger:  logger,
	}

	// Create delegate
	r.delegate = &receiverDelegate{
		msgChan: msgChan,
		logger:  logger,
	}

	return r, nil
}

// Start begins receiving gossip messages.
func (r *MemberlistReceiver) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("receiver already running")
	}

	// Parse bind address
	host, portStr, err := net.SplitHostPort(r.config.BindAddress)
	if err != nil {
		return fmt.Errorf("invalid bind address: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	// Decode encryption key
	keyBytes, _ := base64.StdEncoding.DecodeString(r.config.EncryptionKey)

	// Create memberlist configuration
	mlConfig := memberlist.DefaultLANConfig()
	if r.config.NodeName != "" {
		mlConfig.Name = r.config.NodeName
	}
	mlConfig.BindAddr = host
	mlConfig.BindPort = port
	mlConfig.AdvertisePort = port

	// Set encryption key
	mlConfig.SecretKey = keyBytes

	// Set timing parameters
	mlConfig.ProbeInterval = r.config.ProbeInterval
	mlConfig.ProbeTimeout = r.config.ProbeTimeout
	mlConfig.GossipInterval = r.config.GossipInterval

	// Set delegates
	mlConfig.Delegate = r.delegate
	mlConfig.Events = r.delegate

	// Disable memberlist logging (we use our own)
	mlConfig.LogOutput = &discardWriter{}

	// Create memberlist
	list, err := memberlist.Create(mlConfig)
	if err != nil {
		return fmt.Errorf("failed to create memberlist: %w", err)
	}

	r.list = list
	r.running = true
	r.ctx, r.cancel = context.WithCancel(ctx)

	r.logger.Info("gossip receiver started",
		"bind_address", r.config.BindAddress,
		"node_name", mlConfig.Name,
	)

	return nil
}

// Stop halts the receiver.
func (r *MemberlistReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	r.running = false
	if r.cancel != nil {
		r.cancel()
	}

	if r.list != nil {
		// Leave the cluster gracefully
		if err := r.list.Leave(5 * time.Second); err != nil {
			r.logger.Warn("error leaving memberlist cluster", "error", err)
		}
		if err := r.list.Shutdown(); err != nil {
			r.logger.Warn("error shutting down memberlist", "error", err)
		}
	}

	close(r.msgChan)

	r.logger.Info("gossip receiver stopped")
	return nil
}

// MessageChan returns the channel for received messages.
func (r *MemberlistReceiver) MessageChan() <-chan overwatch.GossipMessage {
	return r.msgChan
}

// Members returns the current cluster members.
func (r *MemberlistReceiver) Members() []*memberlist.Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.list == nil {
		return nil
	}
	return r.list.Members()
}

// NumMembers returns the number of cluster members.
func (r *MemberlistReceiver) NumMembers() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.list == nil {
		return 0
	}
	return r.list.NumMembers()
}

// LocalNode returns the local node.
func (r *MemberlistReceiver) LocalNode() *memberlist.Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.list == nil {
		return nil
	}
	return r.list.LocalNode()
}

// receiverDelegate implements memberlist.Delegate and memberlist.EventDelegate.
type receiverDelegate struct {
	msgChan chan<- overwatch.GossipMessage
	logger  *slog.Logger

	mu sync.RWMutex
}

// NodeMeta returns metadata for this node (implements memberlist.Delegate).
func (d *receiverDelegate) NodeMeta(limit int) []byte {
	d.mu.RLock()
	defer d.mu.RUnlock()

	meta := NodeMeta{
		Role:      RoleOverwatch,
		Version:   "1.0.0",
		Timestamp: time.Now(),
	}

	data, _ := json.Marshal(meta)
	if len(data) > limit {
		return nil
	}
	return data
}

// NotifyMsg handles incoming messages (implements memberlist.Delegate).
func (d *receiverDelegate) NotifyMsg(msg []byte) {
	if len(msg) == 0 {
		return
	}

	var gossipMsg overwatch.GossipMessage
	if err := json.Unmarshal(msg, &gossipMsg); err != nil {
		d.logger.Warn("failed to unmarshal gossip message",
			"error", err,
			"size", len(msg),
		)
		return
	}

	// Send to message channel (non-blocking with buffer)
	select {
	case d.msgChan <- gossipMsg:
		d.logger.Debug("received gossip message",
			"type", gossipMsg.Type,
			"agent_id", gossipMsg.AgentID,
			"region", gossipMsg.Region,
		)
	default:
		d.logger.Warn("message channel full, dropping message",
			"type", gossipMsg.Type,
			"agent_id", gossipMsg.AgentID,
		)
	}
}

// GetBroadcasts returns broadcasts to send (implements memberlist.Delegate).
func (d *receiverDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	// Overwatch doesn't broadcast - it only receives
	return nil
}

// LocalState returns local state for anti-entropy (implements memberlist.Delegate).
func (d *receiverDelegate) LocalState(join bool) []byte {
	// No local state to synchronize
	return nil
}

// MergeRemoteState merges remote state (implements memberlist.Delegate).
func (d *receiverDelegate) MergeRemoteState(buf []byte, join bool) {
	// No state merging needed
}

// NotifyJoin is called when a node joins (implements memberlist.EventDelegate).
func (d *receiverDelegate) NotifyJoin(node *memberlist.Node) {
	var meta NodeMeta
	if err := json.Unmarshal(node.Meta, &meta); err == nil {
		d.logger.Info("node joined gossip cluster",
			"name", node.Name,
			"address", node.Addr.String(),
			"role", meta.Role,
		)
	} else {
		d.logger.Info("node joined gossip cluster",
			"name", node.Name,
			"address", node.Addr.String(),
		)
	}
}

// NotifyLeave is called when a node leaves (implements memberlist.EventDelegate).
func (d *receiverDelegate) NotifyLeave(node *memberlist.Node) {
	d.logger.Info("node left gossip cluster",
		"name", node.Name,
		"address", node.Addr.String(),
	)
}

// NotifyUpdate is called when a node updates (implements memberlist.EventDelegate).
func (d *receiverDelegate) NotifyUpdate(node *memberlist.Node) {
	d.logger.Debug("node updated in gossip cluster",
		"name", node.Name,
		"address", node.Addr.String(),
	)
}

// discardWriter discards all writes (for suppressing memberlist logs).
type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
