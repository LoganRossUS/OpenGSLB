// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"reflect"
	"strings"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/miekg/dns"
)

// mustZones builds a zone table or fails the test.
func mustZones(t *testing.T, nodeID string, zones ...config.Zone) *ZoneTable {
	t.Helper()
	table, err := NewZoneTable(zones, nodeID)
	if err != nil {
		t.Fatalf("NewZoneTable: %v", err)
	}
	return table
}

func TestNewZoneTable_Defaults(t *testing.T) {
	table := mustZones(t, "overwatch-1", config.Zone{Name: "GSLB.Example.com"})

	zone := table.Find("app.gslb.example.com.")
	if zone == nil {
		t.Fatal("zone not found for a name inside it")
	}
	want := Zone{
		Name:        "gslb.example.com.",
		TTL:         3600,
		PrimaryNS:   "overwatch-1.gslb.example.com.",
		Contact:     "hostmaster.gslb.example.com.",
		Serial:      1,
		Refresh:     3600,
		Retry:       600,
		Expire:      604800,
		NegativeTTL: 60,
		Nameservers: []string{"overwatch-1.gslb.example.com."},
	}
	if !reflect.DeepEqual(*zone, want) {
		t.Errorf("zone = %+v\nwant   %+v", *zone, want)
	}
}

func TestNewZoneTable_PrimaryNSFromNodeIdentity(t *testing.T) {
	tests := []struct {
		nodeID string
		want   string
	}{
		{"overwatch-1", "overwatch-1.gslb.example.com."},
		{"OW1.DC1.Example.NET", "ow1.dc1.example.net."},
		{"ow1.dc1.example.net.", "ow1.dc1.example.net."},
		{"My Node_01", "my-node-01.gslb.example.com."},
		{"node_a.dc1", "node-a-dc1.gslb.example.com."},
		{"", "ns.gslb.example.com."},
		{"___", "ns.gslb.example.com."},
		{strings.Repeat("x", 80), strings.Repeat("x", 63) + ".gslb.example.com."},
	}
	for _, tt := range tests {
		t.Run(tt.nodeID, func(t *testing.T) {
			zone := mustZones(t, tt.nodeID, config.Zone{Name: "gslb.example.com"}).Find("gslb.example.com.")
			if zone.PrimaryNS != tt.want {
				t.Errorf("PrimaryNS = %q, want %q", zone.PrimaryNS, tt.want)
			}
			if !reflect.DeepEqual(zone.Nameservers, []string{tt.want}) {
				t.Errorf("Nameservers = %v, want [%s]", zone.Nameservers, tt.want)
			}
			if _, ok := dns.IsDomainName(zone.PrimaryNS); !ok {
				t.Errorf("PrimaryNS %q is not a valid DNS name", zone.PrimaryNS)
			}
		})
	}
}

func TestNewZoneTable_Overrides(t *testing.T) {
	table := mustZones(t, "overwatch-1", config.Zone{
		Name: "internal.corp",
		TTL:  7200,
		SOA: config.ZoneSOA{
			PrimaryNS:   "NS1.internal.corp",
			Contact:     "dns.admin@internal.corp",
			Serial:      2026112001,
			Refresh:     7200,
			Retry:       900,
			Expire:      1209600,
			NegativeTTL: 30,
		},
		Nameservers: []string{"ns1.internal.corp", "NS2.internal.corp."},
	})
	want := Zone{
		Name:        "internal.corp.",
		TTL:         7200,
		PrimaryNS:   "ns1.internal.corp.",
		Contact:     `dns\.admin.internal.corp.`,
		Serial:      2026112001,
		Refresh:     7200,
		Retry:       900,
		Expire:      1209600,
		NegativeTTL: 30,
		Nameservers: []string{"ns1.internal.corp.", "ns2.internal.corp."},
	}
	if got := *table.Find("internal.corp."); !reflect.DeepEqual(got, want) {
		t.Errorf("zone = %+v\nwant   %+v", got, want)
	}
}

// The NS set defaults to the SOA primary nameserver, including an overridden
// one, so that setting only soa.primary_ns does not leave a synthesized name
// in the NS set.
func TestNewZoneTable_NameserversFollowPrimaryNS(t *testing.T) {
	zone := mustZones(t, "overwatch-1", config.Zone{
		Name: "gslb.example.com",
		SOA:  config.ZoneSOA{PrimaryNS: "ns1.example.net"},
	}).Find("gslb.example.com.")
	if !reflect.DeepEqual(zone.Nameservers, []string{"ns1.example.net."}) {
		t.Errorf("Nameservers = %v, want [ns1.example.net.]", zone.Nameservers)
	}
}

// TestNewZoneTable_PrimaryNSFollowsNameservers checks the other direction:
// with an NS set and no soa.primary_ns, the SOA names the first nameserver
// of the set and not a name derived from the node identity. Overwatches
// that share a zone configuration then answer the same SOA whatever their
// node IDs are.
func TestNewZoneTable_PrimaryNSFollowsNameservers(t *testing.T) {
	zones := []config.Zone{{
		Name:        "gslb.example.com",
		Nameservers: []string{"NS1.example.net", "ns2.example.net"},
	}}
	for _, nodeID := range []string{"overwatch-1", "overwatch-2", ""} {
		zone := mustZones(t, nodeID, zones...).Find("gslb.example.com.")
		if zone.PrimaryNS != "ns1.example.net." {
			t.Errorf("node %q: PrimaryNS = %q, want ns1.example.net.", nodeID, zone.PrimaryNS)
		}
		if !reflect.DeepEqual(zone.Nameservers, []string{"ns1.example.net.", "ns2.example.net."}) {
			t.Errorf("node %q: Nameservers = %v", nodeID, zone.Nameservers)
		}
	}
}

func TestNewZoneTable_Errors(t *testing.T) {
	for name, zones := range map[string][]config.Zone{
		"empty name":    {{Name: ""}},
		"root":          {{Name: "."}},
		"duplicate":     {{Name: "a.example"}, {Name: "A.example."}},
		"bad name":      {{Name: "a..example"}},
		"bad contact":   {{Name: "a.example", SOA: config.ZoneSOA{Contact: "x@"}}},
		"bad ns":        {{Name: "a.example", Nameservers: []string{"ns..x"}}},
		"empty ns name": {{Name: "a.example", Nameservers: []string{""}}},
	} {
		if _, err := NewZoneTable(zones, "n"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestZoneTable_Find(t *testing.T) {
	table := mustZones(t, "n",
		config.Zone{Name: "example.com"},
		config.Zone{Name: "gslb.example.com"},
	)
	tests := []struct {
		name string
		want string // "" means no zone
	}{
		{"gslb.example.com.", "gslb.example.com."},
		{"app.gslb.example.com.", "gslb.example.com."},
		{"a.b.gslb.example.com.", "gslb.example.com."},
		{"www.example.com.", "example.com."},
		{"example.com.", "example.com."},
		// A suffix match that is not on a label boundary is not a match.
		{"notexample.com.", ""},
		{"com.", ""},
		{".", ""},
		{"www.google.com.", ""},
	}
	for _, tt := range tests {
		zone := table.Find(tt.name)
		got := ""
		if zone != nil {
			got = zone.Name
		}
		if got != tt.want {
			t.Errorf("Find(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}

	var nilTable *ZoneTable
	if nilTable.Find("example.com.") != nil {
		t.Error("nil table must find nothing")
	}
}

func TestZone_NegativeSOA_TTL(t *testing.T) {
	// RFC 2308 section 3: the TTL is the minimum of the SOA TTL and MINIMUM.
	z := &Zone{Name: "z.example.", TTL: 3600, NegativeTTL: 60}
	if got := z.negativeSOA().Hdr.Ttl; got != 60 {
		t.Errorf("TTL = %d, want 60 (the smaller MINIMUM)", got)
	}
	z = &Zone{Name: "z.example.", TTL: 30, NegativeTTL: 300}
	if got := z.negativeSOA().Hdr.Ttl; got != 30 {
		t.Errorf("TTL = %d, want 30 (the smaller SOA TTL)", got)
	}
	if got := z.negativeSOA().Minttl; got != 300 {
		t.Errorf("MINIMUM = %d, want 300", got)
	}
}

// TestHandler_ApexSOAAndNS covers Phase 1B task 2: SOA and NS are answered at
// the apex of a configured zone, with the querier's spelling as owner name.
func TestHandler_ApexSOAAndNS(t *testing.T) {
	registry := NewRegistry()
	registry.Register(v4Entry("app.gslb.example.com"))
	handler := NewHandler(HandlerConfig{
		Registry: registry,
		Zones: mustZones(t, "overwatch-1", config.Zone{
			Name:        "gslb.example.com",
			Nameservers: []string{"ns1.example.net", "ns2.example.net"},
			SOA:         config.ZoneSOA{PrimaryNS: "ns1.example.net", Contact: "ops@example.net", Serial: 7},
		}),
		DefaultTTL: 60,
	})

	t.Run("SOA", func(t *testing.T) {
		resp := serveQuery(t, handler, "udp", question("GSLB.example.com.", dns.TypeSOA))
		if resp.Rcode != dns.RcodeSuccess || !resp.Authoritative {
			t.Fatalf("rcode = %s, AA = %v, want NOERROR with AA", dns.RcodeToString[resp.Rcode], resp.Authoritative)
		}
		if len(resp.Answer) != 1 {
			t.Fatalf("answer count = %d, want 1", len(resp.Answer))
		}
		soa, ok := resp.Answer[0].(*dns.SOA)
		if !ok {
			t.Fatalf("answer = %v, want an SOA record", resp.Answer[0])
		}
		if soa.Hdr.Name != "GSLB.example.com." {
			t.Errorf("owner = %q, want the query name as spelled", soa.Hdr.Name)
		}
		if soa.Ns != "ns1.example.net." || soa.Mbox != "ops.example.net." || soa.Serial != 7 {
			t.Errorf("SOA = %v", soa)
		}
		if soa.Hdr.Ttl != 3600 || soa.Refresh != 3600 || soa.Retry != 600 || soa.Expire != 604800 || soa.Minttl != 60 {
			t.Errorf("SOA timers = %v", soa)
		}
	})

	t.Run("NS", func(t *testing.T) {
		resp := serveQuery(t, handler, "udp", question("gslb.example.com.", dns.TypeNS))
		if resp.Rcode != dns.RcodeSuccess || !resp.Authoritative {
			t.Fatalf("rcode = %s, AA = %v, want NOERROR with AA", dns.RcodeToString[resp.Rcode], resp.Authoritative)
		}
		var targets []string
		for _, rr := range resp.Answer {
			ns, ok := rr.(*dns.NS)
			if !ok {
				t.Fatalf("answer = %v, want NS records only", rr)
			}
			if ns.Hdr.Ttl != 3600 {
				t.Errorf("NS TTL = %d, want 3600", ns.Hdr.Ttl)
			}
			targets = append(targets, ns.Ns)
		}
		if !reflect.DeepEqual(targets, []string{"ns1.example.net.", "ns2.example.net."}) {
			t.Errorf("NS targets = %v", targets)
		}
	})
}

// TestHandler_ImplicitZoneApex checks that a GSLB domain outside every
// configured zone is served as its own zone: SOA and NS are answered at the
// domain name with defaults derived from the node identity. This keeps
// configurations without dns.zones working.
func TestHandler_ImplicitZoneApex(t *testing.T) {
	registry := NewRegistry()
	registry.Register(v4Entry("app.other.example"))
	handler := NewHandler(HandlerConfig{Registry: registry, DefaultTTL: 60})

	resp := serveQuery(t, handler, "udp", question("app.other.example.", dns.TypeSOA))
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("SOA rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("SOA answer count = %d, want 1", len(resp.Answer))
	}
	soa, ok := resp.Answer[0].(*dns.SOA)
	if !ok || soa.Ns != "ns.app.other.example." || soa.Mbox != "hostmaster.app.other.example." {
		t.Errorf("SOA = %v", resp.Answer[0])
	}

	resp = serveQuery(t, handler, "udp", question("app.other.example.", dns.TypeNS))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("NS rcode = %s, answers = %d, want NOERROR with 1 answer", dns.RcodeToString[resp.Rcode], len(resp.Answer))
	}
	if ns, ok := resp.Answer[0].(*dns.NS); !ok || ns.Ns != "ns.app.other.example." {
		t.Errorf("NS = %v", resp.Answer[0])
	}
}

func TestHandler_SetZones(t *testing.T) {
	registry := NewRegistry()
	registry.Register(v4Entry("app.gslb.example.com"))
	handler := NewHandler(HandlerConfig{Registry: registry, DefaultTTL: 60})

	if got := len(handler.Zones()); got != 0 {
		t.Fatalf("configured zones = %d, want 0", got)
	}

	handler.SetZones(mustZones(t, "n", config.Zone{Name: "gslb.example.com", SOA: config.ZoneSOA{Serial: 42}}))
	zones := handler.Zones()
	if len(zones) != 1 || zones[0].Name != "gslb.example.com." || zones[0].Serial != 42 {
		t.Fatalf("Zones() = %+v", zones)
	}
	resp := serveQuery(t, handler, "udp", question("gslb.example.com.", dns.TypeSOA))
	if len(resp.Answer) != 1 {
		t.Fatalf("apex SOA after SetZones: answers = %d, want 1", len(resp.Answer))
	}

	// nil resets to "no configured zones" without panicking.
	handler.SetZones(nil)
	if got := len(handler.Zones()); got != 0 {
		t.Fatalf("configured zones after SetZones(nil) = %d, want 0", got)
	}
}
