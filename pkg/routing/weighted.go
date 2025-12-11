// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

// WeightedRouter implements weighted random server selection.
type WeightedRouter struct {
	mu   sync.Mutex
	rand *rand.Rand
}

// NewWeightedRouter creates a new weighted router.
func NewWeightedRouter() *WeightedRouter {
	return &WeightedRouter{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Route selects a server based on weights.
// Servers with higher weights have proportionally higher selection probability.
func (r *WeightedRouter) Route(ctx context.Context, pool ServerPool) (*Server, error) {
	servers := pool.Servers()
	if len(servers) == 0 {
		return nil, ErrNoHealthyServers
	}

	// Calculate total weight
	totalWeight := 0
	for _, s := range servers {
		weight := s.Weight
		if weight <= 0 {
			weight = 1 // Default weight
		}
		totalWeight += weight
	}

	if totalWeight == 0 {
		return nil, ErrNoHealthyServers
	}

	// Select random point in weight space
	r.mu.Lock()
	point := r.rand.Intn(totalWeight)
	r.mu.Unlock()

	// Find the server at that point
	cumulative := 0
	for _, s := range servers {
		weight := s.Weight
		if weight <= 0 {
			weight = 1
		}
		cumulative += weight
		if point < cumulative {
			return s, nil
		}
	}

	// Fallback (shouldn't happen)
	return servers[len(servers)-1], nil
}

// Algorithm returns the algorithm name.
func (r *WeightedRouter) Algorithm() string {
	return AlgorithmWeighted
}
