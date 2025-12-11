// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"testing"
	"time"
)

// mockLatencyProvider implements LatencyProvider for testing.
type mockLatencyProvider struct {
	latencies map[string]LatencyInfo // key: "address:port"
}

func newMockLatencyProvider() *mockLatencyProvider {
	return &mockLatencyProvider{
		latencies: make(map[string]LatencyInfo),
	}
}

func (m *mockLatencyProvider) SetLatency(address string, port int, info LatencyInfo) {
	key := serverKey(address, port)
	m.latencies[key] = info
}

func (m *mockLatencyProvider) GetLatency(address string, port int) LatencyInfo {
	key := serverKey(address, port)
	if info, ok := m.latencies[key]; ok {
		return info
	}
	return LatencyInfo{HasData: false}
}

func serverKey(address string, port int) string {
	return address
}

func TestLatencyRouter_Algorithm(t *testing.T) {
	router := NewLatencyRouter(LatencyRouterConfig{})
	if router.Algorithm() != AlgorithmLatency {
		t.Errorf("expected algorithm %q, got %q", AlgorithmLatency, router.Algorithm())
	}
}

func TestLatencyRouter_NoProvider_FallsBackToRoundRobin(t *testing.T) {
	router := NewLatencyRouter(LatencyRouterConfig{})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Without a provider, should use round-robin
	ctx := context.Background()
	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected == nil {
		t.Fatal("expected a server to be selected")
	}
}

func TestLatencyRouter_EmptyPool_ReturnsError(t *testing.T) {
	provider := newMockLatencyProvider()
	router := NewLatencyRouter(LatencyRouterConfig{
		Provider: provider,
	})

	pool := NewSimpleServerPool([]*Server{})

	_, err := router.Route(context.Background(), pool)
	if err != ErrNoHealthyServers {
		t.Errorf("expected ErrNoHealthyServers, got %v", err)
	}
}

func TestLatencyRouter_SelectsLowestLatency(t *testing.T) {
	provider := newMockLatencyProvider()
	provider.SetLatency("10.0.1.1", 8080, LatencyInfo{
		SmoothedLatency: 100 * time.Millisecond,
		Samples:         5,
		HasData:         true,
	})
	provider.SetLatency("10.0.1.2", 8080, LatencyInfo{
		SmoothedLatency: 50 * time.Millisecond, // Lower latency - should be selected
		Samples:         5,
		HasData:         true,
	})
	provider.SetLatency("10.0.1.3", 8080, LatencyInfo{
		SmoothedLatency: 200 * time.Millisecond,
		Samples:         5,
		HasData:         true,
	})

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:     provider,
		MinSamples:   3,
		MaxLatencyMs: 500,
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
		{Address: "10.0.1.3", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Should select the server with lowest latency (10.0.1.2)
	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Address != "10.0.1.2" {
		t.Errorf("expected server 10.0.1.2 (lowest latency), got %s", selected.Address)
	}
}

func TestLatencyRouter_InsufficientSamples_FallsBackToRoundRobin(t *testing.T) {
	provider := newMockLatencyProvider()
	provider.SetLatency("10.0.1.1", 8080, LatencyInfo{
		SmoothedLatency: 100 * time.Millisecond,
		Samples:         1, // Not enough samples
		HasData:         true,
	})
	provider.SetLatency("10.0.1.2", 8080, LatencyInfo{
		SmoothedLatency: 50 * time.Millisecond,
		Samples:         2, // Not enough samples
		HasData:         true,
	})

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:   provider,
		MinSamples: 3, // Requires 3 samples
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Should fall back to round-robin since no server has enough samples
	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected == nil {
		t.Fatal("expected a server to be selected")
	}
}

func TestLatencyRouter_MixedSamples_SelectsFromValidOnly(t *testing.T) {
	provider := newMockLatencyProvider()
	provider.SetLatency("10.0.1.1", 8080, LatencyInfo{
		SmoothedLatency: 200 * time.Millisecond, // Higher latency but enough samples
		Samples:         5,
		HasData:         true,
	})
	provider.SetLatency("10.0.1.2", 8080, LatencyInfo{
		SmoothedLatency: 50 * time.Millisecond, // Lower latency but insufficient samples
		Samples:         1,
		HasData:         true,
	})

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:   provider,
		MinSamples: 3,
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Should select 10.0.1.1 because 10.0.1.2 doesn't have enough samples
	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Address != "10.0.1.1" {
		t.Errorf("expected server 10.0.1.1 (only one with enough samples), got %s", selected.Address)
	}
}

func TestLatencyRouter_MaxLatencyThreshold(t *testing.T) {
	provider := newMockLatencyProvider()
	provider.SetLatency("10.0.1.1", 8080, LatencyInfo{
		SmoothedLatency: 600 * time.Millisecond, // Above threshold
		Samples:         5,
		HasData:         true,
	})
	provider.SetLatency("10.0.1.2", 8080, LatencyInfo{
		SmoothedLatency: 100 * time.Millisecond, // Below threshold
		Samples:         5,
		HasData:         true,
	})
	provider.SetLatency("10.0.1.3", 8080, LatencyInfo{
		SmoothedLatency: 800 * time.Millisecond, // Above threshold
		Samples:         5,
		HasData:         true,
	})

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:     provider,
		MinSamples:   3,
		MaxLatencyMs: 500, // 500ms threshold
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
		{Address: "10.0.1.3", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Should select 10.0.1.2 (only one below threshold)
	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Address != "10.0.1.2" {
		t.Errorf("expected server 10.0.1.2 (only one below threshold), got %s", selected.Address)
	}
}

func TestLatencyRouter_AllAboveThreshold_SelectsLowest(t *testing.T) {
	provider := newMockLatencyProvider()
	provider.SetLatency("10.0.1.1", 8080, LatencyInfo{
		SmoothedLatency: 600 * time.Millisecond, // Above threshold - but lowest
		Samples:         5,
		HasData:         true,
	})
	provider.SetLatency("10.0.1.2", 8080, LatencyInfo{
		SmoothedLatency: 800 * time.Millisecond, // Above threshold
		Samples:         5,
		HasData:         true,
	})

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:     provider,
		MinSamples:   3,
		MaxLatencyMs: 500, // All servers above this
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// When all servers are above threshold, should still select the lowest
	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Address != "10.0.1.1" {
		t.Errorf("expected server 10.0.1.1 (lowest latency even though above threshold), got %s", selected.Address)
	}
}

func TestLatencyRouter_NoLatencyData_FallsBack(t *testing.T) {
	provider := newMockLatencyProvider()
	// No latency data set for any server

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:   provider,
		MinSamples: 3,
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Should fall back to round-robin
	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected == nil {
		t.Fatal("expected a server to be selected")
	}
}

func TestLatencyRouter_SetProvider(t *testing.T) {
	router := NewLatencyRouter(LatencyRouterConfig{})

	// Initially no provider
	if router.GetProvider() != nil {
		t.Error("expected nil provider initially")
	}

	// Set provider
	provider := newMockLatencyProvider()
	router.SetProvider(provider)

	if router.GetProvider() != provider {
		t.Error("expected provider to be set")
	}
}

func TestLatencyRouter_SetMaxLatency(t *testing.T) {
	provider := newMockLatencyProvider()
	provider.SetLatency("10.0.1.1", 8080, LatencyInfo{
		SmoothedLatency: 300 * time.Millisecond,
		Samples:         5,
		HasData:         true,
	})
	provider.SetLatency("10.0.1.2", 8080, LatencyInfo{
		SmoothedLatency: 100 * time.Millisecond,
		Samples:         5,
		HasData:         true,
	})

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:     provider,
		MinSamples:   3,
		MaxLatencyMs: 200, // Only 10.0.1.2 is below
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Initially should select 10.0.1.2
	selected, _ := router.Route(context.Background(), pool)
	if selected.Address != "10.0.1.2" {
		t.Errorf("expected 10.0.1.2, got %s", selected.Address)
	}

	// Raise the threshold to allow 10.0.1.1
	router.SetMaxLatency(400)

	// Now both are valid, should select lowest
	selected, _ = router.Route(context.Background(), pool)
	if selected.Address != "10.0.1.2" {
		t.Errorf("expected 10.0.1.2 (still lowest), got %s", selected.Address)
	}
}

func TestLatencyRouter_SetMinSamples(t *testing.T) {
	provider := newMockLatencyProvider()
	provider.SetLatency("10.0.1.1", 8080, LatencyInfo{
		SmoothedLatency: 200 * time.Millisecond,
		Samples:         5,
		HasData:         true,
	})
	provider.SetLatency("10.0.1.2", 8080, LatencyInfo{
		SmoothedLatency: 50 * time.Millisecond,
		Samples:         2, // Initially not enough
		HasData:         true,
	})

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:   provider,
		MinSamples: 3,
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Initially should select 10.0.1.1 (only one with enough samples)
	selected, _ := router.Route(context.Background(), pool)
	if selected.Address != "10.0.1.1" {
		t.Errorf("expected 10.0.1.1, got %s", selected.Address)
	}

	// Lower the required samples
	router.SetMinSamples(2)

	// Now 10.0.1.2 qualifies and is lower latency
	selected, _ = router.Route(context.Background(), pool)
	if selected.Address != "10.0.1.2" {
		t.Errorf("expected 10.0.1.2 (now qualifies with lower latency), got %s", selected.Address)
	}
}

func TestLatencyRouter_ZeroMaxLatency_NoThreshold(t *testing.T) {
	provider := newMockLatencyProvider()
	provider.SetLatency("10.0.1.1", 8080, LatencyInfo{
		SmoothedLatency: 5000 * time.Millisecond, // Very high latency
		Samples:         5,
		HasData:         true,
	})
	provider.SetLatency("10.0.1.2", 8080, LatencyInfo{
		SmoothedLatency: 100 * time.Millisecond,
		Samples:         5,
		HasData:         true,
	})

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:     provider,
		MinSamples:   3,
		MaxLatencyMs: 0, // No threshold - but default applies
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
		{Address: "10.0.1.2", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Should select the lowest latency
	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Address != "10.0.1.2" {
		t.Errorf("expected 10.0.1.2 (lowest latency), got %s", selected.Address)
	}
}

func TestLatencyRouter_SingleServer(t *testing.T) {
	provider := newMockLatencyProvider()
	provider.SetLatency("10.0.1.1", 8080, LatencyInfo{
		SmoothedLatency: 100 * time.Millisecond,
		Samples:         5,
		HasData:         true,
	})

	router := NewLatencyRouter(LatencyRouterConfig{
		Provider:   provider,
		MinSamples: 3,
	})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 8080},
	}
	pool := NewSimpleServerPool(servers)

	// Should select the only server
	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Address != "10.0.1.1" {
		t.Errorf("expected 10.0.1.1, got %s", selected.Address)
	}
}

func TestNewRouter_Latency(t *testing.T) {
	router, err := NewRouter("latency")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if router.Algorithm() != AlgorithmLatency {
		t.Errorf("expected algorithm %q, got %q", AlgorithmLatency, router.Algorithm())
	}
}
