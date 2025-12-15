// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

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
	"github.com/loganrossus/OpenGSLB/pkg/agent"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// MemberlistSender implements agent.GossipSender using hashicorp/memberlist.
// It connects to Overwatch nodes and sends health updates and heartbeats.
type MemberlistSender struct {
	config   SenderConfig
	list     *memberlist.Memberlist
	delegate *senderDelegate
	logger   *slog.Logger

	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// SenderConfig configures the MemberlistSender.
type SenderConfig struct {
	// NodeName is the unique name for this Agent node.
	// Typically the AgentID.
	NodeName string

	// BindAddress is the address to listen for gossip responses (host:port).
	// Default: "0.0.0.0:0" (random port)
	BindAddress string

	// OverwatchNodes is a list of Overwatch gossip addresses to connect to.
	// Format: "host:port" (e.g., "overwatch-1.internal:7946")
	OverwatchNodes []string

	// EncryptionKey is the 32-byte base64-encoded encryption key.
	// REQUIRED - must match the Overwatch nodes' key.
	EncryptionKey string

	// Region is the agent's geographic region.
	Region string

	// Logger for gossip operations.
	Logger *slog.Logger
}

// NewMemberlistSender creates a new memberlist-based gossip sender for Agents.
func NewMemberlistSender(cfg SenderConfig) (*MemberlistSender, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Validate required fields
	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("encryption key is required (ADR-015 mandates encryption)")
	}
	if len(cfg.OverwatchNodes) == 0 {
		return nil, fmt.Errorf("at least one Overwatch node address is required")
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
		cfg.BindAddress = "0.0.0.0:0"
	}

	s := &MemberlistSender{
		config: cfg,
		logger: logger,
	}

	// Create delegate
	s.delegate = &senderDelegate{
		nodeName: cfg.NodeName,
		region:   cfg.Region,
		logger:   logger,
	}

	return s, nil
}

// Start connects to Overwatch nodes and begins gossip.
func (s *MemberlistSender) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("sender already running")
	}

	// Parse bind address
	host, portStr, err := net.SplitHostPort(s.config.BindAddress)
	if err != nil {
		// Try with just port
		host = "0.0.0.0"
		portStr = "0"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 0
	}

	// Decode encryption key
	keyBytes, _ := base64.StdEncoding.DecodeString(s.config.EncryptionKey)

	// Create memberlist configuration
	mlConfig := memberlist.DefaultLANConfig()
	if s.config.NodeName != "" {
		mlConfig.Name = s.config.NodeName
	}
	mlConfig.BindAddr = host
	mlConfig.BindPort = port

	// Set encryption key
	mlConfig.SecretKey = keyBytes

	// Set delegate
	mlConfig.Delegate = s.delegate
	mlConfig.Events = s.delegate

	// Disable memberlist logging
	mlConfig.LogOutput = &discardWriter{}

	// Create memberlist
	list, err := memberlist.Create(mlConfig)
	if err != nil {
		return fmt.Errorf("failed to create memberlist: %w", err)
	}

	s.list = list
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Join the cluster via Overwatch nodes
	joined, err := list.Join(s.config.OverwatchNodes)
	if err != nil {
		list.Shutdown()
		return fmt.Errorf("failed to join gossip cluster: %w", err)
	}

	s.running = true

	s.logger.Info("gossip sender started",
		"bind_address", s.config.BindAddress,
		"node_name", mlConfig.Name,
		"overwatch_nodes", s.config.OverwatchNodes,
		"joined", joined,
	)

	return nil
}

// Stop disconnects from Overwatch nodes.
func (s *MemberlistSender) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	if s.cancel != nil {
		s.cancel()
	}

	if s.list != nil {
		// Leave gracefully
		if err := s.list.Leave(5 * time.Second); err != nil {
			s.logger.Warn("error leaving memberlist cluster", "error", err)
		}
		if err := s.list.Shutdown(); err != nil {
			s.logger.Warn("error shutting down memberlist", "error", err)
		}
	}

	s.logger.Info("gossip sender stopped")
	return nil
}

// SendHealthUpdate sends a health update to all Overwatch nodes.
func (s *MemberlistSender) SendHealthUpdate(msg agent.HealthUpdateMessage) error {
	s.mu.RLock()
	if !s.running || s.list == nil {
		s.mu.RUnlock()
		return fmt.Errorf("gossip sender not running")
	}
	s.mu.RUnlock()

	// Convert agent health update to gossip message
	var backends []overwatch.BackendHeartbeat
	for _, b := range msg.Backends {
		backends = append(backends, overwatch.BackendHeartbeat{
			Service: b.Service,
			Address: b.Address,
			Port:    b.Port,
			Weight:  b.Weight,
			Healthy: b.Healthy,
		})
	}

	gossipMsg := overwatch.GossipMessage{
		Type:      overwatch.MessageHeartbeat,
		AgentID:   msg.AgentID,
		Region:    msg.Region,
		Timestamp: msg.Timestamp,
		Payload: overwatch.HeartbeatPayload{
			Backends: backends,
		},
	}

	return s.broadcast(gossipMsg)
}

// SendHeartbeat sends a heartbeat message to all Overwatch nodes.
func (s *MemberlistSender) SendHeartbeat(msg agent.HeartbeatMessage) error {
	s.mu.RLock()
	if !s.running || s.list == nil {
		s.mu.RUnlock()
		return fmt.Errorf("gossip sender not running")
	}
	s.mu.RUnlock()

	// Heartbeats are sent as lightweight messages without backend data
	gossipMsg := overwatch.GossipMessage{
		Type:      overwatch.MessageHeartbeat,
		AgentID:   msg.AgentID,
		Region:    msg.Region,
		Timestamp: msg.Timestamp,
		Payload: overwatch.HeartbeatPayload{
			Backends: nil, // Empty for pure heartbeat
		},
	}

	return s.broadcast(gossipMsg)
}

// SendRegister sends a backend registration message.
func (s *MemberlistSender) SendRegister(agentID, region, service, address string, port, weight int) error {
	s.mu.RLock()
	if !s.running || s.list == nil {
		s.mu.RUnlock()
		return fmt.Errorf("gossip sender not running")
	}
	s.mu.RUnlock()

	gossipMsg := overwatch.GossipMessage{
		Type:      overwatch.MessageRegister,
		AgentID:   agentID,
		Region:    region,
		Timestamp: time.Now(),
		Payload: overwatch.RegisterPayload{
			Service: service,
			Address: address,
			Port:    port,
			Weight:  weight,
		},
	}

	return s.broadcast(gossipMsg)
}

// SendDeregister sends a backend deregistration message.
func (s *MemberlistSender) SendDeregister(agentID, region, service, address string, port int) error {
	s.mu.RLock()
	if !s.running || s.list == nil {
		s.mu.RUnlock()
		return fmt.Errorf("gossip sender not running")
	}
	s.mu.RUnlock()

	gossipMsg := overwatch.GossipMessage{
		Type:      overwatch.MessageDeregister,
		AgentID:   agentID,
		Region:    region,
		Timestamp: time.Now(),
		Payload: overwatch.DeregisterPayload{
			Service: service,
			Address: address,
			Port:    port,
		},
	}

	return s.broadcast(gossipMsg)
}

// SendAgentAuth sends an agent authentication message.
func (s *MemberlistSender) SendAgentAuth(agentID, region string, certPEM []byte, serviceToken, fingerprint string) error {
	s.mu.RLock()
	if !s.running || s.list == nil {
		s.mu.RUnlock()
		return fmt.Errorf("gossip sender not running")
	}
	s.mu.RUnlock()

	gossipMsg := overwatch.GossipMessage{
		Type:      overwatch.MessageAgentAuth,
		AgentID:   agentID,
		Region:    region,
		Timestamp: time.Now(),
		Payload: overwatch.AgentAuthPayload{
			CertificatePEM: certPEM,
			ServiceToken:   serviceToken,
			Fingerprint:    fingerprint,
		},
	}

	return s.broadcast(gossipMsg)
}

// broadcast sends a message to all cluster members.
func (s *MemberlistSender) broadcast(msg overwatch.GossipMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal gossip message: %w", err)
	}

	// Send to all members except self
	members := s.list.Members()
	var lastErr error
	sentCount := 0

	for _, member := range members {
		if member.Name == s.list.LocalNode().Name {
			continue // Skip self
		}

		// Parse node metadata to check if it's an Overwatch
		var meta NodeMeta
		if err := json.Unmarshal(member.Meta, &meta); err == nil {
			if meta.Role != RoleOverwatch {
				continue // Only send to Overwatch nodes
			}
		}

		// Send using UDP (fast, best-effort)
		if err := s.list.SendBestEffort(member, data); err != nil {
			lastErr = err
			s.logger.Warn("failed to send to member",
				"member", member.Name,
				"error", err,
			)
		} else {
			sentCount++
		}
	}

	if sentCount == 0 && lastErr != nil {
		return fmt.Errorf("failed to send to any members: %w", lastErr)
	}

	s.logger.Debug("broadcast gossip message",
		"type", msg.Type,
		"sent_to", sentCount,
	)

	return nil
}

// Members returns the current cluster members.
func (s *MemberlistSender) Members() []*memberlist.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.list == nil {
		return nil
	}
	return s.list.Members()
}

// NumMembers returns the number of cluster members.
func (s *MemberlistSender) NumMembers() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.list == nil {
		return 0
	}
	return s.list.NumMembers()
}

// senderDelegate implements memberlist.Delegate for agents.
type senderDelegate struct {
	nodeName string
	region   string
	logger   *slog.Logger
}

// NodeMeta returns metadata for this node.
func (d *senderDelegate) NodeMeta(limit int) []byte {
	meta := NodeMeta{
		Role:      RoleAgent,
		Region:    d.region,
		Version:   "1.0.0",
		Timestamp: time.Now(),
	}

	data, _ := json.Marshal(meta)
	if len(data) > limit {
		return nil
	}
	return data
}

// NotifyMsg handles incoming messages (agents typically don't receive, but can).
func (d *senderDelegate) NotifyMsg(msg []byte) {
	// Agents primarily send, but log any received messages
	if len(msg) > 0 {
		d.logger.Debug("agent received gossip message", "size", len(msg))
	}
}

// GetBroadcasts returns broadcasts to send.
func (d *senderDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	return nil
}

// LocalState returns local state.
func (d *senderDelegate) LocalState(join bool) []byte {
	return nil
}

// MergeRemoteState merges remote state.
func (d *senderDelegate) MergeRemoteState(buf []byte, join bool) {
}

// NotifyJoin is called when a node joins.
func (d *senderDelegate) NotifyJoin(node *memberlist.Node) {
	d.logger.Debug("node joined cluster", "name", node.Name)
}

// NotifyLeave is called when a node leaves.
func (d *senderDelegate) NotifyLeave(node *memberlist.Node) {
	d.logger.Debug("node left cluster", "name", node.Name)
}

// NotifyUpdate is called when a node updates.
func (d *senderDelegate) NotifyUpdate(node *memberlist.Node) {
}
