// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

// Common gossip errors.
var (
	ErrGossipNotRunning = errors.New("gossip is not running")
	ErrNoMembers        = errors.New("no cluster members available")
)

// GossipConfig holds configuration for the gossip protocol.
type GossipConfig struct {
	// NodeID is the unique identifier for this node.
	NodeID string

	// NodeName is the human-readable name for this node.
	NodeName string

	// BindAddr is the address to bind for gossip (IP only).
	BindAddr string

	// BindPort is the port to bind for gossip.
	BindPort int

	// AdvertiseAddr is the address to advertise to other nodes.
	AdvertiseAddr string

	// AdvertisePort is the port to advertise to other nodes.
	AdvertisePort int

	// EncryptionKey is an optional 32-byte encryption key (base64 encoded).
	EncryptionKey string

	// Seeds are the addresses of nodes to join on startup.
	Seeds []string

	// ProbeInterval is the interval between failure probes.
	ProbeInterval time.Duration

	// ProbeTimeout is the timeout for a probe.
	ProbeTimeout time.Duration

	// GossipInterval is the interval between gossip messages.
	GossipInterval time.Duration

	// PushPullInterval is the interval for full state sync.
	PushPullInterval time.Duration
}

// DefaultGossipConfig returns sensible defaults for gossip configuration.
func DefaultGossipConfig() *GossipConfig {
	return &GossipConfig{
		BindPort:         7946,
		ProbeInterval:    1 * time.Second,
		ProbeTimeout:     500 * time.Millisecond,
		GossipInterval:   200 * time.Millisecond,
		PushPullInterval: 30 * time.Second,
	}
}

// GossipManager manages the gossip protocol for cluster communication.
type GossipManager struct {
	config   *GossipConfig
	logger   *slog.Logger
	list     *memberlist.Memberlist
	delegate *GossipDelegate
	events   *GossipEventDelegate

	mu        sync.RWMutex
	running   bool
	handlers  []MessageHandler
	startTime time.Time

	// Callbacks
	onHealthUpdate func(*HealthUpdate, string)
	onPredictive   func(*PredictiveSignal, string)
	onOverride     func(*OverrideCommand, string)
}

// NewGossipManager creates a new GossipManager.
func NewGossipManager(cfg *GossipConfig, logger *slog.Logger) (*GossipManager, error) {
	if cfg == nil {
		cfg = DefaultGossipConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.NodeID == "" {
		return nil, errors.New("NodeID is required")
	}

	gm := &GossipManager{
		config:   cfg,
		logger:   logger,
		handlers: make([]MessageHandler, 0),
	}

	return gm, nil
}

// Start initializes and starts the gossip protocol.
func (g *GossipManager) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.running {
		return errors.New("gossip manager already running")
	}

	g.logger.Info("starting gossip manager",
		"node_id", g.config.NodeID,
		"bind_addr", g.config.BindAddr,
		"bind_port", g.config.BindPort,
	)

	// Create memberlist configuration
	mlConfig := memberlist.DefaultLANConfig()
	mlConfig.Name = g.config.NodeID

	// Bind configuration
	if g.config.BindAddr != "" {
		mlConfig.BindAddr = g.config.BindAddr
	}
	mlConfig.BindPort = g.config.BindPort

	// Advertise configuration
	if g.config.AdvertiseAddr != "" {
		mlConfig.AdvertiseAddr = g.config.AdvertiseAddr
	}
	if g.config.AdvertisePort > 0 {
		mlConfig.AdvertisePort = g.config.AdvertisePort
	}

	// Timing configuration
	if g.config.ProbeInterval > 0 {
		mlConfig.ProbeInterval = g.config.ProbeInterval
	}
	if g.config.ProbeTimeout > 0 {
		mlConfig.ProbeTimeout = g.config.ProbeTimeout
	}
	if g.config.GossipInterval > 0 {
		mlConfig.GossipInterval = g.config.GossipInterval
	}
	if g.config.PushPullInterval > 0 {
		mlConfig.PushPullInterval = g.config.PushPullInterval
	}

	// Encryption
	if g.config.EncryptionKey != "" {
		key, err := base64.StdEncoding.DecodeString(g.config.EncryptionKey)
		if err != nil {
			return fmt.Errorf("failed to decode encryption key: %w", err)
		}
		if len(key) != 32 {
			return fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
		}
		mlConfig.SecretKey = key
	}

	// Set up delegates
	g.delegate = NewGossipDelegate(g.config.NodeID, g, g.logger.With("component", "gossip-delegate"))
	g.events = NewGossipEventDelegate(g.logger.With("component", "gossip-events"))

	mlConfig.Delegate = g.delegate
	mlConfig.Events = g.events

	// Use our logger adapter
	mlConfig.Logger = nil // Disable default logging
	mlConfig.LogOutput = &slogWriter{logger: g.logger.With("component", "memberlist")}

	// Create memberlist
	list, err := memberlist.Create(mlConfig)
	if err != nil {
		return fmt.Errorf("failed to create memberlist: %w", err)
	}
	g.list = list
	g.running = true
	g.startTime = time.Now()

	// Join seed nodes if provided
	if len(g.config.Seeds) > 0 {
		if err := g.joinSeeds(ctx); err != nil {
			g.logger.Warn("failed to join seed nodes on startup",
				"error", err,
				"seeds", g.config.Seeds,
			)
			// Don't fail startup - we'll retry later
		}
	}

	g.logger.Info("gossip manager started",
		"local_node", g.list.LocalNode().Name,
		"members", g.list.NumMembers(),
	)

	return nil
}

// joinSeeds attempts to join the cluster using seed addresses.
func (g *GossipManager) joinSeeds(ctx context.Context) error {
	seeds := make([]string, 0, len(g.config.Seeds))
	for _, seed := range g.config.Seeds {
		// Skip self
		if seed == fmt.Sprintf("%s:%d", g.config.BindAddr, g.config.BindPort) {
			continue
		}
		if seed == fmt.Sprintf("%s:%d", g.config.AdvertiseAddr, g.config.AdvertisePort) {
			continue
		}
		seeds = append(seeds, seed)
	}

	if len(seeds) == 0 {
		return nil
	}

	n, err := g.list.Join(seeds)
	if err != nil {
		return fmt.Errorf("failed to join seeds: %w", err)
	}

	g.logger.Info("joined cluster via seeds",
		"contacted", n,
		"seeds", seeds,
	)

	return nil
}

// Stop gracefully shuts down the gossip protocol.
func (g *GossipManager) Stop(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.running {
		return nil
	}

	g.logger.Info("stopping gossip manager")

	// Leave the cluster gracefully
	if err := g.list.Leave(5 * time.Second); err != nil {
		g.logger.Warn("error leaving cluster", "error", err)
	}

	// Shutdown memberlist
	if err := g.list.Shutdown(); err != nil {
		g.logger.Warn("error shutting down memberlist", "error", err)
	}

	g.running = false
	g.logger.Info("gossip manager stopped")
	return nil
}

// HandleMessage implements MessageHandler interface.
// Routes messages to appropriate handlers.
func (g *GossipManager) HandleMessage(msg *GossipMessage) error {
	switch msg.Type {
	case MsgHealthUpdate:
		update, err := DecodeHealthUpdate(msg.Payload)
		if err != nil {
			return fmt.Errorf("failed to decode health update: %w", err)
		}
		g.mu.RLock()
		handler := g.onHealthUpdate
		g.mu.RUnlock()
		if handler != nil {
			handler(update, msg.NodeID)
		}
		return nil

	case MsgPredictive:
		signal, err := DecodePredictiveSignal(msg.Payload)
		if err != nil {
			return fmt.Errorf("failed to decode predictive signal: %w", err)
		}
		g.mu.RLock()
		handler := g.onPredictive
		g.mu.RUnlock()
		if handler != nil {
			handler(signal, msg.NodeID)
		}
		return nil

	case MsgOverride:
		override, err := DecodeOverrideCommand(msg.Payload)
		if err != nil {
			return fmt.Errorf("failed to decode override command: %w", err)
		}
		g.mu.RLock()
		handler := g.onOverride
		g.mu.RUnlock()
		if handler != nil {
			handler(override, msg.NodeID)
		}
		return nil

	case MsgNodeState:
		// Node state updates are handled during merge
		return nil

	default:
		return fmt.Errorf("unknown message type: %d", msg.Type)
	}
}

// OnHealthUpdate sets the callback for health update messages.
func (g *GossipManager) OnHealthUpdate(fn func(*HealthUpdate, string)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onHealthUpdate = fn
}

// OnPredictive sets the callback for predictive signal messages.
func (g *GossipManager) OnPredictive(fn func(*PredictiveSignal, string)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onPredictive = fn
}

// OnOverride sets the callback for override command messages.
func (g *GossipManager) OnOverride(fn func(*OverrideCommand, string)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onOverride = fn
}

// OnNodeJoin sets the callback for node join events.
func (g *GossipManager) OnNodeJoin(fn func(node *memberlist.Node)) {
	if g.events != nil {
		g.events.OnJoin(fn)
	}
}

// OnNodeLeave sets the callback for node leave events.
func (g *GossipManager) OnNodeLeave(fn func(node *memberlist.Node)) {
	if g.events != nil {
		g.events.OnLeave(fn)
	}
}

// BroadcastHealthUpdate sends a health update to all cluster members.
func (g *GossipManager) BroadcastHealthUpdate(update *HealthUpdate) error {
	g.mu.RLock()
	if !g.running {
		g.mu.RUnlock()
		return ErrGossipNotRunning
	}
	g.mu.RUnlock()

	msg, err := NewHealthUpdateMessage(g.config.NodeID, update)
	if err != nil {
		return fmt.Errorf("failed to create health update message: %w", err)
	}

	return g.broadcast(msg)
}

// BroadcastPredictive sends a predictive signal to all cluster members.
func (g *GossipManager) BroadcastPredictive(signal *PredictiveSignal) error {
	g.mu.RLock()
	if !g.running {
		g.mu.RUnlock()
		return ErrGossipNotRunning
	}
	g.mu.RUnlock()

	msg, err := NewPredictiveMessage(g.config.NodeID, signal)
	if err != nil {
		return fmt.Errorf("failed to create predictive message: %w", err)
	}

	return g.broadcast(msg)
}

// BroadcastOverride sends an override command to all cluster members.
func (g *GossipManager) BroadcastOverride(override *OverrideCommand) error {
	g.mu.RLock()
	if !g.running {
		g.mu.RUnlock()
		return ErrGossipNotRunning
	}
	g.mu.RUnlock()

	msg, err := NewOverrideMessage(g.config.NodeID, override)
	if err != nil {
		return fmt.Errorf("failed to create override message: %w", err)
	}

	return g.broadcast(msg)
}

// broadcast sends a message to all cluster members.
func (g *GossipManager) broadcast(msg *GossipMessage) error {
	data, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	members := g.list.Members()
	localName := g.list.LocalNode().Name

	var lastErr error
	sentCount := 0

	for _, member := range members {
		if member.Name == localName {
			continue // Skip self
		}

		if err := g.list.SendReliable(member, data); err != nil {
			g.logger.Warn("failed to send message to member",
				"member", member.Name,
				"error", err,
			)
			lastErr = err
			continue
		}
		sentCount++
	}

	g.logger.Debug("broadcast message",
		"type", msg.Type.String(),
		"sent_to", sentCount,
		"total_members", len(members)-1, // Exclude self
	)

	if sentCount == 0 && len(members) > 1 {
		return fmt.Errorf("failed to send to any member: %w", lastErr)
	}

	return nil
}

// SendToNode sends a message to a specific node.
func (g *GossipManager) SendToNode(nodeID string, msg *GossipMessage) error {
	g.mu.RLock()
	if !g.running {
		g.mu.RUnlock()
		return ErrGossipNotRunning
	}
	g.mu.RUnlock()

	data, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	// Find the target node
	for _, member := range g.list.Members() {
		if member.Name == nodeID {
			return g.list.SendReliable(member, data)
		}
	}

	return fmt.Errorf("node not found: %s", nodeID)
}

// Members returns information about all cluster members.
func (g *GossipManager) Members() []GossipMember {
	g.mu.RLock()
	if !g.running {
		g.mu.RUnlock()
		return nil
	}
	g.mu.RUnlock()

	members := g.list.Members()
	result := make([]GossipMember, 0, len(members))

	for _, m := range members {
		result = append(result, GossipMember{
			Name:    m.Name,
			Address: m.Address(),
			Port:    m.Port,
			State:   memberStateToString(m.State),
		})
	}

	return result
}

// NumMembers returns the number of cluster members.
func (g *GossipManager) NumMembers() int {
	g.mu.RLock()
	if !g.running {
		g.mu.RUnlock()
		return 0
	}
	g.mu.RUnlock()

	return g.list.NumMembers()
}

// LocalNode returns information about the local node.
func (g *GossipManager) LocalNode() *GossipMember {
	g.mu.RLock()
	if !g.running {
		g.mu.RUnlock()
		return nil
	}
	g.mu.RUnlock()

	node := g.list.LocalNode()
	return &GossipMember{
		Name:    node.Name,
		Address: node.Address(),
		Port:    node.Port,
		State:   memberStateToString(node.State),
	}
}

// IsRunning returns true if the gossip manager is running.
func (g *GossipManager) IsRunning() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.running
}

// Uptime returns how long the gossip manager has been running.
func (g *GossipManager) Uptime() time.Duration {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if !g.running {
		return 0
	}
	return time.Since(g.startTime)
}

// Join attempts to join additional nodes to the cluster.
func (g *GossipManager) Join(addresses []string) (int, error) {
	g.mu.RLock()
	if !g.running {
		g.mu.RUnlock()
		return 0, ErrGossipNotRunning
	}
	g.mu.RUnlock()

	return g.list.Join(addresses)
}

// GossipMember represents a cluster member.
type GossipMember struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    uint16 `json:"port"`
	State   string `json:"state"`
}

// memberStateToString converts memberlist.NodeStateType to string.
func memberStateToString(state memberlist.NodeStateType) string {
	switch state {
	case memberlist.StateAlive:
		return "alive"
	case memberlist.StateSuspect:
		return "suspect"
	case memberlist.StateDead:
		return "dead"
	case memberlist.StateLeft:
		return "left"
	default:
		return "unknown"
	}
}

// slogWriter adapts slog.Logger to io.Writer for memberlist logging.
type slogWriter struct {
	logger *slog.Logger
}

func (w *slogWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// Trim trailing newline
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	// memberlist logs are quite verbose, log at debug level
	w.logger.Debug(msg)
	return len(p), nil
}

// Ensure slogWriter implements io.Writer
var _ io.Writer = (*slogWriter)(nil)
