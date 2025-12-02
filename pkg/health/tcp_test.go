package health

import (
	"context"
	"net"
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
	// Use a non-routable IP that will cause connection to hang/timeout
	// 10.255.255.1 is in TEST-NET range and should not be routable
	checker := NewTCPChecker()
	target := Target{
		Address: "10.255.255.1",
		Port:    80,
	}

	// Use a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := checker.Check(ctx, target)
	elapsed := time.Since(start)

	if result.Healthy {
		t.Error("expected unhealthy for timeout")
	}
	if result.Error == nil {
		t.Error("expected error for timeout")
	}
	// Should have timed out around 100ms, not hung for seconds
	if elapsed > 500*time.Millisecond {
		t.Errorf("check took too long (%v), context timeout may not be working", elapsed)
	}
}

func TestTCPChecker_UnreachableHost(t *testing.T) {
	checker := NewTCPChecker()
	// Use a non-routable IP to simulate unreachable host
	target := Target{
		Address: "10.255.255.1", // RFC 5737 - TEST-NET, should be unreachable
		Port:    80,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result := checker.Check(ctx, target)

	if result.Healthy {
		t.Error("expected unhealthy for unreachable host")
	}
	if result.Error == nil {
		t.Error("expected error for unreachable host")
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
	checker := NewTCPChecker()
	target := Target{
		Address: "10.255.255.1", // Non-routable
		Port:    80,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	result := checker.Check(ctx, target)

	if result.Healthy {
		t.Error("expected unhealthy for canceled context")
	}
	if result.Error == nil {
		t.Error("expected error for canceled context")
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

func parseTCPListener(listener net.Listener) Target {
	addr := listener.Addr().(*net.TCPAddr)
	return Target{
		Address: addr.IP.String(),
		Port:    addr.Port,
	}
}
