package dns

import (
	"context"
	"net"
	"testing"
	"time"

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

func (m *mockHealthProvider) IsHealthy(address string, _ int) bool {
	healthy, ok := m.healthyAddresses[address]
	if !ok {
		return true
	}
	return healthy
}

// mockResponseWriter captures DNS responses for testing.
type mockResponseWriter struct {
	msg *dns.Msg
}

func (m *mockResponseWriter) LocalAddr() net.Addr         { return nil }
func (m *mockResponseWriter) RemoteAddr() net.Addr        { return nil }
func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error { m.msg = msg; return nil }
func (m *mockResponseWriter) Write([]byte) (int, error)   { return 0, nil }
func (m *mockResponseWriter) Close() error                { return nil }
func (m *mockResponseWriter) TsigStatus() error           { return nil }
func (m *mockResponseWriter) TsigTimersOnly(bool)         {}
func (m *mockResponseWriter) Hijack()                     {}

func TestHandler_A_Record(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "us-east"},
		},
	})

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 30,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg == nil {
		t.Fatal("no response received")
	}

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	if len(w.msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(w.msg.Answer))
	}

	aRecord, ok := w.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("answer is not A record")
	}

	if !aRecord.A.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("expected 10.0.0.1, got %s", aRecord.A)
	}

	if aRecord.Hdr.Ttl != 60 {
		t.Errorf("expected TTL 60, got %d", aRecord.Hdr.Ttl)
	}
}

func TestHandler_AAAA_Record(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    120,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Region: "us-east"},
		},
	})

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 30,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeAAAA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg == nil {
		t.Fatal("no response received")
	}

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	if len(w.msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(w.msg.Answer))
	}

	aaaaRecord, ok := w.msg.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatal("answer is not AAAA record")
	}

	expected := net.ParseIP("2001:db8::1")
	if !aaaaRecord.AAAA.Equal(expected) {
		t.Errorf("expected %s, got %s", expected, aaaaRecord.AAAA)
	}

	if aaaaRecord.Hdr.Ttl != 120 {
		t.Errorf("expected TTL 120, got %d", aaaaRecord.Hdr.Ttl)
	}
}

func TestHandler_MixedAddressFamily(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "mixed.example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "us-east"},
			{Address: net.ParseIP("10.0.0.2"), Port: 80, Region: "us-west"},
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Region: "eu-west"},
			{Address: net.ParseIP("2001:db8::2"), Port: 80, Region: "ap-east"},
		},
	})

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 30,
	})

	t.Run("A query returns IPv4 only", func(t *testing.T) {
		req := new(dns.Msg)
		req.SetQuestion("mixed.example.com.", dns.TypeA)

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		if w.msg.Rcode != dns.RcodeSuccess {
			t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
		}

		if len(w.msg.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(w.msg.Answer))
		}

		aRecord, ok := w.msg.Answer[0].(*dns.A)
		if !ok {
			t.Fatal("answer is not A record")
		}

		if aRecord.A.To4() == nil {
			t.Errorf("expected IPv4 address, got %s", aRecord.A)
		}
	})

	t.Run("AAAA query returns IPv6 only", func(t *testing.T) {
		req := new(dns.Msg)
		req.SetQuestion("mixed.example.com.", dns.TypeAAAA)

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		if w.msg.Rcode != dns.RcodeSuccess {
			t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
		}

		if len(w.msg.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(w.msg.Answer))
		}

		aaaaRecord, ok := w.msg.Answer[0].(*dns.AAAA)
		if !ok {
			t.Fatal("answer is not AAAA record")
		}

		if aaaaRecord.AAAA.To4() != nil {
			t.Errorf("expected IPv6 address, got %s", aaaaRecord.AAAA)
		}
	})
}

func TestHandler_AAAA_NoIPv6Servers(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "ipv4only.example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "us-east"},
			{Address: net.ParseIP("10.0.0.2"), Port: 80, Region: "us-west"},
		},
	})

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 30,
	})

	req := new(dns.Msg)
	req.SetQuestion("ipv4only.example.com.", dns.TypeAAAA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	if len(w.msg.Answer) != 0 {
		t.Errorf("expected 0 answers for IPv4-only domain with AAAA query, got %d", len(w.msg.Answer))
	}
}

func TestHandler_A_NoIPv4Servers(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "ipv6only.example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Region: "eu-west"},
			{Address: net.ParseIP("2001:db8::2"), Port: 80, Region: "ap-east"},
		},
	})

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 30,
	})

	req := new(dns.Msg)
	req.SetQuestion("ipv6only.example.com.", dns.TypeA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	if len(w.msg.Answer) != 0 {
		t.Errorf("expected 0 answers for IPv6-only domain with A query, got %d", len(w.msg.Answer))
	}
}

func TestHandler_NXDOMAIN(t *testing.T) {
	registry := NewRegistry()
	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 30,
	})

	t.Run("A query for unknown domain", func(t *testing.T) {
		req := new(dns.Msg)
		req.SetQuestion("unknown.example.com.", dns.TypeA)

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		if w.msg.Rcode != dns.RcodeNameError {
			t.Errorf("expected NXDOMAIN, got %s", dns.RcodeToString[w.msg.Rcode])
		}
	})

	t.Run("AAAA query for unknown domain", func(t *testing.T) {
		req := new(dns.Msg)
		req.SetQuestion("unknown.example.com.", dns.TypeAAAA)

		w := &mockResponseWriter{}
		handler.ServeDNS(w, req)

		if w.msg.Rcode != dns.RcodeNameError {
			t.Errorf("expected NXDOMAIN, got %s", dns.RcodeToString[w.msg.Rcode])
		}
	})
}

func TestHandler_HealthyServerFiltering_IPv6(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Region: "eu-west"},
			{Address: net.ParseIP("2001:db8::2"), Port: 80, Region: "ap-east"},
		},
	})

	healthProvider := newMockHealthProvider()
	healthProvider.SetHealthy("2001:db8::1", false)
	healthProvider.SetHealthy("2001:db8::2", true)

	handler := NewHandler(HandlerConfig{
		Registry:       registry,
		HealthProvider: healthProvider,
		DefaultTTL:     30,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeAAAA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	if len(w.msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(w.msg.Answer))
	}

	aaaaRecord := w.msg.Answer[0].(*dns.AAAA)
	expected := net.ParseIP("2001:db8::2")
	if !aaaaRecord.AAAA.Equal(expected) {
		t.Errorf("expected healthy server %s, got %s", expected, aaaaRecord.AAAA)
	}
}

func TestHandler_AllIPv6ServersUnhealthy(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Region: "eu-west"},
			{Address: net.ParseIP("2001:db8::2"), Port: 80, Region: "ap-east"},
		},
	})

	healthProvider := newMockHealthProvider()
	healthProvider.SetHealthy("2001:db8::1", false)
	healthProvider.SetHealthy("2001:db8::2", false)

	handler := NewHandler(HandlerConfig{
		Registry:       registry,
		HealthProvider: healthProvider,
		DefaultTTL:     30,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeAAAA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	if len(w.msg.Answer) != 0 {
		t.Errorf("expected 0 answers when all IPv6 servers unhealthy, got %d", len(w.msg.Answer))
	}
}

func TestHandler_DefaultTTL(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    0,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Region: "eu-west"},
		},
	})

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 45,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeAAAA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	aaaaRecord := w.msg.Answer[0].(*dns.AAAA)
	if aaaaRecord.Hdr.Ttl != 45 {
		t.Errorf("expected default TTL 45, got %d", aaaaRecord.Hdr.Ttl)
	}
}

func TestFilterByAddressFamily(t *testing.T) {
	servers := []ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "r1"},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Region: "r2"},
		{Address: net.ParseIP("2001:db8::1"), Port: 80, Region: "r3"},
		{Address: net.ParseIP("2001:db8::2"), Port: 80, Region: "r4"},
	}

	t.Run("filter IPv4", func(t *testing.T) {
		ipv4 := filterByAddressFamily(servers, true)
		if len(ipv4) != 2 {
			t.Errorf("expected 2 IPv4 servers, got %d", len(ipv4))
		}
		for _, s := range ipv4 {
			if s.Address.To4() == nil {
				t.Errorf("expected IPv4, got %s", s.Address)
			}
		}
	})

	t.Run("filter IPv6", func(t *testing.T) {
		ipv6 := filterByAddressFamily(servers, false)
		if len(ipv6) != 2 {
			t.Errorf("expected 2 IPv6 servers, got %d", len(ipv6))
		}
		for _, s := range ipv6 {
			if s.Address.To4() != nil {
				t.Errorf("expected IPv6, got %s", s.Address)
			}
		}
	})

	t.Run("empty input", func(t *testing.T) {
		result := filterByAddressFamily([]ServerInfo{}, true)
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d", len(result))
		}
	})

	t.Run("no matching family", func(t *testing.T) {
		ipv4Only := []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "r1"},
		}
		result := filterByAddressFamily(ipv4Only, false)
		if len(result) != 0 {
			t.Errorf("expected 0 IPv6 servers, got %d", len(result))
		}
	})
}

func TestServer_Integration_AAAA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "test.local",
		TTL:    30,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Region: "test-region"},
		},
	})

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 30,
	})

	server := NewServer(ServerConfig{
		Address: "127.0.0.1:15354",
		Handler: handler,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	client := new(dns.Client)
	msg := new(dns.Msg)
	msg.SetQuestion("test.local.", dns.TypeAAAA)

	resp, _, err := client.Exchange(msg, "127.0.0.1:15354")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if resp.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[resp.Rcode])
	}

	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}

	aaaaRecord, ok := resp.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatal("answer is not AAAA record")
	}

	expected := net.ParseIP("2001:db8::1")
	if !aaaaRecord.AAAA.Equal(expected) {
		t.Errorf("expected %s, got %s", expected, aaaaRecord.AAAA)
	}

	cancel()
	select {
	case <-errChan:
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down in time")
	}
}

// Test per-domain routing algorithms
func TestHandler_PerDomainRouting(t *testing.T) {
	// Create two mock routers that track which one was called
	router1Called := false
	router2Called := false

	mockRouter1 := &trackingRouter{
		algorithm: "algo1",
		onRoute: func() {
			router1Called = true
		},
	}
	mockRouter2 := &trackingRouter{
		algorithm: "algo2",
		onRoute: func() {
			router2Called = true
		},
	}

	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "domain1.example.com",
		TTL:    60,
		Router: mockRouter1,
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "r1"},
		},
	})
	registry.Register(&DomainEntry{
		Name:   "domain2.example.com",
		TTL:    60,
		Router: mockRouter2,
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.2"), Port: 80, Region: "r2"},
		},
	})

	handler := NewHandler(HandlerConfig{
		Registry:   registry,
		DefaultTTL: 30,
	})

	// Query domain1 - should use router1
	req1 := new(dns.Msg)
	req1.SetQuestion("domain1.example.com.", dns.TypeA)
	w1 := &mockResponseWriter{}
	handler.ServeDNS(w1, req1)

	if !router1Called {
		t.Error("router1 should have been called for domain1")
	}
	if router2Called {
		t.Error("router2 should NOT have been called for domain1")
	}

	// Reset
	router1Called = false
	router2Called = false

	// Query domain2 - should use router2
	req2 := new(dns.Msg)
	req2.SetQuestion("domain2.example.com.", dns.TypeA)
	w2 := &mockResponseWriter{}
	handler.ServeDNS(w2, req2)

	if router1Called {
		t.Error("router1 should NOT have been called for domain2")
	}
	if !router2Called {
		t.Error("router2 should have been called for domain2")
	}
}

// trackingRouter is a router that tracks when it's called
type trackingRouter struct {
	algorithm string
	onRoute   func()
}

func (r *trackingRouter) Route(_ context.Context, _ string, servers []ServerInfo) (*ServerInfo, error) {
	if r.onRoute != nil {
		r.onRoute()
	}
	if len(servers) == 0 {
		return nil, nil
	}
	return &servers[0], nil
}

func (r *trackingRouter) Algorithm() string {
	return r.algorithm
}
