// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"log/slog"
	"sync"

	"github.com/hashicorp/memberlist"
)

// GossipDelegate implements the memberlist.Delegate interface.
// It handles incoming gossip messages and state synchronization.
type GossipDelegate struct {
	nodeID   string
	logger   *slog.Logger
	handler  MessageHandler
	metadata []byte

	mu              sync.RWMutex
	localState      []byte
	mergeInProgress bool
}

// MessageHandler processes incoming gossip messages.
type MessageHandler interface {
	// HandleMessage processes a decoded gossip message.
	HandleMessage(msg *GossipMessage) error
}

// NewGossipDelegate creates a new GossipDelegate.
func NewGossipDelegate(nodeID string, handler MessageHandler, logger *slog.Logger) *GossipDelegate {
	if logger == nil {
		logger = slog.Default()
	}
	return &GossipDelegate{
		nodeID:  nodeID,
		logger:  logger,
		handler: handler,
	}
}

// NodeMeta returns metadata about this node.
// This is included in the memberlist node info.
func (d *GossipDelegate) NodeMeta(limit int) []byte {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.metadata) > limit {
		return d.metadata[:limit]
	}
	return d.metadata
}

// SetMetadata sets the node metadata.
func (d *GossipDelegate) SetMetadata(data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.metadata = data
}

// NotifyMsg is called when a user-data message is received.
// This is the primary channel for gossip message propagation.
func (d *GossipDelegate) NotifyMsg(msg []byte) {
	if len(msg) == 0 {
		return
	}

	gossipMsg, err := DecodeGossipMessage(msg)
	if err != nil {
		d.logger.Warn("failed to decode gossip message",
			"error", err,
			"msg_len", len(msg),
		)
		return
	}

	d.logger.Debug("received gossip message",
		"type", gossipMsg.Type.String(),
		"from", gossipMsg.NodeID,
		"timestamp", gossipMsg.Timestamp,
	)

	if d.handler != nil {
		if err := d.handler.HandleMessage(gossipMsg); err != nil {
			d.logger.Warn("failed to handle gossip message",
				"type", gossipMsg.Type.String(),
				"from", gossipMsg.NodeID,
				"error", err,
			)
		}
	}
}

// GetBroadcasts returns any messages that should be broadcast to the cluster.
// This is called periodically by memberlist.
func (d *GossipDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	// We don't use proactive broadcasting; instead we use SendReliable
	// for targeted message delivery.
	return nil
}

// LocalState returns the local node state for push/pull synchronization.
// This is used during state exchange between nodes.
func (d *GossipDelegate) LocalState(join bool) []byte {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.localState
}

// SetLocalState updates the local state that will be shared during sync.
func (d *GossipDelegate) SetLocalState(state []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.localState = state
}

// MergeRemoteState is called when receiving state from another node.
func (d *GossipDelegate) MergeRemoteState(buf []byte, join bool) {
	if len(buf) == 0 {
		return
	}

	d.mu.Lock()
	d.mergeInProgress = true
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.mergeInProgress = false
		d.mu.Unlock()
	}()

	// Treat remote state as a node state update message
	gossipMsg, err := DecodeGossipMessage(buf)
	if err != nil {
		// Try to decode as NodeStateUpdate directly (for backwards compatibility)
		stateUpdate, err := DecodeNodeStateUpdate(buf)
		if err != nil {
			d.logger.Warn("failed to decode remote state",
				"error", err,
				"buf_len", len(buf),
				"join", join,
			)
			return
		}

		d.logger.Debug("merged remote state (direct)",
			"from", stateUpdate.NodeID,
			"health_states", len(stateUpdate.HealthStates),
			"join", join,
		)
		return
	}

	d.logger.Debug("merged remote state",
		"type", gossipMsg.Type.String(),
		"from", gossipMsg.NodeID,
		"join", join,
	)

	if d.handler != nil {
		if err := d.handler.HandleMessage(gossipMsg); err != nil {
			d.logger.Warn("failed to handle merged state",
				"error", err,
			)
		}
	}
}

// GossipEventDelegate implements memberlist.EventDelegate for membership events.
type GossipEventDelegate struct {
	logger   *slog.Logger
	onJoin   func(node *memberlist.Node)
	onLeave  func(node *memberlist.Node)
	onUpdate func(node *memberlist.Node)
}

// NewGossipEventDelegate creates a new GossipEventDelegate.
func NewGossipEventDelegate(logger *slog.Logger) *GossipEventDelegate {
	if logger == nil {
		logger = slog.Default()
	}
	return &GossipEventDelegate{
		logger: logger,
	}
}

// OnJoin sets the callback for node join events.
func (e *GossipEventDelegate) OnJoin(fn func(node *memberlist.Node)) {
	e.onJoin = fn
}

// OnLeave sets the callback for node leave events.
func (e *GossipEventDelegate) OnLeave(fn func(node *memberlist.Node)) {
	e.onLeave = fn
}

// OnUpdate sets the callback for node update events.
func (e *GossipEventDelegate) OnUpdate(fn func(node *memberlist.Node)) {
	e.onUpdate = fn
}

// NotifyJoin is called when a node joins the cluster.
func (e *GossipEventDelegate) NotifyJoin(node *memberlist.Node) {
	e.logger.Info("node joined cluster",
		"name", node.Name,
		"address", node.Address(),
	)
	if e.onJoin != nil {
		e.onJoin(node)
	}
}

// NotifyLeave is called when a node leaves the cluster.
func (e *GossipEventDelegate) NotifyLeave(node *memberlist.Node) {
	e.logger.Info("node left cluster",
		"name", node.Name,
		"address", node.Address(),
	)
	if e.onLeave != nil {
		e.onLeave(node)
	}
}

// NotifyUpdate is called when a node's metadata is updated.
func (e *GossipEventDelegate) NotifyUpdate(node *memberlist.Node) {
	e.logger.Debug("node updated",
		"name", node.Name,
		"address", node.Address(),
	)
	if e.onUpdate != nil {
		e.onUpdate(node)
	}
}
