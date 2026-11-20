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
	"github.com/miekg/dns"
)

// rcodeFixture is a handler with one configured zone (gslb.example.com) and
// these names:
//
//	v4.gslb.example.com      IPv4 servers only
//	v6.gslb.example.com      IPv6 servers only
//	dual.gslb.example.com    one IPv4 and one IPv6 server
//	app.eu.gslb.example.com  IPv4 only (eu.gslb.example.com is an empty non-terminal)
//	app.other.example        IPv4 only, outside the configured zone (implicit zone)
type rcodeFixture struct {
	handler *Handler
	health  *mockHealthProvider
}

func newRcodeFixture(t *testing.T) *rcodeFixture {
	t.Helper()
	registry := NewRegistry()
	registry.Register(v4Entry("v4.gslb.example.com"))
	registry.Register(&DomainEntry{
		Name:   "v6.gslb.example.com",
		TTL:    30,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("2001:db8::1"), Port: 80, Weight: 100},
		},
	})
	registry.Register(&DomainEntry{
		Name:   "dual.gslb.example.com",
		TTL:    30,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.9"), Port: 80, Weight: 100},
			{Address: net.ParseIP("2001:db8::9"), Port: 80, Weight: 100},
		},
	})
	registry.Register(v4Entry("app.eu.gslb.example.com"))
	registry.Register(v4Entry("app.other.example"))

	health := newMockHealthProvider()
	return &rcodeFixture{
		health: health,
		handler: NewHandler(HandlerConfig{
			Registry:       registry,
			Zones:          mustZones(t, "overwatch-1", config.Zone{Name: "gslb.example.com"}),
			HealthProvider: health,
			DefaultTTL:     60,
		}),
	}
}

// assertNegative checks a negative response: the given rcode, AA set, an
// empty answer section and exactly the SOA of zone in the authority section
// with the RFC 2308 TTL, min(SOA TTL, SOA MINIMUM) = min(3600, 60).
func assertNegative(t *testing.T, resp *dns.Msg, rcode int, zone string) {
	t.Helper()
	if resp.Rcode != rcode {
		t.Errorf("rcode = %s, want %s", dns.RcodeToString[resp.Rcode], dns.RcodeToString[rcode])
	}
	if !resp.Authoritative {
		t.Error("AA bit is clear, want it set")
	}
	if len(resp.Answer) != 0 {
		t.Errorf("answer section = %v, want it empty", resp.Answer)
	}
	if len(resp.Ns) != 1 {
		t.Fatalf("authority section has %d records, want exactly the zone SOA: %v", len(resp.Ns), resp.Ns)
	}
	soa, ok := resp.Ns[0].(*dns.SOA)
	if !ok {
		t.Fatalf("authority record = %v, want an SOA", resp.Ns[0])
	}
	if soa.Hdr.Name != zone {
		t.Errorf("SOA owner = %q, want %q", soa.Hdr.Name, zone)
	}
	if soa.Hdr.Ttl != 60 || soa.Minttl != 60 {
		t.Errorf("SOA TTL = %d, MINIMUM = %d, want 60 and 60", soa.Hdr.Ttl, soa.Minttl)
	}
}

// TestHandler_NoServersOfFamily covers finding 1B #2: a name that exists but
// has no configured servers of the queried address family answers NOERROR
// with an empty answer and the zone SOA, not SERVFAIL.
func TestHandler_NoServersOfFamily(t *testing.T) {
	f := newRcodeFixture(t)

	t.Run("AAAA for an IPv4-only name", func(t *testing.T) {
		resp := serveQuery(t, f.handler, "udp", question("v4.gslb.example.com.", dns.TypeAAAA))
		assertNegative(t, resp, dns.RcodeSuccess, "gslb.example.com.")
	})
	t.Run("A for an IPv6-only name", func(t *testing.T) {
		resp := serveQuery(t, f.handler, "udp", question("v6.gslb.example.com.", dns.TypeA))
		assertNegative(t, resp, dns.RcodeSuccess, "gslb.example.com.")
	})
	t.Run("AAAA for an IPv4-only name in an implicit zone", func(t *testing.T) {
		resp := serveQuery(t, f.handler, "udp", question("app.other.example.", dns.TypeAAAA))
		assertNegative(t, resp, dns.RcodeSuccess, "app.other.example.")
	})
	t.Run("AAAA for an IPv4-only name whose servers are all down", func(t *testing.T) {
		f.health.SetHealthy("10.0.0.1", false)
		defer f.health.SetHealthy("10.0.0.1", true)
		resp := serveQuery(t, f.handler, "udp", question("v4.gslb.example.com.", dns.TypeAAAA))
		assertNegative(t, resp, dns.RcodeSuccess, "gslb.example.com.")
	})
}

// TestHandler_ServersOfFamilyAllDown is the other half of finding 1B #2:
// SERVFAIL remains the answer when servers of the queried family are
// configured and none is usable.
func TestHandler_ServersOfFamilyAllDown(t *testing.T) {
	f := newRcodeFixture(t)
	f.health.SetHealthy("10.0.0.9", false)

	resp := serveQuery(t, f.handler, "udp", question("dual.gslb.example.com.", dns.TypeA))
	if resp.Rcode != dns.RcodeServerFailure {
		t.Errorf("A rcode = %s, want SERVFAIL", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 0 || len(resp.Ns) != 0 {
		t.Errorf("SERVFAIL carries records: answer %v, authority %v", resp.Answer, resp.Ns)
	}

	// The other family of the same name is unaffected.
	resp = serveQuery(t, f.handler, "udp", question("dual.gslb.example.com.", dns.TypeAAAA))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Errorf("AAAA rcode = %s, answers = %d, want NOERROR with 1 answer", dns.RcodeToString[resp.Rcode], len(resp.Answer))
	}
}
