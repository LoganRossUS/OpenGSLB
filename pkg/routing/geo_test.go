// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"net"
	"testing"
)

// =============================================================================
// GeoRouter Tests
// =============================================================================

func TestGeoRouter_Algorithm(t *testing.T) {
	router := NewGeoRouter(GeoRouterConfig{})
	if router.Algorithm() != AlgorithmGeolocation {
		t.Errorf("expected algorithm %s, got %s", AlgorithmGeolocation, router.Algorithm())
	}
}

func TestGeoRouter_NoServers(t *testing.T) {
	router := NewGeoRouter(GeoRouterConfig{})
	pool := NewSimpleServerPool([]*Server{})

	_, err := router.Route(context.Background(), pool)
	if err != ErrNoHealthyServers {
		t.Errorf("expected ErrNoHealthyServers, got %v", err)
	}
}

func TestGeoRouter_NoClientIP_FallsBackToRoundRobin(t *testing.T) {
	router := NewGeoRouter(GeoRouterConfig{})
	servers := []*Server{
		{Address: "10.0.1.10", Port: 80, Region: "us-east-1"},
		{Address: "10.0.2.10", Port: 80, Region: "us-west-2"},
	}
	pool := NewSimpleServerPool(servers)

	// Without client IP in context, should fall back to round-robin
	server, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server == nil {
		t.Fatal("expected server, got nil")
	}
}

func TestGeoRouter_WithClientIP_NoResolver(t *testing.T) {
	router := NewGeoRouter(GeoRouterConfig{})
	servers := []*Server{
		{Address: "10.0.1.10", Port: 80, Region: "us-east-1"},
		{Address: "10.0.2.10", Port: 80, Region: "us-west-2"},
	}
	pool := NewSimpleServerPool(servers)

	// With client IP but no resolver, should fall back to round-robin
	ctx := WithClientIP(context.Background(), net.ParseIP("192.168.1.100"))
	server, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server == nil {
		t.Fatal("expected server, got nil")
	}
}

func TestGeoRouter_FilterByRegion(t *testing.T) {
	router := NewGeoRouter(GeoRouterConfig{DefaultRegion: "us-east-1"})
	servers := []*Server{
		{Address: "10.0.1.10", Port: 80, Region: "us-east-1"},
		{Address: "10.0.1.11", Port: 80, Region: "us-east-1"},
		{Address: "10.0.2.10", Port: 80, Region: "us-west-2"},
		{Address: "10.0.3.10", Port: 80, Region: "eu-west-1"},
	}

	filtered := router.filterByRegion(servers, "us-east-1")
	if len(filtered) != 2 {
		t.Errorf("expected 2 servers in us-east-1, got %d", len(filtered))
	}

	filtered = router.filterByRegion(servers, "us-west-2")
	if len(filtered) != 1 {
		t.Errorf("expected 1 server in us-west-2, got %d", len(filtered))
	}

	filtered = router.filterByRegion(servers, "nonexistent")
	if len(filtered) != 0 {
		t.Errorf("expected 0 servers in nonexistent, got %d", len(filtered))
	}
}

func TestWithClientIP_AndGetClientIP(t *testing.T) {
	testIP := net.ParseIP("192.168.1.100")
	ctx := WithClientIP(context.Background(), testIP)

	gotIP := GetClientIP(ctx)
	if !gotIP.Equal(testIP) {
		t.Errorf("expected IP %v, got %v", testIP, gotIP)
	}

	// Test with no IP in context
	emptyIP := GetClientIP(context.Background())
	if emptyIP != nil {
		t.Errorf("expected nil IP, got %v", emptyIP)
	}
}

func TestGeoRouter_FactoryCreation(t *testing.T) {
	// Test that Factory can create GeoRouter
	factory := NewFactory(FactoryConfig{
		DefaultRegion: "us-east-1",
	})

	router, err := factory.NewRouter("geolocation")
	if err != nil {
		t.Fatalf("failed to create geo router: %v", err)
	}

	if router.Algorithm() != AlgorithmGeolocation {
		t.Errorf("expected algorithm %s, got %s", AlgorithmGeolocation, router.Algorithm())
	}
}

func TestGeoRouter_AliasNames(t *testing.T) {
	// Test that "geo" alias works
	router, err := NewRouter("geo")
	if err != nil {
		t.Fatalf("failed to create router with 'geo' alias: %v", err)
	}

	if router.Algorithm() != AlgorithmGeolocation {
		t.Errorf("expected algorithm %s, got %s", AlgorithmGeolocation, router.Algorithm())
	}
}
