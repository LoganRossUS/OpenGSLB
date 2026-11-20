// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package conformance

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	ogdns "github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/geo"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
)

// geoDatabase is the GeoIP fixture shared with the API contract tests. The
// geolocation cases only rely on the custom CIDR mappings of the test
// configuration, but the resolver cannot be created without a database.
const geoDatabase = "../apicontract/testdata/geo-fixture.mmdb"

// bigZoneNameservers is the size of the NS set of big.example.net. Its NS
// answer is larger than any UDP limit the server accepts by default, which
// makes it the truncation case.
const bigZoneNameservers = 40

// baseConfig is the configuration every conformance server starts from.
//
// Zones:
//   - gslb.example.com: plain string form, every SOA and NS value defaulted
//     from the node identity "conformance-1";
//   - corp.example.net: object form with every SOA field and the NS set given;
//   - big.example.net: object form with a large NS set (added by testConfig);
//   - standalone.example.org is a domain outside every configured zone and
//     is served as its own implicit zone.
//
// Names (the service of a server is the domain it belongs to):
//   - app.gslb.example.com: one IPv4 server;
//   - v6.gslb.example.com: one IPv6 server;
//   - dual.gslb.example.com: one IPv4 and one IPv6 server;
//   - down.gslb.example.com: one IPv4 server, unhealthy; no IPv6 server;
//   - api.eu.gslb.example.com: one IPv4 server; makes eu.gslb.example.com an
//     empty non-terminal;
//   - geo.gslb.example.com: geolocation routing, one server per region;
//   - www.corp.example.net and standalone.example.org: one IPv4 server.
const baseConfig = `
mode: overwatch

overwatch:
  identity:
    node_id: conformance-1
  gossip:
    # Validation requires a gossip key. The server under test never opens a
    # gossip listener; this is the 32-byte string "0123456789abcdef" twice.
    encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  dnssec:
    # Validation only accepts "enabled: false" together with an
    # acknowledgment text. The server under test is built without a signer
    # either way, as cmd/opengslb builds it today.
    enabled: true
  geolocation:
    database_path: "` + geoDatabase + `"
    default_region: eu
    ecs_enabled: true
    custom_mappings:
      - cidr: "203.0.113.0/24"
        region: us
      - cidr: "2001:db8:aa00::/40"
        region: us
      - cidr: "198.51.100.0/24"
        region: eu

dns:
  listen_address: "127.0.0.1:0"
  default_ttl: 60
  zones:
    - gslb.example.com
    - name: corp.example.net
      ttl: 7200
      soa:
        primary_ns: ns1.corp.example.net
        contact: dns.admin@example.net
        serial: 2026112001
        refresh: 7200
        retry: 900
        expire: 1209600
        negative_ttl: 300
      nameservers:
        - ns1.corp.example.net
        - ns2.corp.example.net

regions:
  - name: eu
    servers:
      - {address: "10.0.1.10", port: 80, weight: 100, service: app.gslb.example.com}
      - {address: "2001:db8::10", port: 80, weight: 100, service: v6.gslb.example.com}
      - {address: "10.0.2.10", port: 80, weight: 100, service: dual.gslb.example.com}
      - {address: "2001:db8::20", port: 80, weight: 100, service: dual.gslb.example.com}
      - {address: "10.0.3.10", port: 80, weight: 100, service: down.gslb.example.com}
      - {address: "10.0.4.10", port: 80, weight: 100, service: api.eu.gslb.example.com}
      - {address: "10.0.5.10", port: 80, weight: 100, service: geo.gslb.example.com}
      - {address: "10.0.6.10", port: 80, weight: 100, service: www.corp.example.net}
      - {address: "10.0.7.10", port: 80, weight: 100, service: standalone.example.org}
    health_check: {type: http, interval: 30s, timeout: 5s, path: /health}
  - name: us
    servers:
      - {address: "10.1.5.10", port: 80, weight: 100, service: geo.gslb.example.com}
    health_check: {type: http, interval: 30s, timeout: 5s, path: /health}

domains:
  - {name: app.gslb.example.com, routing_algorithm: round-robin, regions: [eu], ttl: 30}
  - {name: v6.gslb.example.com, routing_algorithm: round-robin, regions: [eu], ttl: 30}
  - {name: dual.gslb.example.com, routing_algorithm: round-robin, regions: [eu], ttl: 30}
  - {name: down.gslb.example.com, routing_algorithm: round-robin, regions: [eu], ttl: 30}
  - {name: api.eu.gslb.example.com, routing_algorithm: round-robin, regions: [eu], ttl: 30}
  - {name: geo.gslb.example.com, routing_algorithm: geolocation, regions: [eu, us], ttl: 30}
  - {name: www.corp.example.net, routing_algorithm: round-robin, regions: [eu]}
  - {name: standalone.example.org, routing_algorithm: round-robin, regions: [eu], ttl: 30}
`

// The SOA records the test configuration produces, in presentation format
// with single spaces, as they appear in the authority section of negative
// answers (TTL = min(SOA TTL, SOA MINIMUM), RFC 2308 section 3).
const (
	gslbNegativeSOA       = "gslb.example.com. 60 IN SOA conformance-1.gslb.example.com. hostmaster.gslb.example.com. 1 3600 600 604800 60"
	corpNegativeSOA       = `corp.example.net. 300 IN SOA ns1.corp.example.net. dns\.admin.example.net. 2026112001 7200 900 1209600 300`
	standaloneNegativeSOA = "standalone.example.org. 60 IN SOA conformance-1.standalone.example.org. hostmaster.standalone.example.org. 1 3600 600 604800 60"
)

// unhealthyAddresses are the servers the health provider reports as down.
var unhealthyAddresses = map[string]bool{"10.0.3.10": true}

// staticHealth is the health provider of the conformance server: every server
// is healthy except the listed addresses.
type staticHealth map[string]bool

func (h staticHealth) IsHealthy(address string, _ int) bool { return !h[address] }

// testConfig parses and validates baseConfig, adds the big.example.net zone
// and applies mutate, if any.
func testConfig(t *testing.T, mutate func(cfg *config.Config)) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(baseConfig))
	if err != nil {
		t.Fatalf("parse test configuration: %v", err)
	}

	big := config.Zone{Name: "big.example.net"}
	for i := 1; i <= bigZoneNameservers; i++ {
		big.Nameservers = append(big.Nameservers, fmt.Sprintf("ns%02d-authoritative-dns-conformance.big.example.net", i))
	}
	cfg.DNS.Zones = append(cfg.DNS.Zones, big)

	if mutate != nil {
		mutate(cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("test configuration is not valid: %v", err)
	}
	return cfg
}

// server is a running conformance server.
type server struct {
	addr string
}

// startServer builds the DNS server from the test configuration the way
// cmd/opengslb does (router factory, registry, zone table, handler, server)
// and runs it on a free loopback port until the test ends.
func startServer(t *testing.T, mutate func(cfg *config.Config)) *server {
	t.Helper()
	cfg := testConfig(t, mutate)
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	resolver, err := geo.NewResolver(geo.ResolverConfig{
		DatabasePath:   cfg.Overwatch.Geolocation.DatabasePath,
		DefaultRegion:  cfg.Overwatch.Geolocation.DefaultRegion,
		CustomMappings: cfg.Overwatch.Geolocation.CustomMappings,
		Regions:        cfg.Regions,
		Logger:         logger,
	})
	if err != nil {
		t.Fatalf("create geo resolver: %v", err)
	}
	t.Cleanup(func() { _ = resolver.Close() })

	factory := routing.NewFactory(routing.FactoryConfig{
		GeoResolver:   resolver,
		DefaultRegion: cfg.Overwatch.Geolocation.DefaultRegion,
		Logger:        logger,
	})
	registry, err := ogdns.BuildRegistry(cfg, factory.NewRouter)
	if err != nil {
		t.Fatalf("build DNS registry: %v", err)
	}
	zones, err := ogdns.BuildZones(cfg)
	if err != nil {
		t.Fatalf("build zone table: %v", err)
	}
	handler := ogdns.NewHandler(ogdns.HandlerConfig{
		Registry:          registry,
		Zones:             zones,
		HealthProvider:    staticHealth(unhealthyAddresses),
		DefaultTTL:        ttlSeconds(cfg.DNS.DefaultTTL),
		ECSEnabled:        cfg.Overwatch.Geolocation.ECSEnabled,
		MaxUDPSize:        cfg.DNS.MaxUDPSize,
		ReturnLastHealthy: cfg.DNS.ReturnLastHealthy,
		Logger:            logger,
	})

	// The server takes one address for UDP and TCP, so the port must be free
	// for both. Another process can take it between the probe and the bind;
	// in that case try again with another port.
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		addr, err := freeLoopbackAddr()
		if err != nil {
			lastErr = err
			continue
		}
		srv := ogdns.NewServer(ogdns.ServerConfig{Address: addr, Handler: handler, Logger: logger})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- srv.Start(ctx) }()

		if err := waitUntilServing(addr, done); err != nil {
			cancel()
			lastErr = err
			continue
		}
		t.Cleanup(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("DNS server did not stop within 10s")
			}
		})
		return &server{addr: addr}
	}
	t.Fatalf("could not start the DNS server: %v", lastErr)
	return nil
}

// ttlSeconds converts a configured TTL to the 32-bit value of a DNS record,
// limited to the range of a DNS TTL.
func ttlSeconds(seconds int) uint32 {
	if seconds < 0 {
		return 0
	}
	if seconds > math.MaxInt32 {
		return math.MaxInt32
	}
	return uint32(seconds)
}

// freeLoopbackAddr returns a 127.0.0.1 address whose port is free for both
// TCP and UDP at the time of the call.
func freeLoopbackAddr() (string, error) {
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer tcp.Close()
	addr := tcp.Addr().String()
	udp, err := net.ListenPacket("udp", addr)
	if err != nil {
		return "", err
	}
	defer udp.Close()
	return addr, nil
}

// waitUntilServing returns nil once the server answers over UDP and TCP, and
// an error when it exits or does not come up in time.
func waitUntilServing(addr string, done <-chan error) error {
	probe := new(dns.Msg)
	probe.SetQuestion("probe.invalid.", dns.TypeA)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			return fmt.Errorf("server exited: %v", err)
		default:
		}
		ready := true
		for _, network := range []string{"udp", "tcp"} {
			client := &dns.Client{Net: network, Timeout: 200 * time.Millisecond}
			if _, _, err := client.Exchange(probe, addr); err != nil {
				ready = false
			}
		}
		if ready {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("server on %s did not answer within 10s", addr)
}

// exchange sends q over the given transport and returns the response and its
// size on the wire in bytes (without the two-byte length prefix of TCP).
func (s *server) exchange(t *testing.T, network string, q *dns.Msg) (*dns.Msg, int) {
	t.Helper()
	conn, err := net.DialTimeout(network, s.addr, 5*time.Second)
	if err != nil {
		t.Fatalf("%s dial %s: %v", network, s.addr, err)
	}
	defer conn.Close()

	// The read buffer is larger than any size the queries advertise, so a
	// server that ignores the advertised size is caught and not cut short.
	dc := &dns.Conn{Conn: conn, UDPSize: dns.MaxMsgSize}
	if err := dc.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := dc.WriteMsg(q); err != nil {
		t.Fatalf("%s write: %v", network, err)
	}
	wire, err := dc.ReadMsgHeader(nil)
	if err != nil {
		t.Fatalf("%s read: %v", network, err)
	}
	resp := new(dns.Msg)
	if err := resp.Unpack(wire); err != nil {
		t.Fatalf("%s response does not parse: %v", network, err)
	}
	return resp, len(wire)
}

// newQuery builds a query the way a recursive resolver sends it to an
// authoritative server: RD clear. ednsSize 0 means no OPT record.
func newQuery(name string, qtype uint16, ednsSize uint16) *dns.Msg {
	q := new(dns.Msg)
	q.SetQuestion(name, qtype)
	q.RecursionDesired = false
	if ednsSize > 0 {
		q.SetEdns0(ednsSize, false)
	}
	return q
}

// rrStrings returns the records in presentation format with single spaces,
// sorted, without OPT records.
func rrStrings(rrs []dns.RR) []string {
	out := make([]string, 0, len(rrs))
	for _, rr := range rrs {
		if rr.Header().Rrtype == dns.TypeOPT {
			continue
		}
		out = append(out, strings.Join(strings.Fields(rr.String()), " "))
	}
	sort.Strings(out)
	return out
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

// sameStrings reports whether got and want hold the same strings in any
// order. want is not modified.
func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	for i := range got {
		if got[i] != sorted[i] {
			return false
		}
	}
	return true
}

// rcodeName returns the name of the response code of m, taking the extended
// rcode bits of the OPT record into account.
func rcodeName(rcode int) string {
	if rcode == dns.RcodeBadVers {
		return "BADVERS"
	}
	if name, ok := dns.RcodeToString[rcode]; ok {
		return name
	}
	return fmt.Sprintf("RCODE%d", rcode)
}

// checkEnvelope asserts what holds for every response of an
// authoritative-only server, whatever the question: it is a response to this
// query, it echoes the question exactly as asked, it does not offer
// recursion, and it has an OPT record if and only if the query had one.
func checkEnvelope(t *testing.T, q, resp *dns.Msg) {
	t.Helper()
	if !resp.Response {
		t.Error("QR bit is clear in the response")
	}
	if resp.Id != q.Id {
		t.Errorf("response ID = %d, want %d", resp.Id, q.Id)
	}
	if resp.Opcode != q.Opcode {
		t.Errorf("response opcode = %d, want %d", resp.Opcode, q.Opcode)
	}
	if resp.RecursionAvailable {
		t.Error("RA bit is set: an authoritative-only server does not offer recursion")
	}
	if resp.RecursionDesired != q.RecursionDesired {
		t.Errorf("RD bit = %v, want it copied from the query (%v)", resp.RecursionDesired, q.RecursionDesired)
	}
	if len(resp.Question) != len(q.Question) {
		t.Errorf("response has %d questions, want %d", len(resp.Question), len(q.Question))
	} else if len(q.Question) == 1 && resp.Question[0] != q.Question[0] {
		// The name is compared byte for byte: resolvers that randomize the
		// case of the query name (0x20) drop a response that changes it.
		t.Errorf("question = %v, want it echoed exactly as asked: %v", resp.Question[0], q.Question[0])
	}

	wantOPT := 0
	if q.IsEdns0() != nil {
		wantOPT = 1
	}
	opts := optRecords(resp)
	if len(opts) != wantOPT {
		t.Errorf("response has %d OPT records, want %d (RFC 6891: OPT in the response if and only if in the query)", len(opts), wantOPT)
	}
	for _, opt := range opts {
		if opt.Version() != 0 {
			t.Errorf("response EDNS version = %d, want 0", opt.Version())
		}
	}
	if extra := rrStrings(resp.Extra); len(extra) != 0 {
		t.Errorf("additional section holds records other than OPT: %v", extra)
	}
}
