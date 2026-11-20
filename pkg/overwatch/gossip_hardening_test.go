// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

// Regression tests for defects in the gossip decoder that were found while
// writing the fuzz targets in gossip_fuzz_test.go. Every message here is raw
// JSON taken through the receiver's decoding, as it arrives from the network.

// A heartbeat that registers two backends whose weights sum past the int
// range made the weighted router call rand.Intn with a negative bound, which
// panics. The panic happens on the DNS path, in a goroutine of the DNS
// server, so one gossip message took the whole Overwatch down on the next
// query for that domain.
func TestGossipHeartbeat_HugeWeightsDoNotCrashDNS(t *testing.T) {
	fuzzSilenceDefaultLogger(t)
	env := newFuzzGossipEnv(t)
	before := env.dnsServerCount("weighted.fuzz.example")

	env.handleRaw(t, `{"type":"heartbeat","agent_id":"agent-evil","region":"eu-west-1","timestamp":"2026-11-20T12:00:00Z","payload":{"backends":[`+
		`{"service":"weighted.fuzz.example","address":"10.6.6.1","port":80,"weight":5e18,"healthy":true},`+
		`{"service":"weighted.fuzz.example","address":"10.6.6.2","port":80,"weight":5e18,"healthy":true}]}}`)

	// The weighted router picks among the servers of the domain on every
	// query. Ask it directly, then through the DNS handler. No RCODE is
	// asserted for the handler: what it answers is its own business, that it
	// survives the query is the point.
	for i := 0; i < 20; i++ {
		if _, ok := env.routeNoPanic(t, "weighted.fuzz.example"); !ok {
			t.Fatalf("route %d: the weighted router has no server to select", i)
		}
		env.queryNoPanic(t, "weighted.fuzz.example", dns.TypeA)
		env.queryNoPanic(t, "weighted.fuzz.example", dns.TypeAAAA)
	}

	if got := env.dnsServerCount("weighted.fuzz.example"); got != before {
		t.Errorf("DNS registry has %d servers for the domain, want %d (out-of-range backends must not be added)", got, before)
	}
	for _, addr := range []string{"10.6.6.1", "10.6.6.2"} {
		if _, ok := env.registry.GetBackend("weighted.fuzz.example", addr, 80); ok {
			t.Errorf("backend %s with weight 5e18 was registered", addr)
		}
	}
}

func TestGossip_RejectsOutOfRangePortAndWeight(t *testing.T) {
	fuzzSilenceDefaultLogger(t)

	cases := []struct {
		name     string
		port     string
		weight   string
		accepted bool
		wantPort int
		wantWt   int
	}{
		{"lowest port", "1", "100", true, 1, 100},
		{"highest port", "65535", "100", true, 65535, 100},
		{"weight zero", "80", "0", true, 80, 0},
		{"highest weight", "80", fmt.Sprint(MaxGossipWeight), true, 80, MaxGossipWeight},
		{"port zero", "0", "100", false, 0, 0},
		{"port negative", "-1", "100", false, 0, 0},
		{"port above range", "65536", "100", false, 0, 0},
		{"port beyond int64", "1e30", "100", false, 0, 0},
		{"port below int64", "-1e30", "100", false, 0, 0},
		{"weight negative", "80", "-1", false, 0, 0},
		{"weight above limit", "80", fmt.Sprint(MaxGossipWeight + 1), false, 0, 0},
		{"weight beyond int64", "80", "1e30", false, 0, 0},
		{"weight below int64", "80", "-1e30", false, 0, 0},
	}

	for _, tc := range cases {
		for _, msgType := range []string{"register", "heartbeat"} {
			t.Run(msgType+"/"+tc.name, func(t *testing.T) {
				env := newFuzzGossipEnv(t)
				before := env.dnsServerCount("rr.fuzz.example")

				backend := fmt.Sprintf(`{"service":"rr.fuzz.example","address":"10.7.7.7","port":%s,"weight":%s,"healthy":true}`, tc.port, tc.weight)
				payload := backend
				if msgType == "heartbeat" {
					payload = `{"backends":[` + backend + `]}`
				}
				env.handleRaw(t, fmt.Sprintf(`{"type":%q,"agent_id":"agent-2","region":"eu-west-1","payload":%s}`, msgType, payload))

				var found *Backend
				for _, b := range env.registry.GetBackends("rr.fuzz.example") {
					if b.Address == "10.7.7.7" {
						found = b
					}
				}
				if tc.accepted {
					if found == nil {
						t.Fatalf("valid backend (port %s, weight %s) was rejected", tc.port, tc.weight)
					}
					if found.Port != tc.wantPort || found.Weight != tc.wantWt {
						t.Errorf("registered port %d weight %d, want port %d weight %d", found.Port, found.Weight, tc.wantPort, tc.wantWt)
					}
				} else if found != nil {
					t.Errorf("backend with port %s weight %s was registered as port %d weight %d", tc.port, tc.weight, found.Port, found.Weight)
				}

				// Only heartbeats feed the DNS registry.
				wantServers := before
				if tc.accepted && msgType == "heartbeat" {
					wantServers++
				}
				if got := env.dnsServerCount("rr.fuzz.example"); got != wantServers {
					t.Errorf("DNS registry has %d servers, want %d", got, wantServers)
				}
			})
		}
	}
}

// An invalid backend must not take the rest of the heartbeat with it.
func TestGossipHeartbeat_InvalidBackendDoesNotDropValidOnes(t *testing.T) {
	fuzzSilenceDefaultLogger(t)
	env := newFuzzGossipEnv(t)

	env.handleRaw(t, `{"type":"heartbeat","agent_id":"agent-2","region":"eu-west-1","payload":{"backends":[`+
		`{"service":"rr.fuzz.example","address":"10.7.7.1","port":70000,"weight":100,"healthy":true},`+
		`{"service":"rr.fuzz.example","address":"10.7.7.2","port":8080,"weight":100,"healthy":true}],`+
		`"predictive":{"bleeding":true,"bleed_reason":"cpu_threshold_exceeded","cpu_percent":99}}}`)

	if _, ok := env.registry.GetBackend("rr.fuzz.example", "10.7.7.1", 70000); ok {
		t.Error("backend with port 70000 was registered")
	}
	b, ok := env.registry.GetBackend("rr.fuzz.example", "10.7.7.2", 8080)
	if !ok {
		t.Fatal("valid backend in the same heartbeat was not registered")
	}
	if !b.Draining {
		t.Error("predictive state of the heartbeat was not applied to the valid backend")
	}
}

func counterSeries(t *testing.T, c prometheus.Collector) int {
	t.Helper()
	ch := make(chan prometheus.Metric, 4096)
	c.Collect(ch)
	close(ch)
	n := 0
	for range ch {
		n++
	}
	return n
}

// The message type is sender-controlled. Using it as a metric label verbatim
// let a cluster member create one time series per message, without bound.
func TestGossip_UnknownMessageTypesShareOneMetricSeries(t *testing.T) {
	fuzzSilenceDefaultLogger(t)
	env := newFuzzGossipEnv(t)
	metrics.GossipMessagesReceivedTotal.Reset()
	t.Cleanup(metrics.GossipMessagesReceivedTotal.Reset)

	for i := 0; i < 200; i++ {
		env.handleRaw(t, fmt.Sprintf(`{"type":"made-up-type-%d","agent_id":"agent-evil","payload":{}}`, i))
	}
	env.handleRaw(t, `{"type":"heartbeat","agent_id":"agent-2","payload":{}}`)
	env.handleRaw(t, `{"type":"deregister","agent_id":"agent-2","payload":{"service":"x","address":"10.0.0.9","port":80}}`)

	if got := counterSeries(t, metrics.GossipMessagesReceivedTotal); got != 3 {
		t.Errorf("gossip_messages_received_total has %d series after 200 unknown types, heartbeat and deregister, want 3", got)
	}
}

// encoding/json writes a []byte field as base64. The decoder used the base64
// text as if it were the PEM, so no certificate sent by SendAgentAuth could
// ever be parsed.
func TestGossipAgentAuth_CertificateSurvivesTheWire(t *testing.T) {
	fuzzSilenceDefaultLogger(t)
	env := newFuzzGossipEnv(t)

	certPEM, _, err := generateTestCertificate(fuzzRegion, time.Hour)
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}

	// Exactly what MemberlistSender.SendAgentAuth puts on the wire.
	wire := gossipSeed(t, MessageAgentAuth, AgentAuthPayload{
		CertificatePEM: certPEM,
		ServiceToken:   fuzzServiceToken,
	})
	env.handleRaw(t, string(wire))

	pinned, err := env.handler.auth.GetPinnedCertificate(context.Background(), fuzzAgentID)
	if err != nil {
		t.Fatalf("agent was not pinned after a valid agent_auth message: %v", err)
	}
	if string(pinned.CertificatePEM) != string(certPEM) {
		t.Errorf("pinned certificate differs from the one sent")
	}
}

// A sender that puts the PEM text itself in the JSON string keeps working.
func TestGossipAgentAuth_AcceptsPlainPEMString(t *testing.T) {
	fuzzSilenceDefaultLogger(t)
	env := newFuzzGossipEnv(t)

	certPEM, _, err := generateTestCertificate(fuzzRegion, time.Hour)
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}
	wire := gossipSeed(t, MessageAgentAuth, map[string]any{
		"certificate_pem": string(certPEM),
		"service_token":   fuzzServiceToken,
	})
	env.handleRaw(t, string(wire))

	if _, err := env.handler.auth.GetPinnedCertificate(context.Background(), fuzzAgentID); err != nil {
		t.Fatalf("agent was not pinned after an agent_auth message with a plain PEM string: %v", err)
	}
}

// When the learned latency table was full, every new subnet in a latency
// report scanned the whole table to evict one subnet. One report with n new
// subnets cost n full scans under the table's write lock, on the only
// goroutine that processes gossip. With the default limit of 100000 subnets a
// single message held the Overwatch's gossip processing for many seconds.
//
// The test counts the scans instead of measuring time, so it gives the same
// result on a loaded machine and under the race detector. Before the fix the
// report below made 2000 scans of a 20000 subnet table (40 million entry
// visits), it now makes 10.
func TestGossipLatencyReport_FullTableDoesNotStall(t *testing.T) {
	fuzzSilenceDefaultLogger(t)
	env := newFuzzGossipEnv(t)

	const (
		maxEntries = 20000
		newSubnets = 2000
		batch      = maxEntries / evictionBatchDivisor
		// One scan frees room for batch new subnets.
		maxScans = (newSubnets + batch - 1) / batch
	)
	table := NewLearnedLatencyTable(LearnedLatencyConfig{MaxEntries: maxEntries, Logger: fuzzDiscardLogger()})
	env.handler.SetLatencyTable(table)

	subnet := func(i int) string {
		return fmt.Sprintf("%d.%d.%d.0/24", 1+(i>>16)&0xff, (i>>8)&0xff, i&0xff)
	}
	existing := make([]SubnetLatencyData, 0, maxEntries)
	for i := 0; i < maxEntries; i++ {
		existing = append(existing, SubnetLatencyData{Subnet: subnet(i), EWMA: int64(time.Millisecond), SampleCount: 10})
	}
	table.Update("agent-1", fuzzRegion, "weighted.fuzz.example", existing)
	if got := table.SubnetCount(); got != maxEntries {
		t.Fatalf("table holds %d subnets after filling, want %d", got, maxEntries)
	}
	// The entries of the report must be newer than the ones already stored,
	// whatever the resolution of the clock. Age the stored ones.
	table.mu.Lock()
	if table.evictionScans != 0 {
		t.Errorf("filling the table up to its limit made %d eviction scans, want 0", table.evictionScans)
	}
	past := time.Now().Add(-time.Minute)
	for _, backendMap := range table.data {
		for _, entry := range backendMap {
			entry.LastUpdated = past
		}
	}
	table.mu.Unlock()

	var report strings.Builder
	report.WriteString(`{"type":"latency_report","agent_id":"agent-2","region":"eu-west-1","payload":{"backend":"weighted.fuzz.example","subnets":[`)
	for i := 0; i < newSubnets; i++ {
		if i > 0 {
			report.WriteByte(',')
		}
		fmt.Fprintf(&report, `{"subnet":%q,"ewma_ns":2000000,"sample_count":10}`, subnet(maxEntries+i))
	}
	report.WriteString(`]}}`)

	env.handleRaw(t, report.String())

	table.mu.RLock()
	scans := table.evictionScans
	table.mu.RUnlock()
	if scans > maxScans {
		t.Errorf("one latency report with %d new subnets made %d full scans of a table of %d subnets, want at most %d",
			newSubnets, scans, maxEntries, maxScans)
	}
	if got := table.SubnetCount(); got > maxEntries {
		t.Errorf("table holds %d subnets, limit is %d", got, maxEntries)
	}
	// Eviction removes the oldest subnets, never the ones just reported.
	for _, i := range []int{0, newSubnets / 2, newSubnets - 1} {
		prefix := netip.MustParsePrefix(subnet(maxEntries + i))
		if _, ok := table.GetLatencyForBackendInRegion(prefix.Addr(), "weighted.fuzz.example", fuzzRegion); !ok {
			t.Errorf("subnet %s from the report is missing from the table", prefix)
		}
	}
	// Eviction must not empty the table either.
	if got := table.SubnetCount(); got < maxEntries*9/10 {
		t.Errorf("table holds %d subnets after the report, eviction removed far more than needed", got)
	}
}

// Eviction frees a batch per pass. It must still take the oldest subnets and
// leave the table within its limit.
func TestLearnedLatencyTable_EvictsOldestSubnetsFirst(t *testing.T) {
	for _, maxEntries := range []int{4, 300} {
		t.Run(fmt.Sprintf("max_%d", maxEntries), func(t *testing.T) {
			table := NewLearnedLatencyTable(LearnedLatencyConfig{MaxEntries: maxEntries, Logger: fuzzDiscardLogger()})
			batch := maxEntries / evictionBatchDivisor
			if batch < 1 {
				batch = 1
			}
			subnet := func(i int) string { return fmt.Sprintf("10.%d.%d.0/24", i>>8, i&0xff) }
			has := func(i int) bool {
				_, ok := table.GetLatencyForBackend(netip.MustParsePrefix(subnet(i)).Addr(), "svc")
				return ok
			}

			for i := 0; i < maxEntries; i++ {
				table.Update("agent-1", fuzzRegion, "svc", []SubnetLatencyData{{Subnet: subnet(i), EWMA: 1, SampleCount: 1}})
			}
			if got := table.SubnetCount(); got != maxEntries {
				t.Fatalf("table holds %d subnets after filling, want %d", got, maxEntries)
			}
			// Give every subnet its own age, oldest first, without relying
			// on the clock moving between two updates.
			base := time.Now().Add(-time.Hour)
			table.mu.Lock()
			for i := 0; i < maxEntries; i++ {
				for _, entry := range table.data[netip.MustParsePrefix(subnet(i))] {
					entry.LastUpdated = base.Add(time.Duration(i) * time.Second)
				}
			}
			table.mu.Unlock()

			table.Update("agent-1", fuzzRegion, "svc", []SubnetLatencyData{{Subnet: subnet(maxEntries), EWMA: 1, SampleCount: 1}})

			if got, want := table.SubnetCount(), maxEntries-batch+1; got != want {
				t.Errorf("table holds %d subnets after one insert into a full table, want %d", got, want)
			}
			for i := 0; i < batch; i++ {
				if has(i) {
					t.Errorf("subnet %s is one of the %d oldest and was not evicted", subnet(i), batch)
				}
			}
			for _, i := range []int{batch, maxEntries - 1, maxEntries} {
				if !has(i) {
					t.Errorf("subnet %s is newer than the evicted batch and is missing", subnet(i))
				}
			}
		})
	}
}
