// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package conformance

import (
	"testing"

	"github.com/miekg/dns"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// TestReturnLastHealthy checks dns.return_last_healthy over the wire. With
// the default (false) a name whose servers are all down is SERVFAIL; that is
// a row of TestConformance. With true the server answers as if the configured
// servers of the queried family were healthy. A family without configured
// servers stays an empty answer.
func TestReturnLastHealthy(t *testing.T) {
	srv := startServer(t, func(cfg *config.Config) { cfg.DNS.ReturnLastHealthy = true })

	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			q := newQuery("down.gslb.example.com.", dns.TypeA, tr.ednsSize)
			resp, _ := srv.exchange(t, tr.network, q)
			checkEnvelope(t, q, resp)
			checkExpectation(t, resp, answerOf("down.gslb.example.com. 30 IN A 10.0.3.10"))

			q = newQuery("down.gslb.example.com.", dns.TypeAAAA, tr.ednsSize)
			resp, _ = srv.exchange(t, tr.network, q)
			checkEnvelope(t, q, resp)
			checkExpectation(t, resp, noData(gslbNegativeSOA))

			// A healthy name is not affected.
			q = newQuery("app.gslb.example.com.", dns.TypeA, tr.ednsSize)
			resp, _ = srv.exchange(t, tr.network, q)
			checkEnvelope(t, q, resp)
			checkExpectation(t, resp, answerOf("app.gslb.example.com. 30 IN A 10.0.1.10"))
		})
	}
}

// TestRequestsTheServerDoesNotServe covers requests that are not ordinary
// class IN queries. None of them may be answered from zone data, and none is
// authoritative.
func TestRequestsTheServerDoesNotServe(t *testing.T) {
	srv := startServer(t, nil)

	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			// A class other than IN is refused, also for a name that exists.
			q := newQuery("app.gslb.example.com.", dns.TypeA, tr.ednsSize)
			q.Question[0].Qclass = dns.ClassCHAOS
			resp, _ := srv.exchange(t, tr.network, q)
			checkEnvelope(t, q, resp)
			checkExpectation(t, resp, refused())

			// An opcode other than QUERY is not implemented.
			for _, opcode := range []int{dns.OpcodeNotify, dns.OpcodeUpdate, dns.OpcodeStatus} {
				q = newQuery("gslb.example.com.", dns.TypeSOA, tr.ednsSize)
				q.Opcode = opcode
				resp, _ = srv.exchange(t, tr.network, q)
				checkEnvelope(t, q, resp)
				checkExpectation(t, resp, expectation{rcode: dns.RcodeNotImplemented})
			}

			// A message without a question is a format error.
			q = new(dns.Msg)
			q.Id = dns.Id()
			if tr.ednsSize > 0 {
				q.SetEdns0(tr.ednsSize, false)
			}
			resp, _ = srv.exchange(t, tr.network, q)
			checkEnvelope(t, q, resp)
			checkExpectation(t, resp, expectation{rcode: dns.RcodeFormatError})
		})
	}
}
