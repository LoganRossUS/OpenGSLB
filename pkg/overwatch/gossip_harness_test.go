// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	gslbdns "github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/miekg/dns"
)

// Shared harness for the gossip decoder tests: the regression tests in
// gossip_hardening_test.go and the fuzz targets in gossip_fuzz_test.go. It
// builds one Overwatch worth of state behind a GossipHandler and feeds it
// messages the way the memberlist receiver does.

const (
	fuzzServiceToken = "fuzz-service-token"
	fuzzAgentID      = "agent-fuzz-1"
	fuzzRegion       = "eu-west-1"
)

// fuzzDomains are the domains configured in the DNS registry used by the
// harness, one per routing algorithm, so that servers registered through
// gossip are then selected by every router.
var fuzzDomains = []struct {
	name      string
	algorithm string
}{
	{"weighted.fuzz.example", routing.AlgorithmWeighted},
	{"rr.fuzz.example", routing.AlgorithmRoundRobin},
	{"failover.fuzz.example", routing.AlgorithmFailover},
	{"geo.fuzz.example", routing.AlgorithmGeolocation},
	{"latency.fuzz.example", routing.AlgorithmLatency},
}

// fuzzDiscardLogger returns a logger that drops everything. slog.DiscardHandler
// needs Go 1.24, this works on every toolchain the module supports.
func fuzzDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// fuzzSilenceDefaultLogger replaces the process default logger for the
// duration of a test. pkg/dns logs through slog.Default().
func fuzzSilenceDefaultLogger(tb testing.TB) {
	tb.Helper()
	previous := slog.Default()
	slog.SetDefault(fuzzDiscardLogger())
	tb.Cleanup(func() { slog.SetDefault(previous) })
}

// fuzzDNSWriter is a dns.ResponseWriter that records what the handler writes.
type fuzzDNSWriter struct {
	writes int
	last   *dns.Msg
	packed []byte
	err    error
}

func (w *fuzzDNSWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(192, 0, 2, 53), Port: 53}
}

func (w *fuzzDNSWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 40000}
}

// WriteMsg packs the message like the real server does, so a response that
// cannot be put on the wire is reported.
func (w *fuzzDNSWriter) WriteMsg(m *dns.Msg) error {
	w.writes++
	w.last = m
	w.packed, w.err = m.Pack()
	return w.err
}

func (w *fuzzDNSWriter) Write(b []byte) (int, error) {
	w.writes++
	w.packed = append([]byte(nil), b...)
	return len(b), nil
}

func (w *fuzzDNSWriter) Close() error        { return nil }
func (w *fuzzDNSWriter) TsigStatus() error   { return nil }
func (w *fuzzDNSWriter) TsigTimersOnly(bool) {}
func (w *fuzzDNSWriter) Hijack()             {}

// fuzzGossipEnv is one Overwatch worth of state behind a GossipHandler: the
// backend registry with a store, the DNS registry and handler that serve
// gossip-registered servers, the learned latency table and TOFU agent auth.
type fuzzGossipEnv struct {
	handler      *GossipHandler
	registry     *Registry
	dnsRegistry  *gslbdns.Registry
	dnsHandler   *gslbdns.Handler
	latencyTable *LearnedLatencyTable
	store        *mockStore
}

func newFuzzGossipEnv(tb testing.TB) *fuzzGossipEnv {
	tb.Helper()
	logger := fuzzDiscardLogger()
	st := newMockStore()

	registry := NewRegistry(RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
		Logger:         logger,
	}, st)

	dnsRegistry := gslbdns.NewRegistry()
	for _, d := range fuzzDomains {
		router, err := routing.NewRouter(d.algorithm)
		if err != nil {
			tb.Fatalf("router %q: %v", d.algorithm, err)
		}
		dnsRegistry.Register(&gslbdns.DomainEntry{
			Name:             d.name,
			TTL:              30,
			RoutingAlgorithm: d.algorithm,
			Router:           router,
			Servers: []gslbdns.ServerInfo{
				{Address: net.IPv4(10, 0, 0, 1), Port: 80, Weight: 100, Region: fuzzRegion},
				{Address: net.ParseIP("2001:db8::1"), Port: 80, Weight: 100, Region: fuzzRegion},
			},
		})
	}

	// The backend registry is the health authority for the DNS handler, as it
	// is in the Overwatch (cmd/opengslb wraps it, the verdict comes from here).
	dnsHandler := gslbdns.NewHandler(gslbdns.HandlerConfig{
		Registry:       dnsRegistry,
		HealthProvider: registry,
		ECSEnabled:     true,
		DefaultTTL:     30,
		Logger:         logger,
	})

	// A small table so the eviction path is reached by a single message.
	latencyTable := NewLearnedLatencyTable(LearnedLatencyConfig{
		MaxEntries: 8,
		Logger:     logger,
	})

	handler := NewGossipHandler(registry, dnsRegistry, logger)
	handler.SetLatencyTable(latencyTable)
	handler.SetAuth(NewAgentAuth(&AgentAuthConfig{
		ServiceTokens: []string{fuzzServiceToken},
		Logger:        logger,
	}, st))
	// Start would set the context and spawn the channel reader. The fuzz
	// target calls handleMessage synchronously instead.
	handler.ctx, handler.cancel = context.WithCancel(context.Background())
	tb.Cleanup(handler.cancel)

	// State that exists before the message arrives, so update, drain and
	// deregister paths are reachable with one message.
	if err := registry.Register(fuzzAgentID, fuzzRegion, "weighted.fuzz.example", "10.0.0.1", 80, 100, true); err != nil {
		tb.Fatalf("pre-register agent backend: %v", err)
	}
	if err := registry.RegisterStatic("rr.fuzz.example", "10.0.0.1", 80, 100, fuzzRegion, "tcp"); err != nil {
		tb.Fatalf("pre-register static backend: %v", err)
	}

	return &fuzzGossipEnv{
		handler:      handler,
		registry:     registry,
		dnsRegistry:  dnsRegistry,
		dnsHandler:   dnsHandler,
		latencyTable: latencyTable,
		store:        st,
	}
}

// decodeLikeReceiver mirrors receiverDelegate.NotifyMsg in
// pkg/gossip/receiver.go, which is the only producer of the messages that
// reach handleMessage in production. pkg/gossip imports this package, so the
// delegate itself cannot be used here. FuzzReceiverNotifyMsg in pkg/gossip
// fails if the delegate stops decoding this way.
func decodeLikeReceiver(data []byte) (GossipMessage, bool) {
	if len(data) == 0 {
		return GossipMessage{}, false
	}
	var msg GossipMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return GossipMessage{}, false
	}
	return msg, true
}

// gossipSeed builds a message exactly the way pkg/gossip/sender.go does:
// a GossipMessage with a typed payload, encoded with encoding/json.
func gossipSeed(tb testing.TB, msgType GossipMessageType, payload any) []byte {
	tb.Helper()
	data, err := json.Marshal(GossipMessage{
		Type:      msgType,
		AgentID:   fuzzAgentID,
		Region:    fuzzRegion,
		Timestamp: time.Date(2026, 11, 20, 12, 0, 0, 123456789, time.UTC),
		Payload:   payload,
	})
	if err != nil {
		tb.Fatalf("encode seed %s: %v", msgType, err)
	}
	return data
}

// handleRaw decodes data like the memberlist receiver and hands it to the
// gossip handler.
func (e *fuzzGossipEnv) handleRaw(t *testing.T, data string) {
	t.Helper()
	msg, ok := decodeLikeReceiver([]byte(data))
	if !ok {
		t.Fatalf("receiver would drop the test message: %s", data)
	}
	e.handler.handleMessage(msg)
}

// queryNoPanic sends one DNS query through the handler and reports a panic as
// a test failure instead of taking the test binary down.
func (e *fuzzGossipEnv) queryNoPanic(t *testing.T, name string, qtype uint16) *dns.Msg {
	t.Helper()
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(name), qtype)
	w := &fuzzDNSWriter{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("DNS handler panicked answering %s %s: %v", name, dns.TypeToString[qtype], r)
			}
		}()
		e.dnsHandler.ServeDNS(w, q)
	}()
	if w.writes != 1 || w.last == nil {
		t.Fatalf("DNS handler wrote %d responses for %s, want 1", w.writes, name)
	}
	return w.last
}

func (e *fuzzGossipEnv) dnsServerCount(name string) int {
	entry := e.dnsRegistry.Lookup(dns.Fqdn(name))
	if entry == nil {
		return -1
	}
	return len(entry.Servers)
}
