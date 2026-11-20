// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/miekg/dns"
)

// TestHandler_NXDOMAINHasSOA covers finding 1B #4: NXDOMAIN carries the zone
// SOA in the authority section so resolvers can cache it (RFC 2308).
func TestHandler_NXDOMAINHasSOA(t *testing.T) {
	f := newRcodeFixture(t)

	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT, dns.TypeSOA, dns.TypeDNSKEY} {
		t.Run(dns.TypeToString[qtype]+" in a configured zone", func(t *testing.T) {
			resp := serveQuery(t, f.handler, "udp", question("missing.gslb.example.com.", qtype))
			assertNegative(t, resp, dns.RcodeNameError, "gslb.example.com.")
		})
	}
	t.Run("below a GSLB name in a configured zone", func(t *testing.T) {
		resp := serveQuery(t, f.handler, "udp", question("x.v4.gslb.example.com.", dns.TypeA))
		assertNegative(t, resp, dns.RcodeNameError, "gslb.example.com.")
	})
	t.Run("below a GSLB name in an implicit zone", func(t *testing.T) {
		resp := serveQuery(t, f.handler, "udp", question("x.app.other.example.", dns.TypeA))
		assertNegative(t, resp, dns.RcodeNameError, "app.other.example.")
	})
}

// TestHandler_EmptyNonTerminal checks that a name that has no records itself
// but has a GSLB domain beneath it is not answered with NXDOMAIN. NXDOMAIN
// means that nothing exists at or below the name (RFC 8020), and now that the
// answer is cacheable a resolver would stop resolving the names beneath it.
func TestHandler_EmptyNonTerminal(t *testing.T) {
	f := newRcodeFixture(t)

	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT} {
		resp := serveQuery(t, f.handler, "udp", question("eu.gslb.example.com.", qtype))
		assertNegative(t, resp, dns.RcodeSuccess, "gslb.example.com.")
	}

	// The name beneath it still resolves.
	resp := serveQuery(t, f.handler, "udp", question("app.eu.gslb.example.com.", dns.TypeA))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("rcode = %s, answers = %d, want NOERROR with 1 answer", dns.RcodeToString[resp.Rcode], len(resp.Answer))
	}

	// Once the only domain beneath it is removed, the name is gone.
	f.handler.state().registry.Remove("app.eu.gslb.example.com")
	resp = serveQuery(t, f.handler, "udp", question("eu.gslb.example.com.", dns.TypeA))
	assertNegative(t, resp, dns.RcodeNameError, "gslb.example.com.")
}

// TestHandler_NameAboveNestedZoneApex checks that a name between two
// configured zones, which has the inner apex beneath it, is an empty answer
// in the outer zone and not NXDOMAIN.
func TestHandler_NameAboveNestedZoneApex(t *testing.T) {
	handler := NewHandler(HandlerConfig{
		Registry: NewRegistry(),
		Zones: mustZones(t, "n",
			config.Zone{Name: "example.com"},
			config.Zone{Name: "gslb.dc1.example.com"},
		),
		DefaultTTL: 60,
	})

	resp := serveQuery(t, handler, "udp", question("dc1.example.com.", dns.TypeA))
	assertNegative(t, resp, dns.RcodeSuccess, "example.com.")

	resp = serveQuery(t, handler, "udp", question("dc2.example.com.", dns.TypeA))
	assertNegative(t, resp, dns.RcodeNameError, "example.com.")

	// A name in the inner zone is answered from the inner zone.
	resp = serveQuery(t, handler, "udp", question("x.gslb.dc1.example.com.", dns.TypeA))
	assertNegative(t, resp, dns.RcodeNameError, "gslb.dc1.example.com.")
}

// TestRegistry_DescendantTracking checks the bookkeeping behind empty
// non-terminals across every way the set of domains can change.
func TestRegistry_DescendantTracking(t *testing.T) {
	r := NewRegistry()
	has := func(name string) bool { return r.hasDescendant(name) }

	r.Register(v4Entry("a.b.example.com"))
	r.Register(v4Entry("c.b.example.com"))
	if !has("b.example.com.") || !has("example.com.") || !has("com.") {
		t.Fatal("ancestors of registered domains must have descendants")
	}
	if has("a.b.example.com.") {
		t.Error("a leaf domain has no descendant")
	}

	// Registering the same name again must not count it twice.
	r.Register(v4Entry("A.B.example.com"))
	r.Remove("a.b.example.com")
	if !has("b.example.com.") {
		t.Error("b.example.com. still has c.b.example.com. beneath it")
	}
	r.Remove("c.b.example.com")
	if has("b.example.com.") || has("example.com.") || has("com.") {
		t.Error("no domain is left, nothing has a descendant")
	}

	// Removing a name that is not registered changes nothing.
	r.Register(v4Entry("x.y.example.com"))
	r.Remove("nope.y.example.com")
	if !has("y.example.com.") {
		t.Error("removing an unknown name dropped a descendant count")
	}

	r.ReplaceAll([]*DomainEntry{v4Entry("p.q.example.net")})
	if has("y.example.com.") || !has("q.example.net.") {
		t.Error("ReplaceAll must rebuild the descendant counts")
	}

	r.Clear()
	if has("q.example.net.") {
		t.Error("Clear must reset the descendant counts")
	}

	if err := r.RegisterDomain("n.m.example.org", 30, "round-robin", func(string) (routing.Router, error) {
		return &mockRouter{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !has("m.example.org.") {
		t.Error("RegisterDomain must count descendants")
	}
}
