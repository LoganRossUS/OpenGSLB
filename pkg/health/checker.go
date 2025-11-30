package health

import (
	"context"
	"time"
)

// Target represents a server to health check.
type Target struct {
	Address string
	Port    int

	// HTTP-specific fields
	Path   string
	Scheme string // "http" or "https"

	// Check configuration
	Timeout time.Duration
}

// Checker performs health checks against targets.
type Checker interface {
	// Check performs a health check against the target.
	// The context should be used for cancellation and timeout.
	Check(ctx context.Context, target Target) Result

	// Type returns the health check type (e.g., "http", "tcp").
	Type() string
}

// CheckerFunc is a function adapter for Checker interface.
type CheckerFunc func(ctx context.Context, target Target) Result

func (f CheckerFunc) Check(ctx context.Context, target Target) Result {
	return f(ctx, target)
}

func (f CheckerFunc) Type() string {
	return "func"
}
