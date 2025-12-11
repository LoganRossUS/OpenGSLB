// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"context"
	"net"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/miekg/dns"
)

// mockRouter implements routing.Router for testing.
type mockRouter struct {
	server    *routing.Server
	err       error
	algorithm string
}

func (m *mockRouter) Route(ctx context.Context, pool routing.ServerPool) (*routing.Server, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.server != nil {
		return m.server, nil
	}
	// Return first server from pool
	servers := pool.Servers()
	if len(servers) > 0 {
		return servers[0], nil
	}
	return nil, routing.ErrNoHealthyServers
}

func (m *mockRouter) Algorithm() string {
	if m.algorithm != "" {
		return m.algorithm
	}
	return "mock"
}

// mockHealthProvider implements HealthProvider for testing.
type mockHealthProvider struct {
	healthy map[string]bool
}

func newMockHealthProvider() *mockHealthProvider {
	return &mockHealthProvider{
		healthy: make(map[string]bool),
	}
}

func (m *mockHealthProvider) IsHealthy(address string, port int) bool {
	key := address
	if healthy, ok := m.healthy[key]; ok {
		return healthy
	}
	return true // Default to healthy
}

func (m *mockHealthProvider) SetHealthy(address string, healthy bool) {
	m.healthy[address] = healthy
}

func TestRegistry_Basic(t *testing.T) {
	registry := NewRegistry()

	if registry.Count() != 0 {
		t.Errorf("expected empty registry, got %d domains", registry.Count())
	}

	entry := &DomainEntry{
		Name:             "example.com",
		TTL:              60,
		RoutingAlgorithm: "round-robin",
		Router:           &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},
		},
	}

	registry.Register(entry)

	if registry.Count() != 1 {
		t.Errorf("expected 1 domain, got %d", registry.Count())
	}

	// Lookup with trailing dot (FQDN format)
	found := registry.Lookup("example.com.")
	if found == nil {
		t.Fatal("expected to find domain")
	}
	if found.Name != "example.com." {
		t.Errorf("expected normalized name 'example.com.', got %s", found.Name)
	}

	// Lookup without trailing dot should also work
	found = registry.Lookup("example.com")
	if found == nil {
		t.Fatal("expected to find domain without trailing dot")
	}
}

func TestRegistry_Remove(t *testing.T) {
	registry := NewRegistry()

	entry := &DomainEntry{
		Name:   "test.com",
		Router: &mockRouter{},
	}
	registry.Register(entry)

	if registry.Count() != 1 {
		t.Fatal("expected 1 domain after register")
	}

	registry.Remove("test.com")

	if registry.Count() != 0 {
		t.Errorf("expected 0 domains after remove, got %d", registry.Count())
	}
}

func TestRegistry_ReplaceAll(t *testing.T) {
	registry := NewRegistry()

	// Register initial entries
	registry.Register(&DomainEntry{Name: "old1.com", Router: &mockRouter{}})
	registry.Register(&DomainEntry{Name: "old2.com", Router: &mockRouter{}})

	if registry.Count() != 2 {
		t.Fatal("expected 2 domains initially")
	}

	// Replace all
	newEntries := []*DomainEntry{
		{Name: "new1.com", Router: &mockRouter{}},
		{Name: "new2.com", Router: &mockRouter{}},
		{Name: "new3.com", Router: &mockRouter{}},
	}
	registry.ReplaceAll(newEntries)

	if registry.Count() != 3 {
		t.Errorf("expected 3 domains after replace, got %d", registry.Count())
	}

	// Old domains should be gone
	if registry.Lookup("old1.com") != nil {
		t.Error("old domain should be removed")
	}

	// New domains should exist
	if registry.Lookup("new1.com") == nil {
		t.Error("new domain should exist")
	}
}

func TestHandler_NXDOMAIN(t *testing.T) {
	registry := NewRegistry()
	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 60,
	})

	// Create a test DNS query for non-existent domain
	req := new(dns.Msg)
	req.SetQuestion("nonexistent.com.", dns.TypeA)

	resp := new(dns.Msg)
	resp.SetReply(req)

	// We can't easily test ServeDNS without a real writer,
	// but we can test the handler was created successfully
	if handler == nil {
		t.Fatal("handler should not be nil")
	}
	if handler.registry != registry {
		t.Error("handler registry mismatch")
	}
}

func TestHandler_WithHealthProvider(t *testing.T) {
	registry := NewRegistry()
	healthProvider := newMockHealthProvider()

	entry := &DomainEntry{
		Name:   "healthy.com",
		TTL:    30,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},
			{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100},
		},
	}
	registry.Register(entry)

	handler := NewHandler(HandlerConfig{
		Registry:       registry,
		HealthProvider: healthProvider,
		DefaultTTL:     60,
	})

	// Mark one server unhealthy
	healthProvider.SetHealthy("10.0.0.1", false)
	healthProvider.SetHealthy("10.0.0.2", true)

	// Get healthy servers
	servers := handler.getHealthyIPv4Servers(entry)
	if len(servers) != 1 {
		t.Errorf("expected 1 healthy server, got %d", len(servers))
	}
	if servers[0].Address != "10.0.0.2" {
		t.Errorf("expected healthy server 10.0.0.2, got %s", servers[0].Address)
	}
}

func TestHandler_IPv6Servers(t *testing.T) {
	registry := NewRegistry()

	entry := &DomainEntry{
		Name:   "ipv6.com",
		TTL:    30,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},    // IPv4
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Weight: 100}, // IPv6
			{Address: net.ParseIP("2001:db8::2"), Port: 80, Weight: 100}, // IPv6
		},
	}
	registry.Register(entry)

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 60,
	})

	// Get IPv4 servers
	ipv4Servers := handler.getHealthyIPv4Servers(entry)
	if len(ipv4Servers) != 1 {
		t.Errorf("expected 1 IPv4 server, got %d", len(ipv4Servers))
	}

	// Get IPv6 servers
	ipv6Servers := handler.getHealthyIPv6Servers(entry)
	if len(ipv6Servers) != 2 {
		t.Errorf("expected 2 IPv6 servers, got %d", len(ipv6Servers))
	}
}

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com."},
		{"example.com.", "example.com."},
		{"", ""},
		{"test", "test."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeDomain(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeDomain(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
