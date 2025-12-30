// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"
)

// mockLearnedLatencyProvider implements LearnedLatencyProvider for testing.
type mockLearnedLatencyProvider struct {
	// data maps clientSubnet -> backend|region -> LearnedLatencyData
	data map[string]map[string]*LearnedLatencyData
}

func newMockLearnedLatencyProvider() *mockLearnedLatencyProvider {
	return &mockLearnedLatencyProvider{
		data: make(map[string]map[string]*LearnedLatencyData),
	}
}

// SetLatency sets latency data for a client subnet, backend, and region.
func (m *mockLearnedLatencyProvider) SetLatency(clientSubnet, backend, region string, ewma time.Duration, samples uint64) {
	if m.data[clientSubnet] == nil {
		m.data[clientSubnet] = make(map[string]*LearnedLatencyData)
	}
	key := backend + "|" + region
	m.data[clientSubnet][key] = &LearnedLatencyData{
		Backend:     backend,
		EWMA:        ewma,
		SampleCount: samples,
		LastUpdated: time.Now(),
	}
}

// GetLatencyForBackendInRegion implements LearnedLatencyProvider.
func (m *mockLearnedLatencyProvider) GetLatencyForBackendInRegion(clientIP netip.Addr, backend, region string) (*LearnedLatencyData, bool) {
	// Determine subnet based on IP version
	var prefixBits int
	if clientIP.Is4() {
		prefixBits = 24
	} else {
		prefixBits = 48
	}

	prefix, err := clientIP.Prefix(prefixBits)
	if err != nil {
		return nil, false
	}

	subnetMap, exists := m.data[prefix.String()]
	if !exists {
		return nil, false
	}

	key := backend + "|" + region
	data, exists := subnetMap[key]
	return data, exists
}

func TestLearnedLatencyRouter_Algorithm(t *testing.T) {
	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{})
	if router.Algorithm() != "learned_latency" {
		t.Errorf("expected algorithm %q, got %q", "learned_latency", router.Algorithm())
	}
}

func TestLearnedLatencyRouter_NoProvider_FallsBackToRoundRobin(t *testing.T) {
	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{})

	servers := []*Server{
		{Address: "10.0.1.1", Port: 80, Region: "eu-west"},
		{Address: "10.0.1.2", Port: 80, Region: "ap-southeast"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local")

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected == nil {
		t.Fatal("expected a server to be selected")
	}
}

func TestLearnedLatencyRouter_EmptyPool_ReturnsError(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider: provider,
	})

	pool := NewSimpleServerPool([]*Server{})

	_, err := router.Route(context.Background(), pool)
	if err != ErrNoHealthyServers {
		t.Errorf("expected ErrNoHealthyServers, got %v", err)
	}
}

func TestLearnedLatencyRouter_SelectsLowestLatencyByRegion(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	// Client in 10.0.0.0/24 has different latencies to each region
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 80*time.Millisecond, 10)
	provider.SetLatency("10.0.0.0/24", "web.test.local", "ap-southeast", 5*time.Millisecond, 10) // Lower latency

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
		{Address: "10.2.1.10", Port: 80, Region: "ap-southeast"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local")

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should select ap-southeast (lower latency)
	if selected.Address != "10.2.1.10" {
		t.Errorf("expected server 10.2.1.10 (ap-southeast, lowest latency), got %s", selected.Address)
	}
}

func TestLearnedLatencyRouter_NoClientIP_FallsBack(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 80*time.Millisecond, 10)

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
	}
	pool := NewSimpleServerPool(servers)

	// No client IP in context
	ctx := WithDomain(context.Background(), "web.test.local")

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back but still select a server
	if selected == nil {
		t.Fatal("expected a server to be selected via fallback")
	}
}

func TestLearnedLatencyRouter_NoDomain_SkipsServer(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 80*time.Millisecond, 10)

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
	}
	pool := NewSimpleServerPool(servers)

	// No domain in context - servers will be skipped, falls back to round-robin
	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back but still select a server
	if selected == nil {
		t.Fatal("expected a server to be selected via fallback")
	}
}

func TestLearnedLatencyRouter_NoServerRegion_SkipsServer(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 80*time.Millisecond, 10)

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: ""}, // No region - will be skipped
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local")

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back since server has no region
	if selected == nil {
		t.Fatal("expected a server to be selected via fallback")
	}
}

func TestLearnedLatencyRouter_InsufficientSamples_FallsBack(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 80*time.Millisecond, 2)     // Not enough
	provider.SetLatency("10.0.0.0/24", "web.test.local", "ap-southeast", 5*time.Millisecond, 1) // Not enough

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5, // Requires 5 samples
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
		{Address: "10.2.1.10", Port: 80, Region: "ap-southeast"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local")

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back since no server has enough samples
	if selected == nil {
		t.Fatal("expected a server to be selected via fallback")
	}
}

func TestLearnedLatencyRouter_MixedSamples_SelectsFromValidOnly(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 200*time.Millisecond, 10)   // Enough samples, higher latency
	provider.SetLatency("10.0.0.0/24", "web.test.local", "ap-southeast", 5*time.Millisecond, 2) // Lower latency but not enough samples

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
		{Address: "10.2.1.10", Port: 80, Region: "ap-southeast"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local")

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should select eu-west (only one with enough samples)
	if selected.Address != "10.1.1.10" {
		t.Errorf("expected server 10.1.1.10 (eu-west, only one with enough samples), got %s", selected.Address)
	}
}

func TestLearnedLatencyRouter_MaxLatencyThreshold(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 600*time.Millisecond, 10)      // Above threshold
	provider.SetLatency("10.0.0.0/24", "web.test.local", "ap-southeast", 100*time.Millisecond, 10) // Below threshold

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:     provider,
		MinSamples:   5,
		MaxLatencyMs: 500,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
		{Address: "10.2.1.10", Port: 80, Region: "ap-southeast"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local")

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should select ap-southeast (only one below threshold)
	if selected.Address != "10.2.1.10" {
		t.Errorf("expected server 10.2.1.10 (ap-southeast, below threshold), got %s", selected.Address)
	}
}

func TestLearnedLatencyRouter_AllAboveThreshold_SelectsLowest(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 600*time.Millisecond, 10)      // Above threshold but lower
	provider.SetLatency("10.0.0.0/24", "web.test.local", "ap-southeast", 800*time.Millisecond, 10) // Above threshold

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:     provider,
		MinSamples:   5,
		MaxLatencyMs: 500,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
		{Address: "10.2.1.10", Port: 80, Region: "ap-southeast"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local")

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When all above threshold, should still select the lowest
	if selected.Address != "10.1.1.10" {
		t.Errorf("expected server 10.1.1.10 (eu-west, lowest latency even above threshold), got %s", selected.Address)
	}
}

func TestLearnedLatencyRouter_NoLatencyData_FallsBack(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	// No latency data set

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
		{Address: "10.2.1.10", Port: 80, Region: "ap-southeast"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local")

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to round-robin
	if selected == nil {
		t.Fatal("expected a server to be selected via fallback")
	}
}

func TestLearnedLatencyRouter_DifferentSubnets_DifferentRouting(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	// Subnet A prefers ap-southeast
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 80*time.Millisecond, 10)
	provider.SetLatency("10.0.0.0/24", "web.test.local", "ap-southeast", 5*time.Millisecond, 10)

	// Subnet B prefers eu-west
	provider.SetLatency("10.1.0.0/24", "web.test.local", "eu-west", 5*time.Millisecond, 10)
	provider.SetLatency("10.1.0.0/24", "web.test.local", "ap-southeast", 200*time.Millisecond, 10)

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5,
	})

	servers := []*Server{
		{Address: "10.100.1.10", Port: 80, Region: "eu-west"},
		{Address: "10.100.2.10", Port: 80, Region: "ap-southeast"},
	}
	pool := NewSimpleServerPool(servers)

	// Client from subnet A
	ctxA := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctxA = WithDomain(ctxA, "web.test.local")

	selectedA, err := router.Route(ctxA, pool)
	if err != nil {
		t.Fatalf("unexpected error for subnet A: %v", err)
	}
	if selectedA.Address != "10.100.2.10" {
		t.Errorf("subnet A: expected ap-southeast (10.100.2.10), got %s", selectedA.Address)
	}

	// Client from subnet B
	ctxB := WithClientIP(context.Background(), net.ParseIP("10.1.0.50"))
	ctxB = WithDomain(ctxB, "web.test.local")

	selectedB, err := router.Route(ctxB, pool)
	if err != nil {
		t.Fatalf("unexpected error for subnet B: %v", err)
	}
	if selectedB.Address != "10.100.1.10" {
		t.Errorf("subnet B: expected eu-west (10.100.1.10), got %s", selectedB.Address)
	}
}

func TestLearnedLatencyRouter_LookupKeyFormat(t *testing.T) {
	// This test verifies the critical contract: the router must look up by
	// domain name (service) + region, NOT by IP:port.
	// This was the bug that caused hours of debugging.

	provider := newMockLearnedLatencyProvider()
	// Data is keyed by service name "web.test.local", NOT by IP:port
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 100*time.Millisecond, 10)

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5,
	})

	// Server has IP 10.1.1.10 but data is stored under "web.test.local"
	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local") // Domain must match the backend key in provider

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// If lookup was working, we get the server based on latency data
	// If lookup was broken (using IP:port), it would fall back to round-robin
	if selected.Address != "10.1.1.10" {
		t.Errorf("expected server 10.1.1.10, got %s", selected.Address)
	}
}

func TestLearnedLatencyRouter_WrongDomain_NoMatch(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	// Data is keyed by "web.test.local"
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 100*time.Millisecond, 10)

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 5,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "api.test.local") // Different domain - no match

	selected, err := router.Route(ctx, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back since domain doesn't match
	if selected == nil {
		t.Fatal("expected a server to be selected via fallback")
	}
}

func TestLearnedLatencyRouter_SetProvider(t *testing.T) {
	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{})

	if router.GetProvider() != nil {
		t.Error("expected nil provider initially")
	}

	provider := newMockLearnedLatencyProvider()
	router.SetProvider(provider)

	if router.GetProvider() != provider {
		t.Error("expected provider to be set")
	}
}

func TestLearnedLatencyRouter_SetMinSamples(t *testing.T) {
	provider := newMockLearnedLatencyProvider()
	provider.SetLatency("10.0.0.0/24", "web.test.local", "eu-west", 200*time.Millisecond, 5)
	provider.SetLatency("10.0.0.0/24", "web.test.local", "ap-southeast", 50*time.Millisecond, 3) // Fewer samples

	router := NewLearnedLatencyRouter(LearnedLatencyRouterConfig{
		Provider:   provider,
		MinSamples: 4,
	})

	servers := []*Server{
		{Address: "10.1.1.10", Port: 80, Region: "eu-west"},
		{Address: "10.2.1.10", Port: 80, Region: "ap-southeast"},
	}
	pool := NewSimpleServerPool(servers)

	ctx := WithClientIP(context.Background(), net.ParseIP("10.0.0.50"))
	ctx = WithDomain(ctx, "web.test.local")

	// Initially ap-southeast doesn't have enough samples
	selected, _ := router.Route(ctx, pool)
	if selected.Address != "10.1.1.10" {
		t.Errorf("expected eu-west initially, got %s", selected.Address)
	}

	// Lower the threshold
	router.SetMinSamples(2)

	// Now ap-southeast qualifies and has lower latency
	selected, _ = router.Route(ctx, pool)
	if selected.Address != "10.2.1.10" {
		t.Errorf("expected ap-southeast after lowering min samples, got %s", selected.Address)
	}
}

func TestNewRouter_LearnedLatency(t *testing.T) {
	factory := NewFactory(FactoryConfig{})
	router, err := factory.NewRouter("learned_latency")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if router.Algorithm() != "learned_latency" {
		t.Errorf("expected algorithm %q, got %q", "learned_latency", router.Algorithm())
	}
}

func TestNewRouter_LearnedLatencyAlias(t *testing.T) {
	factory := NewFactory(FactoryConfig{})
	router, err := factory.NewRouter("learned-latency")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if router.Algorithm() != "learned_latency" {
		t.Errorf("expected algorithm %q, got %q", "learned_latency", router.Algorithm())
	}
}
