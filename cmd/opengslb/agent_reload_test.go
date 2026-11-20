// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// agentBackend is one backend in a generated agent configuration file.
type agentBackend struct {
	service string
	address string
	port    int
	weight  int
}

// agentEnv is a running agent plus the file it was configured from.
type agentEnv struct {
	t       *testing.T
	app     *Application
	logger  *slog.Logger
	logs    *syncBuffer
	cfgPath string
	dir     string

	overwatchGossip string // "host:port" of the overwatch to join
	heartbeat       string // agent.heartbeat.interval
	logLevel        string

	stop func()
}

func newAgentEnv(t *testing.T, overwatchGossip string) *agentEnv {
	t.Helper()
	logs := &syncBuffer{}
	dir := t.TempDir()
	return &agentEnv{
		t:               t,
		logs:            logs,
		logger:          slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		cfgPath:         filepath.Join(dir, "agent.yaml"),
		dir:             dir,
		overwatchGossip: overwatchGossip,
		heartbeat:       "1s",
		logLevel:        "info",
	}
}

func (e *agentEnv) writeConfig(backends []agentBackend) {
	e.t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, `mode: agent
agent:
  identity:
    service_token: "reload-test-token-0123456789"
    region: us-east
    cert_path: %q
    key_path: %q
  gossip:
    encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
    overwatch_nodes:
      - %q
  heartbeat:
    interval: %s
    missed_threshold: 3
  backends:
`, filepath.Join(e.dir, "agent.crt"), filepath.Join(e.dir, "agent.key"), e.overwatchGossip, e.heartbeat)
	for _, be := range backends {
		weight := be.weight
		if weight == 0 {
			weight = 100
		}
		fmt.Fprintf(&b, `    - service: %s
      address: %s
      port: %d
      weight: %d
      health_check:
        type: http
        path: /
        interval: 1s
        timeout: 1s
        failure_threshold: 2
        success_threshold: 1
`, be.service, be.address, be.port, weight)
	}
	fmt.Fprintf(&b, "logging:\n  level: %s\n  format: text\nmetrics:\n  enabled: false\n", e.logLevel)
	if err := os.WriteFile(e.cfgPath, []byte(b.String()), 0o600); err != nil {
		e.t.Fatalf("write agent config: %v", err)
	}
}

// start runs the agent the way main does between config.Load and the signal loop.
func (e *agentEnv) start() {
	e.t.Helper()
	cfg, err := config.Load(e.cfgPath)
	if err != nil {
		e.t.Fatalf("load agent config: %v", err)
	}
	applyModeOverride(cfg, e.logger)
	if err := validateModeFlags(cfg); err != nil {
		e.t.Fatalf("validateModeFlags: %v", err)
	}

	e.app = NewApplication(cfg, e.logger)
	if err := e.app.Initialize(); err != nil {
		e.t.Fatalf("Initialize agent: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := e.app.Start(ctx); err != nil {
			e.t.Errorf("agent Start: %v", err)
		}
	}()
	var once sync.Once
	e.stop = func() {
		once.Do(func() {
			sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer scancel()
			_ = e.app.Shutdown(sctx)
			cancel()
			<-done
		})
	}
	e.t.Cleanup(e.stop)
}

// reload calls handleReload, the function the SIGHUP handler in main calls,
// on the agent's configuration file.
func (e *agentEnv) reload() error {
	previous := configPath
	configPath = e.cfgPath
	defer func() { configPath = previous }()
	return handleReload(e.app, e.logger)
}

// monitored returns the "service:address:port" keys the agent is checking.
func (e *agentEnv) monitored() []string {
	var keys []string
	for _, h := range e.app.agentInstance.GetBackendManager().GetAllHealth() {
		keys = append(keys, fmt.Sprintf("%s:%s:%d", h.Service, h.Address, h.Port))
	}
	sort.Strings(keys)
	return keys
}

// eventually polls cond until it returns "" or the timeout expires.
func eventually(t *testing.T, timeout time.Duration, cond func() string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for {
		if last = cond(); last == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %s: %s", timeout, last)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestReload_AgentBackendsReachOverwatch is the end-to-end check for agent
// reload: an agent gossips to a real overwatch, its configuration file is
// changed, and after a reload the overwatch serves the new backend set.
func TestReload_AgentBackendsReachOverwatch(t *testing.T) {
	const service = reloadStaticDomain
	staticPort := liveBackend(t, "127.0.0.1")
	removedPort := liveBackend(t, "127.0.0.2")
	keptPort := liveBackend(t, "127.0.0.3")
	addedPort := liveBackend(t, "127.0.0.4")

	ow := startReloadOverwatch(t,
		[]reloadDomain{{name: service}},
		[]reloadServer{{address: "127.0.0.1", port: staticPort, service: service}})

	ag := newAgentEnv(t, fmt.Sprintf("127.0.0.1:%d", ow.gossipPort))
	ag.writeConfig([]agentBackend{
		{service: service, address: "127.0.0.2", port: removedPort},
		{service: service, address: "127.0.0.3", port: keptPort, weight: 100},
	})
	ag.start()

	resolvesTo := func(want ...string) func() string {
		sort.Strings(want)
		return func() string {
			seen := map[string]bool{}
			for i := 0; i < 24; i++ {
				rcode, ips, err := ow.query(service)
				if err != nil || rcode != 0 {
					return fmt.Sprintf("query: rcode %d err %v", rcode, err)
				}
				for _, ip := range ips {
					seen[ip] = true
				}
			}
			got := make([]string, 0, len(seen))
			for ip := range seen {
				got = append(got, ip)
			}
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				return fmt.Sprintf("%s resolves to %v, want %v", service, got, want)
			}
			return ""
		}
	}
	eventually(t, 20*time.Second, resolvesTo("127.0.0.1", "127.0.0.2", "127.0.0.3"))

	// New file: one backend removed, one added, one with a new weight.
	ag.writeConfig([]agentBackend{
		{service: service, address: "127.0.0.3", port: keptPort, weight: 30},
		{service: service, address: "127.0.0.4", port: addedPort},
	})
	if err := ag.reload(); err != nil {
		t.Fatalf("agent reload: %v", err)
	}

	want := []string{
		fmt.Sprintf("%s:127.0.0.3:%d", service, keptPort),
		fmt.Sprintf("%s:127.0.0.4:%d", service, addedPort),
	}
	if got := ag.monitored(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("agent monitors %v after reload, want %v", got, want)
	}

	eventually(t, 20*time.Second, resolvesTo("127.0.0.1", "127.0.0.3", "127.0.0.4"))
	eventually(t, 20*time.Second, func() string {
		b, ok := ow.app.backendRegistry.GetBackend(service, "127.0.0.3", keptPort)
		if !ok {
			return "kept backend is not registered on the overwatch"
		}
		if b.Weight != 30 {
			return fmt.Sprintf("kept backend has weight %d on the overwatch, want 30", b.Weight)
		}
		return ""
	})
	if _, ok := ow.app.backendRegistry.GetBackend(service, "127.0.0.2", removedPort); ok {
		t.Errorf("overwatch still holds the backend the agent stopped monitoring")
	}
}

// TestReload_AgentNeverReportsSuccessForANoOp: the agent applies its backend
// list on reload and nothing else. A change to any other agent setting must
// be reported as needing a restart, not as a successful reload.
func TestReload_AgentNeverReportsSuccessForANoOp(t *testing.T) {
	const service = reloadStaticDomain
	staticPort := liveBackend(t, "127.0.0.1")
	backendPort := liveBackend(t, "127.0.0.2")

	ow := startReloadOverwatch(t,
		[]reloadDomain{{name: service}},
		[]reloadServer{{address: "127.0.0.1", port: staticPort, service: service}})

	backends := []agentBackend{{service: service, address: "127.0.0.2", port: backendPort}}
	ag := newAgentEnv(t, fmt.Sprintf("127.0.0.1:%d", ow.gossipPort))
	ag.writeConfig(backends)
	ag.start()

	const success = "configuration reloaded successfully"
	const partial = "restart required for settings that cannot change at runtime"
	reloadLogs := func() string {
		t.Helper()
		before := len(ag.logs.String())
		if err := ag.reload(); err != nil {
			t.Fatalf("agent reload: %v", err)
		}
		return ag.logs.String()[before:]
	}

	// Unchanged file: the agent says that nothing changed.
	logs := reloadLogs()
	if !strings.Contains(logs, "agent backends unchanged") || !strings.Contains(logs, success) {
		t.Errorf("unchanged file: want 'agent backends unchanged' and the success line, got:\n%s", logs)
	}
	if strings.Contains(logs, "agent backends reconfigured") {
		t.Errorf("unchanged file reported as a reconfiguration:\n%s", logs)
	}

	// A setting the agent only reads at startup.
	ag.heartbeat = "7s"
	ag.writeConfig(backends)
	logs = reloadLogs()
	if !strings.Contains(logs, "level=WARN") || !strings.Contains(logs, partial) || !strings.Contains(logs, "agent.heartbeat") {
		t.Errorf("changed agent.heartbeat: want a WARN naming it, got:\n%s", logs)
	}
	if strings.Contains(logs, success) {
		t.Errorf("changed agent.heartbeat: reload reported plain success:\n%s", logs)
	}

	// A backend change next to it is still applied.
	addedPort := liveBackend(t, "127.0.0.3")
	backends = append(backends, agentBackend{service: service, address: "127.0.0.3", port: addedPort})
	ag.writeConfig(backends)
	logs = reloadLogs()
	if !strings.Contains(logs, "agent backends reconfigured") || !strings.Contains(logs, "added=1") {
		t.Errorf("added backend: want 'agent backends reconfigured added=1', got:\n%s", logs)
	}
	if !strings.Contains(logs, "agent.heartbeat") || strings.Contains(logs, success) {
		t.Errorf("the pending agent.heartbeat change is no longer reported:\n%s", logs)
	}
	if got := len(ag.monitored()); got != 2 {
		t.Errorf("agent monitors %d backends, want 2", got)
	}
}
