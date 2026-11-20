// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package conformance

import (
	"net"
	"testing"

	"github.com/miekg/dns"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// TestEDNS_OPTRecord checks the OPT record of a response (RFC 6891): the
// advertised UDP payload size is the configured maximum (1232 by default),
// the version is 0 and the DO bit is copied from the query. That an OPT
// record is present exactly when the query had one is checked for every case
// of TestConformance.
func TestEDNS_OPTRecord(t *testing.T) {
	srv := startServer(t, nil)

	for _, network := range []string{"udp", "tcp"} {
		for _, do := range []bool{false, true} {
			q := newQuery("app.gslb.example.com.", dns.TypeA, 4096)
			q.IsEdns0().SetDo(do)
			resp, _ := srv.exchange(t, network, q)
			checkEnvelope(t, q, resp)

			opts := optRecords(resp)
			if len(opts) != 1 {
				t.Fatalf("%s: response has %d OPT records, want 1", network, len(opts))
			}
			if got := opts[0].UDPSize(); got != 1232 {
				t.Errorf("%s: advertised UDP payload size = %d, want 1232", network, got)
			}
			if got := opts[0].Do(); got != do {
				t.Errorf("%s: DO bit = %v, want %v (copied from the query)", network, got, do)
			}
		}
	}
}

// TestEDNS_UnsupportedVersion checks that a query with an EDNS version other
// than 0 is answered BADVERS with an OPT record that states version 0 (RFC
// 6891 section 6.1.3).
func TestEDNS_UnsupportedVersion(t *testing.T) {
	srv := startServer(t, nil)

	for _, network := range []string{"udp", "tcp"} {
		q := newQuery("app.gslb.example.com.", dns.TypeA, 1232)
		q.IsEdns0().SetVersion(1)
		resp, _ := srv.exchange(t, network, q)
		checkEnvelope(t, q, resp)
		checkExpectation(t, resp, expectation{rcode: dns.RcodeBadVers})
	}
}

// TestEDNS_UDPSizeAndTruncation asks for the 40-record NS set of
// big.example.net, which does not fit in any default UDP response.
//
// Over UDP the response must fit in the size the requester can take: 512
// bytes without EDNS, the advertised size with EDNS (at least 512), and never
// more than dns.max_udp_size. What does not fit is left out as whole records
// and the TC bit is set. Over TCP nothing is ever truncated.
func TestEDNS_UDPSizeAndTruncation(t *testing.T) {
	const name = "big.example.net."

	t.Run("default max_udp_size", func(t *testing.T) {
		srv := startServer(t, nil)

		// The complete answer, over TCP, is the reference.
		full, fullSize := srv.exchange(t, "tcp", newQuery(name, dns.TypeNS, 0))
		if len(full.Answer) != bigZoneNameservers || full.Truncated {
			t.Fatalf("tcp: got %d answers (TC %v), want all %d and TC clear", len(full.Answer), full.Truncated, bigZoneNameservers)
		}
		if fullSize <= 1232 {
			t.Fatalf("the NS answer is only %d bytes: too small to test truncation at 1232", fullSize)
		}

		for _, tc := range []struct {
			name     string
			ednsSize uint16
			limit    int
		}{
			{"no EDNS: 512 bytes", 0, 512},
			{"EDNS 512", 512, 512},
			{"EDNS below 512 counts as 512", 100, 512},
			{"EDNS 900 is honored", 900, 900},
			{"EDNS 1232", 1232, 1232},
			{"EDNS 4096 is capped at 1232", 4096, 1232},
			{"EDNS 65535 is capped at 1232", 65535, 1232},
		} {
			t.Run("udp/"+tc.name, func(t *testing.T) {
				q := newQuery(name, dns.TypeNS, tc.ednsSize)
				resp, size := srv.exchange(t, "udp", q)
				checkEnvelope(t, q, resp)

				if size > tc.limit {
					t.Errorf("response is %d bytes on the wire, limit is %d", size, tc.limit)
				}
				if !resp.Truncated {
					t.Error("TC bit is clear on a response that left records out")
				}
				if len(resp.Answer) >= bigZoneNameservers {
					t.Errorf("response has %d answers, expected fewer than %d", len(resp.Answer), bigZoneNameservers)
				}
				if resp.Rcode != dns.RcodeSuccess || !resp.Authoritative {
					t.Errorf("rcode = %s, AA = %v, want NOERROR with AA set", rcodeName(resp.Rcode), resp.Authoritative)
				}
				// Whatever was kept must be part of the real answer.
				want := map[string]bool{}
				for _, rr := range rrStrings(full.Answer) {
					want[rr] = true
				}
				for _, rr := range rrStrings(resp.Answer) {
					if !want[rr] {
						t.Errorf("truncated response holds a record that is not in the full answer: %s", rr)
					}
				}
			})
		}

		// A larger limit keeps more records: the limits are really applied
		// and the server does not simply empty every truncated response.
		small, _ := srv.exchange(t, "udp", newQuery(name, dns.TypeNS, 512))
		large, _ := srv.exchange(t, "udp", newQuery(name, dns.TypeNS, 1232))
		if len(large.Answer) <= len(small.Answer) {
			t.Errorf("EDNS 1232 kept %d answers and EDNS 512 kept %d, want more with the larger size", len(large.Answer), len(small.Answer))
		}

		for _, ednsSize := range []uint16{0, 512, 1232} {
			q := newQuery(name, dns.TypeNS, ednsSize)
			resp, _ := srv.exchange(t, "tcp", q)
			checkEnvelope(t, q, resp)
			if resp.Truncated || len(resp.Answer) != bigZoneNameservers {
				t.Errorf("tcp with EDNS size %d: %d answers, TC %v; want all %d and TC clear (TCP is never truncated)",
					ednsSize, len(resp.Answer), resp.Truncated, bigZoneNameservers)
			}
		}
	})

	t.Run("max_udp_size 4096", func(t *testing.T) {
		srv := startServer(t, func(cfg *config.Config) { cfg.DNS.MaxUDPSize = 4096 })

		// The requester takes 4096 and so does the server: nothing is cut.
		q := newQuery(name, dns.TypeNS, 4096)
		resp, size := srv.exchange(t, "udp", q)
		checkEnvelope(t, q, resp)
		if resp.Truncated || len(resp.Answer) != bigZoneNameservers {
			t.Errorf("udp, EDNS 4096: %d answers, TC %v, %d bytes; want all %d and TC clear", len(resp.Answer), resp.Truncated, size, bigZoneNameservers)
		}
		if got := optRecords(resp)[0].UDPSize(); got != 4096 {
			t.Errorf("advertised UDP payload size = %d, want 4096", got)
		}

		// The requester's smaller size still wins.
		resp, size = srv.exchange(t, "udp", newQuery(name, dns.TypeNS, 1232))
		if !resp.Truncated || size > 1232 {
			t.Errorf("udp, EDNS 1232: TC %v, %d bytes; want TC set and at most 1232 bytes", resp.Truncated, size)
		}
		// And without EDNS the limit stays 512.
		resp, size = srv.exchange(t, "udp", newQuery(name, dns.TypeNS, 0))
		if !resp.Truncated || size > 512 {
			t.Errorf("udp, no EDNS: TC %v, %d bytes; want TC set and at most 512 bytes", resp.Truncated, size)
		}
	})
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

// TestEDNS_ClientSubnet checks EDNS Client Subnet (RFC 7871 section 7.2.1).
// With ECS enabled, a response to a query with an ECS option carries the
// option back with the same FAMILY, SOURCE PREFIX-LENGTH and ADDRESS. The
// SCOPE PREFIX-LENGTH is the source prefix length when the client subnet
// decided the answer (geolocation routing) and 0 when the answer is the same
// for every client.
func TestEDNS_ClientSubnet(t *testing.T) {
	srv := startServer(t, nil)

	// The test configuration maps 203.0.113.0/24 and 2001:db8:aa00::/40 to
	// region us (server 10.1.5.10) and 198.51.100.0/24 to region eu (server
	// 10.0.5.10). The default region, used for the loopback source address
	// of the test itself, is eu.
	const (
		geoName  = "geo.gslb.example.com."
		euAnswer = "geo.gslb.example.com. 30 IN A 10.0.5.10"
		usAnswer = "geo.gslb.example.com. 30 IN A 10.1.5.10"
	)

	for _, tc := range []struct {
		rule      string
		qname     string
		family    uint16
		address   string
		source    uint8
		want      expectation
		wantScope uint8
	}{
		{"geolocation answer chosen for an IPv4 client subnet", geoName, 1, "203.0.113.0", 24, answerOf(usAnswer), 24},
		{"geolocation answer chosen for another IPv4 client subnet", geoName, 1, "198.51.100.0", 24, answerOf(euAnswer), 24},
		{"geolocation answer chosen for an IPv6 client subnet", geoName, 2, "2001:db8:aa00::", 40, answerOf(usAnswer), 40},
		{"source prefix length 0: the client subnet is not used", geoName, 1, "0.0.0.0", 0, answerOf(euAnswer), 0},
		{"round-robin answer does not depend on the client", "app.gslb.example.com.", 1, "203.0.113.0", 24,
			answerOf("app.gslb.example.com. 30 IN A 10.0.1.10"), 0},
		{"empty answer does not depend on the client", geoName, 1, "203.0.113.0", 24, noData(gslbNegativeSOA), 0},
		{"NXDOMAIN does not depend on the client", "missing.gslb.example.com.", 1, "203.0.113.0", 24, nxDomain(gslbNegativeSOA), 0},
		{"apex SOA does not depend on the client", "corp.example.net.", 1, "203.0.113.0", 24,
			answerOf(`corp.example.net. 7200 IN SOA ns1.corp.example.net. dns\.admin.example.net. 2026112001 7200 900 1209600 300`), 0},
	} {
		for _, network := range []string{"udp", "tcp"} {
			t.Run(tc.rule+"/"+network, func(t *testing.T) {
				qtype := dns.TypeA
				switch {
				case len(tc.want.answer) == 0 && tc.want.rcode == dns.RcodeSuccess:
					qtype = dns.TypeTXT
				case tc.qname == "corp.example.net.":
					qtype = dns.TypeSOA
				}
				q := withECS(newQuery(tc.qname, qtype, 1232), tc.family, tc.address, tc.source)
				resp, _ := srv.exchange(t, network, q)
				checkEnvelope(t, q, resp)
				checkExpectation(t, resp, tc.want)

				ecs := ecsOption(resp)
				if ecs == nil {
					t.Fatal("response has no ECS option although the query had one and ECS is enabled")
				}
				if ecs.Family != tc.family || ecs.SourceNetmask != tc.source || !ecs.Address.Equal(net.ParseIP(tc.address)) {
					t.Errorf("ECS = family %d, source /%d, address %s; want family %d, source /%d, address %s (as in the query)",
						ecs.Family, ecs.SourceNetmask, ecs.Address, tc.family, tc.source, tc.address)
				}
				if ecs.SourceScope != tc.wantScope {
					t.Errorf("ECS scope prefix length = %d, want %d", ecs.SourceScope, tc.wantScope)
				}
			})
		}
	}

	t.Run("no ECS in the query, none in the response", func(t *testing.T) {
		q := newQuery(geoName, dns.TypeA, 1232)
		resp, _ := srv.exchange(t, "udp", q)
		checkEnvelope(t, q, resp)
		checkExpectation(t, resp, answerOf(euAnswer))
		if ecs := ecsOption(resp); ecs != nil {
			t.Errorf("response has an ECS option although the query had none: %v", ecs)
		}
	})

	t.Run("ECS disabled: the option is ignored and not returned", func(t *testing.T) {
		off := startServer(t, func(cfg *config.Config) { cfg.Overwatch.Geolocation.ECSEnabled = false })
		q := withECS(newQuery(geoName, dns.TypeA, 1232), 1, "203.0.113.0", 24)
		resp, _ := off.exchange(t, "udp", q)
		checkEnvelope(t, q, resp)
		// The answer is chosen for the source address of the query
		// (loopback, default region eu), not for the ECS subnet (us).
		checkExpectation(t, resp, answerOf(euAnswer))
		if ecs := ecsOption(resp); ecs != nil {
			t.Errorf("response has an ECS option although ECS is disabled: %v", ecs)
		}
	})
}
