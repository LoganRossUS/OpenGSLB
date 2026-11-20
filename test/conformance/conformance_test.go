// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package conformance

import (
	"testing"

	"github.com/miekg/dns"
)

// expectation is what a conformance case asserts about a response: the
// response code, the AA flag and the complete answer and authority sections.
// Records are in presentation format with single spaces; their order within a
// section does not matter.
type expectation struct {
	rcode     int
	aa        bool
	answer    []string
	authority []string
}

// answerOf, noData, nxDomain, refused and servFail build the five kinds of
// response the server gives.

// answerOf is an authoritative positive answer: NOERROR, AA set, the given
// records and an empty authority section.
func answerOf(records ...string) expectation {
	return expectation{rcode: dns.RcodeSuccess, aa: true, answer: records}
}

// noData is an authoritative empty answer: the name exists and has no data of
// the queried type. NOERROR, AA set, no answer, the zone SOA in the authority
// section (RFC 2308 section 2.2).
func noData(soa string) expectation {
	return expectation{rcode: dns.RcodeSuccess, aa: true, authority: []string{soa}}
}

// nxDomain is an authoritative name error: nothing exists at or below the
// name. NXDOMAIN, AA set, the zone SOA in the authority section (RFC 2308
// section 2.1).
func nxDomain(soa string) expectation {
	return expectation{rcode: dns.RcodeNameError, aa: true, authority: []string{soa}}
}

// refused is the response for a name outside every zone the server serves:
// REFUSED, AA clear, no records.
func refused() expectation {
	return expectation{rcode: dns.RcodeRefused}
}

// servFail is the response when servers of the queried address family are
// configured and none is usable: SERVFAIL, AA clear, no records.
func servFail() expectation {
	return expectation{rcode: dns.RcodeServerFailure}
}

// queryCase is one row of the conformance table.
type queryCase struct {
	// rule says what the case checks, in words.
	rule  string
	qname string
	qtype uint16
	want  expectation
}

// typesWithoutData are query types OpenGSLB holds no records for at a GSLB
// name. Each must be answered NOERROR with an empty answer and the zone SOA,
// never NOTIMP. SOA and NS are in the list because the name is not an apex.
// TYPE65280 is from the private use range: a type the server has never heard
// of gets the same answer.
var typesWithoutData = []uint16{
	dns.TypeTXT, dns.TypeCNAME, dns.TypeMX, dns.TypeHTTPS, dns.TypeSVCB,
	dns.TypeANY, dns.TypeDS, dns.TypeDNSKEY, dns.TypeSRV, dns.TypeCAA,
	dns.TypePTR, dns.TypeNAPTR, dns.TypeNSEC, dns.TypeSOA, dns.TypeNS,
	65280,
}

// conformanceTable returns the table of queries and expected responses.
func conformanceTable() []queryCase {
	const (
		gslbApexSOA       = "3600 IN SOA conformance-1.gslb.example.com. hostmaster.gslb.example.com. 1 3600 600 604800 60"
		corpApexSOA       = `corp.example.net. 7200 IN SOA ns1.corp.example.net. dns\.admin.example.net. 2026112001 7200 900 1209600 300`
		standaloneApexSOA = "standalone.example.org. 3600 IN SOA conformance-1.standalone.example.org. hostmaster.standalone.example.org. 1 3600 600 604800 60"
	)

	cases := []queryCase{
		// Positive answers.
		{"A for a name with an IPv4 server", "app.gslb.example.com.", dns.TypeA,
			answerOf("app.gslb.example.com. 30 IN A 10.0.1.10")},
		{"AAAA for a name with an IPv6 server", "v6.gslb.example.com.", dns.TypeAAAA,
			answerOf("v6.gslb.example.com. 30 IN AAAA 2001:db8::10")},
		{"A for a dual-stack name", "dual.gslb.example.com.", dns.TypeA,
			answerOf("dual.gslb.example.com. 30 IN A 10.0.2.10")},
		{"AAAA for a dual-stack name", "dual.gslb.example.com.", dns.TypeAAAA,
			answerOf("dual.gslb.example.com. 30 IN AAAA 2001:db8::20")},
		{"A in a zone configured in object form, default TTL", "www.corp.example.net.", dns.TypeA,
			answerOf("www.corp.example.net. 60 IN A 10.0.6.10")},
		{"A for a domain outside every configured zone (implicit zone)", "standalone.example.org.", dns.TypeA,
			answerOf("standalone.example.org. 30 IN A 10.0.7.10")},

		// Case-insensitive lookup; the owner name of the answer keeps the
		// querier's spelling (the question echo is checked for every case).
		{"mixed-case name is found, answer keeps the querier's case", "APP.GSLB.Example.COM.", dns.TypeA,
			answerOf("APP.GSLB.Example.COM. 30 IN A 10.0.1.10")},
		{"0x20-randomized name is found", "aPp.GsLb.eXaMpLe.CoM.", dns.TypeA,
			answerOf("aPp.GsLb.eXaMpLe.CoM. 30 IN A 10.0.1.10")},
		{"mixed-case name, empty answer", "APP.gslb.EXAMPLE.com.", dns.TypeAAAA,
			noData(gslbNegativeSOA)},
		{"mixed-case name that does not exist", "MISSING.Gslb.Example.Com.", dns.TypeA,
			nxDomain(gslbNegativeSOA)},
		{"mixed-case apex SOA, owner keeps the querier's case", "GSLB.Example.COM.", dns.TypeSOA,
			answerOf("GSLB.Example.COM. " + gslbApexSOA)},

		// Address families.
		{"AAAA for a name with only IPv4 servers is an empty answer, not SERVFAIL", "app.gslb.example.com.", dns.TypeAAAA,
			noData(gslbNegativeSOA)},
		{"A for a name with only IPv6 servers is an empty answer, not SERVFAIL", "v6.gslb.example.com.", dns.TypeA,
			noData(gslbNegativeSOA)},
		{"A when IPv4 servers are configured and none is healthy is SERVFAIL", "down.gslb.example.com.", dns.TypeA,
			servFail()},
		{"AAAA for the same name, which has no IPv6 server, is an empty answer", "down.gslb.example.com.", dns.TypeAAAA,
			noData(gslbNegativeSOA)},

		// Names that do not exist.
		{"A for a missing name", "missing.gslb.example.com.", dns.TypeA, nxDomain(gslbNegativeSOA)},
		{"AAAA for a missing name", "missing.gslb.example.com.", dns.TypeAAAA, nxDomain(gslbNegativeSOA)},
		{"TXT for a missing name", "missing.gslb.example.com.", dns.TypeTXT, nxDomain(gslbNegativeSOA)},
		{"SOA for a missing name", "missing.gslb.example.com.", dns.TypeSOA, nxDomain(gslbNegativeSOA)},
		{"ANY for a missing name", "missing.gslb.example.com.", dns.TypeANY, nxDomain(gslbNegativeSOA)},
		{"missing name below an existing name", "x.app.gslb.example.com.", dns.TypeA, nxDomain(gslbNegativeSOA)},
		{"missing name in the object-form zone carries that zone's SOA", "missing.corp.example.net.", dns.TypeA,
			nxDomain(corpNegativeSOA)},
		{"missing name below an implicit zone", "missing.standalone.example.org.", dns.TypeA,
			nxDomain(standaloneNegativeSOA)},

		// Empty non-terminal: eu.gslb.example.com has no records but a name
		// beneath it exists, so it is not NXDOMAIN (RFC 8020).
		{"A for an empty non-terminal", "eu.gslb.example.com.", dns.TypeA, noData(gslbNegativeSOA)},
		{"TXT for an empty non-terminal", "eu.gslb.example.com.", dns.TypeTXT, noData(gslbNegativeSOA)},
		{"A for the name beneath it", "api.eu.gslb.example.com.", dns.TypeA,
			answerOf("api.eu.gslb.example.com. 30 IN A 10.0.4.10")},

		// Names outside every served zone.
		{"A outside every zone", "www.google.com.", dns.TypeA, refused()},
		{"AAAA outside every zone", "www.google.com.", dns.TypeAAAA, refused()},
		{"TXT outside every zone", "www.google.com.", dns.TypeTXT, refused()},
		{"SOA outside every zone", "www.google.com.", dns.TypeSOA, refused()},
		{"NS outside every zone", "www.google.com.", dns.TypeNS, refused()},
		{"ANY outside every zone", "www.google.com.", dns.TypeANY, refused()},
		{"parent of a configured zone", "example.com.", dns.TypeSOA, refused()},
		{"parent of the object-form zone", "example.net.", dns.TypeNS, refused()},
		{"parent of an implicit zone", "example.org.", dns.TypeA, refused()},
		{"top-level domain", "com.", dns.TypeNS, refused()},
		{"the root", ".", dns.TypeNS, refused()},

		// Zone apex records.
		{"SOA at the apex of a string-form zone, defaults from the node identity", "gslb.example.com.", dns.TypeSOA,
			answerOf("gslb.example.com. " + gslbApexSOA)},
		{"NS at the apex of a string-form zone", "gslb.example.com.", dns.TypeNS,
			answerOf("gslb.example.com. 3600 IN NS conformance-1.gslb.example.com.")},
		{"SOA at the apex of an object-form zone, configured values", "corp.example.net.", dns.TypeSOA,
			answerOf(corpApexSOA)},
		{"NS at the apex of an object-form zone", "corp.example.net.", dns.TypeNS,
			answerOf("corp.example.net. 7200 IN NS ns1.corp.example.net.", "corp.example.net. 7200 IN NS ns2.corp.example.net.")},
		{"SOA at the apex of an implicit zone", "standalone.example.org.", dns.TypeSOA,
			answerOf(standaloneApexSOA)},
		{"NS at the apex of an implicit zone", "standalone.example.org.", dns.TypeNS,
			answerOf("standalone.example.org. 3600 IN NS conformance-1.standalone.example.org.")},
		{"A at an apex that is not a GSLB domain", "gslb.example.com.", dns.TypeA, noData(gslbNegativeSOA)},
		{"TXT at an apex", "corp.example.net.", dns.TypeTXT, noData(corpNegativeSOA)},

		// Zone transfers are not offered.
		{"AXFR is refused", "gslb.example.com.", dns.TypeAXFR, refused()},
		{"IXFR is refused", "gslb.example.com.", dns.TypeIXFR, refused()},
	}

	// Every type without data, at a GSLB name in each kind of zone.
	for _, qtype := range typesWithoutData {
		name := dns.Type(qtype).String()
		cases = append(cases,
			queryCase{name + " at a GSLB name is an empty answer, not NOTIMP", "app.gslb.example.com.", qtype, noData(gslbNegativeSOA)},
			queryCase{name + " at a GSLB name in the object-form zone", "www.corp.example.net.", qtype, noData(corpNegativeSOA)},
		)
	}
	return cases
}

// transports are the ways each case is asked: over UDP and TCP, without and
// with an EDNS(0) OPT record.
var transports = []struct {
	name     string
	network  string
	ednsSize uint16
}{
	{"udp", "udp", 0},
	{"udp+edns", "udp", 1232},
	{"tcp", "tcp", 0},
	{"tcp+edns", "tcp", 1232},
}

// TestConformance asks every query of the table over every transport and
// checks the response code, the AA flag and the answer and authority
// sections. The response must be the same on all four transports, apart from
// the OPT record, which is present exactly when the query had one.
func TestConformance(t *testing.T) {
	srv := startServer(t, nil)

	for _, tc := range conformanceTable() {
		t.Run(tc.rule+"/"+tc.qname+"/"+dns.Type(tc.qtype).String(), func(t *testing.T) {
			for _, tr := range transports {
				t.Run(tr.name, func(t *testing.T) {
					q := newQuery(tc.qname, tc.qtype, tr.ednsSize)
					resp, _ := srv.exchange(t, tr.network, q)

					checkEnvelope(t, q, resp)
					checkExpectation(t, resp, tc.want)
					if resp.Truncated {
						t.Error("TC bit is set on a response that fits")
					}
				})
			}
		})
	}
}

// checkExpectation compares a response with what a case expects.
func checkExpectation(t *testing.T, resp *dns.Msg, want expectation) {
	t.Helper()
	if resp.Rcode != want.rcode {
		t.Errorf("rcode = %s, want %s", rcodeName(resp.Rcode), rcodeName(want.rcode))
	}
	if resp.Authoritative != want.aa {
		t.Errorf("AA = %v, want %v", resp.Authoritative, want.aa)
	}
	if got := rrStrings(resp.Answer); !sameStrings(got, want.answer) {
		t.Errorf("answer section:\n  got  %q\n  want %q", got, want.answer)
	}
	if got := rrStrings(resp.Ns); !sameStrings(got, want.authority) {
		t.Errorf("authority section:\n  got  %q\n  want %q", got, want.authority)
	}
}

// TestConformance_TableCoversTheBrief guards the table itself: it must keep
// at least one case of every kind of response, so that a later edit cannot
// quietly drop, say, all REFUSED cases.
func TestConformance_TableCoversTheBrief(t *testing.T) {
	kinds := map[string]int{}
	for _, tc := range conformanceTable() {
		switch {
		case tc.want.rcode == dns.RcodeSuccess && len(tc.want.answer) > 0:
			kinds["answer"]++
			if tc.qtype == dns.TypeSOA || tc.qtype == dns.TypeNS {
				kinds["apex"]++
			}
		case tc.want.rcode == dns.RcodeSuccess:
			kinds["nodata"]++
		case tc.want.rcode == dns.RcodeNameError:
			kinds["nxdomain"]++
		case tc.want.rcode == dns.RcodeRefused:
			kinds["refused"]++
		case tc.want.rcode == dns.RcodeServerFailure:
			kinds["servfail"]++
		}
	}
	for _, kind := range []string{"answer", "apex", "nodata", "nxdomain", "refused", "servfail"} {
		if kinds[kind] == 0 {
			t.Errorf("the conformance table has no %s case", kind)
		}
	}
}
