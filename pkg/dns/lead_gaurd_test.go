// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// mockLeaderChecker allows tests to control leadership status.
type mockLeaderChecker struct {
	isLeader atomic.Bool
}

func newMockLeaderChecker(isLeader bool) *mockLeaderChecker {
	m := &mockLeaderChecker{}
	m.isLeader.Store(isLeader)
	return m
}

func (m *mockLeaderChecker) IsLeader() bool {
	return m.isLeader.Load()
}

func (m *mockLeaderChecker) SetLeader(isLeader bool) {
	m.isLeader.Store(isLeader)
}

func TestHandler_LeaderGuard_RefusesWhenNotLeader(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "us-east"},
		},
	})

	leaderChecker := newMockLeaderChecker(false) // Not leader

	handler := NewHandler(HandlerConfig{
		Registry:      registry,
		LeaderChecker: leaderChecker,
		DefaultTTL:    30,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg == nil {
		t.Fatal("no response received")
	}

	// Should return REFUSED when not leader
	if w.msg.Rcode != dns.RcodeRefused {
		t.Errorf("expected REFUSED, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// Should have no answers
	if len(w.msg.Answer) != 0 {
		t.Errorf("expected 0 answers, got %d", len(w.msg.Answer))
	}
}

func TestHandler_LeaderGuard_ServesWhenLeader(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "us-east"},
		},
	})

	leaderChecker := newMockLeaderChecker(true) // Is leader

	handler := NewHandler(HandlerConfig{
		Registry:      registry,
		LeaderChecker: leaderChecker,
		DefaultTTL:    30,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg == nil {
		t.Fatal("no response received")
	}

	// Should return NOERROR when leader
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// Should have an answer
	if len(w.msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(w.msg.Answer))
	}
}

func TestHandler_LeaderGuard_StandaloneMode(t *testing.T) {
	// When LeaderChecker is nil, should default to standalone (always leader)
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
		Registry:      registry,
		LeaderChecker: nil, // Standalone mode
		DefaultTTL:    30,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg == nil {
		t.Fatal("no response received")
	}

	// Standalone should always serve queries
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR in standalone mode, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	if len(w.msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(w.msg.Answer))
	}
}

func TestHandler_LeaderGuard_LeaderTransition(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "us-east"},
		},
	})

	leaderChecker := newMockLeaderChecker(false) // Start as follower

	handler := NewHandler(HandlerConfig{
		Registry:      registry,
		LeaderChecker: leaderChecker,
		DefaultTTL:    30,
	})

	// Query as follower - should refuse
	req1 := new(dns.Msg)
	req1.SetQuestion("example.com.", dns.TypeA)
	w1 := &mockResponseWriter{}
	handler.ServeDNS(w1, req1)

	if w1.msg.Rcode != dns.RcodeRefused {
		t.Errorf("expected REFUSED as follower, got %s", dns.RcodeToString[w1.msg.Rcode])
	}

	// Transition to leader
	leaderChecker.SetLeader(true)

	// Query as leader - should serve
	req2 := new(dns.Msg)
	req2.SetQuestion("example.com.", dns.TypeA)
	w2 := &mockResponseWriter{}
	handler.ServeDNS(w2, req2)

	if w2.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR as leader, got %s", dns.RcodeToString[w2.msg.Rcode])
	}

	// Transition back to follower
	leaderChecker.SetLeader(false)

	// Query as follower again - should refuse
	req3 := new(dns.Msg)
	req3.SetQuestion("example.com.", dns.TypeA)
	w3 := &mockResponseWriter{}
	handler.ServeDNS(w3, req3)

	if w3.msg.Rcode != dns.RcodeRefused {
		t.Errorf("expected REFUSED after losing leadership, got %s", dns.RcodeToString[w3.msg.Rcode])
	}
}

func TestHandler_LeaderGuard_AAAA_RefusesWhenNotLeader(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Region: "eu-west"},
		},
	})

	leaderChecker := newMockLeaderChecker(false) // Not leader

	handler := NewHandler(HandlerConfig{
		Registry:      registry,
		LeaderChecker: leaderChecker,
		DefaultTTL:    30,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeAAAA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg.Rcode != dns.RcodeRefused {
		t.Errorf("expected REFUSED for AAAA query when not leader, got %s", dns.RcodeToString[w.msg.Rcode])
	}
}

func TestHandler_LeaderGuard_RefusedWithEDNS(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "us-east"},
		},
	})

	leaderChecker := newMockLeaderChecker(false) // Not leader

	handler := NewHandler(HandlerConfig{
		Registry:      registry,
		LeaderChecker: leaderChecker,
		DefaultTTL:    30,
	})

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	req.SetEdns0(4096, false) // Add EDNS

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	if w.msg.Rcode != dns.RcodeRefused {
		t.Errorf("expected REFUSED, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// Should still include EDNS in response when client sent it
	var responseOPT *dns.OPT
	for _, rr := range w.msg.Extra {
		if opt, ok := rr.(*dns.OPT); ok {
			responseOPT = opt
			break
		}
	}

	if responseOPT == nil {
		t.Error("expected EDNS OPT in REFUSED response when client sent EDNS")
	}
}

func TestHandler_LeaderGuard_UnknownDomainWhenNotLeader(t *testing.T) {
	// Even for unknown domains, non-leader should return REFUSED, not NXDOMAIN
	registry := NewRegistry()

	leaderChecker := newMockLeaderChecker(false) // Not leader

	handler := NewHandler(HandlerConfig{
		Registry:      registry,
		LeaderChecker: leaderChecker,
		DefaultTTL:    30,
	})

	req := new(dns.Msg)
	req.SetQuestion("unknown.example.com.", dns.TypeA)

	w := &mockResponseWriter{}
	handler.ServeDNS(w, req)

	// Non-leader refuses ALL queries, even for unknown domains
	if w.msg.Rcode != dns.RcodeRefused {
		t.Errorf("expected REFUSED for any query when not leader, got %s", dns.RcodeToString[w.msg.Rcode])
	}
}

func TestHandler_LeaderGuard_ConcurrentAccess(t *testing.T) {
	// Test thread safety of leadership check during concurrent queries
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "example.com",
		TTL:    60,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "us-east"},
		},
	})

	leaderChecker := newMockLeaderChecker(true)

	handler := NewHandler(HandlerConfig{
		Registry:      registry,
		LeaderChecker: leaderChecker,
		DefaultTTL:    30,
	})

	// Run concurrent queries while toggling leadership
	done := make(chan bool)
	queryCount := 100

	// Toggle leadership in a goroutine
	go func() {
		for i := 0; i < queryCount/2; i++ {
			leaderChecker.SetLeader(i%2 == 0)
			time.Sleep(time.Microsecond)
		}
	}()

	// Send concurrent queries
	for i := 0; i < queryCount; i++ {
		go func() {
			req := new(dns.Msg)
			req.SetQuestion("example.com.", dns.TypeA)
			w := &mockResponseWriter{}
			handler.ServeDNS(w, req)

			// Response should be either SUCCESS or REFUSED, never panic
			if w.msg == nil {
				t.Error("nil response during concurrent access")
				return
			}
			if w.msg.Rcode != dns.RcodeSuccess && w.msg.Rcode != dns.RcodeRefused {
				t.Errorf("unexpected rcode: %s", dns.RcodeToString[w.msg.Rcode])
			}
			done <- true
		}()
	}

	// Wait for all queries to complete
	for i := 0; i < queryCount; i++ {
		<-done
	}
}

func TestServer_Integration_LeaderGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "test.local",
		TTL:    30,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Region: "test-region"},
		},
	})

	leaderChecker := newMockLeaderChecker(false) // Start as non-leader

	handler := NewHandler(HandlerConfig{
		Registry:      registry,
		LeaderChecker: leaderChecker,
		DefaultTTL:    30,
	})

	server := NewServer(ServerConfig{
		Address: "127.0.0.1:15355",
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

	// Query as non-leader - should get REFUSED
	msg := new(dns.Msg)
	msg.SetQuestion("test.local.", dns.TypeA)

	resp, _, err := client.Exchange(msg, "127.0.0.1:15355")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if resp.Rcode != dns.RcodeRefused {
		t.Errorf("expected REFUSED as non-leader, got %s", dns.RcodeToString[resp.Rcode])
	}

	// Become leader
	leaderChecker.SetLeader(true)

	// Query as leader - should get response
	resp2, _, err := client.Exchange(msg, "127.0.0.1:15355")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if resp2.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR as leader, got %s", dns.RcodeToString[resp2.Rcode])
	}

	if len(resp2.Answer) != 1 {
		t.Errorf("expected 1 answer, got %d", len(resp2.Answer))
	}

	cancel()
	select {
	case <-errChan:
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down in time")
	}
}
