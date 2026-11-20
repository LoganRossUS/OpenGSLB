// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// respRecorder is a dns.ResponseWriter that captures the reply written by the
// handler. network selects the transport the handler believes it is serving
// ("udp" or "tcp").
type respRecorder struct {
	network string
	msg     *dns.Msg
}

func (w *respRecorder) LocalAddr() net.Addr {
	if w.network == "tcp" {
		return &net.TCPAddr{IP: net.ParseIP("192.0.2.53"), Port: 53}
	}
	return &net.UDPAddr{IP: net.ParseIP("192.0.2.53"), Port: 53}
}

func (w *respRecorder) RemoteAddr() net.Addr {
	if w.network == "tcp" {
		return &net.TCPAddr{IP: net.ParseIP("198.51.100.7"), Port: 40000}
	}
	return &net.UDPAddr{IP: net.ParseIP("198.51.100.7"), Port: 40000}
}

func (w *respRecorder) WriteMsg(m *dns.Msg) error { w.msg = m; return nil }
func (w *respRecorder) Write([]byte) (int, error) { return 0, nil }
func (w *respRecorder) Close() error              { return nil }
func (w *respRecorder) TsigStatus() error         { return nil }
func (w *respRecorder) TsigTimersOnly(bool)       {}
func (w *respRecorder) Hijack()                   {}

// serveQuery runs one query through the handler over the given transport and
// returns the reply.
func serveQuery(t *testing.T, h *Handler, network string, q *dns.Msg) *dns.Msg {
	t.Helper()
	w := &respRecorder{network: network}
	h.ServeDNS(w, q)
	if w.msg == nil {
		t.Fatal("handler wrote no response")
	}
	return w.msg
}

// question builds a query for name and qtype without EDNS.
func question(name string, qtype uint16) *dns.Msg {
	q := new(dns.Msg)
	q.SetQuestion(name, qtype)
	q.RecursionDesired = false
	return q
}

// v4Entry returns a domain entry with one IPv4 server.
func v4Entry(name string) *DomainEntry {
	return &DomainEntry{
		Name:   name,
		TTL:    30,
		Router: &mockRouter{},
		Servers: []ServerInfo{
			{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100},
		},
	}
}

// counterValue reads a counter without pulling in the prometheus testutil
// module.
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return m.GetCounter().GetValue()
}
