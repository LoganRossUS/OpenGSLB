// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"net"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/miekg/dns"
)

// lastHealthyFixture builds a handler for lh.gslb.example.com with two IPv4
// servers and one IPv6 server, and v4only.gslb.example.com with one IPv4
// server.
func lastHealthyFixture(t *testing.T, returnLastHealthy bool) (*Handler, *mockHealthProvider) {
	t.Helper()
	registry := NewRegistry()
	registry.Register(&DomainEntry{
		Name:   "lh.gslb.example.com",
		TTL:    30,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},
			{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100},
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Weight: 100},
		},
	})
	registry.Register(&DomainEntry{
		Name:   "v4only.gslb.example.com",
		TTL:    30,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 100},
		},
	})
	health := newMockHealthProvider()
	handler := NewHandler(HandlerConfig{
		Registry:          registry,
		Zones:             mustZones(t, "n", config.Zone{Name: "gslb.example.com"}),
		HealthProvider:    health,
		DefaultTTL:        60,
		ReturnLastHealthy: returnLastHealthy,
	})
	return handler, health
}

// TestHandler_ReturnLastHealthy covers finding 1B #7: with
// dns.return_last_healthy set and every server of the queried family
// unhealthy, the handler answers as if all of them were healthy and counts
// the answer in opengslb_dns_last_healthy_answers_total.
func TestHandler_ReturnLastHealthy(t *testing.T) {
	handler, health := lastHealthyFixture(t, true)
	counter := metrics.DNSLastHealthyAnswersTotal.WithLabelValues("lh.gslb.example.com.", "A")
	counterAAAA := metrics.DNSLastHealthyAnswersTotal.WithLabelValues("lh.gslb.example.com.", "AAAA")

	// One server healthy: normal answer from the healthy one, not counted.
	health.SetHealthy("10.0.0.1", false)
	before := counterValue(t, counter)
	resp := serveQuery(t, handler, "udp", question("lh.gslb.example.com.", dns.TypeA))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("one healthy server: rcode = %s, answers = %d", dns.RcodeToString[resp.Rcode], len(resp.Answer))
	}
	if got := resp.Answer[0].(*dns.A).A.String(); got != "10.0.0.2" {
		t.Errorf("one healthy server: answer = %s, want the healthy 10.0.0.2", got)
	}
	if got := counterValue(t, counter) - before; got != 0 {
		t.Errorf("one healthy server: last-healthy counter increased by %v, want 0", got)
	}

	// Every IPv4 server unhealthy: answered from the configured IPv4 servers.
	health.SetHealthy("10.0.0.2", false)
	before = counterValue(t, counter)
	resp = serveQuery(t, handler, "udp", question("lh.gslb.example.com.", dns.TypeA))
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("all down: rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if !resp.Authoritative {
		t.Error("all down: AA bit is clear")
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("all down: answers = %d, want 1", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || (a.A.String() != "10.0.0.1" && a.A.String() != "10.0.0.2") {
		t.Errorf("all down: answer = %v, want one of the configured IPv4 servers", resp.Answer[0])
	}
	if a != nil && a.Hdr.Ttl != 30 {
		t.Errorf("all down: TTL = %d, want the domain TTL 30", a.Hdr.Ttl)
	}
	if got := counterValue(t, counter) - before; got != 1 {
		t.Errorf("all down: last-healthy counter increased by %v, want 1", got)
	}

	// The IPv6 server of the same name is healthy: AAAA is a normal answer.
	before = counterValue(t, counterAAAA)
	resp = serveQuery(t, handler, "udp", question("lh.gslb.example.com.", dns.TypeAAAA))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("AAAA: rcode = %s, answers = %d", dns.RcodeToString[resp.Rcode], len(resp.Answer))
	}
	if got := counterValue(t, counterAAAA) - before; got != 0 {
		t.Errorf("AAAA with a healthy server: last-healthy counter increased by %v, want 0", got)
	}

	// Every IPv6 server unhealthy too: AAAA falls back in the same way.
	health.SetHealthy("2001:db8::1", false)
	before = counterValue(t, counterAAAA)
	resp = serveQuery(t, handler, "udp", question("lh.gslb.example.com.", dns.TypeAAAA))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("AAAA all down: rcode = %s, answers = %d", dns.RcodeToString[resp.Rcode], len(resp.Answer))
	}
	if got := resp.Answer[0].(*dns.AAAA).AAAA.String(); got != "2001:db8::1" {
		t.Errorf("AAAA all down: answer = %s, want 2001:db8::1", got)
	}
	if got := counterValue(t, counterAAAA) - before; got != 1 {
		t.Errorf("AAAA all down: last-healthy counter increased by %v, want 1", got)
	}

	// A family with no configured servers stays an empty answer: there is
	// nothing to fall back to, and it is not counted.
	health.SetHealthy("10.0.0.3", false)
	resp = serveQuery(t, handler, "udp", question("v4only.gslb.example.com.", dns.TypeAAAA))
	assertNegative(t, resp, dns.RcodeSuccess, "gslb.example.com.")
}

// TestHandler_ReturnLastHealthyDisabled checks the default: SERVFAIL when
// every server of the queried family is unhealthy, and nothing is counted.
func TestHandler_ReturnLastHealthyDisabled(t *testing.T) {
	handler, health := lastHealthyFixture(t, false)
	health.SetHealthy("10.0.0.1", false)
	health.SetHealthy("10.0.0.2", false)
	counter := metrics.DNSLastHealthyAnswersTotal.WithLabelValues("lh.gslb.example.com.", "A")
	before := counterValue(t, counter)

	resp := serveQuery(t, handler, "udp", question("lh.gslb.example.com.", dns.TypeA))
	if resp.Rcode != dns.RcodeServerFailure || len(resp.Answer) != 0 {
		t.Fatalf("rcode = %s, answers = %d, want SERVFAIL without an answer", dns.RcodeToString[resp.Rcode], len(resp.Answer))
	}
	if got := counterValue(t, counter) - before; got != 0 {
		t.Errorf("last-healthy counter increased by %v, want 0", got)
	}

	// The setting can be switched at runtime (configuration reload).
	handler.SetReturnLastHealthy(true)
	resp = serveQuery(t, handler, "udp", question("lh.gslb.example.com.", dns.TypeA))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("after enabling: rcode = %s, answers = %d, want NOERROR with 1 answer", dns.RcodeToString[resp.Rcode], len(resp.Answer))
	}
	handler.SetReturnLastHealthy(false)
	resp = serveQuery(t, handler, "udp", question("lh.gslb.example.com.", dns.TypeA))
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("after disabling: rcode = %s, want SERVFAIL", dns.RcodeToString[resp.Rcode])
	}
}
