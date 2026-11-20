// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/health"
)

// recordingChecker is a health.Checker that counts checks per address and
// remembers the last target it was given for each.
type recordingChecker struct {
	mu        sync.Mutex
	counts    map[string]int
	last      map[string]health.Target
	unhealthy map[string]bool
}

func newRecordingChecker() *recordingChecker {
	return &recordingChecker{
		counts:    make(map[string]int),
		last:      make(map[string]health.Target),
		unhealthy: make(map[string]bool),
	}
}

func (c *recordingChecker) Check(_ context.Context, target health.Target) health.Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[target.Address]++
	c.last[target.Address] = target
	result := health.Result{Healthy: !c.unhealthy[target.Address], Timestamp: time.Now()}
	if !result.Healthy {
		result.Error = fmt.Errorf("unhealthy")
	}
	return result
}

func (c *recordingChecker) Type() string { return "recording" }

func (c *recordingChecker) count(address string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[address]
}

func (c *recordingChecker) lastTarget(address string) health.Target {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last[address]
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testBackend(address string, weight int) BackendConfig {
	return BackendConfig{
		Service: "webapp",
		Address: address,
		Port:    8080,
		Weight:  weight,
		HealthCheck: HealthCheckConfig{
			Type:             "http",
			Path:             "/health",
			Interval:         20 * time.Millisecond,
			Timeout:          10 * time.Millisecond,
			FailureThreshold: 2,
			SuccessThreshold: 1,
		},
	}
}

func addresses(cfgs []BackendConfig) string {
	out := make([]string, 0, len(cfgs))
	for _, cfg := range cfgs {
		out = append(out, cfg.Address)
	}
	return strings.Join(out, ",")
}

func managedAddresses(m *BackendManager) string {
	var out []string
	for _, h := range m.GetAllHealth() {
		out = append(out, h.Address)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// waitFor polls cond until it is true or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestBackendManager_Reconfigure_AddsRemovesAndUpdates(t *testing.T) {
	m := NewBackendManager(newRecordingChecker(), quietLogger())
	for _, cfg := range []BackendConfig{testBackend("10.0.0.1", 100), testBackend("10.0.0.2", 100)} {
		if err := m.AddBackend(cfg); err != nil {
			t.Fatalf("AddBackend: %v", err)
		}
	}

	result, err := m.Reconfigure([]BackendConfig{testBackend("10.0.0.2", 30), testBackend("10.0.0.3", 100)})
	if err != nil {
		t.Fatalf("Reconfigure: %v", err)
	}

	if got := addresses(result.Added); got != "10.0.0.3" {
		t.Errorf("Added = %q, want 10.0.0.3", got)
	}
	if got := addresses(result.Removed); got != "10.0.0.1" {
		t.Errorf("Removed = %q, want 10.0.0.1", got)
	}
	if got := addresses(result.Updated); got != "10.0.0.2" {
		t.Errorf("Updated = %q, want 10.0.0.2", got)
	}
	if !result.Changed() {
		t.Error("Changed() = false after changes")
	}
	if got := managedAddresses(m); got != "10.0.0.2,10.0.0.3" {
		t.Errorf("managed backends = %q, want 10.0.0.2,10.0.0.3", got)
	}
	snapshot, ok := m.GetHealth("webapp", "10.0.0.2", 8080)
	if !ok || snapshot.Weight != 30 {
		t.Errorf("updated backend weight = %d (found %v), want 30", snapshot.Weight, ok)
	}
}

// The same list again, including one written without the optional settings,
// is not a change.
func TestBackendManager_Reconfigure_UnchangedIsNotAChange(t *testing.T) {
	m := NewBackendManager(newRecordingChecker(), quietLogger())
	bare := BackendConfig{Service: "webapp", Address: "10.0.0.1", Port: 8080}
	full := testBackend("10.0.0.2", 100)
	for _, cfg := range []BackendConfig{bare, full} {
		if err := m.AddBackend(cfg); err != nil {
			t.Fatalf("AddBackend: %v", err)
		}
	}

	result, err := m.Reconfigure([]BackendConfig{full, bare})
	if err != nil {
		t.Fatalf("Reconfigure: %v", err)
	}
	if result.Changed() {
		t.Errorf("unchanged configuration reported as a change: %+v", result)
	}
}

func TestBackendManager_Reconfigure_RejectsDuplicatesWithoutChanging(t *testing.T) {
	m := NewBackendManager(newRecordingChecker(), quietLogger())
	if err := m.AddBackend(testBackend("10.0.0.1", 100)); err != nil {
		t.Fatalf("AddBackend: %v", err)
	}

	result, err := m.Reconfigure([]BackendConfig{testBackend("10.0.0.2", 100), testBackend("10.0.0.2", 50)})
	if err == nil {
		t.Fatal("Reconfigure accepted the same backend twice")
	}
	if result.Changed() {
		t.Errorf("failed Reconfigure reported changes: %+v", result)
	}
	if got := managedAddresses(m); got != "10.0.0.1" {
		t.Errorf("managed backends after a failed Reconfigure = %q, want 10.0.0.1", got)
	}
}

// A running manager starts checking added backends and stops checking removed
// ones, and Stop still returns.
func TestBackendManager_Reconfigure_StartsAndStopsChecks(t *testing.T) {
	checker := newRecordingChecker()
	m := NewBackendManager(checker, quietLogger())
	if err := m.AddBackend(testBackend("10.0.0.1", 100)); err != nil {
		t.Fatalf("AddBackend: %v", err)
	}
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 2*time.Second, "first check of 10.0.0.1", func() bool { return checker.count("10.0.0.1") > 0 })

	if _, err := m.Reconfigure([]BackendConfig{testBackend("10.0.0.2", 100)}); err != nil {
		t.Fatalf("Reconfigure: %v", err)
	}
	waitFor(t, 2*time.Second, "checks of the added backend", func() bool { return checker.count("10.0.0.2") >= 2 })

	// A check that was in flight when the backend was removed may still
	// finish. After that the count must stand still.
	time.Sleep(50 * time.Millisecond)
	settled := checker.count("10.0.0.1")
	time.Sleep(120 * time.Millisecond) // six check intervals
	if got := checker.count("10.0.0.1"); got != settled {
		t.Errorf("removed backend was checked %d more times", got-settled)
	}

	stopped := make(chan struct{})
	go func() {
		_ = m.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after a Reconfigure")
	}
}

// Changing the settings of a healthy backend must not make it unhealthy, and
// the new settings must be the ones used from then on.
func TestBackendManager_Reconfigure_KeepsHealthAndAppliesNewSettings(t *testing.T) {
	checker := newRecordingChecker()
	m := NewBackendManager(checker, quietLogger())

	var mu sync.Mutex
	var updates []BackendHealthUpdate
	m.OnHealthChange(func(u BackendHealthUpdate) {
		mu.Lock()
		defer mu.Unlock()
		updates = append(updates, u)
	})

	if err := m.AddBackend(testBackend("10.0.0.1", 100)); err != nil {
		t.Fatalf("AddBackend: %v", err)
	}
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop() }()
	waitFor(t, 2*time.Second, "backend to become healthy", func() bool {
		h, _ := m.GetHealth("webapp", "10.0.0.1", 8080)
		return h.Healthy
	})

	changed := testBackend("10.0.0.1", 40)
	changed.HealthCheck.Path = "/ready"
	changed.HealthCheck.Type = "tcp"
	changed.HealthCheck.FailureThreshold = 5
	result, err := m.Reconfigure([]BackendConfig{changed})
	if err != nil {
		t.Fatalf("Reconfigure: %v", err)
	}
	if got := addresses(result.Updated); got != "10.0.0.1" || len(result.Added)+len(result.Removed) != 0 {
		t.Fatalf("result = %+v, want one update", result)
	}

	// Still healthy right after the change, with the new weight.
	h, ok := m.GetHealth("webapp", "10.0.0.1", 8080)
	if !ok || !h.Healthy {
		t.Fatalf("backend is not healthy right after a settings change: %+v", h)
	}
	if h.Weight != 40 {
		t.Errorf("weight = %d, want 40", h.Weight)
	}

	waitFor(t, 2*time.Second, "a check with the new settings", func() bool {
		target := checker.lastTarget("10.0.0.1")
		return target.Path == "/ready" && target.Scheme == "tcp"
	})

	// The new failure threshold is in effect: four failures do not flip it.
	checker.mu.Lock()
	checker.unhealthy["10.0.0.1"] = true
	checker.mu.Unlock()
	waitFor(t, 2*time.Second, "the backend to become unhealthy", func() bool {
		h, _ := m.GetHealth("webapp", "10.0.0.1", 8080)
		if h.Healthy && h.ConsecutiveFails >= 5 {
			t.Fatalf("still healthy after %d failures with threshold 5", h.ConsecutiveFails)
		}
		if !h.Healthy && h.ConsecutiveFails < 5 {
			t.Fatalf("unhealthy after %d failures, the new threshold is 5", h.ConsecutiveFails)
		}
		return !h.Healthy
	})

	// The only transitions reported are healthy at the start and unhealthy at
	// the end: the settings change itself reported nothing.
	waitFor(t, 2*time.Second, "the unhealthy notification", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(updates) >= 2
	})
	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 2 || !updates[0].Healthy || updates[1].Healthy {
		t.Errorf("health changes reported = %+v, want healthy then unhealthy", updates)
	}
	if updates[1].Weight != 40 {
		t.Errorf("update after the change carries weight %d, want 40", updates[1].Weight)
	}
}

func TestBackendManager_Services(t *testing.T) {
	m := NewBackendManager(newRecordingChecker(), quietLogger())
	for _, cfg := range []BackendConfig{
		{Service: "web", Address: "10.0.0.1", Port: 80},
		{Service: "api", Address: "10.0.0.1", Port: 8080},
		{Service: "web", Address: "10.0.0.2", Port: 80},
	} {
		if err := m.AddBackend(cfg); err != nil {
			t.Fatalf("AddBackend: %v", err)
		}
	}
	if got := strings.Join(m.Services(), ","); got != "api,web" {
		t.Errorf("Services() = %q, want api,web", got)
	}
}

func reconfigureTestConfig(t *testing.T, backends ...config.AgentBackend) *config.Config {
	t.Helper()
	dir := t.TempDir()
	return &config.Config{
		Mode: config.ModeAgent,
		Agent: config.AgentConfig{
			Identity: config.AgentIdentityConfig{
				ServiceToken: "test-token-12345678",
				Region:       "eu-west",
				CertPath:     filepath.Join(dir, "agent.crt"),
				KeyPath:      filepath.Join(dir, "agent.key"),
			},
			Backends:  backends,
			Heartbeat: config.HeartbeatConfig{Interval: time.Hour, MissedThreshold: 3},
		},
	}
}

func agentBackendConfig(service, address string, weight int) config.AgentBackend {
	return config.AgentBackend{
		Service: service,
		Address: address,
		Port:    9000,
		Weight:  weight,
		HealthCheck: config.HealthCheck{
			Type:             "tcp",
			Interval:         time.Hour,
			Timeout:          50 * time.Millisecond,
			FailureThreshold: 2,
			SuccessThreshold: 1,
		},
	}
}

// A running agent tells the Overwatch nodes about a reconfiguration at once.
func TestAgent_Reconfigure_AnnouncesChanges(t *testing.T) {
	gossip := NewMockGossipSender()
	cfg := reconfigureTestConfig(t,
		agentBackendConfig("api", "127.0.0.1", 100),
		agentBackendConfig("web", "127.0.0.2", 100),
	)
	agent, err := NewAgent(AgentConfig{Config: cfg, Logger: quietLogger(), Gossip: gossip})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = agent.Stop(stopCtx)
	}()

	// Let the initial checks (and the updates they trigger) settle, then
	// start from a clean record.
	waitFor(t, 2*time.Second, "initial checks", func() bool {
		for _, h := range agent.GetBackendManager().GetAllHealth() {
			if h.LastCheck.IsZero() {
				return false
			}
		}
		return true
	})
	time.Sleep(100 * time.Millisecond)
	gossip.Clear()

	next := reconfigureTestConfig(t,
		agentBackendConfig("api", "127.0.0.1", 25),   // weight changed
		agentBackendConfig("shop", "127.0.0.3", 100), // added; web is removed
	)
	result, err := agent.Reconfigure(next)
	if err != nil {
		t.Fatalf("Reconfigure: %v", err)
	}
	if len(result.Added) != 1 || len(result.Removed) != 1 || len(result.Updated) != 1 {
		t.Fatalf("result = %+v, want one added, one removed, one updated", result)
	}

	deregistered := gossip.Deregistrations()
	if len(deregistered) != 1 {
		t.Fatalf("deregistrations = %+v, want exactly the removed backend", deregistered)
	}
	d := deregistered[0]
	if d.Service != "web" || d.Address != "127.0.0.2" || d.Port != 9000 ||
		d.AgentID != agent.GetIdentity().AgentID || d.Region != "eu-west" {
		t.Errorf("deregistration = %+v", d)
	}

	updates := gossip.HealthUpdates()
	if len(updates) == 0 {
		t.Fatal("no health update was sent after the reconfiguration")
	}
	weights := map[string]int{}
	for _, b := range updates[0].Backends {
		weights[b.Service] = b.Weight
	}
	if len(weights) != 2 || weights["api"] != 25 || weights["shop"] != 100 {
		t.Errorf("health update after the reconfiguration carries %v, want api (25) and shop (100)", weights)
	}

	if got := strings.Join(agent.GetBackendManager().Services(), ","); got != "api,shop" {
		t.Errorf("services after the reconfiguration = %q, want api,shop", got)
	}
}

// Nothing is sent when nothing changed, or when the agent is not running.
func TestAgent_Reconfigure_SilentWithoutChangesOrWhenStopped(t *testing.T) {
	gossip := NewMockGossipSender()
	cfg := reconfigureTestConfig(t, agentBackendConfig("api", "127.0.0.1", 100))
	agent, err := NewAgent(AgentConfig{Config: cfg, Logger: quietLogger(), Gossip: gossip})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	// Not started: the backend set changes, nothing is announced.
	result, err := agent.Reconfigure(reconfigureTestConfig(t, agentBackendConfig("web", "127.0.0.2", 100)))
	if err != nil {
		t.Fatalf("Reconfigure (stopped): %v", err)
	}
	if !result.Changed() || agent.GetBackendManager().BackendCount() != 1 {
		t.Errorf("backends were not reconfigured on a stopped agent: %+v", result)
	}
	if len(gossip.Deregistrations())+len(gossip.HealthUpdates()) != 0 {
		t.Error("a stopped agent sent gossip messages")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = agent.Stop(stopCtx)
	}()
	waitFor(t, 2*time.Second, "initial check", func() bool {
		h := agent.GetBackendManager().GetAllHealth()
		return len(h) == 1 && !h[0].LastCheck.IsZero()
	})
	time.Sleep(100 * time.Millisecond)
	gossip.Clear()

	// Running, same backends: not a change.
	result, err = agent.Reconfigure(reconfigureTestConfig(t, agentBackendConfig("web", "127.0.0.2", 100)))
	if err != nil {
		t.Fatalf("Reconfigure (unchanged): %v", err)
	}
	if result.Changed() {
		t.Errorf("unchanged backends reported as a change: %+v", result)
	}
	if len(gossip.Deregistrations())+len(gossip.HealthUpdates()) != 0 {
		t.Error("gossip messages were sent although nothing changed")
	}
}
