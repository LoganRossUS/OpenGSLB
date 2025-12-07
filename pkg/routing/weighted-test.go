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
	"math"
	"net"
	"sync"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
)

func TestWeighted_Algorithm(t *testing.T) {
	w := NewWeighted()
	if got := w.Algorithm(); got != "weighted" {
		t.Errorf("Algorithm() = %q, want %q", got, "weighted")
	}
}

func TestWeighted_Route_EmptySlice(t *testing.T) {
	w := NewWeighted()

	_, err := w.Route(context.Background(), "example.com", []dns.ServerInfo{})
	if err != ErrNoHealthyServers {
		t.Errorf("Route() error = %v, want ErrNoHealthyServers", err)
	}
}

func TestWeighted_Route_AllZeroWeight(t *testing.T) {
	w := NewWeighted()
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 0},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 0},
	}

	_, err := w.Route(context.Background(), "example.com", servers)
	if err != ErrNoHealthyServers {
		t.Errorf("Route() error = %v, want ErrNoHealthyServers", err)
	}
}

func TestWeighted_Route_SingleServer(t *testing.T) {
	w := NewWeighted()
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},
	}

	for i := 0; i < 10; i++ {
		got, err := w.Route(context.Background(), "example.com", servers)
		if err != nil {
			t.Fatalf("Route() error = %v", err)
		}
		if !got.Address.Equal(net.ParseIP("10.0.0.1")) {
			t.Errorf("Route() address = %v, want 10.0.0.1", got.Address)
		}
	}
}

func TestWeighted_Route_EqualWeights(t *testing.T) {
	w := NewWeighted()
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100},
		{Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 100},
	}

	counts := make(map[string]int)
	iterations := 3000

	for i := 0; i < iterations; i++ {
		got, err := w.Route(context.Background(), "example.com", servers)
		if err != nil {
			t.Fatalf("Route() error = %v", err)
		}
		counts[got.Address.String()]++
	}

	// With equal weights, expect roughly equal distribution (33% each)
	expected := float64(iterations) / 3.0
	tolerance := 0.15 // 15% tolerance

	for addr, count := range counts {
		ratio := float64(count) / expected
		if math.Abs(ratio-1.0) > tolerance {
			t.Errorf("Server %s: got %d selections (%.1f%%), expected ~%.0f (33.3%%)",
				addr, count, float64(count)/float64(iterations)*100, expected)
		}
	}
}

func TestWeighted_Route_ProportionalDistribution(t *testing.T) {
	w := NewWeighted()
	// Weights: 100, 50, 150 = total 300
	// Expected: 33.3%, 16.7%, 50%
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100}, // 33.3%
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 50},  // 16.7%
		{Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 150}, // 50%
	}

	counts := make(map[string]int)
	iterations := 6000

	for i := 0; i < iterations; i++ {
		got, err := w.Route(context.Background(), "example.com", servers)
		if err != nil {
			t.Fatalf("Route() error = %v", err)
		}
		counts[got.Address.String()]++
	}

	// Verify proportional distribution within tolerance
	totalWeight := 300.0
	tolerance := 0.10 // 10% tolerance

	expectations := map[string]float64{
		"10.0.0.1": 100.0 / totalWeight, // 0.333
		"10.0.0.2": 50.0 / totalWeight,  // 0.167
		"10.0.0.3": 150.0 / totalWeight, // 0.500
	}

	for addr, expectedRatio := range expectations {
		actualRatio := float64(counts[addr]) / float64(iterations)
		diff := math.Abs(actualRatio - expectedRatio)
		if diff > tolerance {
			t.Errorf("Server %s: got %.1f%%, expected %.1f%% (diff %.1f%% > %.1f%% tolerance)",
				addr, actualRatio*100, expectedRatio*100, diff*100, tolerance*100)
		}
	}
}

func TestWeighted_Route_ZeroWeightExcluded(t *testing.T) {
	w := NewWeighted()
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 0}, // Should be excluded
		{Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 100},
	}

	counts := make(map[string]int)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		got, err := w.Route(context.Background(), "example.com", servers)
		if err != nil {
			t.Fatalf("Route() error = %v", err)
		}
		counts[got.Address.String()]++
	}

	if counts["10.0.0.2"] > 0 {
		t.Errorf("Zero-weight server selected %d times, expected 0", counts["10.0.0.2"])
	}

	// Other servers should each get roughly half
	for _, addr := range []string{"10.0.0.1", "10.0.0.3"} {
		ratio := float64(counts[addr]) / float64(iterations)
		if ratio < 0.4 || ratio > 0.6 {
			t.Errorf("Server %s: got %.1f%%, expected ~50%%", addr, ratio*100)
		}
	}
}

func TestWeighted_Route_Concurrent(t *testing.T) {
	w := NewWeighted()
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100},
	}

	var wg sync.WaitGroup
	iterations := 100
	goroutines := 10

	errors := make(chan error, goroutines*iterations)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, err := w.Route(context.Background(), "example.com", servers)
				if err != nil {
					errors <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent Route() error: %v", err)
	}
}

func TestWeighted_Route_DifferentDomains(t *testing.T) {
	w := NewWeighted()
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100},
	}

	// Weighted routing doesn't use domain for state (unlike round-robin),
	// but verify it works correctly with different domains
	for _, domain := range []string{"a.example.com", "b.example.com"} {
		got, err := w.Route(context.Background(), domain, servers)
		if err != nil {
			t.Fatalf("Route(%s) error = %v", domain, err)
		}
		if got == nil {
			t.Errorf("Route(%s) returned nil", domain)
		}
	}
}

func TestWeighted_ImplementsRouter(t *testing.T) {
	// Compile-time check that Weighted implements dns.Router
	var _ dns.Router = (*Weighted)(nil)
}
