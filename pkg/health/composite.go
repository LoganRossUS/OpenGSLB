package health

import (
	"context"
	"fmt"
	"time"
)

// CompositeChecker dispatches health checks to the appropriate checker based on
// the target's scheme. This allows a single Manager to handle multiple check types.
type CompositeChecker struct {
	checkers map[string]Checker
}

// NewCompositeChecker creates a checker that can handle multiple check types.
func NewCompositeChecker() *CompositeChecker {
	return &CompositeChecker{
		checkers: make(map[string]Checker),
	}
}

// Register adds a checker for a specific type (e.g., "http", "tcp").
func (c *CompositeChecker) Register(checkType string, checker Checker) {
	c.checkers[checkType] = checker
}

// Type returns "composite".
func (c *CompositeChecker) Type() string {
	return "composite"
}

// Check dispatches to the appropriate checker based on the target's Scheme field.
//
// Scheme mapping:
//   - "http", "https", "" -> HTTP checker
//   - "tcp" -> TCP checker
//
// If no matching checker is found, returns an error result.
func (c *CompositeChecker) Check(ctx context.Context, target Target) Result {
	start := time.Now()

	// Determine which checker to use based on scheme
	checkType := target.Scheme

	// Map schemes to checker types
	switch checkType {
	case "", "http", "https":
		checkType = "http" // HTTP checker handles both http and https
	case "tcp":
		checkType = "tcp"
	}

	checker, ok := c.checkers[checkType]
	if !ok {
		return Result{
			Healthy:   false,
			Error:     fmt.Errorf("no checker registered for type: %s", target.Scheme),
			Timestamp: start,
			Latency:   time.Since(start),
		}
	}

	return checker.Check(ctx, target)
}

// HasChecker returns true if a checker is registered for the given type.
func (c *CompositeChecker) HasChecker(checkType string) bool {
	_, ok := c.checkers[checkType]
	return ok
}

// RegisteredTypes returns a list of registered checker types.
func (c *CompositeChecker) RegisteredTypes() []string {
	types := make([]string, 0, len(c.checkers))
	for t := range c.checkers {
		types = append(types, t)
	}
	return types
}
