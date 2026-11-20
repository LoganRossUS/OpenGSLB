// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"fmt"
	"testing"

	"github.com/miekg/dns"
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

	for i := 0; i < 20; i++ {
		resp := env.queryNoPanic(t, "weighted.fuzz.example", dns.TypeA)
		if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
			t.Fatalf("query %d: rcode %s with %d answers, want NOERROR with 1", i, dns.RcodeToString[resp.Rcode], len(resp.Answer))
		}
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
