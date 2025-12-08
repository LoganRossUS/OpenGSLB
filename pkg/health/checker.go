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
	"time"
)

// Target represents a server to health check.
type Target struct {
	Address string
	Port    int

	// HTTP-specific fields
	Path   string
	Scheme string // "http" or "https"
	Host   string // Host header for HTTPS (for TLS SNI and certificate validation)

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
