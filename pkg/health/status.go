// Package health provides health checking functionality for backend servers.
package health

import (
	"sync"
	"time"
)

// Status represents the health state of a server.
type Status int

const (
	StatusUnknown Status = iota
	StatusHealthy
	StatusUnhealthy
)

func (s Status) String() string {
	switch s {
	case StatusHealthy:
		return "healthy"
	case StatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// Result represents the outcome of a single health check.
type Result struct {
	Healthy   bool
	Latency   time.Duration
	Error     error
	Timestamp time.Time
}

// ServerHealth tracks the health state of a single server.
type ServerHealth struct {
	mu sync.RWMutex

	address           string
	status            Status
	lastCheck         time.Time
	lastHealthy       time.Time
	consecutiveFails  int
	consecutivePasses int
	lastError         error

	// Thresholds for status transitions
	failThreshold int
	passThreshold int
}

// NewServerHealth creates a new ServerHealth tracker.
func NewServerHealth(address string, failThreshold, passThreshold int) *ServerHealth {
	if failThreshold < 1 {
		failThreshold = 3
	}
	if passThreshold < 1 {
		passThreshold = 2
	}
	return &ServerHealth{
		address:       address,
		status:        StatusUnknown,
		failThreshold: failThreshold,
		passThreshold: passThreshold,
	}
}

// Address returns the server address.
func (sh *ServerHealth) Address() string {
	return sh.address
}

// Status returns the current health status.
func (sh *ServerHealth) Status() Status {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.status
}

// IsHealthy returns true if the server is currently healthy.
func (sh *ServerHealth) IsHealthy() bool {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.status == StatusHealthy
}

// LastCheck returns the timestamp of the last health check.
func (sh *ServerHealth) LastCheck() time.Time {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.lastCheck
}

// LastHealthy returns the timestamp when the server was last healthy.
func (sh *ServerHealth) LastHealthy() time.Time {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.lastHealthy
}

// LastError returns the most recent error, if any.
func (sh *ServerHealth) LastError() error {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.lastError
}

// RecordResult updates the health state based on a check result.
// Returns true if the status changed.
func (sh *ServerHealth) RecordResult(result Result) bool {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	sh.lastCheck = result.Timestamp
	previousStatus := sh.status

	if result.Healthy {
		sh.consecutiveFails = 0
		sh.consecutivePasses++
		sh.lastError = nil
		sh.lastHealthy = result.Timestamp

		// Transition to healthy after passThreshold consecutive successes
		if sh.status != StatusHealthy && sh.consecutivePasses >= sh.passThreshold {
			sh.status = StatusHealthy
		}
	} else {
		sh.consecutivePasses = 0
		sh.consecutiveFails++
		sh.lastError = result.Error

		// Transition to unhealthy after failThreshold consecutive failures
		if sh.status != StatusUnhealthy && sh.consecutiveFails >= sh.failThreshold {
			sh.status = StatusUnhealthy
		}
	}

	return sh.status != previousStatus
}

// Snapshot returns a point-in-time copy of the health state.
type Snapshot struct {
	Address           string
	Status            Status
	LastCheck         time.Time
	LastHealthy       time.Time
	ConsecutiveFails  int
	ConsecutivePasses int
	LastError         error
}

// Snapshot returns a point-in-time copy of the health state.
func (sh *ServerHealth) Snapshot() Snapshot {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return Snapshot{
		Address:           sh.address,
		Status:            sh.status,
		LastCheck:         sh.lastCheck,
		LastHealthy:       sh.lastHealthy,
		ConsecutiveFails:  sh.consecutiveFails,
		ConsecutivePasses: sh.consecutivePasses,
		LastError:         sh.lastError,
	}
}
