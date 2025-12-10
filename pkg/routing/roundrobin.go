// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"sync/atomic"
)

// RoundRobinRouter implements round-robin server selection.
type RoundRobinRouter struct {
	counter uint64
}

// NewRoundRobinRouter creates a new round-robin router.
func NewRoundRobinRouter() *RoundRobinRouter {
	return &RoundRobinRouter{}
}

// Route selects the next server in round-robin order.
func (r *RoundRobinRouter) Route(ctx context.Context, pool ServerPool) (*Server, error) {
	servers := pool.Servers()
	if len(servers) == 0 {
		return nil, ErrNoHealthyServers
	}

	// Atomically increment and get the index
	idx := atomic.AddUint64(&r.counter, 1) - 1
	selected := servers[idx%uint64(len(servers))]

	return selected, nil
}

// Algorithm returns the algorithm name.
func (r *RoundRobinRouter) Algorithm() string {
	return AlgorithmRoundRobin
}
