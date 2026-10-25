// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"bytes"
	"log/slog"
	"net"
	"testing"
	"time"

	miekg "github.com/miekg/dns"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
)

// recordingResponseWriter captures the DNS reply written by the handler.
type recordingResponseWriter struct {
	msg *miekg.Msg
}

func (w *recordingResponseWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("10.0.10.53"), Port: 53}
}
func (w *recordingResponseWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("10.0.100.10"), Port: 40000}
}
func (w *recordingResponseWriter) WriteMsg(m *miekg.Msg) error { w.msg = m; return nil }
func (w *recordingResponseWriter) Write([]byte) (int, error)   { return 0, nil }
func (w *recordingResponseWriter) Close() error                { return nil }
func (w *recordingResponseWriter) TsigStatus() error           { return nil }
func (w *recordingResponseWriter) TsigTimersOnly(bool)         {}
func (w *recordingResponseWriter) Hijack()                     {}

// TestStandaloneOverwatch_FailoverReturnsToPrimary drives the real DNS path
// used in overwatch mode (backend registry -> combinedHealthProvider -> DNS
// handler -> failover router) with the lab's two-region layout and no agents.
// The answer must leave the primary on the first failed validation and return
// to it on the first passing validation, with no dwell time in between.
func TestStandaloneOverwatch_FailoverReturnsToPrimary(t *testing.T) {
	const (
		service = "www.app.lab.internal"
		primary = "10.107.0.10"
		standby = "10.110.0.10"
	)
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	registry := overwatch.NewRegistry(overwatch.RegistryConfig{
		StaleThreshold: 15 * time.Second,
		RemoveAfter:    60 * time.Second,
		Logger:         logger,
	}, nil)
	for _, b := range []struct{ addr, region string }{{primary, "ch7"}, {standby, "dc10"}} {
		if err := registry.RegisterStatic(service, b.addr, 80, 100, b.region, "http"); err != nil {
			t.Fatalf("register %s: %v", b.addr, err)
		}
	}

	router, err := routing.NewRouter("failover")
	if err != nil {
		t.Fatal(err)
	}
	dnsRegistry := dns.NewRegistry()
	dnsRegistry.Register(&dns.DomainEntry{
		Name:             service,
		TTL:              5,
		RoutingAlgorithm: "failover",
		Router:           router,
		Servers: []dns.ServerInfo{
			{Address: net.ParseIP(primary), Port: 80, Weight: 100, Region: "ch7"},
			{Address: net.ParseIP(standby), Port: 80, Weight: 100, Region: "dc10"},
		},
	})

	handler := dns.NewHandler(dns.HandlerConfig{
		Registry:       dnsRegistry,
		HealthProvider: &combinedHealthProvider{backendRegistry: registry, logger: logger},
		DefaultTTL:     5,
		Logger:         logger,
	})

	answer := func() string {
		t.Helper()
		q := new(miekg.Msg)
		q.SetQuestion(miekg.Fqdn(service), miekg.TypeA)
		w := &recordingResponseWriter{}
		handler.ServeDNS(w, q)
		if w.msg == nil {
			t.Fatal("handler wrote no response")
		}
		if w.msg.Rcode != miekg.RcodeSuccess {
			return miekg.RcodeToString[w.msg.Rcode]
		}
		for _, rr := range w.msg.Answer {
			if a, ok := rr.(*miekg.A); ok {
				return a.A.String()
			}
		}
		return "no A record"
	}

	validate := func(addr string, healthy bool) {
		t.Helper()
		errMsg := ""
		if !healthy {
			errMsg = "connection refused"
		}
		if err := registry.UpdateValidation(service, addr, 80, healthy, errMsg); err != nil {
			t.Fatal(err)
		}
	}

	// Steady state: both VIPs validated healthy, the primary is answered.
	validate(primary, true)
	validate(standby, true)
	if got := answer(); got != primary {
		t.Fatalf("steady state: want %s, got %s", primary, got)
	}

	// Primary VIP stops: the very next query goes to the standby.
	validate(primary, false)
	if got := answer(); got != standby {
		t.Fatalf("after primary failure: want %s, got %s", standby, got)
	}

	// Primary VIP restarts and passes one validation: the very next query
	// must return to the primary. This is the return the lab measured at
	// 24.8 s and 57.7 s; the decision layer itself adds no delay.
	validate(primary, true)
	if got := answer(); got != primary {
		t.Fatalf("after primary recovery: want %s, got %s", primary, got)
	}
}
