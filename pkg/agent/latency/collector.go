// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package latency

import (
	"context"
	"fmt"
	"runtime"
)

// Collector reads TCP RTT data from the operating system.
// Implementations are platform-specific (Linux, Windows).
type Collector interface {
	// Start begins collecting RTT data.
	Start(ctx context.Context) error

	// Observations returns a channel of raw RTT observations.
	// Each observation is for a single connection at poll time.
	Observations() <-chan Observation

	// Close stops the collector and releases resources.
	Close() error
}

// ErrPlatformNotSupported is returned when latency collection isn't available.
var ErrPlatformNotSupported = fmt.Errorf("latency collection not supported on %s", runtime.GOOS)

// ErrInsufficientPrivileges is returned when the process lacks required privileges.
var ErrInsufficientPrivileges = fmt.Errorf("insufficient privileges for latency collection")

// New creates the appropriate collector for the current OS.
// Returns nil, nil if the OS doesn't support latency collection but that's acceptable.
// Returns nil, error if collection is supported but failed to initialize.
func New(cfg CollectorConfig) (Collector, error) {
	// Apply defaults
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultCollectorConfig().PollInterval
	}
	if cfg.MinConnectionAge == 0 {
		cfg.MinConnectionAge = DefaultCollectorConfig().MinConnectionAge
	}

	return newPlatformCollector(cfg)
}

// newPlatformCollector is implemented per-platform in linux.go and windows.go.
// For unsupported platforms, collector_other.go provides a stub.
