// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/metrics"
)

// Overwatch manages leader-initiated external health checks and veto logic.
type Overwatch struct {
	config config.OverwatchConfig
	checks []config.Region // Use regions to know about servers
	gossip *GossipManager
	raft   *RaftNode
	logger *slog.Logger

	mu            sync.RWMutex
	latestClaims  map[string]*HealthUpdate // key: server_addr (or unique ID)
	vetoes        map[string]time.Time     // key: server_addr -> expiry
	checkFailures map[string]int           // failure counter for external checks
}

// NewOverwatch creates a new Overwatch instance.
func NewOverwatch(cfg config.OverwatchConfig, regions []config.Region, gossip *GossipManager, raft *RaftNode, logger *slog.Logger) *Overwatch {
	if logger == nil {
		logger = slog.Default()
	}
	return &Overwatch{
		config:        cfg,
		checks:        regions,
		gossip:        gossip,
		raft:          raft,
		logger:        logger,
		latestClaims:  make(map[string]*HealthUpdate),
		vetoes:        make(map[string]time.Time),
		checkFailures: make(map[string]int),
	}
}

// Start begins the overwatch loop.
func (o *Overwatch) Start(ctx context.Context) error {
	o.logger.Info("starting overwatch",
		"interval", o.config.ExternalCheckInterval,
		"mode", o.config.VetoMode,
	)

	// Subscribe to health updates to keep our "latestClaims" view current
	if o.gossip != nil {
		o.gossip.OnHealthUpdate(func(update *HealthUpdate, fromNode string) {
			o.mu.Lock()
			o.latestClaims[update.ServerAddr] = update
			// If agent says unhealthy, we can clear our failure count (agreement)
			if !update.Healthy {
				delete(o.checkFailures, update.ServerAddr)
			}
			o.mu.Unlock()
		})
	}

	go o.runLoop(ctx)
	return nil
}

func (o *Overwatch) runLoop(ctx context.Context) {
	ticker := time.NewTicker(o.config.ExternalCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if o.shouldRunChecks() {
				o.performChecks(ctx)
			}
			o.cleanupVetoes()
		}
	}
}

func (o *Overwatch) shouldRunChecks() bool {
	// Only run checks if we are the leader
	if o.raft == nil {
		return false
	}
	return o.raft.IsLeader()
}

func (o *Overwatch) performChecks(ctx context.Context) {
	var wg sync.WaitGroup

	// Flatten server list for checking
	for _, region := range o.checks {
		for _, server := range region.Servers {
			// Find the corresponding config for this server to know check details?
			// The Region struct has HealthCheck config.
			healthCheck := region.HealthCheck
			serverAddr := fmt.Sprintf("%s:%d", server.Address, server.Port)

			// We only check if the agent claims it is HEALTHY.
			// If agent thinks it's unhealthy, we trust it (or at least don't need to VETO it being healthy).
			// Exception: "Strict" mode might want to verify even unhealthy claims?
			// For now, let's focus on VETOing false positives (Agent=Healthy, External=Fail).
			o.mu.RLock()
			claim, exists := o.latestClaims[serverAddr]
			o.mu.RUnlock()

			if exists && !claim.Healthy {
				// Agent says unhealthy. Agreement.
				continue
			}

			// If we haven't heard from agent yet, assume healthy? Or assume unknown?
			// Let's check anyway.

			wg.Add(1)
			go func(addr string, hc config.HealthCheck, host string) {
				defer wg.Done()
				o.checkServer(ctx, addr, hc, host)
			}(serverAddr, healthCheck, server.Host)
		}
	}
	wg.Wait()
}

func (o *Overwatch) checkServer(ctx context.Context, addr string, hc config.HealthCheck, host string) {
	healthy := o.executeProbe(ctx, addr, hc, host)

	o.mu.Lock()
	defer o.mu.Unlock()

	claim := o.latestClaims[addr]
	// Default to assuming agent says healthy if we have no data (so we don't accidentally allow a broken server if gossip is slow)
	agentSaysHealthy := true
	if claim != nil {
		agentSaysHealthy = claim.Healthy
	}

	o.decideVeto(addr, agentSaysHealthy, healthy)
}

func (o *Overwatch) executeProbe(ctx context.Context, addr string, hc config.HealthCheck, host string) bool {
	// Simple implementation supporting HTTP and TCP
	timeout := hc.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	if hc.Type == "tcp" {
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}

	// Default HTTP
	url := fmt.Sprintf("http://%s%s", addr, hc.Path)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}
	if host != "" {
		req.Host = host
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}
	return false
}

func (o *Overwatch) decideVeto(addr string, agentHealthy bool, externalHealthy bool) {
	// Decision Matrix:
	// Mode        | Agent | External | Decision | Action
	// ------------|-------|----------|----------|-------
	// Any         | H     | H        | Healthy  | Clear Veto
	// Any         | U     | H        | Unhealthy| Clear Veto (Trust agent's local failure)
	// Any         | U     | F        | Unhealthy| Clear Veto
	// Strict      | H     | F        | VETO     | Add Veto
	// Balanced    | H     | F        | VETO     | Add Veto (after threshold?)
	// Permissive  | H     | F        | Healthy  | Trust Agent (Log warning)

	if externalHealthy {
		// If external check passes, we generally don't veto.
		// Even if agent says unhealthy, we trust the agent (it might see disk full, etc).
		o.deleteVeto(addr)
		delete(o.checkFailures, addr)
		return
	}

	// External Check Failed
	if !agentHealthy {
		// Agreement: Both say unhealthy.
		delete(o.checkFailures, addr)
		return
	}

	// DISAGREEMENT: Agent says Healthy, External says Unhealthy.
	o.checkFailures[addr]++

	threshold := 3 // Hardcoded threshold for now, or use config?
	if o.checkFailures[addr] < threshold {
		return // Wait for more failures
	}

	// Apply Veto logic based on mode
	switch o.config.VetoMode {
	case "permissive":
		o.logger.Warn("overwatch disagreement (permissive)", "server", addr, "agent", "healthy", "external", "failed")
		// Do not veto
	case "strict", "balanced":
		// For now, strict and balanced behave similarly for clear failure: VETO.
		// "Balanced" might be more nuanced in future (e.g. check neighbor views).
		o.applyVeto(addr, "external_check_failed")
	default:
		// Default to balanced behavior
		o.applyVeto(addr, "external_check_failed")
	}
}

func (o *Overwatch) applyVeto(addr string, reason string) {
	expiry := time.Now().Add(o.config.ExternalCheckInterval * 2)
	o.vetoes[addr] = expiry

	o.logger.Warn("vetoing server", "server", addr, "reason", reason, "expiry", expiry)
	metrics.RecordOverwatchVeto(reason)

	// Broadcast override to cluster?
	// The implementation plan said: "Call gossip.BroadcastOverride".
	// But simply maintaining local veto state might be enough if "Leader Routing" uses this struct.
	// However, if we want *all* nodes (if we had distributed routing) to know, we would broadcast.
	// Since currently only Leader routes, local state affects DNS responses.
	// Broadcasting is useful if leadership changes or for UI/debug.

	if o.gossip != nil {
		cmd := &OverrideCommand{
			ServerAddr: addr,
			Action:     "force_unhealthy",
			Reason:     reason,
			Expiry:     expiry.Unix(),
		}
		// Fire and forget, but log validation
		go func() {
			if err := o.gossip.BroadcastOverride(cmd); err != nil {
				o.logger.Error("failed to broadcast veto override", "error", err)
			}
		}()
	}
}

func (o *Overwatch) deleteVeto(addr string) {
	if _, exists := o.vetoes[addr]; exists {
		delete(o.vetoes, addr)
		o.logger.Info("veto cleared", "server", addr)

		if o.gossip != nil {
			cmd := &OverrideCommand{
				ServerAddr: addr,
				Action:     "clear",
				Reason:     "external_check_passed",
			}
			go func() {
				if err := o.gossip.BroadcastOverride(cmd); err != nil {
					o.logger.Error("failed to broadcast veto clear", "error", err)
				}
			}()
		}
	}
}

func (o *Overwatch) cleanupVetoes() {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := time.Now()
	for addr, expiry := range o.vetoes {
		if now.After(expiry) {
			delete(o.vetoes, addr)
		}
	}
}

// IsServeable returns true if the server is allowed to serve traffic.
// It checks if there is an active veto.
func (o *Overwatch) IsServeable(addr string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if expiry, exists := o.vetoes[addr]; exists {
		if time.Now().Before(expiry) {
			return false
		}
	}
	return true
}
