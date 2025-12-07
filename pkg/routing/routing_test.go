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
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
)

// helper to create ServerInfo for tests
func makeServer(ip string, port int, region string) dns.ServerInfo {
	return dns.ServerInfo{
		Address: net.ParseIP(ip),
		Port:    port,
		Region:  region,
	}
}

func TestRoundRobin_Route(t *testing.T) {
	t.Run("returns error for empty server list", func(t *testing.T) {
		rr := NewRoundRobin()
		ctx := context.Background()

		_, err := rr.Route(ctx, "example.com", []dns.ServerInfo{})
		if err == nil {
			t.Fatal("expected error for empty server list")
		}
		if !errors.Is(err, ErrNoHealthyServers) {
			t.Errorf("expected ErrNoHealthyServers, got %v", err)
		}
	})

	t.Run("returns single server", func(t *testing.T) {
		rr := NewRoundRobin()
		ctx := context.Background()
		servers := []dns.ServerInfo{
			makeServer("10.0.1.10", 80, "us-east-1"),
		}

		server, err := rr.Route(ctx, "example.com", servers)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !server.Address.Equal(net.ParseIP("10.0.1.10")) {
			t.Errorf("expected 10.0.1.10, got %s", server.Address)
		}
	})

	t.Run("rotates through servers evenly", func(t *testing.T) {
		rr := NewRoundRobin()
		ctx := context.Background()
		servers := []dns.ServerInfo{
			makeServer("10.0.1.10", 80, "us-east-1"),
			makeServer("10.0.1.11", 80, "us-east-1"),
			makeServer("10.0.1.12", 80, "us-east-1"),
		}

		// Track how many times each server is selected
		counts := make(map[string]int)
		iterations := 300

		for i := 0; i < iterations; i++ {
			server, err := rr.Route(ctx, "example.com", servers)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			counts[server.Address.String()]++
		}

		// Each server should be selected exactly iterations/len(servers) times
		expected := iterations / len(servers)
		for ip, count := range counts {
			if count != expected {
				t.Errorf("server %s selected %d times, expected %d", ip, count, expected)
			}
		}
	})

	t.Run("per-domain isolation", func(t *testing.T) {
		rr := NewRoundRobin()
		ctx := context.Background()
		servers := []dns.ServerInfo{
			makeServer("10.0.1.10", 80, "us-east-1"),
			makeServer("10.0.1.11", 80, "us-east-1"),
		}

		// Query domain A twice
		serverA1, _ := rr.Route(ctx, "domain-a.com", servers)
		serverA2, _ := rr.Route(ctx, "domain-a.com", servers)

		// Query domain B once - should start at index 0, not affected by domain A
		serverB1, _ := rr.Route(ctx, "domain-b.com", servers)

		// Domain A should have rotated: first=10, second=11
		if !serverA1.Address.Equal(net.ParseIP("10.0.1.10")) {
			t.Errorf("domain A first query: expected 10.0.1.10, got %s", serverA1.Address)
		}
		if !serverA2.Address.Equal(net.ParseIP("10.0.1.11")) {
			t.Errorf("domain A second query: expected 10.0.1.11, got %s", serverA2.Address)
		}

		// Domain B should start fresh at index 0
		if !serverB1.Address.Equal(net.ParseIP("10.0.1.10")) {
			t.Errorf("domain B first query: expected 10.0.1.10, got %s", serverB1.Address)
		}
	})

	t.Run("wraps around after reaching end", func(t *testing.T) {
		rr := NewRoundRobin()
		ctx := context.Background()
		servers := []dns.ServerInfo{
			makeServer("10.0.1.10", 80, "us-east-1"),
			makeServer("10.0.1.11", 80, "us-east-1"),
		}

		// Query 4 times - should wrap around
		results := make([]string, 4)
		for i := 0; i < 4; i++ {
			server, _ := rr.Route(ctx, "example.com", servers)
			results[i] = server.Address.String()
		}

		expected := []string{"10.0.1.10", "10.0.1.11", "10.0.1.10", "10.0.1.11"}
		for i, exp := range expected {
			if results[i] != exp {
				t.Errorf("query %d: expected %s, got %s", i, exp, results[i])
			}
		}
	})

	t.Run("handles changing server list size", func(t *testing.T) {
		rr := NewRoundRobin()
		ctx := context.Background()

		// Start with 3 servers, query a few times to advance index
		servers3 := []dns.ServerInfo{
			makeServer("10.0.1.10", 80, "us-east-1"),
			makeServer("10.0.1.11", 80, "us-east-1"),
			makeServer("10.0.1.12", 80, "us-east-1"),
		}

		for i := 0; i < 5; i++ {
			_, err := rr.Route(ctx, "example.com", servers3)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}

		// Now one server is unhealthy, only 2 servers passed
		servers2 := []dns.ServerInfo{
			makeServer("10.0.1.10", 80, "us-east-1"),
			makeServer("10.0.1.11", 80, "us-east-1"),
		}

		// Should still work with modulo on smaller list
		server, err := rr.Route(ctx, "example.com", servers2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Index was 5, 5 % 2 = 1, so should return second server
		if !server.Address.Equal(net.ParseIP("10.0.1.11")) {
			t.Errorf("expected 10.0.1.11, got %s", server.Address)
		}
	})
}

func TestRoundRobin_Algorithm(t *testing.T) {
	rr := NewRoundRobin()
	if rr.Algorithm() != "round-robin" {
		t.Errorf("expected 'round-robin', got %q", rr.Algorithm())
	}
}

func TestRoundRobin_Concurrent(t *testing.T) {
	rr := NewRoundRobin()
	ctx := context.Background()
	servers := []dns.ServerInfo{
		makeServer("10.0.1.10", 80, "us-east-1"),
		makeServer("10.0.1.11", 80, "us-east-1"),
		makeServer("10.0.1.12", 80, "us-east-1"),
	}

	// Run many concurrent requests
	var wg sync.WaitGroup
	goroutines := 100
	requestsPerGoroutine := 100

	counts := make(map[string]int)
	var countsMu sync.Mutex

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				server, err := rr.Route(ctx, "example.com", servers)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				countsMu.Lock()
				counts[server.Address.String()]++
				countsMu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Verify total requests
	total := 0
	for _, count := range counts {
		total += count
	}

	expectedTotal := goroutines * requestsPerGoroutine
	if total != expectedTotal {
		t.Errorf("expected %d total requests, got %d", expectedTotal, total)
	}

	// Verify roughly even distribution (within 1% tolerance for concurrency)
	expectedPerServer := expectedTotal / len(servers)
	tolerance := expectedPerServer / 100 // 1%
	if tolerance < 1 {
		tolerance = 1
	}

	for ip, count := range counts {
		diff := count - expectedPerServer
		if diff < 0 {
			diff = -diff
		}
		// Allow some variance due to timing, but should be close
		if diff > tolerance*10 { // Allow 10% variance for concurrent execution
			t.Errorf("server %s: expected ~%d requests, got %d (diff: %d)",
				ip, expectedPerServer, count, diff)
		}
	}
}

func TestRoundRobin_MultipleDomainsConcurrent(t *testing.T) {
	rr := NewRoundRobin()
	ctx := context.Background()
	servers := []dns.ServerInfo{
		makeServer("10.0.1.10", 80, "us-east-1"),
		makeServer("10.0.1.11", 80, "us-east-1"),
	}

	domains := []string{"domain-a.com", "domain-b.com", "domain-c.com"}
	var wg sync.WaitGroup

	// Each domain gets its own goroutine making requests
	for _, domain := range domains {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, err := rr.Route(ctx, d, servers)
				if err != nil {
					t.Errorf("unexpected error for %s: %v", d, err)
					return
				}
			}
		}(domain)
	}

	wg.Wait()

	// Verify each domain has its own index
	// After 100 requests each, indices should be 100
	rr.mu.Lock()
	for _, domain := range domains {
		idx := rr.indices[domain]
		if idx != 100 {
			t.Errorf("domain %s: expected index 100, got %d", domain, idx)
		}
	}
	rr.mu.Unlock()
}
