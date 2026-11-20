// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/health"
)

// recordingChecker records every target it is asked to check.
type recordingChecker struct {
	mu      sync.Mutex
	targets []health.Target
	block   chan struct{} // when set, Check waits for it (or the context)
}

func (c *recordingChecker) Check(ctx context.Context, target health.Target) health.Result {
	c.mu.Lock()
	c.targets = append(c.targets, target)
	block := c.block
	c.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return health.Result{Healthy: false, Error: ctx.Err()}
		}
	}
	return health.Result{Healthy: true, Latency: time.Millisecond}
}

func (c *recordingChecker) Type() string { return "recording" }

func (c *recordingChecker) count(address string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.targets {
		if t.Address == address {
			n++
		}
	}
	return n
}

func (c *recordingChecker) last(address string) (health.Target, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.targets) - 1; i >= 0; i-- {
		if c.targets[i].Address == address {
			return c.targets[i], true
		}
	}
	return health.Target{}, false
}

// The validator probes a backend with the health check that was set for it
// through the API, and other backends with the node-wide settings.
func TestValidator_UsesBackendHealthCheck(t *testing.T) {
	registry := NewRegistry(RegistryConfig{}, nil)
	checker := &recordingChecker{}
	validator := NewValidator(ValidatorConfig{
		Enabled:       true,
		CheckInterval: time.Hour,
		CheckTimeout:  3 * time.Second,
	}, registry, checker)

	if err := registry.RegisterAPIBackend(APIBackend{
		Service: "web", Address: "192.168.1.1", Port: 443, Weight: 1,
		HealthCheck: &BackendHealthCheck{Type: "https", Path: "/healthz", Host: "web.example.com", Timeout: 700 * time.Millisecond},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterAPI("web", "192.168.1.2", 80, 1, "us-east"); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterStatic("web", "192.168.1.3", 80, 1, "us-east", "tcp"); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterAPIBackend(APIBackend{
		Service: "web", Address: "192.168.1.4", Port: 80, Weight: 1,
		HealthCheck: &BackendHealthCheck{Disabled: true},
	}); err != nil {
		t.Fatal(err)
	}

	validator.ValidateNow()

	own, ok := checker.last("192.168.1.1")
	if !ok {
		t.Fatal("the backend with its own check was not probed")
	}
	if own.Scheme != "https" || own.Path != "/healthz" || own.Host != "web.example.com" || own.Timeout != 700*time.Millisecond || own.Port != 443 {
		t.Errorf("probe did not use the backend's health check: %+v", own)
	}
	plain, _ := checker.last("192.168.1.2")
	if plain.Scheme != "http" || plain.Path != "" || plain.Timeout != 3*time.Second {
		t.Errorf("a backend without its own check must use the node-wide settings: %+v", plain)
	}
	static, _ := checker.last("192.168.1.3")
	if static.Scheme != "tcp" {
		t.Errorf("a static backend must keep its region's check type: %+v", static)
	}
	if n := checker.count("192.168.1.4"); n != 0 {
		t.Errorf("a backend whose check is turned off was probed %d times", n)
	}
	if b, _ := registry.GetBackend("web", "192.168.1.4", 80); b.ValidationHealthy != nil {
		t.Error("a backend whose check is turned off has a validation verdict")
	}
}

// A backend with its own interval is probed on that interval, whatever the
// node-wide interval is.
func TestValidator_PerBackendInterval(t *testing.T) {
	registry := NewRegistry(RegistryConfig{}, nil)
	checker := &recordingChecker{}
	validator := NewValidator(ValidatorConfig{
		Enabled:       true,
		CheckInterval: time.Hour,
		CheckTimeout:  time.Second,
		tick:          5 * time.Millisecond,
	}, registry, checker)

	if err := registry.RegisterAPIBackend(APIBackend{
		Service: "web", Address: "192.168.1.1", Port: 80, Weight: 1,
		HealthCheck: &BackendHealthCheck{Type: "tcp", Interval: 30 * time.Millisecond},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterAPI("web", "192.168.1.2", 80, 1, "us-east"); err != nil {
		t.Fatal(err)
	}

	if err := validator.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = validator.Stop() }()

	deadline := time.Now().Add(5 * time.Second)
	for checker.count("192.168.1.1") < 4 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := checker.count("192.168.1.1"); n < 4 {
		t.Fatalf("backend with a 30ms interval was probed %d times, want at least 4", n)
	}
	if n := checker.count("192.168.1.2"); n != 1 {
		t.Errorf("backend on the node-wide 1h interval was probed %d times, want exactly 1 (the initial probe)", n)
	}

	// A backend registered while the validator runs gets its first probe at
	// the next tick, not one node-wide interval later.
	if err := registry.RegisterAPI("web", "192.168.1.3", 80, 1, "us-east"); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for checker.count("192.168.1.3") == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if checker.count("192.168.1.3") == 0 {
		t.Error("a backend registered at runtime was not probed")
	}
}

// A probe that hangs must not hold up the probes of other backends, and the
// same backend must not be probed twice at once.
func TestValidator_SlowProbeDoesNotBlockOthers(t *testing.T) {
	registry := NewRegistry(RegistryConfig{}, nil)
	slow := &recordingChecker{block: make(chan struct{})}
	fast := &recordingChecker{}
	checker := health.CheckerFunc(func(ctx context.Context, target health.Target) health.Result {
		if target.Address == "192.168.1.1" {
			return slow.Check(ctx, target)
		}
		return fast.Check(ctx, target)
	})
	validator := NewValidator(ValidatorConfig{
		Enabled:       true,
		CheckInterval: 20 * time.Millisecond,
		CheckTimeout:  30 * time.Second,
		tick:          5 * time.Millisecond,
	}, registry, checker)
	_ = registry.RegisterAPI("web", "192.168.1.1", 80, 1, "us-east")
	_ = registry.RegisterAPI("web", "192.168.1.2", 80, 1, "us-east")

	if err := validator.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for fast.count("192.168.1.2") < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := fast.count("192.168.1.2"); n < 3 {
		t.Errorf("the healthy backend was probed %d times while another probe hung, want at least 3", n)
	}
	if n := slow.count("192.168.1.1"); n != 1 {
		t.Errorf("the hanging backend was probed %d times at once, want 1", n)
	}

	// Stop cancels the hanging probe and returns.
	stopped := make(chan struct{})
	go func() {
		_ = validator.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return while a probe was in flight")
	}
}
