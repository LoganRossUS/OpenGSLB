// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
)

// FailoverRouter implements active/standby failover selection.
// It always selects the first available server in the pool,
// providing predictable failover ordering.
type FailoverRouter struct{}

// NewFailoverRouter creates a new failover router.
func NewFailoverRouter() *FailoverRouter {
	return &FailoverRouter{}
}

// Route selects the first server in the pool (active/standby).
// The pool should be ordered by priority (primary first).
func (r *FailoverRouter) Route(ctx context.Context, pool ServerPool) (*Server, error) {
	servers := pool.Servers()
	if len(servers) == 0 {
		return nil, ErrNoHealthyServers
	}

	// Always return the first (highest priority) server
	return servers[0], nil
}

// Algorithm returns the algorithm name.
func (r *FailoverRouter) Algorithm() string {
	return AlgorithmFailover
}
