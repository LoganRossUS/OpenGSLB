package health

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Status Tests
// =============================================================================

func TestStatus_String(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusUnknown, "unknown"},
		{StatusHealthy, "healthy"},
		{StatusUnhealthy, "unhealthy"},
		{Status(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// =============================================================================
// ServerHealth Tests
// =============================================================================

func TestNewServerHealth(t *testing.T) {
	sh := NewServerHealth("10.0.0.1:80", 3, 2)

	if sh.Address() != "10.0.0.1:80" {
		t.Errorf("Address() = %q, want %q", sh.Address(), "10.0.0.1:80")
	}
	if sh.Status() != StatusUnknown {
		t.Errorf("initial Status() = %v, want StatusUnknown", sh.Status())
	}
	if sh.IsHealthy() {
		t.Error("IsHealthy() = true for new server, want false")
	}
}

func TestNewServerHealth_DefaultThresholds(t *testing.T) {
	// Zero or negative thresholds should default
	sh := NewServerHealth("test:80", 0, -1)

	// Record enough passes to trigger healthy with default thresholds
	for i := 0; i < 3; i++ {
		sh.RecordResult(Result{Healthy: true, Timestamp: time.Now()})
	}

	if !sh.IsHealthy() {
		t.Error("should be healthy after sufficient passes with default thresholds")
	}
}

func TestServerHealth_TransitionToHealthy(t *testing.T) {
	sh := NewServerHealth("test:80", 3, 2)

	// First pass - still unknown
	changed := sh.RecordResult(Result{Healthy: true, Timestamp: time.Now()})
	if sh.Status() != StatusUnknown {
		t.Errorf("after 1 pass: Status() = %v, want StatusUnknown", sh.Status())
	}
	if changed {
		t.Error("status should not have changed after 1 pass")
	}

	// Second pass - becomes healthy (passThreshold = 2)
	changed = sh.RecordResult(Result{Healthy: true, Timestamp: time.Now()})
	if sh.Status() != StatusHealthy {
		t.Errorf("after 2 passes: Status() = %v, want StatusHealthy", sh.Status())
	}
	if !changed {
		t.Error("status should have changed after reaching passThreshold")
	}
}

func TestServerHealth_TransitionToUnhealthy(t *testing.T) {
	sh := NewServerHealth("test:80", 3, 2)

	// First, get to healthy state
	for i := 0; i < 2; i++ {
		sh.RecordResult(Result{Healthy: true, Timestamp: time.Now()})
	}
	if !sh.IsHealthy() {
		t.Fatal("prerequisite: should be healthy")
	}

	// Now fail repeatedly
	testErr := errors.New("connection refused")
	for i := 0; i < 2; i++ {
		changed := sh.RecordResult(Result{Healthy: false, Error: testErr, Timestamp: time.Now()})
		if sh.Status() != StatusHealthy {
			t.Errorf("after %d fails: should still be healthy", i+1)
		}
		if changed {
			t.Errorf("after %d fails: status should not have changed yet", i+1)
		}
	}

	// Third failure triggers unhealthy (failThreshold = 3)
	changed := sh.RecordResult(Result{Healthy: false, Error: testErr, Timestamp: time.Now()})
	if sh.Status() != StatusUnhealthy {
		t.Error("after 3 fails: should be unhealthy")
	}
	if !changed {
		t.Error("status should have changed after reaching failThreshold")
	}
	if sh.LastError() != testErr {
		t.Error("LastError should be set")
	}
}

func TestServerHealth_SuccessResetsFailCount(t *testing.T) {
	sh := NewServerHealth("test:80", 3, 2)

	// Get healthy first
	for i := 0; i < 2; i++ {
		sh.RecordResult(Result{Healthy: true, Timestamp: time.Now()})
	}

	// Two failures
	sh.RecordResult(Result{Healthy: false, Timestamp: time.Now()})
	sh.RecordResult(Result{Healthy: false, Timestamp: time.Now()})

	// A success should reset the fail count
	sh.RecordResult(Result{Healthy: true, Timestamp: time.Now()})

	// Two more failures shouldn't trigger unhealthy
	sh.RecordResult(Result{Healthy: false, Timestamp: time.Now()})
	sh.RecordResult(Result{Healthy: false, Timestamp: time.Now()})

	if sh.Status() != StatusHealthy {
		t.Error("should still be healthy - success reset the fail count")
	}
}

func TestServerHealth_Snapshot(t *testing.T) {
	sh := NewServerHealth("test:80", 3, 2)
	now := time.Now()

	sh.RecordResult(Result{Healthy: true, Timestamp: now})
	sh.RecordResult(Result{Healthy: true, Timestamp: now.Add(time.Second)})

	snap := sh.Snapshot()

	if snap.Address != "test:80" {
		t.Errorf("Snapshot.Address = %q, want %q", snap.Address, "test:80")
	}
	if snap.Status != StatusHealthy {
		t.Errorf("Snapshot.Status = %v, want StatusHealthy", snap.Status)
	}
	if snap.ConsecutivePasses != 2 {
		t.Errorf("Snapshot.ConsecutivePasses = %d, want 2", snap.ConsecutivePasses)
	}
}

func TestServerHealth_Concurrency(t *testing.T) {
	sh := NewServerHealth("test:80", 3, 2)
	var wg sync.WaitGroup

	// Concurrent reads and writes
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			sh.RecordResult(Result{Healthy: true, Timestamp: time.Now()})
		}()
		go func() {
			defer wg.Done()
			_ = sh.IsHealthy()
			_ = sh.Status()
			_ = sh.Snapshot()
		}()
	}
	wg.Wait()
}

// =============================================================================
// HTTPChecker Tests
// =============================================================================

func TestHTTPChecker_HealthyServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := NewHTTPChecker()
	target := parseTestServer(server, "/health")

	result := checker.Check(context.Background(), target)

	if !result.Healthy {
		t.Errorf("expected healthy, got error: %v", result.Error)
	}
	if result.Latency <= 0 {
		t.Error("latency should be positive")
	}
}

func TestHTTPChecker_UnhealthyServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	checker := NewHTTPChecker()
	target := parseTestServer(server, "/health")

	result := checker.Check(context.Background(), target)

	if result.Healthy {
		t.Error("expected unhealthy for 503 response")
	}
	if result.Error == nil {
		t.Error("expected error for unhealthy response")
	}
}

func TestHTTPChecker_ConnectionRefused(t *testing.T) {
	checker := NewHTTPChecker()
	target := Target{
		Address: "127.0.0.1",
		Port:    59999, // Unlikely to be in use
		Path:    "/health",
		Scheme:  "http",
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
}

func TestHTTPChecker_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Longer than our timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := NewHTTPChecker()
	target := parseTestServer(server, "/health")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := checker.Check(ctx, target)

	if result.Healthy {
		t.Error("expected unhealthy for timeout")
	}
}

func TestHTTPChecker_CustomStatusCodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // 418
	}))
	defer server.Close()

	checker := NewHTTPChecker(WithValidStatusCodes(418))
	target := parseTestServer(server, "/health")

	result := checker.Check(context.Background(), target)

	if !result.Healthy {
		t.Errorf("expected healthy for custom status code 418, got error: %v", result.Error)
	}
}

func TestHTTPChecker_NoRedirect(t *testing.T) {
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/other", http.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	checker := NewHTTPChecker() // Default: no redirects
	target := parseTestServer(redirectServer, "/health")

	result := checker.Check(context.Background(), target)

	// 301 is not in 2xx range, so should be unhealthy
	if result.Healthy {
		t.Error("expected unhealthy for redirect when not following redirects")
	}
}

func TestHTTPChecker_Type(t *testing.T) {
	checker := NewHTTPChecker()
	if checker.Type() != "http" {
		t.Errorf("Type() = %q, want %q", checker.Type(), "http")
	}
}

// =============================================================================
// Manager Tests
// =============================================================================

func TestManager_AddRemoveServer(t *testing.T) {
	checker := &mockChecker{healthy: true}
	mgr := NewManager(checker, DefaultManagerConfig())

	err := mgr.AddServer(ServerConfig{Address: "10.0.0.1", Port: 80})
	if err != nil {
		t.Fatalf("AddServer failed: %v", err)
	}

	if mgr.ServerCount() != 1 {
		t.Errorf("ServerCount() = %d, want 1", mgr.ServerCount())
	}

	// Duplicate add should fail
	err = mgr.AddServer(ServerConfig{Address: "10.0.0.1", Port: 80})
	if err == nil {
		t.Error("duplicate AddServer should fail")
	}

	// Remove
	err = mgr.RemoveServer("10.0.0.1", 80)
	if err != nil {
		t.Fatalf("RemoveServer failed: %v", err)
	}

	if mgr.ServerCount() != 0 {
		t.Errorf("ServerCount() = %d after remove, want 0", mgr.ServerCount())
	}

	// Remove non-existent should fail
	err = mgr.RemoveServer("10.0.0.1", 80)
	if err == nil {
		t.Error("RemoveServer of non-existent should fail")
	}
}

func TestManager_StartStop(t *testing.T) {
	checker := &mockChecker{healthy: true, delay: 10 * time.Millisecond}
	cfg := DefaultManagerConfig()
	cfg.DefaultInterval = 50 * time.Millisecond
	mgr := NewManager(checker, cfg)

	mgr.AddServer(ServerConfig{Address: "10.0.0.1", Port: 80})

	if err := mgr.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start should fail
	if err := mgr.Start(); err == nil {
		t.Error("double Start should fail")
	}

	// Wait for some checks to run
	time.Sleep(150 * time.Millisecond)

	if checker.checkCount() == 0 {
		t.Error("expected checks to have run")
	}

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Stop should be idempotent
	if err := mgr.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestManager_IsHealthy(t *testing.T) {
	checker := &mockChecker{healthy: true}
	cfg := DefaultManagerConfig()
	cfg.PassThreshold = 1 // Immediate healthy
	cfg.DefaultInterval = 50 * time.Millisecond
	mgr := NewManager(checker, cfg)

	mgr.AddServer(ServerConfig{Address: "10.0.0.1", Port: 80})
	mgr.Start()
	defer mgr.Stop()

	// Wait for initial check
	time.Sleep(100 * time.Millisecond)

	if !mgr.IsHealthy("10.0.0.1", 80) {
		t.Error("server should be healthy")
	}

	// Non-existent server
	if mgr.IsHealthy("10.0.0.2", 80) {
		t.Error("non-existent server should not be healthy")
	}
}

func TestManager_HealthTransitionCallback(t *testing.T) {
	checker := &mockChecker{healthy: true}
	cfg := DefaultManagerConfig()
	cfg.PassThreshold = 1
	cfg.DefaultInterval = 50 * time.Millisecond
	mgr := NewManager(checker, cfg)

	var callbackCalled atomic.Bool
	var callbackAddr atomic.Value
	var callbackStatus atomic.Int32

	mgr.OnStatusChange(func(addr string, status Status) {
		callbackCalled.Store(true)
		callbackAddr.Store(addr)
		callbackStatus.Store(int32(status))
	})

	mgr.AddServer(ServerConfig{Address: "10.0.0.1", Port: 80})
	mgr.Start()
	defer mgr.Stop()

	// Wait for check and callback
	time.Sleep(100 * time.Millisecond)

	if !callbackCalled.Load() {
		t.Error("callback should have been called on status change")
	}
	if addr, _ := callbackAddr.Load().(string); addr != "10.0.0.1:80" {
		t.Errorf("callback address = %q, want %q", addr, "10.0.0.1:80")
	}
	if Status(callbackStatus.Load()) != StatusHealthy {
		t.Errorf("callback status = %v, want StatusHealthy", Status(callbackStatus.Load()))
	}
}

func TestManager_GetHealthyServers(t *testing.T) {
	checker := &mockChecker{healthy: true}
	cfg := DefaultManagerConfig()
	cfg.PassThreshold = 1
	cfg.DefaultInterval = 50 * time.Millisecond
	mgr := NewManager(checker, cfg)

	mgr.AddServer(ServerConfig{Address: "10.0.0.1", Port: 80})
	mgr.AddServer(ServerConfig{Address: "10.0.0.2", Port: 80})
	mgr.Start()
	defer mgr.Stop()

	time.Sleep(100 * time.Millisecond)

	healthy := mgr.GetHealthyServers()
	if len(healthy) != 2 {
		t.Errorf("GetHealthyServers() returned %d servers, want 2", len(healthy))
	}
}

func TestManager_GetStatus(t *testing.T) {
	checker := &mockChecker{healthy: true}
	cfg := DefaultManagerConfig()
	cfg.PassThreshold = 1
	cfg.DefaultInterval = 50 * time.Millisecond
	mgr := NewManager(checker, cfg)

	mgr.AddServer(ServerConfig{Address: "10.0.0.1", Port: 80, Path: "/health"})
	mgr.Start()
	defer mgr.Stop()

	time.Sleep(100 * time.Millisecond)

	snap, ok := mgr.GetStatus("10.0.0.1", 80)
	if !ok {
		t.Fatal("GetStatus returned not ok")
	}
	if snap.Status != StatusHealthy {
		t.Errorf("snapshot status = %v, want StatusHealthy", snap.Status)
	}

	// Non-existent
	_, ok = mgr.GetStatus("10.0.0.99", 80)
	if ok {
		t.Error("GetStatus should return false for non-existent server")
	}
}

func TestManager_AddServerWhileRunning(t *testing.T) {
	checker := &mockChecker{healthy: true}
	cfg := DefaultManagerConfig()
	cfg.PassThreshold = 1
	cfg.DefaultInterval = 50 * time.Millisecond
	mgr := NewManager(checker, cfg)

	mgr.Start()
	defer mgr.Stop()

	// Add server after start
	mgr.AddServer(ServerConfig{Address: "10.0.0.1", Port: 80})

	time.Sleep(100 * time.Millisecond)

	if !mgr.IsHealthy("10.0.0.1", 80) {
		t.Error("server added while running should become healthy")
	}
}

func TestManager_GetAllStatus(t *testing.T) {
	checker := &mockChecker{healthy: true}
	cfg := DefaultManagerConfig()
	cfg.PassThreshold = 1
	cfg.DefaultInterval = 50 * time.Millisecond
	mgr := NewManager(checker, cfg)

	mgr.AddServer(ServerConfig{Address: "10.0.0.1", Port: 80})
	mgr.AddServer(ServerConfig{Address: "10.0.0.2", Port: 8080})
	mgr.Start()
	defer mgr.Stop()

	time.Sleep(100 * time.Millisecond)

	all := mgr.GetAllStatus()
	if len(all) != 2 {
		t.Errorf("GetAllStatus returned %d snapshots, want 2", len(all))
	}
}

// =============================================================================
// Helpers
// =============================================================================

type mockChecker struct {
	healthy bool
	delay   time.Duration
	err     error
	count   atomic.Int64
}

func (m *mockChecker) Check(ctx context.Context, target Target) Result {
	m.count.Add(1)
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return Result{
		Healthy:   m.healthy,
		Error:     m.err,
		Timestamp: time.Now(),
	}
}

func (m *mockChecker) Type() string { return "mock" }

func (m *mockChecker) checkCount() int64 {
	return m.count.Load()
}

func parseTestServer(server *httptest.Server, path string) Target {
	// Use net.SplitHostPort for reliable parsing
	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		host = "127.0.0.1"
		portStr = "80"
	}
	port, _ := strconv.Atoi(portStr)
	return Target{
		Address: host,
		Port:    port,
		Path:    path,
		Scheme:  "http",
	}
}
