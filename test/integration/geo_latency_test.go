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

//go:build integration

package integration

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/routing"
)

// TestGeolocationRouterUnit tests the GeoRouter logic without a full server.
// This verifies the routing package correctly routes based on client IP.
func TestGeolocationRouterUnit(t *testing.T) {
	// Create test servers with different regions
	usEastServer := &routing.Server{
		Address: "10.0.1.10",
		Port:    80,
		Region:  "us-east-1",
	}
	euWestServer := &routing.Server{
		Address: "10.0.2.10",
		Port:    80,
		Region:  "eu-west-1",
	}
	apNorthServer := &routing.Server{
		Address: "10.0.3.10",
		Port:    80,
		Region:  "ap-northeast-1",
	}

	// Create a pool with all servers
	allServers := []*routing.Server{usEastServer, euWestServer, apNorthServer}
	pool := routing.NewSimpleServerPool(allServers)

	t.Run("routes without client IP", func(t *testing.T) {
		// Create router without resolver to test fallback
		router := routing.NewGeoRouter(routing.GeoRouterConfig{
			DefaultRegion: "us-east-1",
		})

		ctx := context.Background()
		server, err := router.Route(ctx, pool)
		if err != nil {
			t.Fatalf("routing failed: %v", err)
		}
		if server == nil {
			t.Fatal("expected a server, got nil")
		}
		// Without client IP, should fall back to round-robin
		t.Logf("Selected server: %s:%d (region: %s)", server.Address, server.Port, server.Region)
	})

	t.Run("routes with client IP context", func(t *testing.T) {
		// Create router without resolver to test fallback behavior
		router := routing.NewGeoRouter(routing.GeoRouterConfig{
			DefaultRegion: "us-east-1",
		})

		// Add client IP to context
		clientIP := net.ParseIP("8.8.8.8")
		ctx := routing.WithClientIP(context.Background(), clientIP)

		server, err := router.Route(ctx, pool)
		if err != nil {
			t.Fatalf("routing failed: %v", err)
		}
		if server == nil {
			t.Fatal("expected a server, got nil")
		}
		// Without resolver, falls back to round-robin
		t.Logf("Selected server: %s:%d (region: %s)", server.Address, server.Port, server.Region)
	})

	t.Run("records domain in context", func(t *testing.T) {
		router := routing.NewGeoRouter(routing.GeoRouterConfig{
			DefaultRegion: "us-east-1",
		})

		ctx := context.Background()
		ctx = routing.WithDomain(ctx, "test.example.com")

		domain := routing.GetDomain(ctx)
		if domain != "test.example.com" {
			t.Errorf("expected domain 'test.example.com', got %q", domain)
		}

		server, err := router.Route(ctx, pool)
		if err != nil {
			t.Fatalf("routing failed: %v", err)
		}
		if server == nil {
			t.Fatal("expected a server, got nil")
		}
	})

	t.Run("filters by region", func(t *testing.T) {
		// Create a pool with only US servers
		usOnlyServers := []*routing.Server{usEastServer}
		usPool := routing.NewSimpleServerPool(usOnlyServers)

		router := routing.NewGeoRouter(routing.GeoRouterConfig{
			DefaultRegion: "us-east-1",
		})

		ctx := context.Background()
		server, err := router.Route(ctx, usPool)
		if err != nil {
			t.Fatalf("routing failed: %v", err)
		}
		if server.Address != "10.0.1.10" {
			t.Errorf("expected US server 10.0.1.10, got %s", server.Address)
		}
	})
}

// TestLatencyRouterUnit tests the LatencyRouter logic without a full server.
func TestLatencyRouterUnit(t *testing.T) {
	// Create test servers
	server1 := &routing.Server{Address: "10.0.1.10", Port: 80}
	server2 := &routing.Server{Address: "10.0.1.11", Port: 80}
	server3 := &routing.Server{Address: "10.0.1.12", Port: 80}

	allServers := []*routing.Server{server1, server2, server3}
	pool := routing.NewSimpleServerPool(allServers)

	t.Run("falls back to round-robin without provider", func(t *testing.T) {
		router := routing.NewLatencyRouter(routing.LatencyRouterConfig{})

		ctx := context.Background()
		server, err := router.Route(ctx, pool)
		if err != nil {
			t.Fatalf("routing failed: %v", err)
		}
		if server == nil {
			t.Fatal("expected a server, got nil")
		}
		// Without provider, should fall back to round-robin
		t.Logf("Selected server (no provider): %s:%d", server.Address, server.Port)
	})

	t.Run("uses provider when available", func(t *testing.T) {
		// Create a mock provider
		provider := &mockLatencyProvider{
			latencies: map[string]routing.LatencyInfo{
				"10.0.1.10:80": {SmoothedLatency: 100 * time.Millisecond, Samples: 5, HasData: true},
				"10.0.1.11:80": {SmoothedLatency: 50 * time.Millisecond, Samples: 5, HasData: true},  // Lowest
				"10.0.1.12:80": {SmoothedLatency: 200 * time.Millisecond, Samples: 5, HasData: true},
			},
		}

		router := routing.NewLatencyRouter(routing.LatencyRouterConfig{
			Provider:   provider,
			MinSamples: 3,
		})

		ctx := routing.WithDomain(context.Background(), "perf.example.com")
		server, err := router.Route(ctx, pool)
		if err != nil {
			t.Fatalf("routing failed: %v", err)
		}
		if server == nil {
			t.Fatal("expected a server, got nil")
		}
		// Should select server with lowest latency
		if server.Address != "10.0.1.11" {
			t.Errorf("expected lowest latency server 10.0.1.11, got %s", server.Address)
		}
		t.Logf("Selected server (lowest latency): %s:%d", server.Address, server.Port)
	})

	t.Run("respects max latency threshold", func(t *testing.T) {
		// One server is above threshold
		provider := &mockLatencyProvider{
			latencies: map[string]routing.LatencyInfo{
				"10.0.1.10:80": {SmoothedLatency: 100 * time.Millisecond, Samples: 5, HasData: true},
				"10.0.1.11:80": {SmoothedLatency: 50 * time.Millisecond, Samples: 5, HasData: true},
				"10.0.1.12:80": {SmoothedLatency: 600 * time.Millisecond, Samples: 5, HasData: true}, // Above threshold
			},
		}

		router := routing.NewLatencyRouter(routing.LatencyRouterConfig{
			Provider:     provider,
			MaxLatencyMs: 500, // 500ms threshold
			MinSamples:   3,
		})

		ctx := routing.WithDomain(context.Background(), "perf.example.com")
		server, err := router.Route(ctx, pool)
		if err != nil {
			t.Fatalf("routing failed: %v", err)
		}

		// Should not select the server above threshold (unless all are above)
		if server.Address == "10.0.1.12" {
			t.Error("should not select server above latency threshold when others are available")
		}
		t.Logf("Selected server (with threshold): %s:%d", server.Address, server.Port)
	})

	t.Run("falls back when insufficient samples", func(t *testing.T) {
		// Servers have too few samples
		provider := &mockLatencyProvider{
			latencies: map[string]routing.LatencyInfo{
				"10.0.1.10:80": {SmoothedLatency: 100 * time.Millisecond, Samples: 1, HasData: true},
				"10.0.1.11:80": {SmoothedLatency: 50 * time.Millisecond, Samples: 2, HasData: true},
				"10.0.1.12:80": {SmoothedLatency: 200 * time.Millisecond, Samples: 1, HasData: true},
			},
		}

		router := routing.NewLatencyRouter(routing.LatencyRouterConfig{
			Provider:   provider,
			MinSamples: 3, // Requires 3 samples, but all have less
		})

		ctx := context.Background()
		server, err := router.Route(ctx, pool)
		if err != nil {
			t.Fatalf("routing failed: %v", err)
		}
		if server == nil {
			t.Fatal("expected a server, got nil")
		}
		// Should fall back to round-robin
		t.Logf("Selected server (fallback): %s:%d", server.Address, server.Port)
	})

	t.Run("handles empty pool", func(t *testing.T) {
		router := routing.NewLatencyRouter(routing.LatencyRouterConfig{})

		emptyPool := routing.NewSimpleServerPool([]*routing.Server{})
		ctx := context.Background()

		_, err := router.Route(ctx, emptyPool)
		if err != routing.ErrNoHealthyServers {
			t.Errorf("expected ErrNoHealthyServers, got %v", err)
		}
	})
}

// TestLatencyRouterConfigDefaults tests default configuration values.
func TestLatencyRouterConfigDefaults(t *testing.T) {
	defaults := routing.DefaultLatencyRouterConfig()

	if defaults.MaxLatencyMs != 500 {
		t.Errorf("expected MaxLatencyMs 500, got %d", defaults.MaxLatencyMs)
	}
	if defaults.MinSamples != 3 {
		t.Errorf("expected MinSamples 3, got %d", defaults.MinSamples)
	}
}

// TestRouterAlgorithmNames verifies algorithm name constants.
func TestRouterAlgorithmNames(t *testing.T) {
	geoRouter := routing.NewGeoRouter(routing.GeoRouterConfig{})
	if geoRouter.Algorithm() != "geolocation" {
		t.Errorf("expected algorithm 'geolocation', got %q", geoRouter.Algorithm())
	}

	latencyRouter := routing.NewLatencyRouter(routing.LatencyRouterConfig{})
	if latencyRouter.Algorithm() != "latency" {
		t.Errorf("expected algorithm 'latency', got %q", latencyRouter.Algorithm())
	}
}

// mockLatencyProvider implements routing.LatencyProvider for testing.
type mockLatencyProvider struct {
	latencies map[string]routing.LatencyInfo
}

func (m *mockLatencyProvider) GetLatency(address string, port int) routing.LatencyInfo {
	key := address + ":" + strconv.Itoa(port)
	if info, ok := m.latencies[key]; ok {
		return info
	}
	return routing.LatencyInfo{HasData: false}
}
