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
	"math/rand"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
)

// Ensure Weighted implements dns.Router interface.
var _ dns.Router = (*Weighted)(nil)

// Weighted implements weighted random selection routing.
// Servers are selected with probability proportional to their weights.
// Servers with weight 0 are excluded from selection.
type Weighted struct {
	mu   sync.Mutex
	rand *rand.Rand
}

// NewWeighted creates a new weighted router.
func NewWeighted() *Weighted {
	return &Weighted{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Route selects a server from the slice using weighted random selection.
// The probability of selecting a server is proportional to its weight.
// Servers with weight 0 are excluded from selection.
//
// Returns ErrNoHealthyServers if no servers with weight > 0 are available.
func (w *Weighted) Route(_ context.Context, _ string, servers []dns.ServerInfo) (*dns.ServerInfo, error) {
	if len(servers) == 0 {
		return nil, ErrNoHealthyServers
	}

	// Filter servers with weight > 0 and calculate total weight
	var candidates []int // indices of eligible servers
	var totalWeight int

	for i, s := range servers {
		if s.Weight > 0 {
			candidates = append(candidates, i)
			totalWeight += s.Weight
		}
	}

	if len(candidates) == 0 {
		return nil, ErrNoHealthyServers
	}

	// Single candidate, return immediately
	if len(candidates) == 1 {
		return &servers[candidates[0]], nil
	}

	// Weighted random selection
	w.mu.Lock()
	target := w.rand.Intn(totalWeight)
	w.mu.Unlock()

	cumulative := 0
	for _, idx := range candidates {
		cumulative += servers[idx].Weight
		if target < cumulative {
			return &servers[idx], nil
		}
	}

	// Should never reach here, but return last candidate as fallback
	return &servers[candidates[len(candidates)-1]], nil
}

// Algorithm returns the name of this routing algorithm.
func (w *Weighted) Algorithm() string {
	return "weighted"
}
