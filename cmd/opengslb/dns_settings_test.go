// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"strings"
	"testing"

	miekg "github.com/miekg/dns"
)

// zonesConfigContent is validConfigContent with a dns.zones list that mixes
// the string form and the object form.
const zonesConfigContent = `
overwatch:
  identity:
    node_id: overwatch-7

dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 60
  zones:
    - example.com
    - name: gslb.example.net
      soa:
        serial: 2026112001
      nameservers:
        - ns1.example.net
        - ns2.example.net

logging:
  level: info
  format: text

regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 80
        weight: 100
        service: app.example.com
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health

domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
    ttl: 30
`

// queryApp sends one query straight to the application's DNS handler.
func queryApp(t *testing.T, app *Application, name string, qtype uint16) *miekg.Msg {
	t.Helper()
	q := new(miekg.Msg)
	q.SetQuestion(name, qtype)
	w := &recordingResponseWriter{}
	app.dnsHandler.ServeDNS(w, q)
	if w.msg == nil {
		t.Fatal("handler wrote no response")
	}
	return w.msg
}

// apexSOA returns the SOA record answered at the apex of zone.
func apexSOA(t *testing.T, app *Application, zone string) *miekg.SOA {
	t.Helper()
	resp := queryApp(t, app, zone, miekg.TypeSOA)
	if resp.Rcode != miekg.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("SOA %s: rcode = %s, answers = %d, want NOERROR with 1 answer", zone, miekg.RcodeToString[resp.Rcode], len(resp.Answer))
	}
	soa, ok := resp.Answer[0].(*miekg.SOA)
	if !ok {
		t.Fatalf("SOA %s: answer = %v", zone, resp.Answer[0])
	}
	return soa
}

// TestApplication_ZonesFromConfig checks that dns.zones reaches the DNS
// handler at startup: the string form gets defaults derived from the node
// identity, the object form its configured values.
func TestApplication_ZonesFromConfig(t *testing.T) {
	cfg := loadTestConfig(t, zonesConfigContent)
	app := NewApplication(cfg, nil)
	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	soa := apexSOA(t, app, "example.com.")
	if soa.Ns != "overwatch-7.example.com." || soa.Mbox != "hostmaster.example.com." || soa.Serial != 1 {
		t.Errorf("string-form zone SOA = %v", soa)
	}

	soa = apexSOA(t, app, "gslb.example.net.")
	if soa.Serial != 2026112001 || soa.Ns != "ns1.example.net." {
		t.Errorf("object-form zone SOA = %v", soa)
	}

	resp := queryApp(t, app, "gslb.example.net.", miekg.TypeNS)
	if len(resp.Answer) != 2 {
		t.Fatalf("NS answers = %d, want 2", len(resp.Answer))
	}
}

// TestApplication_ReloadAppliesZones checks that a configuration reload
// replaces the zone table without a restart.
func TestApplication_ReloadAppliesZones(t *testing.T) {
	cfg := loadTestConfig(t, zonesConfigContent)
	app := NewApplication(cfg, nil)
	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	newCfg := loadTestConfig(t, strings.Replace(zonesConfigContent, "serial: 2026112001", "serial: 2026112002", 1))
	newCfg.Overwatch.DataDir = cfg.Overwatch.DataDir
	if _, err := app.Reload(newCfg); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if got := apexSOA(t, app, "gslb.example.net.").Serial; got != 2026112002 {
		t.Errorf("serial after reload = %d, want 2026112002", got)
	}
}

// advertisedUDPSize returns the EDNS UDP payload size the application's DNS
// handler advertises.
func advertisedUDPSize(t *testing.T, app *Application) uint16 {
	t.Helper()
	q := new(miekg.Msg)
	q.SetQuestion("app.example.com.", miekg.TypeA)
	q.SetEdns0(4096, false)
	w := &recordingResponseWriter{}
	app.dnsHandler.ServeDNS(w, q)
	if w.msg == nil {
		t.Fatal("handler wrote no response")
	}
	opt := w.msg.IsEdns0()
	if opt == nil {
		t.Fatal("response has no OPT record")
	}
	return opt.UDPSize()
}

// TestApplication_MaxUDPSize checks that dns.max_udp_size reaches the DNS
// handler at startup (1232 by default) and on reload.
func TestApplication_MaxUDPSize(t *testing.T) {
	cfg := loadTestConfig(t, zonesConfigContent)
	app := NewApplication(cfg, nil)
	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if got := advertisedUDPSize(t, app); got != 1232 {
		t.Errorf("default advertised UDP size = %d, want 1232", got)
	}

	newCfg := loadTestConfig(t, strings.Replace(zonesConfigContent, "default_ttl: 60", "default_ttl: 60\n  max_udp_size: 4096", 1))
	newCfg.Overwatch.DataDir = cfg.Overwatch.DataDir
	if newCfg.DNS.MaxUDPSize != 4096 {
		t.Fatalf("test config did not set max_udp_size: %d", newCfg.DNS.MaxUDPSize)
	}
	if _, err := app.Reload(newCfg); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if got := advertisedUDPSize(t, app); got != 4096 {
		t.Errorf("advertised UDP size after reload = %d, want 4096", got)
	}
}

// markOnlyBackendDown records a failed validation for the single backend of
// zonesConfigContent, which takes it out of DNS.
func markOnlyBackendDown(t *testing.T, app *Application) {
	t.Helper()
	if err := app.backendRegistry.UpdateValidation("app.example.com", "10.0.1.10", 80, false, "connection refused"); err != nil {
		t.Fatalf("UpdateValidation: %v", err)
	}
}

// TestApplication_ReturnLastHealthy checks that dns.return_last_healthy
// reaches the DNS handler at startup and on reload. With the only backend
// down, the default answer is SERVFAIL and the answer with the option set is
// the configured server.
func TestApplication_ReturnLastHealthy(t *testing.T) {
	cfg := loadTestConfig(t, zonesConfigContent)
	app := NewApplication(cfg, nil)
	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	markOnlyBackendDown(t, app)

	resp := queryApp(t, app, "app.example.com.", miekg.TypeA)
	if resp.Rcode != miekg.RcodeServerFailure {
		t.Fatalf("default: rcode = %s, want SERVFAIL", miekg.RcodeToString[resp.Rcode])
	}

	enabled := strings.Replace(zonesConfigContent, "default_ttl: 60", "default_ttl: 60\n  return_last_healthy: true", 1)
	newCfg := loadTestConfig(t, enabled)
	newCfg.Overwatch.DataDir = cfg.Overwatch.DataDir
	if _, err := app.Reload(newCfg); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	resp = queryApp(t, app, "app.example.com.", miekg.TypeA)
	if resp.Rcode != miekg.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("after reload: rcode = %s, answers = %d, want NOERROR with 1 answer", miekg.RcodeToString[resp.Rcode], len(resp.Answer))
	}
	if a, ok := resp.Answer[0].(*miekg.A); !ok || a.A.String() != "10.0.1.10" {
		t.Errorf("after reload: answer = %v, want A 10.0.1.10", resp.Answer[0])
	}

	// A fresh application started with the option set behaves the same.
	cfg2 := loadTestConfig(t, enabled)
	app2 := NewApplication(cfg2, nil)
	if err := app2.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	markOnlyBackendDown(t, app2)
	resp = queryApp(t, app2, "app.example.com.", miekg.TypeA)
	if resp.Rcode != miekg.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("at startup: rcode = %s, answers = %d, want NOERROR with 1 answer", miekg.RcodeToString[resp.Rcode], len(resp.Answer))
	}
}
