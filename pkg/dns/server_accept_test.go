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

// TestAcceptMsg covers the decision, made from the message header alone,
// whether a message reaches the handler. Requests the handler can answer
// itself must reach it, so that their responses carry the question and the
// OPT record (finding 1B #6); the DNS library's own replies carry neither.
func TestAcceptMsg(t *testing.T) {
	header := func(opcode int, response bool, qd, an, ns, ar uint16) dns.Header {
		bits := uint16(opcode) << 11
		if response {
			bits |= 1 << 15
		}
		return dns.Header{Bits: bits, Qdcount: qd, Ancount: an, Nscount: ns, Arcount: ar}
	}

	for _, tc := range []struct {
		name string
		dh   dns.Header
		want dns.MsgAcceptAction
	}{
		{"ordinary query", header(dns.OpcodeQuery, false, 1, 0, 0, 0), dns.MsgAccept},
		{"query with OPT", header(dns.OpcodeQuery, false, 1, 0, 0, 1), dns.MsgAccept},
		{"query with OPT and TSIG", header(dns.OpcodeQuery, false, 1, 0, 0, 2), dns.MsgAccept},
		{"IXFR query with an SOA in the authority section", header(dns.OpcodeQuery, false, 1, 0, 1, 0), dns.MsgAccept},
		{"NOTIFY with an SOA in the answer section", header(dns.OpcodeNotify, false, 1, 1, 0, 0), dns.MsgAccept},
		// The handler answers these itself: FORMERR and NOTIMP.
		{"no question", header(dns.OpcodeQuery, false, 0, 0, 0, 1), dns.MsgAccept},
		{"STATUS", header(dns.OpcodeStatus, false, 1, 0, 0, 1), dns.MsgAccept},
		{"small UPDATE", header(dns.OpcodeUpdate, false, 1, 0, 1, 1), dns.MsgAccept},
		// Never answer a response.
		{"response", header(dns.OpcodeQuery, true, 1, 1, 0, 0), dns.MsgIgnore},
		{"response without a question", header(dns.OpcodeQuery, true, 0, 0, 0, 0), dns.MsgIgnore},
		// More records than a query can have: rejected before parsing.
		{"two questions", header(dns.OpcodeQuery, false, 2, 0, 0, 0), dns.MsgReject},
		{"many answers", header(dns.OpcodeQuery, false, 1, 50, 0, 0), dns.MsgReject},
		{"many authority records", header(dns.OpcodeQuery, false, 1, 0, 50, 0), dns.MsgReject},
		{"many additional records", header(dns.OpcodeQuery, false, 1, 0, 0, 3), dns.MsgReject},
		{"UPDATE with many records", header(dns.OpcodeUpdate, false, 1, 3, 5, 0), dns.MsgRejectNotImplemented},
	} {
		if got := acceptMsg(tc.dh); got != tc.want {
			t.Errorf("%s: acceptMsg = %d, want %d", tc.name, got, tc.want)
		}
	}
}
