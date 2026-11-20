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

// TestHandler_OutOfZoneRefused covers finding 1B #5: a name outside every
// zone this server is authoritative for is answered REFUSED with the AA bit
// clear, not with an authoritative NXDOMAIN.
func TestHandler_OutOfZoneRefused(t *testing.T) {
	f := newRcodeFixture(t)
	noZones := NewHandler(HandlerConfig{Registry: f.handler.state().registry, DefaultTTL: 60})
	empty := NewHandler(HandlerConfig{Registry: NewRegistry(), DefaultTTL: 60})

	handlers := map[string]*Handler{
		"configured zone":    f.handler,
		"implicit zones":     noZones,
		"nothing configured": empty,
	}
	names := []string{"www.google.com.", "example.com.", "com.", ".", "notgslb.example.com.", "other.example."}
	types := []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT, dns.TypeSOA, dns.TypeNS, dns.TypeDNSKEY, dns.TypeANY}

	for label, handler := range handlers {
		for _, name := range names {
			for _, qtype := range types {
				resp := serveQuery(t, handler, "udp", question(name, qtype))
				if resp.Rcode != dns.RcodeRefused {
					t.Errorf("%s: %s %s: rcode = %s, want REFUSED", label, name, dns.TypeToString[qtype], dns.RcodeToString[resp.Rcode])
				}
				if resp.Authoritative {
					t.Errorf("%s: %s %s: AA bit is set, want it clear", label, name, dns.TypeToString[qtype])
				}
				if len(resp.Answer) != 0 || len(resp.Ns) != 0 {
					t.Errorf("%s: %s %s: REFUSED carries records: answer %v, authority %v", label, name, dns.TypeToString[qtype], resp.Answer, resp.Ns)
				}
			}
		}
	}
}

// TestHandler_NonINClassRefused checks that only class IN is served. A CH or
// ANY class query for a configured name must not get an IN answer.
func TestHandler_NonINClassRefused(t *testing.T) {
	f := newRcodeFixture(t)
	for _, class := range []uint16{dns.ClassCHAOS, dns.ClassHESIOD, dns.ClassANY} {
		q := question("v4.gslb.example.com.", dns.TypeA)
		q.Question[0].Qclass = class
		resp := serveQuery(t, f.handler, "udp", q)
		if resp.Rcode != dns.RcodeRefused || resp.Authoritative || len(resp.Answer) != 0 {
			t.Errorf("class %s: rcode = %s, AA = %v, answers = %d, want REFUSED, AA clear, no answer",
				dns.ClassToString[class], dns.RcodeToString[resp.Rcode], resp.Authoritative, len(resp.Answer))
		}
	}
}

// TestHandler_ErrorResponsesAreNotAuthoritative checks the AA bit on the
// error responses that make no statement about zone data.
func TestHandler_ErrorResponsesAreNotAuthoritative(t *testing.T) {
	f := newRcodeFixture(t)

	// FORMERR: no question.
	resp := serveQuery(t, f.handler, "udp", new(dns.Msg))
	if resp.Rcode != dns.RcodeFormatError || resp.Authoritative {
		t.Errorf("no question: rcode = %s, AA = %v, want FORMERR with AA clear", dns.RcodeToString[resp.Rcode], resp.Authoritative)
	}

	// NOTIMP: an opcode other than QUERY.
	q := question("v4.gslb.example.com.", dns.TypeA)
	q.Opcode = dns.OpcodeNotify
	resp = serveQuery(t, f.handler, "udp", q)
	if resp.Rcode != dns.RcodeNotImplemented || resp.Authoritative || len(resp.Answer) != 0 {
		t.Errorf("NOTIFY: rcode = %s, AA = %v, answers = %d, want NOTIMP, AA clear, no answer",
			dns.RcodeToString[resp.Rcode], resp.Authoritative, len(resp.Answer))
	}

	// SERVFAIL: every server of the family is down.
	f.health.SetHealthy("10.0.0.1", false)
	resp = serveQuery(t, f.handler, "udp", question("v4.gslb.example.com.", dns.TypeA))
	if resp.Rcode != dns.RcodeServerFailure || resp.Authoritative {
		t.Errorf("all down: rcode = %s, AA = %v, want SERVFAIL with AA clear", dns.RcodeToString[resp.Rcode], resp.Authoritative)
	}
}
