package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/miekg/dns"
)

// mockRouter is a simple router that returns the first server.
type mockRouter struct {
	returnErr error
}

func (m *mockRouter) Route(_ context.Context, _ string, servers []ServerInfo) (*ServerInfo, error) {
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	if len(servers) == 0 {
		return nil, nil
	}
	return &servers[0], nil
}

func (m *mockRouter) Algorithm() string {
	return "mock"
}

// mockHealthProvider allows control over which servers are healthy.
type mockHealthProvider struct {
	healthyAddresses map[string]bool
}

func newMockHealthProvider() *mockHealthProvider {
	return &mockHealthProvider{
		healthyAddresses: make(map[string]bool),
	}
}

func (m *mockHealthProvider) SetHealthy(address string, healthy bool) {
	m.healthyAddresses[address] = healthy
}

func (m *mockHealthProvider) IsHealthy(address string) bool {
	healthy, ok := m.healthyAddresses[address]
	if !ok {
		return true // Default to healthy if not explicitly set
	}
	return healthy
}

// mockResponseWriter captures DNS responses for testing.
type mockResponseWriter struct {
	msg *dns.Msg
}

func (m *mockResponseWriter) LocalAddr() net.Addr  { return nil }
func (m *mockResponseWriter) RemoteAddr() net.Addr { return nil }
func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error {
	m.msg = msg
	return nil
}
func (m *mockResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (m *mockResponseWriter) Close() error              { return nil }
func (m *mockResponseWriter) TsigStatus() error         { return nil }
func (m *mockResponseWriter) TsigTimersOnly(bool)       {}
func (m *mockResponseWriter) Hijack()                   {}

// TestRegistry tests the domain registry functionality.
func TestRegistry(t *testing.T) {
	t.Run("register and lookup domain", func(t *testing.T) {
		r := NewRegistry()

		entry := &DomainEntry{
			Name:             "example.com",
			TTL:              60,
			RoutingAlgorithm: "round-robin",
			Servers: []ServerInfo{
				{Address: net.ParseIP("10.0.1.10"), Port: 80, Region: "us-east-1"},
			},
		}

		r.Register(entry)

		// Lookup without trailing dot
		found := r.Lookup("example.com")
		if found == nil {
			t.Fatal("expected to find domain")
		}
		if found.TTL != 60 {
			t.Errorf("expected TTL 60, got %d", found.TTL)
		}

		// Lookup with trailing dot (FQDN format)
		found = r.Lookup("example.com.")
		if found == nil {
			t.Fatal("expected to find domain with FQDN format")
		}
	})

	t.Run("lookup unknown domain returns nil", func(t *testing.T) {
		r := NewRegistry()
		found := r.Lookup("unknown.com")
		if found != nil {
			t.Error("expected nil for unknown domain")
		}
	})

	t.Run("remove domain", func(t *testing.T) {
		r := NewRegistry()

		entry := &DomainEntry{Name: "example.com", TTL: 60}
		r.Register(entry)

		if r.Lookup("example.com") == nil {
			t.Fatal("domain should exist after registration")
		}

		r.Remove("example.com")

		if r.Lookup("example.com") != nil {
			t.Error("domain should not exist after removal")
		}
	})

	t.Run("count domains", func(t *testing.T) {
		r := NewRegistry()

		if r.Count() != 0 {
			t.Errorf("expected 0 domains, got %d", r.Count())
		}

		r.Register(&DomainEntry{Name: "a.com"})
		r.Register(&DomainEntry{Name: "b.com"})

		if r.Count() != 2 {
			t.Errorf("expected 2 domains, got %d", r.Count())
		}
	})

	t.Run("list domains", func(t *testing.T) {
		r := NewRegistry()

		r.Register(&DomainEntry{Name: "a.com"})
		r.Register(&DomainEntry{Name: "b.com"})

		domains := r.Domains()
		if len(domains) != 2 {
			t.Errorf("expected 2 domains, got %d", len(domains))
		}
	})
}

// TestHandler tests the DNS handler.
func TestHandler(t *testing.T) {
	t.Run("A record query returns server IP", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(&DomainEntry{
			Name: "app.example.com",
			TTL:  30,
			Servers: []ServerInfo{
				{Address: net.ParseIP("10.0.1.10"), Port: 80, Region: "us-east-1"},
			},
		})

		handler := NewHandler(HandlerConfig{
			Registry:   registry,
			Router:     &mockRouter{},
			DefaultTTL: 60,
		})

		req := new(dns.Msg)
		req.SetQuestion("app.example.com.", dns.TypeA)

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		if w.msg == nil {
			t.Fatal("expected response message")
		}

		if w.msg.Rcode != dns.RcodeSuccess {
			t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
		}

		if len(w.msg.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(w.msg.Answer))
		}

		a, ok := w.msg.Answer[0].(*dns.A)
		if !ok {
			t.Fatal("expected A record")
		}

		if !a.A.Equal(net.ParseIP("10.0.1.10")) {
			t.Errorf("expected 10.0.1.10, got %s", a.A)
		}

		if a.Hdr.Ttl != 30 {
			t.Errorf("expected TTL 30, got %d", a.Hdr.Ttl)
		}
	})

	t.Run("unknown domain returns NXDOMAIN", func(t *testing.T) {
		registry := NewRegistry()
		handler := NewHandler(HandlerConfig{
			Registry:   registry,
			Router:     &mockRouter{},
			DefaultTTL: 60,
		})

		req := new(dns.Msg)
		req.SetQuestion("unknown.example.com.", dns.TypeA)

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		if w.msg.Rcode != dns.RcodeNameError {
			t.Errorf("expected NXDOMAIN, got %s", dns.RcodeToString[w.msg.Rcode])
		}
	})

	t.Run("uses default TTL when domain TTL is 0", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(&DomainEntry{
			Name: "app.example.com",
			TTL:  0, // No domain-specific TTL
			Servers: []ServerInfo{
				{Address: net.ParseIP("10.0.1.10"), Port: 80},
			},
		})

		handler := NewHandler(HandlerConfig{
			Registry:   registry,
			Router:     &mockRouter{},
			DefaultTTL: 120, // Should use this
		})

		req := new(dns.Msg)
		req.SetQuestion("app.example.com.", dns.TypeA)

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		a := w.msg.Answer[0].(*dns.A)
		if a.Hdr.Ttl != 120 {
			t.Errorf("expected default TTL 120, got %d", a.Hdr.Ttl)
		}
	})

	t.Run("filters unhealthy servers", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(&DomainEntry{
			Name: "app.example.com",
			TTL:  30,
			Servers: []ServerInfo{
				{Address: net.ParseIP("10.0.1.10"), Port: 80, Region: "us-east-1"},
				{Address: net.ParseIP("10.0.1.11"), Port: 80, Region: "us-east-1"},
			},
		})

		healthProvider := newMockHealthProvider()
		healthProvider.SetHealthy("10.0.1.10", false) // Mark first server unhealthy
		healthProvider.SetHealthy("10.0.1.11", true)

		handler := NewHandler(HandlerConfig{
			Registry:       registry,
			Router:         &mockRouter{},
			HealthProvider: healthProvider,
			DefaultTTL:     60,
		})

		req := new(dns.Msg)
		req.SetQuestion("app.example.com.", dns.TypeA)

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		if w.msg.Rcode != dns.RcodeSuccess {
			t.Fatalf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
		}

		a := w.msg.Answer[0].(*dns.A)
		// Should return the healthy server (10.0.1.11)
		if !a.A.Equal(net.ParseIP("10.0.1.11")) {
			t.Errorf("expected healthy server 10.0.1.11, got %s", a.A)
		}
	})

	t.Run("returns SERVFAIL when all servers unhealthy", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(&DomainEntry{
			Name: "app.example.com",
			TTL:  30,
			Servers: []ServerInfo{
				{Address: net.ParseIP("10.0.1.10"), Port: 80},
			},
		})

		healthProvider := newMockHealthProvider()
		healthProvider.SetHealthy("10.0.1.10", false)

		handler := NewHandler(HandlerConfig{
			Registry:       registry,
			Router:         &mockRouter{},
			HealthProvider: healthProvider,
			DefaultTTL:     60,
		})

		req := new(dns.Msg)
		req.SetQuestion("app.example.com.", dns.TypeA)

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		if w.msg.Rcode != dns.RcodeServerFailure {
			t.Errorf("expected SERVFAIL, got %s", dns.RcodeToString[w.msg.Rcode])
		}
	})

	t.Run("empty question returns FORMERR", func(t *testing.T) {
		registry := NewRegistry()
		handler := NewHandler(HandlerConfig{
			Registry:   registry,
			Router:     &mockRouter{},
			DefaultTTL: 60,
		})

		req := new(dns.Msg)
		req.Id = dns.Id()
		// No question set

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		if w.msg.Rcode != dns.RcodeFormatError {
			t.Errorf("expected FORMERR, got %s", dns.RcodeToString[w.msg.Rcode])
		}
	})
}

// TestNormalizeDomain tests domain name normalization.
func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com."},
		{"example.com.", "example.com."},
		{"sub.example.com", "sub.example.com."},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeDomain(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeDomain(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestBuildRegistry tests building registry from configuration.
func TestBuildRegistry(t *testing.T) {
	t.Run("builds registry from config", func(t *testing.T) {
		cfg := &config.Config{
			Regions: []config.Region{
				{
					Name: "us-east-1",
					Servers: []config.Server{
						{Address: "10.0.1.10", Port: 80, Weight: 100},
						{Address: "10.0.1.11", Port: 80, Weight: 100},
					},
				},
				{
					Name: "us-west-2",
					Servers: []config.Server{
						{Address: "10.0.2.10", Port: 80, Weight: 100},
					},
				},
			},
			Domains: []config.Domain{
				{
					Name:             "app.example.com",
					RoutingAlgorithm: "round-robin",
					Regions:          []string{"us-east-1", "us-west-2"},
					TTL:              30,
				},
			},
		}

		registry, err := BuildRegistry(cfg)
		if err != nil {
			t.Fatalf("BuildRegistry failed: %v", err)
		}

		entry := registry.Lookup("app.example.com")
		if entry == nil {
			t.Fatal("expected to find domain")
		}

		if len(entry.Servers) != 3 {
			t.Errorf("expected 3 servers, got %d", len(entry.Servers))
		}

		if entry.TTL != 30 {
			t.Errorf("expected TTL 30, got %d", entry.TTL)
		}

		if entry.RoutingAlgorithm != "round-robin" {
			t.Errorf("expected round-robin, got %s", entry.RoutingAlgorithm)
		}
	})

	t.Run("error on unknown region", func(t *testing.T) {
		cfg := &config.Config{
			Regions: []config.Region{
				{Name: "us-east-1", Servers: []config.Server{{Address: "10.0.1.10", Port: 80}}},
			},
			Domains: []config.Domain{
				{Name: "app.example.com", Regions: []string{"unknown-region"}},
			},
		}

		_, err := BuildRegistry(cfg)
		if err == nil {
			t.Error("expected error for unknown region")
		}
	})

	t.Run("error on invalid IP", func(t *testing.T) {
		cfg := &config.Config{
			Regions: []config.Region{
				{Name: "us-east-1", Servers: []config.Server{{Address: "invalid-ip", Port: 80}}},
			},
			Domains: []config.Domain{
				{Name: "app.example.com", Regions: []string{"us-east-1"}},
			},
		}

		_, err := BuildRegistry(cfg)
		if err == nil {
			t.Error("expected error for invalid IP")
		}
	})

	t.Run("error on domain with no servers", func(t *testing.T) {
		cfg := &config.Config{
			Regions: []config.Region{
				{Name: "us-east-1", Servers: []config.Server{}}, // Empty servers
			},
			Domains: []config.Domain{
				{Name: "app.example.com", Regions: []string{"us-east-1"}},
			},
		}

		_, err := BuildRegistry(cfg)
		if err == nil {
			t.Error("expected error for domain with no servers")
		}
	})
}

// TestServerIntegration tests the server start/stop lifecycle.
// This test requires a non-privileged port since port 53 requires root.
func TestServerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name: "test.example.com",
		TTL:  30,
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.1.10"), Port: 80, Region: "us-east-1"},
		},
	})

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		Router:     &mockRouter{},
		DefaultTTL: 60,
	})

	// Use a high port that doesn't require root
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:15353",
		Handler: handler,
	})

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)

	go func() {
		errChan <- server.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	if !server.IsRunning() {
		t.Fatal("server should be running")
	}

	// Send a DNS query
	client := &dns.Client{Net: "udp"}
	msg := new(dns.Msg)
	msg.SetQuestion("test.example.com.", dns.TypeA)

	resp, _, err := client.Exchange(msg, "127.0.0.1:15353")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if resp.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[resp.Rcode])
	}

	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}

	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("expected A record")
	}

	if !a.A.Equal(net.ParseIP("10.0.1.10")) {
		t.Errorf("expected 10.0.1.10, got %s", a.A)
	}

	// Test NXDOMAIN for unknown domain
	msg2 := new(dns.Msg)
	msg2.SetQuestion("unknown.example.com.", dns.TypeA)

	resp2, _, err := client.Exchange(msg2, "127.0.0.1:15353")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if resp2.Rcode != dns.RcodeNameError {
		t.Errorf("expected NXDOMAIN, got %s", dns.RcodeToString[resp2.Rcode])
	}

	// Shutdown
	cancel()

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("server returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not shutdown in time")
	}

	if server.IsRunning() {
		t.Error("server should not be running after shutdown")
	}
}
