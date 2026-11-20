// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"testing"

	"github.com/miekg/dns"
)

// TestHandler_UnsupportedTypes covers finding 1B #3: inside a zone, a type
// the server has no data for answers NOERROR with an empty answer and the SOA
// in the authority section, not NOTIMP.
func TestHandler_UnsupportedTypes(t *testing.T) {
	f := newRcodeFixture(t)

	types := []uint16{
		dns.TypeTXT, dns.TypeCNAME, dns.TypeMX, dns.TypeHTTPS, dns.TypeANY,
		dns.TypeDS, dns.TypeSRV, dns.TypeCAA, dns.TypePTR, dns.TypeSVCB,
		dns.TypeNAPTR, dns.TypeDNSKEY, 65280, // 65280 is in the private-use range
	}
	for _, qtype := range types {
		name := dns.TypeToString[qtype]
		if name == "" {
			name = "TYPE65280"
		}
		t.Run(name+" at a GSLB name", func(t *testing.T) {
			resp := serveQuery(t, f.handler, "udp", question("v4.gslb.example.com.", qtype))
			assertNegative(t, resp, dns.RcodeSuccess, "gslb.example.com.")
		})
		t.Run(name+" at the zone apex", func(t *testing.T) {
			resp := serveQuery(t, f.handler, "udp", question("gslb.example.com.", qtype))
			assertNegative(t, resp, dns.RcodeSuccess, "gslb.example.com.")
		})
	}

	// SOA and NS exist only at the apex. Below it they are empty answers.
	for _, qtype := range []uint16{dns.TypeSOA, dns.TypeNS} {
		t.Run(dns.TypeToString[qtype]+" below the apex", func(t *testing.T) {
			resp := serveQuery(t, f.handler, "udp", question("v4.gslb.example.com.", qtype))
			assertNegative(t, resp, dns.RcodeSuccess, "gslb.example.com.")
		})
	}

	// The apex of a configured zone exists even when no GSLB domain is
	// registered at it, so address queries there are empty answers too.
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		t.Run(dns.TypeToString[qtype]+" at an apex without a GSLB domain", func(t *testing.T) {
			resp := serveQuery(t, f.handler, "udp", question("gslb.example.com.", qtype))
			assertNegative(t, resp, dns.RcodeSuccess, "gslb.example.com.")
		})
	}

	t.Run("TXT in an implicit zone", func(t *testing.T) {
		resp := serveQuery(t, f.handler, "udp", question("app.other.example.", dns.TypeTXT))
		assertNegative(t, resp, dns.RcodeSuccess, "app.other.example.")
	})
}

// TestHandler_ZoneTransfersRefused checks that AXFR and IXFR, which the
// server does not implement, are refused instead of being answered with an
// empty NOERROR that a secondary could mistake for an empty zone.
func TestHandler_ZoneTransfersRefused(t *testing.T) {
	f := newRcodeFixture(t)
	for _, qtype := range []uint16{dns.TypeAXFR, dns.TypeIXFR} {
		resp := serveQuery(t, f.handler, "tcp", question("gslb.example.com.", qtype))
		if resp.Rcode != dns.RcodeRefused || resp.Authoritative {
			t.Errorf("%s: rcode = %s, AA = %v, want REFUSED with AA clear",
				dns.TypeToString[qtype], dns.RcodeToString[resp.Rcode], resp.Authoritative)
		}
		if len(resp.Answer) != 0 || len(resp.Ns) != 0 {
			t.Errorf("%s: refused transfer carries records", dns.TypeToString[qtype])
		}
	}
}

// keyAddingSigner stands in for the DNSSEC signer: it adds a DNSKEY record to
// the answer of a DNSKEY query, as the real signer does.
type keyAddingSigner struct{}

func (keyAddingSigner) SignResponse(msg interface{}) (interface{}, error) {
	m := msg.(*dns.Msg)
	if len(m.Question) == 1 && m.Question[0].Qtype == dns.TypeDNSKEY && m.Rcode == dns.RcodeSuccess {
		m.Answer = append(m.Answer, &dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
			Flags:     257,
			Protocol:  3,
			Algorithm: dns.ECDSAP256SHA256,
			PublicKey: "AA==",
		})
	}
	return m, nil
}

// TestHandler_DNSKEYWithSigner checks that DNSKEY handling is unchanged when
// a DNSSEC signer is active: the handler leaves a successful empty answer for
// the signer to fill in and adds no SOA of its own.
func TestHandler_DNSKEYWithSigner(t *testing.T) {
	f := newRcodeFixture(t)
	f.handler.SetDNSSECSigner(keyAddingSigner{}, true)

	for _, name := range []string{"v4.gslb.example.com.", "gslb.example.com."} {
		resp := serveQuery(t, f.handler, "udp", question(name, dns.TypeDNSKEY))
		if resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("%s: rcode = %s, want NOERROR", name, dns.RcodeToString[resp.Rcode])
		}
		if len(resp.Answer) != 1 || resp.Answer[0].Header().Rrtype != dns.TypeDNSKEY {
			t.Errorf("%s: answer = %v, want the DNSKEY added by the signer", name, resp.Answer)
		}
		if len(resp.Ns) != 0 {
			t.Errorf("%s: authority = %v, want it empty", name, resp.Ns)
		}
	}
}
