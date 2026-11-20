// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"bytes"
	"encoding/json"
	"math"
	"net/netip"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/miekg/dns"
)

// Fuzz tests for the gossip message decoder (ADR-015 gossip input).
//
// Gossip messages reach GossipHandler.handleMessage from any member of the
// memberlist cluster, that is from any host that holds the gossip encryption
// key. The Overwatch does not trust agents, so a message must never be able to
// crash the process, corrupt the registries, or make a later DNS query crash.
//
// Run one target for a while with, for example:
//
//	go test -run='^$' -fuzz='^FuzzGossipHandleMessage$' -fuzztime=60s ./pkg/overwatch/
//
// Without -fuzz the seed corpus below (and testdata/fuzz) runs as a normal,
// fast unit test.

// checkAfterGossip asserts the invariants that must hold whatever a cluster
// member sent.
func (e *fuzzGossipEnv) checkAfterGossip(t *testing.T) {
	t.Helper()

	for _, b := range e.registry.GetAllBackends() {
		if _, err := json.Marshal(b); err != nil {
			t.Fatalf("backend %s/%s:%d cannot be persisted: %v", b.Service, b.Address, b.Port, err)
		}
		if b.Source != SourceAgent {
			continue
		}
		if b.Port < minGossipPort || b.Port > maxGossipPort {
			t.Fatalf("agent backend %q %s registered with port %d, outside %d..%d",
				b.Service, b.Address, b.Port, minGossipPort, maxGossipPort)
		}
		if b.Weight < 0 || b.Weight > MaxGossipWeight {
			t.Fatalf("agent backend %q %s:%d registered with weight %d, outside 0..%d",
				b.Service, b.Address, b.Port, b.Weight, MaxGossipWeight)
		}
	}

	// Servers that gossip put into the DNS registry are what the routers
	// compute with, so they are held to the same bounds.
	for _, d := range fuzzDomains {
		entry := e.dnsRegistry.Lookup(dns.Fqdn(d.name))
		if entry == nil {
			t.Fatalf("configured domain %s disappeared from the DNS registry", d.name)
		}
		for _, srv := range entry.Servers {
			if srv.Address == nil || srv.Port < minGossipPort || srv.Port > maxGossipPort || srv.Weight < 0 || srv.Weight > MaxGossipWeight {
				t.Fatalf("DNS registry holds server %v port %d weight %d for %s", srv.Address, srv.Port, srv.Weight, d.name)
			}
		}
	}

	if n := e.latencyTable.SubnetCount(); n > e.latencyTable.config.MaxEntries {
		t.Fatalf("latency table holds %d subnets, limit is %d", n, e.latencyTable.config.MaxEntries)
	}
	for _, entry := range e.latencyTable.GetAllEntries() {
		if _, err := netip.ParsePrefix(entry.Subnet); err != nil {
			t.Fatalf("latency table holds unparsable subnet %q: %v", entry.Subnet, err)
		}
	}
	client := netip.MustParseAddr("203.0.113.7")
	e.latencyTable.GetBestBackend(client, []string{"weighted.fuzz.example"})
	e.latencyTable.GetLatencyForBackend(client, "weighted.fuzz.example")
	e.latencyTable.GetLatencyForBackendInRegion(client, "weighted.fuzz.example", fuzzRegion)

	// Whatever gossip registered must not break DNS for the configured
	// domains: every router still selects among the registered servers, the
	// handler answers every query exactly once and the answer goes on the
	// wire.
	for _, d := range fuzzDomains {
		e.routeNoPanic(t, d.name)
		for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
			q := new(dns.Msg)
			q.SetQuestion(dns.Fqdn(d.name), qtype)
			w := &fuzzDNSWriter{}
			e.dnsHandler.ServeDNS(w, q)
			if w.writes != 1 {
				t.Fatalf("%s %s: handler wrote %d responses, want 1", d.name, dns.TypeToString[qtype], w.writes)
			}
			if w.err != nil {
				t.Fatalf("%s %s: response does not pack: %v", d.name, dns.TypeToString[qtype], w.err)
			}
		}
	}
}

// fuzzResetMetrics drops the per-backend and per-query series created by one
// execution. Each execution uses a fresh registry, so without this a long fuzz
// run would accumulate one series per backend the fuzzer ever invented.
func fuzzResetMetrics() {
	metrics.BackendHealthy.Reset()
	metrics.DNSQueriesTotal.Reset()
	metrics.DNSQueryDuration.Reset()
	metrics.RoutingDecisionsTotal.Reset()
}

// gossipSeedCorpus returns real encoded messages of every type the decoder
// accepts, then malformed variants and truncations of each.
func gossipSeedCorpus(tb testing.TB) [][]byte {
	tb.Helper()

	certPEM, _, err := generateTestCertificate(fuzzRegion, time.Hour)
	if err != nil {
		tb.Fatalf("generate certificate: %v", err)
	}
	lastSeen := time.Date(2026, 11, 20, 11, 59, 30, 0, time.UTC)

	valid := [][]byte{
		// Heartbeat with backends, as SendHealthUpdate encodes it.
		gossipSeed(tb, MessageHeartbeat, HeartbeatPayload{
			Backends: []BackendHeartbeat{
				{Service: "weighted.fuzz.example", Address: "10.0.0.1", Port: 80, Weight: 100, Healthy: true},
				{Service: "weighted.fuzz.example", Address: "10.0.0.2", Port: 8080, Weight: 50, Healthy: false},
				{Service: "rr.fuzz.example", Address: "2001:db8::2", Port: 443, Weight: 1, Healthy: true},
				{Service: "not-configured.example", Address: "10.9.9.9", Port: 80, Weight: 10, Healthy: true},
			},
		}),
		// Heartbeat with predictive state.
		gossipSeed(tb, MessageHeartbeat, HeartbeatPayload{
			Backends: []BackendHeartbeat{
				{Service: "failover.fuzz.example", Address: "10.0.0.3", Port: 80, Weight: 100, Healthy: true},
			},
			Fingerprint: "ab12cd34",
			Predictive: &PredictiveHeartbeat{
				Bleeding:    true,
				BleedReason: "cpu_threshold_exceeded",
				BleedingAt:  lastSeen,
				CPUPercent:  97.5,
				MemPercent:  41.25,
				ErrorRate:   0.5,
			},
		}),
		// Pure heartbeat, as SendHeartbeat encodes it ("backends":null).
		gossipSeed(tb, MessageHeartbeat, HeartbeatPayload{}),
		gossipSeed(tb, MessageRegister, RegisterPayload{
			Service: "geo.fuzz.example", Address: "10.0.0.4", Port: 8443, Weight: 200,
		}),
		gossipSeed(tb, MessageDeregister, DeregisterPayload{
			Service: "weighted.fuzz.example", Address: "10.0.0.1", Port: 80,
		}),
		gossipSeed(tb, MessageAgentAuth, AgentAuthPayload{
			CertificatePEM: certPEM,
			ServiceToken:   fuzzServiceToken,
			Fingerprint:    "ab12cd34",
		}),
		gossipSeed(tb, MessageAgentAuth, AgentAuthPayload{
			CertificatePEM: []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"),
			ServiceToken:   "wrong-token",
		}),
		gossipSeed(tb, MessageLatencyReport, LatencyReportPayload{
			Backend: "weighted.fuzz.example",
			Subnets: []SubnetLatencyData{
				{Subnet: "203.0.113.0/24", EWMA: int64(12 * time.Millisecond), SampleCount: 42, LastSeen: lastSeen},
				{Subnet: "2001:db8:1::/48", EWMA: int64(80 * time.Millisecond), SampleCount: 7, LastSeen: lastSeen},
				{Subnet: "198.51.100.0/24", EWMA: 1, SampleCount: 1, LastSeen: lastSeen},
			},
		}),
	}

	malformed := [][]byte{
		// Unknown type and payloads of the wrong JSON kind.
		gossipSeed(tb, GossipMessageType("no_such_type"), map[string]any{"x": 1}),
		gossipSeed(tb, MessageHeartbeat, nil),
		gossipSeed(tb, MessageHeartbeat, "a string"),
		gossipSeed(tb, MessageRegister, []any{1, 2, 3}),
		gossipSeed(tb, MessageDeregister, 42),
		gossipSeed(tb, MessageLatencyReport, map[string]any{"backend": 7, "subnets": "x"}),
		gossipSeed(tb, MessageAgentAuth, map[string]any{"certificate_pem": 1, "service_token": []any{}}),
		// Fields of the wrong kind inside otherwise valid payloads.
		gossipSeed(tb, MessageHeartbeat, map[string]any{
			"backends": []any{
				"not an object",
				map[string]any{"service": 1, "address": true, "port": "80", "weight": nil, "healthy": "yes"},
				map[string]any{"service": "weighted.fuzz.example", "address": "not-an-ip", "port": 80, "weight": 1},
			},
			"predictive": map[string]any{"bleeding": "yes", "bleeding_at": "yesterday", "cpu_percent": "high"},
		}),
		// Numbers outside what an agent can legitimately send. The two
		// weights below overflow int when summed by the weighted router.
		[]byte(`{"type":"heartbeat","agent_id":"agent-evil","region":"eu-west-1","timestamp":"2026-11-20T12:00:00Z","payload":{"backends":[` +
			`{"service":"weighted.fuzz.example","address":"10.6.6.1","port":80,"weight":5e18,"healthy":true},` +
			`{"service":"weighted.fuzz.example","address":"10.6.6.2","port":80,"weight":5e18,"healthy":true}]}}`),
		[]byte(`{"type":"register","agent_id":"agent-evil","region":"eu-west-1","payload":{"service":"rr.fuzz.example","address":"10.6.6.3","port":1e30,"weight":-1e30}}`),
		[]byte(`{"type":"register","agent_id":"agent-evil","region":"eu-west-1","payload":{"service":"rr.fuzz.example","address":"10.6.6.4","port":-7,"weight":0.5}}`),
		[]byte(`{"type":"heartbeat","agent_id":"agent-evil","payload":{"backends":[{"service":"latency.fuzz.example","address":"10.6.6.5","port":70000,"weight":2000000}]}}`),
		[]byte(`{"type":"latency_report","agent_id":"agent-evil","payload":{"backend":"weighted.fuzz.example","subnets":[` +
			`{"subnet":"203.0.113.9/24","ewma_ns":-9.3e18,"sample_count":-1,"last_seen":"0000-00-00T00:00:00Z"},` +
			`{"subnet":"0.0.0.0/0","ewma_ns":1e300,"sample_count":1e300},{"subnet":"::/0"},{"subnet":"10.1.0.0/16"},` +
			`{"subnet":"10.2.0.0/16"},{"subnet":"10.3.0.0/16"},{"subnet":"10.4.0.0/16"},{"subnet":"10.5.0.0/16"},` +
			`{"subnet":"10.6.0.0/16"},{"subnet":"10.7.0.0/16"},{"subnet":"10.8.0.0/16"},{"subnet":"bogus"}]}}`),
		// Not a message at all.
		[]byte(`null`),
		[]byte(`[]`),
		[]byte(`"heartbeat"`),
		[]byte(`{}`),
		[]byte(`{"type":123}`),
		[]byte(`{"type":"heartbeat","timestamp":"not a time"}`),
		[]byte(`{"type":"heartbeat","payload":{"backends":[[[[[[[[[[]]]]]]]]]]}}`),
		[]byte("{\"type\":\"heartbeat\",\"agent_id\":\"\xff\xfe\",\"payload\":{}}"),
		{0x00},
		{0xff, 0xfe, 0xfd},
	}

	corpus := append([][]byte{}, valid...)
	corpus = append(corpus, malformed...)
	// Truncated copies of every real message: cut in the envelope, in the
	// middle of the payload and one byte before the end.
	for _, msg := range valid {
		for _, cut := range []int{len(msg) / 4, len(msg) / 2, len(msg) - 1} {
			if cut > 0 && cut < len(msg) {
				corpus = append(corpus, msg[:cut])
			}
		}
	}
	return corpus
}

// FuzzGossipHandleMessage feeds raw gossip bytes through the same decoding
// the memberlist receiver applies and then through GossipHandler, against a
// real backend registry, DNS registry, latency table and agent auth. The same
// message is handled twice because heartbeats repeat.
func FuzzGossipHandleMessage(f *testing.F) {
	fuzzSilenceDefaultLogger(f)
	for _, seed := range gossipSeedCorpus(f) {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		msg, ok := decodeLikeReceiver(data)
		if !ok {
			return // the receiver drops it before the handler sees it
		}

		env := newFuzzGossipEnv(t)
		defer fuzzResetMetrics()

		env.handler.handleMessage(msg)
		env.handler.handleMessage(msg)
		env.checkAfterGossip(t)

		// The type is chosen by the sender. Only the types this build knows
		// may show up as a label of the received-messages counter.
		switch msg.Type {
		case MessageHeartbeat, MessageRegister, MessageDeregister, MessageAgentAuth, MessageLatencyReport, unknownGossipTypeLabel:
		default:
			if metrics.GossipMessagesReceivedTotal.DeleteLabelValues(string(msg.Type)) {
				t.Fatalf("message type %q became a label of gossip_messages_received_total", msg.Type)
			}
		}
	})
}

// roundTripLimit is the largest integer magnitude that survives the decoder
// exactly. The receiver decodes payloads into map[string]any, so every JSON
// number passes through float64 and integers above 2^53 lose precision. No
// field has a legitimate value that large (2^53 ns is 104 days of latency).
const roundTripLimit = 1 << 53

func clampInt(v int64) int64 {
	if v > roundTripLimit {
		return roundTripLimit
	}
	if v < -roundTripLimit {
		return -roundTripLimit
	}
	return v
}

// FuzzGossipPayloadRoundTrip is the differential half: for every message type
// it builds the typed payload an agent would send, encodes it the way the
// sender does, decodes it the way the receiver does, runs the handler's
// payload parser and requires the result to equal what was sent.
func FuzzGossipPayloadRoundTrip(f *testing.F) {
	f.Add(uint8(0), "weighted.fuzz.example", "10.0.0.1", "cpu_threshold_exceeded", []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"),
		int64(80), int64(100), int64(1700000000123456789), true, true, 97.5, 41.25, 0.5, uint64(42))
	f.Add(uint8(1), "rr.fuzz.example", "2001:db8::1", "", []byte(nil),
		int64(443), int64(1), int64(0), false, false, 0.0, 0.0, 0.0, uint64(0))
	f.Add(uint8(2), "geo.fuzz.example", "10.0.0.4", "", []byte{},
		int64(8443), int64(0), int64(-1), false, false, -1.5, 1e300, 5e-324, uint64(1))
	f.Add(uint8(3), "agent-1", "203.0.113.0/24", "token", []byte{0x00, 0xff, 0x10, 0x80},
		int64(65535), int64(1000000), int64(1), true, false, 100.0, 100.0, 1.0, uint64(1)<<53)
	f.Add(uint8(4), "weighted.fuzz.example", "2001:db8:1::/48", "", []byte("x"),
		int64(-1), int64(-1), int64(1763640000000000000), false, true, 0.1, 0.2, 0.3, uint64(7))

	h := NewGossipHandler(nil, nil, fuzzDiscardLogger())

	f.Fuzz(func(t *testing.T, kind uint8, name, address, text string, blob []byte,
		port, weight, nanos int64, healthy, bleeding bool, cpu, mem, errRate float64, samples uint64) {
		// encoding/json replaces invalid UTF-8 while encoding, which is the
		// sender's side of the wire and not what this target checks.
		if !utf8.ValidString(name) || !utf8.ValidString(address) || !utf8.ValidString(text) {
			t.Skip()
		}
		for _, v := range []float64{cpu, mem, errRate} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Skip() // not encodable as JSON, the sender fails before sending
			}
		}
		port, weight, nanos = clampInt(port), clampInt(weight), clampInt(nanos)
		if samples > roundTripLimit {
			samples = roundTripLimit
		}
		when := time.Unix(0, nanos).UTC()

		var msgType GossipMessageType
		var payload any
		switch kind % 5 {
		case 0:
			msgType = MessageHeartbeat
			p := HeartbeatPayload{
				Backends: []BackendHeartbeat{
					{Service: name, Address: address, Port: int(port), Weight: int(weight), Healthy: healthy},
					{Service: text, Address: address, Port: int(weight), Weight: int(port), Healthy: !healthy},
				},
				Fingerprint: text,
			}
			if bleeding || cpu != 0 {
				p.Predictive = &PredictiveHeartbeat{
					Bleeding: bleeding, BleedReason: text, BleedingAt: when,
					CPUPercent: cpu, MemPercent: mem, ErrorRate: errRate,
				}
			}
			payload = p
		case 1:
			msgType = MessageRegister
			payload = RegisterPayload{Service: name, Address: address, Port: int(port), Weight: int(weight)}
		case 2:
			msgType = MessageDeregister
			payload = DeregisterPayload{Service: name, Address: address, Port: int(port)}
		case 3:
			msgType = MessageAgentAuth
			payload = AgentAuthPayload{CertificatePEM: blob, ServiceToken: text, Fingerprint: name}
		case 4:
			msgType = MessageLatencyReport
			payload = LatencyReportPayload{
				Backend: name,
				Subnets: []SubnetLatencyData{
					{Subnet: address, EWMA: nanos, SampleCount: samples, LastSeen: when},
					{Subnet: text, EWMA: port, SampleCount: 0, LastSeen: when.Add(time.Second)},
				},
			}
		}

		wire, err := json.Marshal(GossipMessage{
			Type: msgType, AgentID: name, Region: text, Timestamp: when, Payload: payload,
		})
		if err != nil {
			t.Fatalf("sender-side encoding failed: %v", err)
		}

		got, ok := decodeLikeReceiver(wire)
		if !ok {
			t.Fatalf("receiver rejects a message the sender produced: %s", wire)
		}
		if got.Type != msgType || got.AgentID != name || got.Region != text || !got.Timestamp.Equal(when) {
			t.Fatalf("envelope changed in transit: sent %s, got %+v", wire, got)
		}
		m, ok := got.Payload.(map[string]any)
		if !ok {
			t.Fatalf("payload decoded as %T, the handler only accepts objects: %s", got.Payload, wire)
		}

		switch want := payload.(type) {
		case HeartbeatPayload:
			have := h.parseHeartbeatPayload(m)
			if have.Fingerprint != want.Fingerprint || len(have.Backends) != len(want.Backends) {
				t.Fatalf("heartbeat changed in transit: sent %+v, parsed %+v", want, have)
			}
			for i := range want.Backends {
				if have.Backends[i] != want.Backends[i] {
					t.Fatalf("heartbeat backend %d changed in transit: sent %+v, parsed %+v", i, want.Backends[i], have.Backends[i])
				}
			}
			if (have.Predictive == nil) != (want.Predictive == nil) {
				t.Fatalf("predictive presence changed in transit: sent %+v, parsed %+v", want.Predictive, have.Predictive)
			}
			if want.Predictive != nil {
				hp, wp := *have.Predictive, *want.Predictive
				if !hp.BleedingAt.Equal(wp.BleedingAt) {
					t.Fatalf("bleeding_at changed in transit: sent %v, parsed %v", wp.BleedingAt, hp.BleedingAt)
				}
				hp.BleedingAt, wp.BleedingAt = time.Time{}, time.Time{}
				if hp != wp {
					t.Fatalf("predictive state changed in transit: sent %+v, parsed %+v", wp, hp)
				}
			}
		case RegisterPayload:
			if have := h.parseRegisterPayload(m); have != want {
				t.Fatalf("register changed in transit: sent %+v, parsed %+v", want, have)
			}
		case DeregisterPayload:
			if have := h.parseDeregisterPayload(m); have != want {
				t.Fatalf("deregister changed in transit: sent %+v, parsed %+v", want, have)
			}
		case AgentAuthPayload:
			have := h.parseAgentAuthPayload(m)
			if !bytes.Equal(have.CertificatePEM, want.CertificatePEM) {
				t.Fatalf("certificate_pem changed in transit: sent %q, parsed %q", want.CertificatePEM, have.CertificatePEM)
			}
			if have.ServiceToken != want.ServiceToken || have.Fingerprint != want.Fingerprint {
				t.Fatalf("agent auth changed in transit: sent %+v, parsed %+v", want, have)
			}
		case LatencyReportPayload:
			have := h.parseLatencyReportPayload(m)
			if have.Backend != want.Backend || len(have.Subnets) != len(want.Subnets) {
				t.Fatalf("latency report changed in transit: sent %+v, parsed %+v", want, have)
			}
			for i := range want.Subnets {
				hs, ws := have.Subnets[i], want.Subnets[i]
				if !hs.LastSeen.Equal(ws.LastSeen) {
					t.Fatalf("subnet %d last_seen changed in transit: sent %v, parsed %v", i, ws.LastSeen, hs.LastSeen)
				}
				hs.LastSeen, ws.LastSeen = time.Time{}, time.Time{}
				if hs != ws {
					t.Fatalf("subnet %d changed in transit: sent %+v, parsed %+v", i, ws, hs)
				}
			}
		}
	})
}
