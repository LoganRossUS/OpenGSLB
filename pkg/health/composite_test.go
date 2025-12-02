package health

import (
	"context"
	"testing"
	"time"
)

func TestCompositeChecker_Type(t *testing.T) {
	c := NewCompositeChecker()
	if c.Type() != "composite" {
		t.Errorf("Type() = %q, want %q", c.Type(), "composite")
	}
}

func TestCompositeChecker_Register(t *testing.T) {
	c := NewCompositeChecker()

	if c.HasChecker("http") {
		t.Error("should not have http checker before registration")
	}

	c.Register("http", NewHTTPChecker())

	if !c.HasChecker("http") {
		t.Error("should have http checker after registration")
	}
}

func TestCompositeChecker_RegisteredTypes(t *testing.T) {
	c := NewCompositeChecker()
	c.Register("http", NewHTTPChecker())
	c.Register("tcp", NewTCPChecker())

	types := c.RegisteredTypes()
	if len(types) != 2 {
		t.Errorf("RegisteredTypes() returned %d types, want 2", len(types))
	}

	// Check both types are present (order not guaranteed)
	found := make(map[string]bool)
	for _, typ := range types {
		found[typ] = true
	}
	if !found["http"] || !found["tcp"] {
		t.Errorf("RegisteredTypes() = %v, want http and tcp", types)
	}
}

func TestCompositeChecker_DispatchesToHTTP(t *testing.T) {
	httpChecker := &mockChecker{healthy: true}
	tcpChecker := &mockChecker{healthy: false}

	c := NewCompositeChecker()
	c.Register("http", httpChecker)
	c.Register("tcp", tcpChecker)

	// HTTP scheme should use HTTP checker
	result := c.Check(context.Background(), Target{
		Address: "127.0.0.1",
		Port:    80,
		Scheme:  "http",
	})

	if !result.Healthy {
		t.Error("expected healthy result from HTTP checker")
	}
	if httpChecker.checkCount() != 1 {
		t.Errorf("HTTP checker called %d times, want 1", httpChecker.checkCount())
	}
	if tcpChecker.checkCount() != 0 {
		t.Errorf("TCP checker called %d times, want 0", tcpChecker.checkCount())
	}
}

func TestCompositeChecker_DispatchesToTCP(t *testing.T) {
	httpChecker := &mockChecker{healthy: false}
	tcpChecker := &mockChecker{healthy: true}

	c := NewCompositeChecker()
	c.Register("http", httpChecker)
	c.Register("tcp", tcpChecker)

	// TCP scheme should use TCP checker
	result := c.Check(context.Background(), Target{
		Address: "127.0.0.1",
		Port:    3306,
		Scheme:  "tcp",
	})

	if !result.Healthy {
		t.Error("expected healthy result from TCP checker")
	}
	if tcpChecker.checkCount() != 1 {
		t.Errorf("TCP checker called %d times, want 1", tcpChecker.checkCount())
	}
	if httpChecker.checkCount() != 0 {
		t.Errorf("HTTP checker called %d times, want 0", httpChecker.checkCount())
	}
}

func TestCompositeChecker_EmptySchemeUsesHTTP(t *testing.T) {
	httpChecker := &mockChecker{healthy: true}

	c := NewCompositeChecker()
	c.Register("http", httpChecker)

	// Empty scheme should default to HTTP
	result := c.Check(context.Background(), Target{
		Address: "127.0.0.1",
		Port:    80,
		Scheme:  "", // Empty
	})

	if !result.Healthy {
		t.Error("expected healthy result from HTTP checker")
	}
	if httpChecker.checkCount() != 1 {
		t.Errorf("HTTP checker called %d times, want 1", httpChecker.checkCount())
	}
}

func TestCompositeChecker_HTTPSUsesHTTPChecker(t *testing.T) {
	httpChecker := &mockChecker{healthy: true}

	c := NewCompositeChecker()
	c.Register("http", httpChecker)

	// HTTPS scheme should use HTTP checker
	result := c.Check(context.Background(), Target{
		Address: "127.0.0.1",
		Port:    443,
		Scheme:  "https",
	})

	if !result.Healthy {
		t.Error("expected healthy result from HTTP checker")
	}
	if httpChecker.checkCount() != 1 {
		t.Errorf("HTTP checker called %d times, want 1", httpChecker.checkCount())
	}
}

func TestCompositeChecker_UnregisteredType(t *testing.T) {
	c := NewCompositeChecker()
	// Don't register anything

	result := c.Check(context.Background(), Target{
		Address: "127.0.0.1",
		Port:    80,
		Scheme:  "http",
	})

	if result.Healthy {
		t.Error("expected unhealthy result for unregistered type")
	}
	if result.Error == nil {
		t.Error("expected error for unregistered type")
	}
}

func TestCompositeChecker_UnknownScheme(t *testing.T) {
	c := NewCompositeChecker()
	c.Register("http", &mockChecker{healthy: true})

	// Unknown scheme that doesn't map to any registered checker
	result := c.Check(context.Background(), Target{
		Address: "127.0.0.1",
		Port:    80,
		Scheme:  "unknown",
	})

	if result.Healthy {
		t.Error("expected unhealthy result for unknown scheme")
	}
	if result.Error == nil {
		t.Error("expected error for unknown scheme")
	}
}

func TestCompositeChecker_PassesContext(t *testing.T) {
	// Create a checker that respects context cancellation
	checker := &contextAwareChecker{}

	c := NewCompositeChecker()
	c.Register("http", checker)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	c.Check(ctx, Target{
		Address: "127.0.0.1",
		Port:    80,
		Scheme:  "http",
	})

	if checker.lastCtx == nil {
		t.Error("context was not passed to checker")
	}
	if checker.lastCtx.Err() == nil {
		t.Error("canceled context should have error")
	}
}

func TestCompositeChecker_PassesTarget(t *testing.T) {
	checker := &targetCapturingChecker{}

	c := NewCompositeChecker()
	c.Register("http", checker)

	target := Target{
		Address: "192.168.1.100",
		Port:    8080,
		Path:    "/healthz",
		Scheme:  "http",
		Timeout: 5 * time.Second,
	}

	c.Check(context.Background(), target)

	if checker.lastTarget.Address != target.Address {
		t.Errorf("Address = %q, want %q", checker.lastTarget.Address, target.Address)
	}
	if checker.lastTarget.Port != target.Port {
		t.Errorf("Port = %d, want %d", checker.lastTarget.Port, target.Port)
	}
	if checker.lastTarget.Path != target.Path {
		t.Errorf("Path = %q, want %q", checker.lastTarget.Path, target.Path)
	}
}

// =============================================================================
// Helper mocks for composite tests
// =============================================================================

type contextAwareChecker struct {
	lastCtx context.Context
}

func (c *contextAwareChecker) Check(ctx context.Context, target Target) Result {
	c.lastCtx = ctx
	return Result{Healthy: true, Timestamp: time.Now()}
}

func (c *contextAwareChecker) Type() string { return "context-aware" }

type targetCapturingChecker struct {
	lastTarget Target
}

func (c *targetCapturingChecker) Check(ctx context.Context, target Target) Result {
	c.lastTarget = target
	return Result{Healthy: true, Timestamp: time.Now()}
}

func (c *targetCapturingChecker) Type() string { return "target-capturing" }
