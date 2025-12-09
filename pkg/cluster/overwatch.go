// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
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
	checks []config.Region
	gossip *GossipManager
	raft   *RaftNode
	logger *slog.Logger

	mu            sync.RWMutex
	latestClaims  map[string]*HealthUpdate
	vetoes        map[string]time.Time
	checkFailures map[string]int

	// For graceful shutdown
	stopCh chan struct{}
	doneCh chan struct{}
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
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
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
			if !update.Healthy {
				delete(o.checkFailures, update.ServerAddr)
			}
			o.mu.Unlock()
		})
	}

	go o.runLoop(ctx)
	return nil
}

// Stop gracefully stops the overwatch.
func (o *Overwatch) Stop(ctx context.Context) error {
	close(o.stopCh)

	select {
	case <-o.doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (o *Overwatch) runLoop(ctx context.Context) {
	defer close(o.doneCh)

	ticker := time.NewTicker(o.config.ExternalCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
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
	if o.raft == nil {
		return false
	}
	return o.raft.IsLeader()
}

func (o *Overwatch) performChecks(ctx context.Context) {
	var wg sync.WaitGroup

	for _, region := range o.checks {
		healthCheck := region.HealthCheck

		for _, server := range region.Servers {
			serverAddr := fmt.Sprintf("%s:%d", server.Address, server.Port)

			// Check if agent claims unhealthy - if so, we don't need to verify
			o.mu.RLock()
			claim, exists := o.latestClaims[serverAddr]
			agentClaimsUnhealthy := exists && !claim.Healthy
			o.mu.RUnlock()

			if agentClaimsUnhealthy {
				continue
			}

			// Capture loop variables for goroutine
			addr := serverAddr
			hc := healthCheck
			host := server.Host

			wg.Add(1)
			go func() {
				defer wg.Done()
				o.checkServer(ctx, addr, hc, host)
			}()
		}
	}
	wg.Wait()
}

func (o *Overwatch) checkServer(ctx context.Context, addr string, hc config.HealthCheck, host string) {
	healthy := o.executeProbe(ctx, addr, hc, host)

	o.mu.Lock()
	defer o.mu.Unlock()

	claim := o.latestClaims[addr]
	agentSaysHealthy := true
	if claim != nil {
		agentSaysHealthy = claim.Healthy
	}

	o.decideVetoLocked(addr, agentSaysHealthy, healthy)
}

func (o *Overwatch) executeProbe(ctx context.Context, addr string, hc config.HealthCheck, host string) bool {
	timeout := hc.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	switch hc.Type {
	case "tcp":
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return false
		}
		conn.Close()
		return true

	case "https":
		return o.executeHTTPProbe(ctx, "https", addr, hc.Path, host, timeout)

	default: // "http" or empty
		return o.executeHTTPProbe(ctx, "http", addr, hc.Path, host, timeout)
	}
}

func (o *Overwatch) executeHTTPProbe(ctx context.Context, scheme, addr, path, host string, timeout time.Duration) bool {
	url := fmt.Sprintf("%s://%s%s", scheme, addr, path)

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

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// decideVetoLocked decides whether to veto a server.
// MUST be called with o.mu held.
// decideVeto is a public wrapper around decideVetoLocked for testing.
// It acquires the mutex before calling the internal method.
func (o *Overwatch) decideVeto(addr string, agentHealthy bool, externalHealthy bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.decideVetoLocked(addr, agentHealthy, externalHealthy)
}

func (o *Overwatch) decideVetoLocked(addr string, agentHealthy bool, externalHealthy bool) {
	if externalHealthy {
		o.deleteVetoLocked(addr)
		delete(o.checkFailures, addr)
		return
	}

	// External Check Failed
	if !agentHealthy {
		// Agreement: Both say unhealthy
		delete(o.checkFailures, addr)
		return
	}

	// DISAGREEMENT: Agent says Healthy, External says Unhealthy
	o.checkFailures[addr]++

	threshold := o.config.VetoThreshold
	if threshold == 0 {
		threshold = 3 // Default
	}

	if o.checkFailures[addr] < threshold {
		return
	}

	switch o.config.VetoMode {
	case "permissive":
		o.logger.Warn("overwatch disagreement (permissive mode, not vetoing)",
			"server", addr,
			"agent", "healthy",
			"external", "failed",
		)
	case "strict", "balanced", "":
		o.applyVetoLocked(addr, "external_check_failed")
	default:
		o.applyVetoLocked(addr, "external_check_failed")
	}
}

// applyVetoLocked applies a veto to a server.
// MUST be called with o.mu held.
func (o *Overwatch) applyVetoLocked(addr string, reason string) {
	expiry := time.Now().Add(o.config.ExternalCheckInterval * 2)
	o.vetoes[addr] = expiry

	o.logger.Warn("vetoing server",
		"server", addr,
		"reason", reason,
		"expiry", expiry,
	)
	metrics.RecordOverwatchVeto(reason)

	// Broadcast override to cluster (fire and forget, outside of lock)
	if o.gossip != nil {
		cmd := &OverrideCommand{
			ServerAddr: addr,
			Action:     "force_unhealthy",
			Reason:     reason,
			Expiry:     expiry.Unix(),
		}
		go func() {
			if err := o.gossip.BroadcastOverride(cmd); err != nil {
				o.logger.Error("failed to broadcast veto override", "error", err)
			}
		}()
	}
}

// deleteVetoLocked removes a veto from a server.
// MUST be called with o.mu held.
func (o *Overwatch) deleteVetoLocked(addr string) {
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
			o.logger.Debug("veto expired", "server", addr)
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
