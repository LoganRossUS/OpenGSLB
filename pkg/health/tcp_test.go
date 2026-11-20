// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    → https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package health

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// =============================================================================
// TCPChecker Tests
// =============================================================================

func TestTCPChecker_HealthyServer(t *testing.T) {
	// Start a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	// Accept connections in background (just accept and close)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // Listener closed
			}
			conn.Close()
		}
	}()

	checker := NewTCPChecker()
	target := parseTCPListener(listener)

	result := checker.Check(context.Background(), target)

	if !result.Healthy {
		t.Errorf("expected healthy, got error: %v", result.Error)
	}
	if result.Latency <= 0 {
		t.Error("latency should be positive")
	}
	if result.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

func TestTCPChecker_ConnectionRefused(t *testing.T) {
	checker := NewTCPChecker()
	target := Target{
		Address: "127.0.0.1",
		Port:    59998, // Unlikely to be in use
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := checker.Check(ctx, target)

	if result.Healthy {
		t.Error("expected unhealthy for connection refused")
	}
	if result.Error == nil {
		t.Error("expected error for connection refused")
	}
	if result.Latency <= 0 {
		t.Error("latency should be positive even for failed checks")
	}
}

func TestTCPChecker_Timeout(t *testing.T) {
	// The connect step stalls until the dial context ends, like a connect to
	// a host that silently drops packets.
	var dials atomic.Int32
	checker := NewTCPChecker(WithDialer(stalledDialer(&dials)))
	target := Target{
		Address: "127.0.0.1",
		Port:    80,
	}

	// Use a very short timeout. The clock starts before the context is
	// created so that elapsed can never be shorter than the timeout.
	const timeout = 100 * time.Millisecond
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result := checker.Check(ctx, target)
	elapsed := time.Since(start)

	if result.Healthy {
		t.Error("expected unhealthy for timeout")
	}
	if result.Error == nil {
		t.Fatal("expected error for timeout")
	}
	var netErr net.Error
	if !errors.As(result.Error, &netErr) || !netErr.Timeout() {
		t.Errorf("expected a timeout error, got: %v", result.Error)
	}
	if got := dials.Load(); got != 1 {
		t.Errorf("expected exactly 1 connection attempt, got %d", got)
	}
	if elapsed < timeout {
		t.Errorf("check returned after %v, before the %v context timeout", elapsed, timeout)
	}
	// Should have timed out around 100ms. A check that ignores the context
	// only returns when the stalled dial gives up after stalledDialLimit.
	if elapsed > stalledDialLimit/2 {
		t.Errorf("check took too long (%v), context timeout may not be working", elapsed)
	}
	if result.Latency <= 0 {
		t.Error("latency should be positive even for failed checks")
	}
}

func TestTCPChecker_UnreachableHost(t *testing.T) {
	// The connect step fails the way connect(2) does when there is no route
	// to the host or a router answers with ICMP host unreachable.
	var dials atomic.Int32
	checker := NewTCPChecker(WithDialer(failingDialer(&dials, syscall.EHOSTUNREACH)))
	target := Target{
		Address: "127.0.0.1",
		Port:    80,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result := checker.Check(ctx, target)

	if result.Healthy {
		t.Error("expected unhealthy for unreachable host")
	}
	if result.Error == nil {
		t.Fatal("expected error for unreachable host")
	}
	if !errors.Is(result.Error, syscall.EHOSTUNREACH) {
		t.Errorf("expected the error to wrap EHOSTUNREACH, got: %v", result.Error)
	}
	if got := dials.Load(); got != 1 {
		t.Errorf("expected exactly 1 connection attempt, got %d", got)
	}
	if result.Latency <= 0 {
		t.Error("latency should be positive even for failed checks")
	}
}

func TestTCPChecker_Type(t *testing.T) {
	checker := NewTCPChecker()
	if checker.Type() != "tcp" {
		t.Errorf("Type() = %q, want %q", checker.Type(), "tcp")
	}
}

func TestTCPChecker_WithCustomDialer(t *testing.T) {
	customDialer := &net.Dialer{
		Timeout:   1 * time.Second,
		KeepAlive: -1,
	}

	checker := NewTCPChecker(WithDialer(customDialer))

	if checker.dialer != customDialer {
		t.Error("custom dialer was not set")
	}
}

func TestTCPChecker_ConcurrentChecks(t *testing.T) {
	// Start a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	checker := NewTCPChecker()
	target := parseTCPListener(listener)

	// Run concurrent checks
	const numChecks = 50
	results := make(chan Result, numChecks)

	for i := 0; i < numChecks; i++ {
		go func() {
			results <- checker.Check(context.Background(), target)
		}()
	}

	// Collect results
	healthyCount := 0
	for i := 0; i < numChecks; i++ {
		result := <-results
		if result.Healthy {
			healthyCount++
		}
	}

	if healthyCount != numChecks {
		t.Errorf("expected all %d checks to be healthy, got %d", numChecks, healthyCount)
	}
}

func TestTCPChecker_IPv6(t *testing.T) {
	// Try to listen on IPv6 loopback
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 not available on this system")
	}
	defer listener.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	checker := NewTCPChecker()

	// Parse the listener address - for IPv6, we need just the IP without brackets
	addr := listener.Addr().(*net.TCPAddr)
	target := Target{
		Address: addr.IP.String(), // This returns "::1" without brackets
		Port:    addr.Port,
	}

	result := checker.Check(context.Background(), target)

	if !result.Healthy {
		t.Errorf("expected healthy for IPv6 connection, got error: %v", result.Error)
	}
}

func TestTCPChecker_ContextCancellation(t *testing.T) {
	// The stalled dialer keeps the check off the network. A checker that
	// ignored the canceled context would stall instead of returning at once.
	var dials atomic.Int32
	checker := NewTCPChecker(WithDialer(stalledDialer(&dials)))
	target := Target{
		Address: "127.0.0.1",
		Port:    80,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	start := time.Now()
	result := checker.Check(ctx, target)
	elapsed := time.Since(start)

	if result.Healthy {
		t.Error("expected unhealthy for canceled context")
	}
	if result.Error == nil {
		t.Fatal("expected error for canceled context")
	}
	if !errors.Is(result.Error, context.Canceled) {
		t.Errorf("expected the error to wrap context.Canceled, got: %v", result.Error)
	}
	if elapsed > stalledDialLimit/2 {
		t.Errorf("check took too long (%v), context cancellation may not be working", elapsed)
	}
}

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"127.0.0.1", false},
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"::1", true},
		{"fe80::1", true},
		{"2001:db8::1", true},
		{"::ffff:192.168.1.1", false}, // IPv4-mapped IPv6 - Go treats as IPv4 (To4() returns non-nil)
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := isIPv6(tt.address)
			if got != tt.want {
				t.Errorf("isIPv6(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Integration with Manager Tests
// =============================================================================

func TestManager_WithTCPChecker(t *testing.T) {
	// Start a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)

	checker := NewTCPChecker()
	cfg := DefaultManagerConfig()
	cfg.PassThreshold = 1
	cfg.DefaultInterval = 50 * time.Millisecond
	mgr := NewManager(checker, cfg)

	err = mgr.AddServer(ServerConfig{
		Address: addr.IP.String(),
		Port:    addr.Port,
	})
	if err != nil {
		t.Fatalf("AddServer failed: %v", err)
	}

	if err := mgr.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer mgr.Stop()

	// Wait for health check to run
	time.Sleep(100 * time.Millisecond)

	if !mgr.IsHealthy(addr.IP.String(), addr.Port) {
		t.Error("server should be healthy")
	}
}

func TestManager_TCPChecker_ServerGoesDown(t *testing.T) {
	// Start a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}

	addr := listener.Addr().(*net.TCPAddr)

	// Accept connections initially
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	checker := NewTCPChecker()
	cfg := DefaultManagerConfig()
	cfg.PassThreshold = 1
	cfg.FailThreshold = 2
	cfg.DefaultInterval = 30 * time.Millisecond
	mgr := NewManager(checker, cfg)

	mgr.AddServer(ServerConfig{
		Address: addr.IP.String(),
		Port:    addr.Port,
	})
	mgr.Start()
	defer mgr.Stop()

	// Wait for healthy
	time.Sleep(50 * time.Millisecond)
	if !mgr.IsHealthy(addr.IP.String(), addr.Port) {
		t.Fatal("server should be healthy initially")
	}

	// Close listener - server goes down
	listener.Close()
	<-acceptDone

	// Wait for enough failed checks
	time.Sleep(150 * time.Millisecond)

	if mgr.IsHealthy(addr.IP.String(), addr.Port) {
		t.Error("server should be unhealthy after listener closed")
	}
}

// =============================================================================
// Helpers
// =============================================================================

// stalledDialLimit bounds how long stalledDialer blocks when the dial context
// never ends, so a checker that drops the context fails the test instead of
// hanging it.
const stalledDialLimit = 10 * time.Second

// stalledDialer returns a real net.Dialer whose connection attempts never
// complete. The ControlContext hook runs after the socket is created and
// before connect(2); it blocks until the dial context ends and returns the
// context's error, so no packet is sent. The checker still goes through
// net.Dialer.DialContext exactly as it does in production, and the outcome
// does not depend on how the host network treats an unroutable address.
// dials counts the connection attempts.
func stalledDialer(dials *atomic.Int32) *net.Dialer {
	return &net.Dialer{
		KeepAlive: -1,
		ControlContext: func(ctx context.Context, _, _ string, _ syscall.RawConn) error {
			dials.Add(1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(stalledDialLimit):
				return errors.New("dial context did not end")
			}
		},
	}
}

// failingDialer returns a real net.Dialer whose connection attempts fail with
// err before connect(2) is called, so no packet is sent. dials counts the
// connection attempts.
func failingDialer(dials *atomic.Int32, err error) *net.Dialer {
	return &net.Dialer{
		KeepAlive: -1,
		ControlContext: func(context.Context, string, string, syscall.RawConn) error {
			dials.Add(1)
			return err
		},
	}
}

func parseTCPListener(listener net.Listener) Target {
	addr := listener.Addr().(*net.TCPAddr)
	return Target{
		Address: addr.IP.String(),
		Port:    addr.Port,
	}
}
