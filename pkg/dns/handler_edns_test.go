// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/miekg/dns"
)

// ednsQuestion builds a query with an OPT record advertising udpSize.
func ednsQuestion(name string, qtype uint16, udpSize uint16) *dns.Msg {
	q := question(name, qtype)
	q.SetEdns0(udpSize, false)
	return q
}

// withECS adds an EDNS Client Subnet option to a query that has an OPT record.
func withECS(q *dns.Msg, family uint16, address string, sourcePrefix uint8) *dns.Msg {
	opt := q.IsEdns0()
	opt.Option = append(opt.Option, &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        family,
		SourceNetmask: sourcePrefix,
		Address:       net.ParseIP(address),
	})
	return q
}

// optRecords returns the OPT records of a message.
func optRecords(m *dns.Msg) []*dns.OPT {
	var opts []*dns.OPT
	for _, rr := range m.Extra {
		if opt, ok := rr.(*dns.OPT); ok {
			opts = append(opts, opt)
		}
	}
	return opts
}

// advertisedUDPSize returns the UDP size in the response's OPT record and
// fails the test when the response does not have exactly one.
func advertisedUDPSize(t *testing.T, m *dns.Msg) uint16 {
	t.Helper()
	opts := optRecords(m)
	if len(opts) != 1 {
		t.Fatalf("response has %d OPT records, want exactly 1", len(opts))
	}
	return opts[0].UDPSize()
}

// ecsOption returns the ECS option of a response, or nil.
func ecsOption(m *dns.Msg) *dns.EDNS0_SUBNET {
	for _, opt := range optRecords(m) {
		for _, o := range opt.Option {
			if ecs, ok := o.(*dns.EDNS0_SUBNET); ok {
				return ecs
			}
		}
	}
	return nil
}

// wireLen returns the size of the message on the wire.
func wireLen(t *testing.T, m *dns.Msg) int {
	t.Helper()
	buf, err := m.Pack()
	if err != nil {
		t.Fatalf("pack response: %v", err)
	}
	return len(buf)
}

// clientAwareRouter is a router whose decision depends on the client address,
// like the geolocation router. It records the client IP it was given.
type clientAwareRouter struct {
	mockRouter
	seenClientIP net.IP
}

func (r *clientAwareRouter) Route(ctx context.Context, pool routing.ServerPool) (*routing.Server, error) {
	r.seenClientIP = routing.GetClientIP(ctx)
	return r.mockRouter.Route(ctx, pool)
}

func (r *clientAwareRouter) UsesClientIP() bool { return true }

// ednsFixture is a handler for zone gslb.example.com with:
//
//	rr.gslb.example.com   a router that ignores the client address
//	geo.gslb.example.com  a router that uses the client address
//
// and an NS set that is far larger than any UDP limit.
type ednsFixture struct {
	handler   *Handler
	health    *mockHealthProvider
	geoRouter *clientAwareRouter
}

const ednsFixtureNameservers = 40

func newEDNSFixture(t *testing.T, ecsEnabled bool) *ednsFixture {
	t.Helper()
	registry := NewRegistry()
	registry.Register(v4Entry("rr.gslb.example.com"))
	geoRouter := &clientAwareRouter{}
	geo := v4Entry("geo.gslb.example.com")
	geo.Router = geoRouter
	registry.Register(geo)

	var nameservers []string
	for i := 0; i < ednsFixtureNameservers; i++ {
		// Distinct labels, so that name compression cannot shrink the set
		// below the limits under test.
		nameservers = append(nameservers, fmt.Sprintf("ns%02d-%s.site%02d.example", i, strings.Repeat("x", 30), i))
	}

	health := newMockHealthProvider()
	return &ednsFixture{
		health:    health,
		geoRouter: geoRouter,
		handler: NewHandler(HandlerConfig{
			Registry:       registry,
			Zones:          mustZones(t, "overwatch-1", config.Zone{Name: "gslb.example.com", Nameservers: nameservers}),
			HealthProvider: health,
			ECSEnabled:     ecsEnabled,
			DefaultTTL:     60,
		}),
	}
}

// TestHandler_OPTInResponseIffInQuery covers the first part of finding
// 1B #6 (RFC 6891 section 6.1.1 and 7): a response carries exactly one OPT
// record when the query carried one, on every kind of response, and none
// otherwise.
func TestHandler_OPTInResponseIffInQuery(t *testing.T) {
	f := newEDNSFixture(t, true)
	f.health.SetHealthy("10.0.0.1", true)

	noQuestion := func() *dns.Msg { return new(dns.Msg) }
	cases := []struct {
		name  string
		query func() *dns.Msg
		rcode int
	}{
		{"positive answer", func() *dns.Msg { return question("rr.gslb.example.com.", dns.TypeA) }, dns.RcodeSuccess},
		{"empty answer", func() *dns.Msg { return question("rr.gslb.example.com.", dns.TypeAAAA) }, dns.RcodeSuccess},
		{"NXDOMAIN", func() *dns.Msg { return question("missing.gslb.example.com.", dns.TypeA) }, dns.RcodeNameError},
		{"REFUSED", func() *dns.Msg { return question("www.google.com.", dns.TypeA) }, dns.RcodeRefused},
		{"apex SOA", func() *dns.Msg { return question("gslb.example.com.", dns.TypeSOA) }, dns.RcodeSuccess},
		{"FORMERR", noQuestion, dns.RcodeFormatError},
	}
	for _, tc := range cases {
		for _, network := range []string{"udp", "tcp"} {
			t.Run(tc.name+"/"+network+"/without EDNS", func(t *testing.T) {
				resp := serveQuery(t, f.handler, network, tc.query())
				if resp.Rcode != tc.rcode {
					t.Fatalf("rcode = %s, want %s", dns.RcodeToString[resp.Rcode], dns.RcodeToString[tc.rcode])
				}
				if n := len(optRecords(resp)); n != 0 {
					t.Errorf("response has %d OPT records, want none: the query had none", n)
				}
			})
			t.Run(tc.name+"/"+network+"/with EDNS", func(t *testing.T) {
				q := tc.query()
				q.SetEdns0(4096, true)
				resp := serveQuery(t, f.handler, network, q)
				if resp.Rcode != tc.rcode {
					t.Fatalf("rcode = %s, want %s", dns.RcodeToString[resp.Rcode], dns.RcodeToString[tc.rcode])
				}
				opts := optRecords(resp)
				if len(opts) != 1 {
					t.Fatalf("response has %d OPT records, want exactly 1", len(opts))
				}
				opt := opts[0]
				if opt.Version() != 0 {
					t.Errorf("EDNS version = %d, want 0", opt.Version())
				}
				if opt.UDPSize() != DefaultMaxUDPSize {
					t.Errorf("advertised UDP size = %d, want %d", opt.UDPSize(), DefaultMaxUDPSize)
				}
				if !opt.Do() {
					t.Error("DO bit not copied from the query (RFC 3225 section 3)")
				}
				if resp.Extra[len(resp.Extra)-1] != dns.RR(opt) {
					t.Error("OPT is not the last record of the additional section")
				}
			})
		}
	}
}

// TestHandler_EDNSBadVersion: an EDNS version other than 0 is answered
// BADVERS with an OPT record that states version 0 (RFC 6891 section 6.1.3).
func TestHandler_EDNSBadVersion(t *testing.T) {
	f := newEDNSFixture(t, true)

	q := ednsQuestion("rr.gslb.example.com.", dns.TypeA, 1232)
	q.IsEdns0().SetVersion(1)
	resp := serveQuery(t, f.handler, "udp", q)

	if resp.Rcode != dns.RcodeBadVers {
		t.Fatalf("rcode = %d, want BADVERS (%d)", resp.Rcode, dns.RcodeBadVers)
	}
	if len(resp.Answer) != 0 || resp.Authoritative {
		t.Errorf("BADVERS carries an answer or AA: answer %v, AA %v", resp.Answer, resp.Authoritative)
	}
	opts := optRecords(resp)
	if len(opts) != 1 || opts[0].Version() != 0 {
		t.Fatalf("OPT records = %v, want one with version 0", opts)
	}
	// The extended rcode must survive packing: the upper bits live in the OPT.
	buf, err := resp.Pack()
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	var back dns.Msg
	if err := back.Unpack(buf); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if back.Rcode != dns.RcodeBadVers {
		t.Errorf("rcode on the wire = %d, want BADVERS (%d)", back.Rcode, dns.RcodeBadVers)
	}
}

// TestHandler_UDPSizeAndTruncation covers the size part of finding 1B #6:
// the requester's UDP size is honored with a floor of 512 and a cap of 1232
// by default, TC is set when the answer does not fit, and TCP responses are
// never truncated.
func TestHandler_UDPSizeAndTruncation(t *testing.T) {
	f := newEDNSFixture(t, true)

	// The full NS answer, taken over TCP, is the reference.
	full := serveQuery(t, f.handler, "tcp", question("gslb.example.com.", dns.TypeNS))
	if full.Truncated || len(full.Answer) != ednsFixtureNameservers {
		t.Fatalf("TCP: TC = %v, answers = %d, want the complete set of %d without TC", full.Truncated, len(full.Answer), ednsFixtureNameservers)
	}
	fullSize := wireLen(t, full)
	if fullSize <= 1232 || fullSize > 4096 {
		t.Fatalf("fixture: full answer is %d bytes, need more than 1232 and at most 4096", fullSize)
	}

	udp := []struct {
		name      string
		query     func() *dns.Msg
		wantLimit int
	}{
		{"no EDNS is limited to 512", func() *dns.Msg { return question("gslb.example.com.", dns.TypeNS) }, 512},
		{"EDNS 512", func() *dns.Msg { return ednsQuestion("gslb.example.com.", dns.TypeNS, 512) }, 512},
		{"EDNS below 512 is treated as 512", func() *dns.Msg { return ednsQuestion("gslb.example.com.", dns.TypeNS, 100) }, 512},
		{"EDNS 900 is honored", func() *dns.Msg { return ednsQuestion("gslb.example.com.", dns.TypeNS, 900) }, 900},
		{"EDNS 1232", func() *dns.Msg { return ednsQuestion("gslb.example.com.", dns.TypeNS, 1232) }, 1232},
		{"EDNS 4096 is capped at 1232", func() *dns.Msg { return ednsQuestion("gslb.example.com.", dns.TypeNS, 4096) }, 1232},
		{"EDNS 65535 is capped at 1232", func() *dns.Msg { return ednsQuestion("gslb.example.com.", dns.TypeNS, 65535) }, 1232},
	}
	previous := 0
	for _, tc := range udp {
		t.Run("udp/"+tc.name, func(t *testing.T) {
			q := tc.query()
			resp := serveQuery(t, f.handler, "udp", q)
			size := wireLen(t, resp)
			if size > tc.wantLimit {
				t.Errorf("response is %d bytes, limit is %d", size, tc.wantLimit)
			}
			if !resp.Truncated {
				t.Errorf("TC is clear on a response that dropped records (%d of %d answers)", len(resp.Answer), ednsFixtureNameservers)
			}
			if len(resp.Answer) >= ednsFixtureNameservers {
				t.Errorf("answers = %d, expected fewer than %d", len(resp.Answer), ednsFixtureNameservers)
			}
			// The limit must actually be used, not undershot by a wide
			// margin: one more NS record would not have fit.
			if tc.wantLimit-size > 80 {
				t.Errorf("response is %d bytes with a limit of %d: the allowed size is not used", size, tc.wantLimit)
			}
			if tc.wantLimit > 512 && previous > 0 && len(resp.Answer) < previous {
				t.Errorf("a larger limit produced fewer answers (%d < %d)", len(resp.Answer), previous)
			}
			previous = len(resp.Answer)
			// RFC 6891 section 7: a truncated response still has its OPT.
			if q.IsEdns0() != nil && len(optRecords(resp)) != 1 {
				t.Errorf("truncated response has %d OPT records, want 1", len(optRecords(resp)))
			}
			if resp.Rcode != dns.RcodeSuccess || !resp.Authoritative {
				t.Errorf("rcode = %s, AA = %v, want NOERROR with AA", dns.RcodeToString[resp.Rcode], resp.Authoritative)
			}
		})
	}

	t.Run("udp/small answers are not truncated", func(t *testing.T) {
		for _, q := range []*dns.Msg{
			question("rr.gslb.example.com.", dns.TypeA),
			ednsQuestion("rr.gslb.example.com.", dns.TypeA, 1232),
			question("missing.gslb.example.com.", dns.TypeA),
		} {
			resp := serveQuery(t, f.handler, "udp", q)
			if resp.Truncated {
				t.Errorf("TC set on a %d byte response", wireLen(t, resp))
			}
		}
	})

	t.Run("tcp is never truncated", func(t *testing.T) {
		for _, q := range []*dns.Msg{
			question("gslb.example.com.", dns.TypeNS),
			ednsQuestion("gslb.example.com.", dns.TypeNS, 512),
			ednsQuestion("gslb.example.com.", dns.TypeNS, 4096),
		} {
			resp := serveQuery(t, f.handler, "tcp", q)
			if resp.Truncated || len(resp.Answer) != ednsFixtureNameservers {
				t.Errorf("TCP: TC = %v, answers = %d, want %d without TC", resp.Truncated, len(resp.Answer), ednsFixtureNameservers)
			}
		}
	})

	t.Run("udp/configured maximum raises the cap", func(t *testing.T) {
		f.handler.SetMaxUDPSize(4096)
		defer f.handler.SetMaxUDPSize(0)
		resp := serveQuery(t, f.handler, "udp", ednsQuestion("gslb.example.com.", dns.TypeNS, 4096))
		if resp.Truncated || len(resp.Answer) != ednsFixtureNameservers {
			t.Errorf("TC = %v, answers = %d, want the complete set with a 4096 cap", resp.Truncated, len(resp.Answer))
		}
		if got := advertisedUDPSize(t, resp); got != 4096 {
			t.Errorf("advertised UDP size = %d, want 4096", got)
		}
		// A requester that asks for less still gets less.
		resp = serveQuery(t, f.handler, "udp", ednsQuestion("gslb.example.com.", dns.TypeNS, 1232))
		if size := wireLen(t, resp); size > 1232 || !resp.Truncated {
			t.Errorf("size = %d, TC = %v, want at most 1232 with TC", size, resp.Truncated)
		}
	})

	t.Run("SetMaxUDPSize clamps out-of-range values", func(t *testing.T) {
		defer f.handler.SetMaxUDPSize(0)
		f.handler.SetMaxUDPSize(100)
		resp := serveQuery(t, f.handler, "udp", ednsQuestion("rr.gslb.example.com.", dns.TypeA, 4096))
		if got := advertisedUDPSize(t, resp); got != 512 {
			t.Errorf("advertised UDP size = %d, want 512 (the floor)", got)
		}
	})
}

// TestHandler_ECSEcho covers the ECS part of finding 1B #6 (RFC 7871 section
// 7.2.1): with ECS support enabled, a query that carries an ECS option gets
// one back with the same FAMILY, SOURCE PREFIX-LENGTH and ADDRESS. The SCOPE
// PREFIX-LENGTH is the source prefix when the client subnet decided the
// answer, and 0 when the answer is the same for every client.
func TestHandler_ECSEcho(t *testing.T) {
	f := newEDNSFixture(t, true)

	check := func(t *testing.T, resp *dns.Msg, family uint16, address string, source, scope uint8) {
		t.Helper()
		ecs := ecsOption(resp)
		if ecs == nil {
			t.Fatal("response has no ECS option")
		}
		if ecs.Family != family || ecs.SourceNetmask != source || !ecs.Address.Equal(net.ParseIP(address)) {
			t.Errorf("ECS = family %d, source /%d, address %s; want family %d, source /%d, address %s",
				ecs.Family, ecs.SourceNetmask, ecs.Address, family, source, address)
		}
		if ecs.SourceScope != scope {
			t.Errorf("ECS scope = /%d, want /%d", ecs.SourceScope, scope)
		}
	}

	t.Run("client-aware routing echoes the source prefix as scope", func(t *testing.T) {
		q := withECS(ednsQuestion("geo.gslb.example.com.", dns.TypeA, 1232), 1, "203.0.113.0", 24)
		resp := serveQuery(t, f.handler, "udp", q)
		if len(resp.Answer) != 1 {
			t.Fatalf("answers = %d, want 1", len(resp.Answer))
		}
		check(t, resp, 1, "203.0.113.0", 24, 24)
		if !f.geoRouter.seenClientIP.Equal(net.ParseIP("203.0.113.0")) {
			t.Errorf("router saw client IP %v, want the ECS address", f.geoRouter.seenClientIP)
		}
	})

	t.Run("IPv6 client subnet", func(t *testing.T) {
		q := withECS(ednsQuestion("geo.gslb.example.com.", dns.TypeA, 1232), 2, "2001:db8:1234:5600::", 56)
		resp := serveQuery(t, f.handler, "udp", q)
		check(t, resp, 2, "2001:db8:1234:5600::", 56, 56)
	})

	t.Run("routing that ignores the client answers scope 0", func(t *testing.T) {
		q := withECS(ednsQuestion("rr.gslb.example.com.", dns.TypeA, 1232), 1, "203.0.113.0", 24)
		resp := serveQuery(t, f.handler, "udp", q)
		if len(resp.Answer) != 1 {
			t.Fatalf("answers = %d, want 1", len(resp.Answer))
		}
		check(t, resp, 1, "203.0.113.0", 24, 0)
	})

	t.Run("negative answers are scope 0", func(t *testing.T) {
		for _, q := range []*dns.Msg{
			question("missing.gslb.example.com.", dns.TypeA), // NXDOMAIN
			question("geo.gslb.example.com.", dns.TypeAAAA),  // empty answer
			question("www.google.com.", dns.TypeA),           // REFUSED
		} {
			q.SetEdns0(1232, false)
			resp := serveQuery(t, f.handler, "udp", withECS(q, 1, "203.0.113.0", 24))
			check(t, resp, 1, "203.0.113.0", 24, 0)
		}
	})

	t.Run("source prefix 0 is not used for routing", func(t *testing.T) {
		// RFC 7871 section 7.1.2: /0 means the client does not want its
		// address used. The server falls back to the transport address.
		q := withECS(ednsQuestion("geo.gslb.example.com.", dns.TypeA, 1232), 1, "0.0.0.0", 0)
		resp := serveQuery(t, f.handler, "udp", q)
		check(t, resp, 1, "0.0.0.0", 0, 0)
		if !f.geoRouter.seenClientIP.Equal(net.ParseIP("198.51.100.7")) {
			t.Errorf("router saw client IP %v, want the transport source address 198.51.100.7", f.geoRouter.seenClientIP)
		}
	})

	t.Run("no ECS in the query, none in the response", func(t *testing.T) {
		resp := serveQuery(t, f.handler, "udp", ednsQuestion("geo.gslb.example.com.", dns.TypeA, 1232))
		if ecsOption(resp) != nil {
			t.Error("response has an ECS option although the query had none")
		}
		if !f.geoRouter.seenClientIP.Equal(net.ParseIP("198.51.100.7")) {
			t.Errorf("router saw client IP %v, want the transport source address", f.geoRouter.seenClientIP)
		}
	})

	t.Run("ECS support disabled, no ECS in the response", func(t *testing.T) {
		// RFC 7871 section 7.2.1: a server that has not enabled ECS must
		// not include the option in replies.
		off := newEDNSFixture(t, false)
		q := withECS(ednsQuestion("geo.gslb.example.com.", dns.TypeA, 1232), 1, "203.0.113.0", 24)
		resp := serveQuery(t, off.handler, "udp", q)
		if ecsOption(resp) != nil {
			t.Error("response has an ECS option although ECS support is disabled")
		}
		if len(optRecords(resp)) != 1 {
			t.Error("the OPT record itself must still be present")
		}
		if !off.geoRouter.seenClientIP.Equal(net.ParseIP("198.51.100.7")) {
			t.Errorf("router saw client IP %v, want the transport source address", off.geoRouter.seenClientIP)
		}
	})

	t.Run("the echoed option survives packing", func(t *testing.T) {
		q := withECS(ednsQuestion("geo.gslb.example.com.", dns.TypeA, 1232), 1, "203.0.113.0", 24)
		resp := serveQuery(t, f.handler, "udp", q)
		buf, err := resp.Pack()
		if err != nil {
			t.Fatalf("pack: %v", err)
		}
		var back dns.Msg
		if err := back.Unpack(buf); err != nil {
			t.Fatalf("unpack: %v", err)
		}
		check(t, &back, 1, "203.0.113.0", 24, 24)
	})
}
