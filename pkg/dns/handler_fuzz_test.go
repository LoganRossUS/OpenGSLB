// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// The fuzz target lives in an external test package on purpose: it may only
// use the exported surface of pkg/dns (NewRegistry, Register, NewHandler,
// ServeDNS), the same surface cmd/opengslb uses.
package dns_test

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"

	gslbdns "github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/miekg/dns"
)

// Fuzz test for the DNS handler entry point, Handler.ServeDNS.
//
// The input is the raw bytes of a DNS message as they arrive on the socket.
// They are unpacked the way the miekg/dns server unpacks them before it calls
// the handler, and the handler runs against a registry that has one domain
// per routing algorithm and per address-family mix.
//
// Run it for a while with:
//
//	go test -run='^$' -fuzz='^FuzzHandlerServeDNS$' -fuzztime=60s ./pkg/dns/
//
// Without -fuzz the seed corpus runs as a normal, fast unit test.
//
// The invariants are the ones every DNS response must satisfy, whatever the
// handler decides to answer. No particular RCODE is asserted.
//
//   - ServeDNS does not panic. miekg/dns does not recover handler panics, so
//     a panic here is a remote crash of the Overwatch.
//   - At most one response is written per query.
//   - The response can be packed and the packed bytes unpack again.
//   - The response carries the query ID and has the QR bit set.
//   - The question section echoes the query's question byte for byte,
//     including the case of the name (resolvers that use 0x20 check this).

// fuzzUnhealthy lists the backend addresses the fake health provider reports
// as down.
var fuzzUnhealthy = map[string]bool{
	"10.0.9.1":    true,
	"10.0.9.2":    true,
	"2001:db8::9": true,
}

type fuzzHealth struct{}

func (fuzzHealth) IsHealthy(address string, _ int) bool { return !fuzzUnhealthy[address] }

// fuzzResponseWriter is a dns.ResponseWriter that records what the handler
// writes. WriteMsg packs the message like the real server does.
type fuzzResponseWriter struct {
	tcp    bool
	writes int
	msg    *dns.Msg
	packed []byte
	err    error
}

func (w *fuzzResponseWriter) addr(ip net.IP, port int) net.Addr {
	if w.tcp {
		return &net.TCPAddr{IP: ip, Port: port}
	}
	return &net.UDPAddr{IP: ip, Port: port}
}

func (w *fuzzResponseWriter) LocalAddr() net.Addr  { return w.addr(net.IPv4(192, 0, 2, 53), 53) }
func (w *fuzzResponseWriter) RemoteAddr() net.Addr { return w.addr(net.IPv4(198, 51, 100, 7), 40000) }

func (w *fuzzResponseWriter) WriteMsg(m *dns.Msg) error {
	w.writes++
	w.msg = m
	w.packed, w.err = m.Pack()
	return w.err
}

func (w *fuzzResponseWriter) Write(b []byte) (int, error) {
	w.writes++
	w.msg = nil
	w.packed = append([]byte(nil), b...)
	return len(b), nil
}

func (w *fuzzResponseWriter) Close() error        { return nil }
func (w *fuzzResponseWriter) TsigStatus() error   { return nil }
func (w *fuzzResponseWriter) TsigTimersOnly(bool) {}
func (w *fuzzResponseWriter) Hijack()             {}

func fuzzServers(addrs ...string) []gslbdns.ServerInfo {
	servers := make([]gslbdns.ServerInfo, 0, len(addrs))
	for i, a := range addrs {
		region := "eu-west-1"
		if i%2 == 1 {
			region = "us-east-1"
		}
		servers = append(servers, gslbdns.ServerInfo{
			Address: net.ParseIP(a),
			Port:    80,
			Weight:  100 * (i + 1),
			Region:  region,
		})
	}
	return servers
}

// newFuzzHandler builds a handler over a registry with every routing
// algorithm, IPv4-only, IPv6-only and dual-stack domains, a domain whose
// servers are all down and a domain with no servers at all.
func newFuzzHandler(tb testing.TB, ecs bool) *gslbdns.Handler {
	tb.Helper()

	domains := []struct {
		name      string
		algorithm string
		ttl       uint32
		servers   []gslbdns.ServerInfo
	}{
		{"app.gslb.example.com", routing.AlgorithmRoundRobin, 30, fuzzServers("10.0.1.1", "10.0.1.2", "2001:db8::1", "2001:db8::2")},
		{"v4only.gslb.example.com", routing.AlgorithmWeighted, 60, fuzzServers("10.0.2.1", "10.0.2.2", "10.0.2.3")},
		{"v6only.gslb.example.com", routing.AlgorithmFailover, 0, fuzzServers("2001:db8::3", "2001:db8::4")},
		{"geo.gslb.example.com", routing.AlgorithmGeolocation, 30, fuzzServers("10.0.3.1", "10.0.3.2", "2001:db8::5")},
		{"latency.gslb.example.com", routing.AlgorithmLatency, 30, fuzzServers("10.0.4.1", "2001:db8::6")},
		{"down.gslb.example.com", routing.AlgorithmRoundRobin, 30, fuzzServers("10.0.9.1", "10.0.9.2", "2001:db8::9")},
		{"partial.gslb.example.com", routing.AlgorithmWeighted, 30, fuzzServers("10.0.9.1", "10.0.5.1")},
		{"empty.gslb.example.com", routing.AlgorithmRoundRobin, 30, nil},
		{"other-zone.example.net", routing.AlgorithmFailover, 30, fuzzServers("10.0.6.1")},
	}

	registry := gslbdns.NewRegistry()
	for _, d := range domains {
		router, err := routing.NewRouter(d.algorithm)
		if err != nil {
			tb.Fatalf("router %q: %v", d.algorithm, err)
		}
		registry.Register(&gslbdns.DomainEntry{
			Name:             d.name,
			TTL:              d.ttl,
			RoutingAlgorithm: d.algorithm,
			Router:           router,
			Servers:          d.servers,
		})
	}

	return gslbdns.NewHandler(gslbdns.HandlerConfig{
		Registry:       registry,
		HealthProvider: fuzzHealth{},
		ECSEnabled:     ecs,
		DefaultTTL:     30,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1})),
	})
}

// serverWouldDispatch reports whether a stock miekg/dns server passes this
// packet to the handler: the header must satisfy dns.DefaultMsgAcceptFunc
// (a query or notify with exactly one question and few records) and the
// message must unpack. pkg/dns/server.go does not replace the accept function.
// Messages that unpack but would be rejected (zero or several questions, a
// response, an UPDATE) are still given to the handler below, because the
// handler must not depend on the server's filter to stay up; a failure report
// states which kind of input it was.
func serverWouldDispatch(wire []byte) bool {
	if len(wire) < 12 {
		return false
	}
	header := dns.Header{
		Id:      binary.BigEndian.Uint16(wire[0:]),
		Bits:    binary.BigEndian.Uint16(wire[2:]),
		Qdcount: binary.BigEndian.Uint16(wire[4:]),
		Ancount: binary.BigEndian.Uint16(wire[6:]),
		Nscount: binary.BigEndian.Uint16(wire[8:]),
		Arcount: binary.BigEndian.Uint16(wire[10:]),
	}
	return dns.DefaultMsgAcceptFunc(header) == dns.MsgAccept
}

func mustPack(tb testing.TB, m *dns.Msg) []byte {
	tb.Helper()
	wire, err := m.Pack()
	if err != nil {
		tb.Fatalf("pack seed: %v", err)
	}
	return wire
}

func fuzzQuery(name string, qtype uint16) *dns.Msg {
	m := new(dns.Msg)
	m.Id = 0x1234
	m.RecursionDesired = true
	m.Question = []dns.Question{{Name: name, Qtype: qtype, Qclass: dns.ClassINET}}
	return m
}

func withEDNS(m *dns.Msg, size uint16, do bool, options ...dns.EDNS0) *dns.Msg {
	m.SetEdns0(size, do)
	opt := m.IsEdns0()
	opt.Option = append(opt.Option, options...)
	return m
}

func ecsOption(family uint16, prefix uint8, addr string) *dns.EDNS0_SUBNET {
	return &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        family,
		SourceNetmask: prefix,
		Address:       net.ParseIP(addr),
	}
}

// dnsSeedCorpus returns realistic queries as wire bytes.
func dnsSeedCorpus(tb testing.TB) [][]byte {
	tb.Helper()
	const app = "app.gslb.example.com."

	var msgs []*dns.Msg

	// Plain A and AAAA for every kind of domain, for a missing name inside
	// the zone, for a name outside every zone and for the root.
	for _, name := range []string{
		app,
		"v4only.gslb.example.com.",
		"v6only.gslb.example.com.",
		"geo.gslb.example.com.",
		"latency.gslb.example.com.",
		"down.gslb.example.com.",
		"partial.gslb.example.com.",
		"empty.gslb.example.com.",
		"other-zone.example.net.",
		"missing.gslb.example.com.",
		"gslb.example.com.",
		"www.google.com.",
		".",
	} {
		msgs = append(msgs, fuzzQuery(name, dns.TypeA), fuzzQuery(name, dns.TypeAAAA))
	}

	// Mixed case, as sent by resolvers that use 0x20 randomization.
	msgs = append(msgs,
		fuzzQuery("APP.GSLB.Example.COM.", dns.TypeA),
		fuzzQuery("aPp.gSlB.eXaMpLe.CoM.", dns.TypeAAAA),
		fuzzQuery("V6ONLY.GSLB.EXAMPLE.COM.", dns.TypeA),
	)

	// Types the handler does not synthesize answers for, meta types, and
	// classes other than IN.
	for _, qtype := range []uint16{
		dns.TypeSOA, dns.TypeNS, dns.TypeTXT, dns.TypeCNAME, dns.TypeMX, dns.TypeHTTPS, dns.TypeSVCB,
		dns.TypeANY, dns.TypeDS, dns.TypeDNSKEY, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeNSEC3, dns.TypeCAA,
		dns.TypePTR, dns.TypeSRV, dns.TypeAXFR, dns.TypeIXFR, dns.TypeOPT, dns.TypeTSIG, dns.TypeNone,
		65280, 65535,
	} {
		msgs = append(msgs, fuzzQuery(app, qtype))
	}
	msgs = append(msgs, fuzzQuery("gslb.example.com.", dns.TypeSOA), fuzzQuery("gslb.example.com.", dns.TypeNS))
	for _, class := range []uint16{dns.ClassCHAOS, dns.ClassANY, dns.ClassNONE, 0, 65535} {
		m := fuzzQuery(app, dns.TypeA)
		m.Question[0].Qclass = class
		msgs = append(msgs, m)
	}
	chaos := fuzzQuery("version.bind.", dns.TypeTXT)
	chaos.Question[0].Qclass = dns.ClassCHAOS
	msgs = append(msgs, chaos)

	// EDNS without ECS: usual sizes, the DO bit, a size below 512, a cookie,
	// an NSID request, padding and an option code nobody assigned.
	msgs = append(msgs,
		withEDNS(fuzzQuery(app, dns.TypeA), 1232, false),
		withEDNS(fuzzQuery(app, dns.TypeAAAA), 4096, true),
		withEDNS(fuzzQuery(app, dns.TypeA), 512, false),
		withEDNS(fuzzQuery(app, dns.TypeA), 0, true),
		withEDNS(fuzzQuery(app, dns.TypeA), 65535, false),
		withEDNS(fuzzQuery("missing.gslb.example.com.", dns.TypeA), 1232, true),
		withEDNS(fuzzQuery("www.google.com.", dns.TypeA), 1232, false),
		withEDNS(fuzzQuery(app, dns.TypeA), 1232, false, &dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef"}),
		withEDNS(fuzzQuery(app, dns.TypeA), 1232, false, &dns.EDNS0_NSID{Code: dns.EDNS0NSID}),
		withEDNS(fuzzQuery(app, dns.TypeA), 1232, false, &dns.EDNS0_PADDING{Padding: make([]byte, 32)}),
		withEDNS(fuzzQuery(app, dns.TypeA), 1232, false, &dns.EDNS0_LOCAL{Code: 65001, Data: []byte{1, 2, 3}}),
	)
	// A second EDNS version.
	v1 := withEDNS(fuzzQuery(app, dns.TypeA), 1232, false)
	v1.IsEdns0().SetVersion(1)
	msgs = append(msgs, v1)

	// EDNS with ECS (RFC 7871): IPv4 and IPv6 prefixes of several lengths,
	// the /0 opt-out, family 0, and ECS next to another option.
	msgs = append(msgs,
		withEDNS(fuzzQuery(app, dns.TypeA), 1232, false, ecsOption(1, 24, "203.0.113.0")),
		withEDNS(fuzzQuery(app, dns.TypeAAAA), 1232, false, ecsOption(2, 56, "2001:db8:abcd::")),
		withEDNS(fuzzQuery("geo.gslb.example.com.", dns.TypeA), 4096, true, ecsOption(1, 32, "198.51.100.77")),
		withEDNS(fuzzQuery("geo.gslb.example.com.", dns.TypeAAAA), 1232, false, ecsOption(2, 128, "2001:db8::77")),
		withEDNS(fuzzQuery("latency.gslb.example.com.", dns.TypeA), 1232, false, ecsOption(1, 0, "0.0.0.0")),
		withEDNS(fuzzQuery(app, dns.TypeA), 1232, false, ecsOption(0, 0, "0.0.0.0")),
		withEDNS(fuzzQuery(app, dns.TypeA), 1232, false, ecsOption(1, 20, "10.20.0.0"),
			&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: "0123456789abcdef"}),
		withEDNS(fuzzQuery("missing.gslb.example.com.", dns.TypeA), 1232, false, ecsOption(1, 24, "203.0.113.0")),
		withEDNS(fuzzQuery(app, dns.TypeTXT), 1232, false, ecsOption(1, 24, "203.0.113.0")),
	)

	// Header variations: opcodes, a response sent as a query, flag bits.
	for _, opcode := range []int{dns.OpcodeNotify, dns.OpcodeUpdate, dns.OpcodeStatus, dns.OpcodeIQuery, 15} {
		m := fuzzQuery(app, dns.TypeA)
		m.Opcode = opcode
		msgs = append(msgs, m)
	}
	flags := fuzzQuery(app, dns.TypeA)
	flags.Response, flags.Authoritative, flags.Truncated = true, true, true
	flags.RecursionAvailable, flags.AuthenticatedData, flags.CheckingDisabled, flags.Zero = true, true, true, true
	flags.Rcode = dns.RcodeRefused
	msgs = append(msgs, flags)

	// Several questions, and none.
	multi := fuzzQuery(app, dns.TypeA)
	multi.Question = append(multi.Question,
		dns.Question{Name: "v6only.gslb.example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET},
		dns.Question{Name: "www.google.com.", Qtype: dns.TypeMX, Qclass: dns.ClassINET},
	)
	none := new(dns.Msg)
	none.Id = 0x4321
	noneEDNS := new(dns.Msg)
	noneEDNS.Id = 0x4322
	noneEDNS.SetEdns0(1232, true)
	msgs = append(msgs, multi, none, noneEDNS)

	// Records in the other sections, packed with name compression so the
	// wire form has pointers back into the question.
	compressed := fuzzQuery(app, dns.TypeA)
	compressed.Compress = true
	compressed.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: app, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
		A:   net.IPv4(10, 0, 1, 1),
	}}
	compressed.Ns = []dns.RR{&dns.NS{
		Hdr: dns.RR_Header{Name: "gslb.example.com.", Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 30},
		Ns:  "ns1.gslb.example.com.",
	}}
	compressed.Extra = []dns.RR{&dns.TXT{
		Hdr: dns.RR_Header{Name: "ns1.gslb.example.com.", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 30},
		Txt: []string{"x"},
	}}
	tsig := fuzzQuery(app, dns.TypeA)
	tsig.SetTsig("key.example.", dns.HmacSHA256, 300, 1763640000)
	msgs = append(msgs, compressed, tsig)

	// Names at the limits: 63-octet labels up to the 255-octet maximum, many
	// one-octet labels, and labels with bytes that need escaping.
	label63 := strings.Repeat("a", 63)
	longName := label63 + "." + label63 + "." + label63 + "." + strings.Repeat("b", 61) + "."
	msgs = append(msgs,
		fuzzQuery(longName, dns.TypeA),
		fuzzQuery(strings.Repeat("a.", 127), dns.TypeAAAA),
		fuzzQuery(`a\.b\000\255\032c.gslb.example.com.`, dns.TypeA),
		fuzzQuery("*.gslb.example.com.", dns.TypeA),
		fuzzQuery("_dns.resolver.arpa.", dns.TypeSVCB),
		fuzzQuery("xn--bcher-kva.gslb.example.com.", dns.TypeA),
	)

	var corpus [][]byte
	for _, m := range msgs {
		corpus = append(corpus, mustPack(tb, m))
	}

	// Hand-built wire forms that Pack never produces.
	header := func(qd, an, ns, ar uint16) []byte {
		h := make([]byte, 12)
		binary.BigEndian.PutUint16(h[0:], 0xbeef)
		binary.BigEndian.PutUint16(h[2:], 0x0100) // RD
		binary.BigEndian.PutUint16(h[4:], qd)
		binary.BigEndian.PutUint16(h[6:], an)
		binary.BigEndian.PutUint16(h[8:], ns)
		binary.BigEndian.PutUint16(h[10:], ar)
		return h
	}
	qname := []byte("\x03app\x04gslb\x07example\x03com\x00")
	question := func(name []byte, qtype uint16) []byte {
		q := append([]byte{}, name...)
		q = binary.BigEndian.AppendUint16(q, qtype)
		return binary.BigEndian.AppendUint16(q, dns.ClassINET)
	}
	corpus = append(corpus,
		// Two questions, the second name is a compression pointer to the first.
		append(append(header(2, 0, 0, 0), question(qname, dns.TypeA)...), question([]byte{0xc0, 0x0c}, dns.TypeAAAA)...),
		// One question whose name ends in a pointer to a suffix of itself
		// placed in a second question ("app" + pointer to "gslb.example.com").
		append(append(header(2, 0, 0, 0), question(qname, dns.TypeA)...), question([]byte{0x03, 'w', 'w', 'w', 0xc0, 0x10}, dns.TypeA)...),
		// A pointer that points at itself, and one that points forward.
		append(header(1, 0, 0, 0), question([]byte{0xc0, 0x0c}, dns.TypeA)...),
		append(header(1, 0, 0, 0), question([]byte{0xc0, 0xff}, dns.TypeA)...),
		// OPT with an ECS option whose address is longer than the prefix
		// needs, and one with a truncated option.
		append(append(header(1, 0, 0, 1), question(qname, dns.TypeA)...),
			0x00, 0x00, 0x29, 0x04, 0xd0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0c,
			0x00, 0x08, 0x00, 0x08, 0x00, 0x01, 0x10, 0x00, 0xcb, 0x00, 0x71, 0x07),
		append(append(header(1, 0, 0, 1), question(qname, dns.TypeA)...),
			0x00, 0x00, 0x29, 0x04, 0xd0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x06,
			0x00, 0x08, 0x00, 0x09, 0x00, 0x01),
		// Two OPT records.
		append(append(header(1, 0, 0, 2), question(qname, dns.TypeA)...),
			0x00, 0x00, 0x29, 0x04, 0xd0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x29, 0x02, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00),
		// Counts that promise more than the packet holds.
		append(header(1, 7, 7, 7), question(qname, dns.TypeA)...),
		header(1, 0, 0, 0),
		// Trailing bytes after a complete query.
		append(append(header(1, 0, 0, 0), question(qname, dns.TypeA)...), 0xde, 0xad, 0xbe, 0xef),
	)

	// Truncations of a plain query and of an EDNS query with ECS.
	plain := mustPack(tb, fuzzQuery(app, dns.TypeA))
	withECS := mustPack(tb, withEDNS(fuzzQuery(app, dns.TypeA), 1232, false, ecsOption(1, 24, "203.0.113.0")))
	for _, wire := range [][]byte{plain, withECS} {
		for _, cut := range []int{0, 1, 11, 12, 13, len(wire) / 2, len(wire) - 5, len(wire) - 1} {
			if cut >= 0 && cut < len(wire) {
				corpus = append(corpus, wire[:cut])
			}
		}
	}

	return corpus
}

func describeInput(wire []byte) string {
	if serverWouldDispatch(wire) {
		return "the stock server passes this packet to the handler"
	}
	return "the stock server answers or drops this packet itself (DefaultMsgAcceptFunc), the handler is reached only if that filter is replaced"
}

// checkDNSResponse runs one query through one handler over one transport and
// asserts the invariants listed at the top of this file.
func checkDNSResponse(t *testing.T, h *gslbdns.Handler, wire []byte, tcp bool, label string) {
	t.Helper()

	// The handler gets its own copy of the message, like in the server, and
	// the response is compared with an independent decoding of the packet so
	// that a handler which rewrites its input cannot hide a mismatch.
	req := new(dns.Msg)
	if err := req.Unpack(wire); err != nil {
		return
	}
	want := new(dns.Msg)
	if err := want.Unpack(wire); err != nil {
		t.Fatalf("second unpack of the same packet failed: %v", err)
	}

	w := &fuzzResponseWriter{tcp: tcp}
	h.ServeDNS(w, req)

	if w.writes > 1 {
		t.Fatalf("%s: handler wrote %d responses to one query (%s)", label, w.writes, describeInput(wire))
	}
	if w.writes == 0 {
		return
	}
	if w.err != nil {
		t.Fatalf("%s: response does not pack: %v (%s)\nresponse:\n%v", label, w.err, describeInput(wire), w.msg)
	}

	resp := new(dns.Msg)
	if err := resp.Unpack(w.packed); err != nil {
		t.Fatalf("%s: packed response does not unpack: %v (%s)", label, err, describeInput(wire))
	}
	if resp.Id != want.Id {
		t.Fatalf("%s: response ID %#04x, query ID %#04x (%s)", label, resp.Id, want.Id, describeInput(wire))
	}
	if !resp.Response {
		t.Fatalf("%s: QR bit is not set in the response (%s)", label, describeInput(wire))
	}

	// The question section of the response is the query's question. With
	// one question (the only case the stock server dispatches) it must be
	// echoed exactly. With several, the response may repeat any leading part
	// of them but must not invent or alter one.
	if len(want.Question) == 1 && len(resp.Question) != 1 {
		t.Fatalf("%s: response has %d questions, the query had 1 (%s)", label, len(resp.Question), describeInput(wire))
	}
	if len(resp.Question) > len(want.Question) {
		t.Fatalf("%s: response has %d questions, the query had %d (%s)", label, len(resp.Question), len(want.Question), describeInput(wire))
	}
	for i, q := range resp.Question {
		if q != want.Question[i] {
			t.Fatalf("%s: question %d not echoed: got %q type %d class %d, want %q type %d class %d (%s)",
				label, i, q.Name, q.Qtype, q.Qclass,
				want.Question[i].Name, want.Question[i].Qtype, want.Question[i].Qclass, describeInput(wire))
		}
	}
}

// FuzzHandlerServeDNS is the fuzz target for the DNS handler entry point.
func FuzzHandlerServeDNS(f *testing.F) {
	// pkg/dns logs through the process default logger on the query path.
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1})))
	f.Cleanup(func() { slog.SetDefault(previous) })

	for _, seed := range dnsSeedCorpus(f) {
		f.Add(seed, false)
		f.Add(seed, true)
	}

	// One handler with ECS honored and one without. Both are reused for the
	// whole run: the only state they keep is router rotation.
	withECS := newFuzzHandler(f, true)
	withoutECS := newFuzzHandler(f, false)

	executions := 0
	f.Fuzz(func(t *testing.T, wire []byte, tcp bool) {
		checkDNSResponse(t, withECS, wire, tcp, "ecs enabled")
		checkDNSResponse(t, withoutECS, wire, tcp, "ecs disabled")

		// The handler labels its query metrics with the query name, so every
		// new name the fuzzer invents adds time series to the process. Drop
		// them now and then so a long fuzz run keeps a flat memory profile.
		if executions++; executions%512 == 0 {
			metrics.DNSQueriesTotal.Reset()
			metrics.DNSQueryDuration.Reset()
			metrics.RoutingDecisionsTotal.Reset()
		}
	})
}
