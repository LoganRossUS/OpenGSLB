// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/miekg/dns"
)

// TestHandler_CaseInsensitiveLookup covers finding 1B #1: names are matched
// without regard to case and the reply echoes the question (and the owner
// name of the answer) exactly as the querier spelled it, which resolvers that
// use 0x20 case randomization verify.
func TestHandler_CaseInsensitiveLookup(t *testing.T) {
	registry := NewRegistry()
	registry.Register(v4Entry("app.gslb.example.com"))
	handler := NewHandler(HandlerConfig{Registry: registry, DefaultTTL: 60})

	for _, qname := range []string{
		"app.gslb.example.com.",
		"APP.GSLB.Example.COM.",
		"aPp.GsLb.eXaMpLe.CoM.",
	} {
		t.Run(qname, func(t *testing.T) {
			resp := serveQuery(t, handler, "udp", question(qname, dns.TypeA))
			if resp.Rcode != dns.RcodeSuccess {
				t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
			}
			if len(resp.Question) != 1 || resp.Question[0].Name != qname {
				t.Fatalf("question section = %v, want the query name %q with its original case", resp.Question, qname)
			}
			if len(resp.Answer) != 1 {
				t.Fatalf("answer count = %d, want 1", len(resp.Answer))
			}
			if got := resp.Answer[0].Header().Name; got != qname {
				t.Errorf("answer owner name = %q, want %q", got, qname)
			}
			if a, ok := resp.Answer[0].(*dns.A); !ok || a.A.String() != "10.0.0.1" {
				t.Errorf("answer = %v, want A 10.0.0.1", resp.Answer[0])
			}
		})
	}
}

// TestRegistry_CaseInsensitive covers the registry side of finding 1B #1:
// registration, lookup, server changes and removal all ignore case.
func TestRegistry_CaseInsensitive(t *testing.T) {
	registry := NewRegistry()
	registry.Register(v4Entry("App.GSLB.example.com"))

	if registry.Lookup("APP.gslb.EXAMPLE.com.") == nil {
		t.Fatal("lookup with different case found nothing")
	}
	if registry.Count() != 1 {
		t.Fatalf("count = %d, want 1", registry.Count())
	}

	// Registering the same name with different case replaces the entry.
	registry.Register(v4Entry("app.gslb.example.com"))
	if registry.Count() != 1 {
		t.Fatalf("count after re-register with different case = %d, want 1", registry.Count())
	}

	if err := registry.RegisterServer("APP.GSLB.EXAMPLE.COM", "10.0.0.2", 80, 100, "r1"); err != nil {
		t.Fatalf("RegisterServer with different case: %v", err)
	}
	if got := len(registry.Lookup("app.gslb.example.com").Servers); got != 2 {
		t.Fatalf("servers = %d, want 2", got)
	}

	registry.Remove("APP.GSLB.EXAMPLE.COM.")
	if registry.Count() != 0 {
		t.Fatalf("count after remove with different case = %d, want 0", registry.Count())
	}
}

func TestNormalizeDomain_LowerCases(t *testing.T) {
	if got := normalizeDomain("APP.Example.COM"); got != "app.example.com." {
		t.Errorf("normalizeDomain = %q, want %q", got, "app.example.com.")
	}
}

// TestHandler_MetricsUseCanonicalName checks that queries that differ only in
// case are counted under one domain label. Without this, 0x20 case
// randomization would create a new Prometheus series for every spelling.
func TestHandler_MetricsUseCanonicalName(t *testing.T) {
	registry := NewRegistry()
	registry.Register(v4Entry("metrics-case.gslb.example.com"))
	handler := NewHandler(HandlerConfig{Registry: registry, DefaultTTL: 60})

	counter := metrics.DNSQueriesTotal.WithLabelValues("metrics-case.gslb.example.com.", "A", "NOERROR")
	before := counterValue(t, counter)

	serveQuery(t, handler, "udp", question("METRICS-CASE.gslb.example.com.", dns.TypeA))
	serveQuery(t, handler, "udp", question("Metrics-Case.GSLB.Example.Com.", dns.TypeA))

	if got := counterValue(t, counter) - before; got != 2 {
		t.Errorf("canonical-name counter increased by %v, want 2", got)
	}
}
