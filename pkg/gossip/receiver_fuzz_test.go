// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package gossip

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// Fuzz tests for the memberlist delegate of the Overwatch gossip receiver.
// memberlist hands the delegate bytes that another cluster member sent, so
// every callback here is reachable from the network by any host that holds the
// gossip encryption key.
//
// Run one target for a while with, for example:
//
//	go test -run='^$' -fuzz='^FuzzReceiverNotifyMsg$' -fuzztime=60s ./pkg/gossip/
//
// Without -fuzz the seed corpus runs as a normal, fast unit test. What the
// Overwatch then does with a decoded message is fuzzed in pkg/overwatch
// (FuzzGossipHandleMessage).

func fuzzDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// fuzzSeedMessage encodes a message exactly the way MemberlistSender.broadcast
// does: a GossipMessage with a typed payload through encoding/json.
func fuzzSeedMessage(tb testing.TB, msgType overwatch.GossipMessageType, payload any) []byte {
	tb.Helper()
	data, err := json.Marshal(overwatch.GossipMessage{
		Type:      msgType,
		AgentID:   "agent-fuzz-1",
		Region:    "eu-west-1",
		Timestamp: time.Date(2026, 11, 20, 12, 0, 0, 123456789, time.UTC),
		Payload:   payload,
	})
	if err != nil {
		tb.Fatalf("encode seed %s: %v", msgType, err)
	}
	return data
}

// fuzzSeedMessages returns one real encoded message for every Send method of
// the sender, followed by malformed input and truncations of each message.
func fuzzSeedMessages(tb testing.TB) [][]byte {
	tb.Helper()
	lastSeen := time.Date(2026, 11, 20, 11, 59, 30, 0, time.UTC)

	valid := [][]byte{
		// SendHealthUpdate
		fuzzSeedMessage(tb, overwatch.MessageHeartbeat, overwatch.HeartbeatPayload{
			Backends: []overwatch.BackendHeartbeat{
				{Service: "app.example.com", Address: "10.0.0.1", Port: 80, Weight: 100, Healthy: true},
				{Service: "app.example.com", Address: "2001:db8::1", Port: 8080, Weight: 50, Healthy: false},
			},
			Predictive: &overwatch.PredictiveHeartbeat{
				Bleeding:    true,
				BleedReason: "memory_threshold_exceeded",
				BleedingAt:  lastSeen,
				CPUPercent:  12.5,
				MemPercent:  93,
				ErrorRate:   0.01,
			},
		}),
		// SendHeartbeat
		fuzzSeedMessage(tb, overwatch.MessageHeartbeat, overwatch.HeartbeatPayload{}),
		// SendRegister
		fuzzSeedMessage(tb, overwatch.MessageRegister, overwatch.RegisterPayload{
			Service: "api.example.com", Address: "10.0.0.2", Port: 8443, Weight: 200,
		}),
		// SendDeregister
		fuzzSeedMessage(tb, overwatch.MessageDeregister, overwatch.DeregisterPayload{
			Service: "api.example.com", Address: "10.0.0.2", Port: 8443,
		}),
		// SendAgentAuth
		fuzzSeedMessage(tb, overwatch.MessageAgentAuth, overwatch.AgentAuthPayload{
			CertificatePEM: []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"),
			ServiceToken:   "token",
			Fingerprint:    "ab12cd34",
		}),
		// SendLatencyReport
		fuzzSeedMessage(tb, overwatch.MessageLatencyReport, overwatch.LatencyReportPayload{
			Backend: "app.example.com",
			Subnets: []overwatch.SubnetLatencyData{
				{Subnet: "203.0.113.0/24", EWMA: int64(12 * time.Millisecond), SampleCount: 42, LastSeen: lastSeen},
				{Subnet: "2001:db8:1::/48", EWMA: int64(80 * time.Millisecond), SampleCount: 7, LastSeen: lastSeen},
			},
		}),
	}

	corpus := append([][]byte{}, valid...)
	corpus = append(corpus,
		fuzzSeedMessage(tb, overwatch.GossipMessageType("no_such_type"), map[string]any{"x": 1}),
		fuzzSeedMessage(tb, overwatch.MessageHeartbeat, nil),
		fuzzSeedMessage(tb, overwatch.MessageRegister, []any{1, "two", 3.5}),
		[]byte(`null`),
		[]byte(`[]`),
		[]byte(`{}`),
		[]byte(`"heartbeat"`),
		[]byte(`{"type":123}`),
		[]byte(`{"type":"heartbeat","timestamp":"not a time"}`),
		[]byte(`{"type":"heartbeat","timestamp":"2026-11-20T12:00:00+05:30","payload":1e400}`),
		[]byte(`{"type":"heartbeat","payload":{"backends":[[[[[[[[[[]]]]]]]]]]}}`),
		[]byte("{\"type\":\"heartbeat\",\"agent_id\":\"\xff\xfe\"}"),
		[]byte(`{"type":"heartbeat"} trailing`),
		[]byte{},
		[]byte{0x00},
		[]byte{0xff, 0xfe, 0xfd},
	)
	for _, msg := range valid {
		for _, cut := range []int{len(msg) / 4, len(msg) / 2, len(msg) - 1} {
			if cut > 0 && cut < len(msg) {
				corpus = append(corpus, msg[:cut])
			}
		}
	}
	return corpus
}

// FuzzReceiverNotifyMsg feeds arbitrary bytes to receiverDelegate.NotifyMsg,
// the decoder for every user message an Overwatch receives over gossip.
//
// Invariants: no panic; NotifyMsg never blocks, also when the channel to the
// handler is full; input is delivered if and only if it is a JSON encoded
// overwatch.GossipMessage; what is delivered is exactly that message.
func FuzzReceiverNotifyMsg(f *testing.F) {
	for _, seed := range fuzzSeedMessages(f) {
		f.Add(seed)
	}
	logger := fuzzDiscardLogger()

	f.Fuzz(func(t *testing.T, data []byte) {
		ch := make(chan overwatch.GossipMessage, 1)
		d := &receiverDelegate{msgChan: ch, logger: logger}

		// The reference decoding. pkg/overwatch's FuzzGossipHandleMessage
		// decodes the same way, this comparison keeps the two in step.
		var want overwatch.GossipMessage
		wantDelivered := len(data) > 0 && json.Unmarshal(data, &want) == nil

		d.NotifyMsg(data)

		select {
		case got := <-ch:
			if !wantDelivered {
				t.Fatalf("delivered a message for input that is not a gossip message: %q", data)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("delivered %+v, want %+v", got, want)
			}
			if _, err := json.Marshal(got); err != nil {
				t.Fatalf("delivered message cannot be encoded again: %v", err)
			}
		default:
			if wantDelivered {
				t.Fatalf("dropped a valid message although the channel had room: %q", data)
			}
		}

		// With the channel full the delegate must drop, not block: it runs on
		// memberlist's packet handling goroutine.
		d.NotifyMsg(data)
		d.NotifyMsg(data)
		if len(ch) > 1 {
			t.Fatalf("channel holds %d messages, capacity is 1", len(ch))
		}
	})
}

// FuzzReceiverNodeEvents covers the other delegate callbacks that take data
// chosen by a remote node: its name, address and metadata on join, update and
// leave, and the push/pull state buffer.
func FuzzReceiverNodeEvents(f *testing.F) {
	meta := func(m NodeMeta) []byte {
		data, err := json.Marshal(m)
		if err != nil {
			f.Fatalf("encode node meta: %v", err)
		}
		return data
	}
	agentMeta := meta(NodeMeta{Role: RoleAgent, Region: "eu-west-1", Version: "1.0.0", Timestamp: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), AgentID: "agent-1"})
	overwatchMeta := meta(NodeMeta{Role: RoleOverwatch, Version: "1.0.0", Timestamp: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), NodeID: "ow-1"})

	f.Add("agent-1", []byte{10, 0, 0, 1}, agentMeta, 512)
	f.Add("ow-1", []byte(net.ParseIP("2001:db8::1")), overwatchMeta, 512)
	f.Add("agent-1", []byte{10, 0, 0, 1}, agentMeta[:len(agentMeta)/2], 10)
	f.Add("", []byte{}, []byte{}, 0)
	f.Add("x", []byte{1, 2, 3}, []byte(`{"role":42,"timestamp":"never"}`), -1)
	f.Add("x\x00\xff", []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}, []byte(`null`), 1<<31-1)

	logger := fuzzDiscardLogger()

	f.Fuzz(func(t *testing.T, name string, addr, meta []byte, limit int) {
		ch := make(chan overwatch.GossipMessage, 1)
		d := &receiverDelegate{msgChan: ch, logger: logger}
		node := &memberlist.Node{Name: name, Addr: net.IP(addr), Port: 7946, Meta: meta}

		d.NotifyJoin(node)
		d.NotifyUpdate(node)
		d.NotifyLeave(node)
		d.MergeRemoteState(meta, true)
		d.MergeRemoteState(meta, false)

		if len(ch) != 0 {
			t.Fatalf("node events produced %d gossip messages", len(ch))
		}

		// memberlist requires NodeMeta to respect the limit it passes.
		own := d.NodeMeta(limit)
		if len(own) > limit && len(own) > 0 {
			t.Fatalf("NodeMeta(%d) returned %d bytes", limit, len(own))
		}
		if own != nil {
			var decoded NodeMeta
			if err := json.Unmarshal(own, &decoded); err != nil {
				t.Fatalf("NodeMeta returned undecodable metadata: %v", err)
			}
			if decoded.Role != RoleOverwatch {
				t.Fatalf("NodeMeta role = %q, want %q", decoded.Role, RoleOverwatch)
			}
		}
	})
}
