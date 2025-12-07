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

package routing

import (
	"context"
	"sync"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
)

// RoundRobin implements a round-robin server selection algorithm.
// It maintains per-domain rotation state to ensure even distribution
// of requests across healthy servers for each domain independently.
type RoundRobin struct {
	mu      sync.Mutex
	indices map[string]uint64
}

// NewRoundRobin creates a new round-robin router.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{
		indices: make(map[string]uint64),
	}
}

// Route selects the next server from the pool using round-robin rotation.
// Each domain maintains its own rotation index, ensuring that queries for
// different domains do not affect each other's distribution.
//
// Returns ErrNoHealthyServers if the servers slice is empty.
func (r *RoundRobin) Route(_ context.Context, domain string, servers []dns.ServerInfo) (*dns.ServerInfo, error) {
	if len(servers) == 0 {
		return nil, ErrNoHealthyServers
	}

	r.mu.Lock()
	idx := r.indices[domain] % uint64(len(servers))
	r.indices[domain]++
	r.mu.Unlock()

	return &servers[idx], nil
}

// Algorithm returns the name of this routing algorithm.
func (r *RoundRobin) Algorithm() string {
	return "round-robin"
}
