// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    → https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/store"
)

// migrationConfig is the configuration the fixture store was written with
// (testdata/store-1.1.10/README.md). Validation is off, so a server is
// answered without a listener behind it.
const migrationConfig = `
mode: overwatch
overwatch:
  identity:
    node_id: fixture-overwatch
    region: us-east
  gossip:
    bind_address: "127.0.0.1:%d"
    encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  validation:
    enabled: false
  dnssec:
    enabled: false
    security_acknowledgment: "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses"
dns:
  listen_address: "127.0.0.1:%d"
  default_ttl: 30
  zones:
    - gslb.example.com
api:
  enabled: true
  address: "127.0.0.1:%d"
regions:
  - name: us-east
    servers:
      - address: 127.0.0.1
        port: 18080
        weight: 100
        service: app.gslb.example.com
    health_check:
      type: http
      interval: 30s
      timeout: 2s
      path: /
domains:
  - name: app.gslb.example.com
    routing_algorithm: round-robin
    regions: [us-east]
    ttl: 30
`

// startOverwatchOn starts an overwatch with migrationConfig on dataDir.
func startOverwatchOn(t *testing.T, dataDir string) *alignmentOverwatch {
	t.Helper()
	apiPort := contractFreePort(t)
	dnsPort := contractFreePort(t)
	cfg := loadTestConfig(t, fmt.Sprintf(migrationConfig, contractFreePort(t), dnsPort, apiPort))
	cfg.Overwatch.DataDir = dataDir

	app := NewApplication(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = app.Start(ctx)
	}()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer scancel()
		_ = app.Shutdown(sctx)
		cancel()
		<-done
	}
	t.Cleanup(stop)

	o := &alignmentOverwatch{
		t:    t,
		app:  app,
		api:  fmt.Sprintf("http://127.0.0.1:%d", apiPort),
		dns:  fmt.Sprintf("127.0.0.1:%d", dnsPort),
		stop: stop,
	}
	waitForAPI(t, o.api)
	return o
}

// A store written by version 1.1.10 keeps working: the domain reads with the
// canonical routing field, the backends that were stored under
// domain_backends/ are servers, they are answered in DNS, and the old records
// are gone from the store. A second start changes nothing.
func TestStoreMigration_1_1_10(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "store-1.1.10", "overwatch.db"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "overwatch.db"), fixture, 0600); err != nil {
		t.Fatal(err)
	}

	const dom = "legacy.gslb.example.com"
	// id suffix -> weight, region, priority
	type want struct {
		weight   float64
		region   string
		priority float64
	}
	wantServers := map[string]want{
		dom + ":127.0.0.3:8080": {10, "us-east", 1},
		// Its legacy record said "enabled": false, a flag that had no effect
		// and was stored as false whenever a request left it out. The server
		// was answered before the upgrade and is enabled after it.
		dom + ":127.0.0.4:8080": {20, "eu-west", 0},
		dom + ":127.0.0.5:9001": {30, "us-east", 0},
		// Existed both as a domain backend (weight 5, us-east) and as a
		// server (weight 50, eu-west): the server record wins.
		dom + ":127.0.0.6:8080": {50, "eu-west", 0},
	}

	check := func(o *alignmentOverwatch, pass string) {
		t.Helper()
		body := o.mustCall("GET", "/api/v1/domains/"+dom, "", 200)
		if got := field(t, body, "domain", "routing_algorithm"); got != "weighted" {
			t.Errorf("%s: domain.routing_algorithm = %v, want weighted (stored as routing_policy)", pass, got)
		}
		if got := field(t, body, "domain", "ttl"); got != float64(20) {
			t.Errorf("%s: domain.ttl = %v, want 20", pass, got)
		}
		if entry := o.app.dnsRegistry.Lookup(dom); entry == nil || entry.Router.Algorithm() != "weighted" {
			t.Errorf("%s: the DNS registry does not serve %s with the weighted router", pass, dom)
		}

		body = o.mustCall("GET", "/api/v1/domains/"+dom+"/backends", "", 200)
		listed := field(t, body, "backends").([]any)
		if len(listed) != len(wantServers) {
			t.Errorf("%s: the domain lists %d backends, want %d: %v", pass, len(listed), len(wantServers), listed)
		}
		seenIDs := map[string]bool{}
		for _, b := range listed {
			bm := b.(map[string]any)
			id := fmt.Sprint(bm["id"])
			seenIDs[id] = true
			w, ok := wantServers[id]
			if !ok {
				t.Errorf("%s: unexpected backend %s", pass, id)
				continue
			}
			if bm["weight"] != w.weight || bm["region"] != w.region || bm["priority"] != w.priority {
				t.Errorf("%s: backend %s has weight %v region %v priority %v, want %v %v %v",
					pass, id, bm["weight"], bm["region"], bm["priority"], w.weight, w.region, w.priority)
			}
			if bm["source"] != "api" || bm["enabled"] != true || bm["service"] != dom {
				t.Errorf("%s: backend %s has source %v enabled %v service %v, want api true %s",
					pass, id, bm["source"], bm["enabled"], bm["service"], dom)
			}
		}
		for id := range wantServers {
			if !seenIDs[id] {
				t.Errorf("%s: backend %s is missing from the domain", pass, id)
			}
			// Each one is a server: readable and changeable under /servers.
			o.mustCall("GET", "/api/v1/servers/"+id, "", 200)
		}

		// All four are answered in DNS.
		if last := waitFor(func() string {
			seen := o.answersOver(dom, 40)
			for _, addr := range []string{"127.0.0.3", "127.0.0.4", "127.0.0.5", "127.0.0.6"} {
				if seen[addr] == 0 {
					return fmt.Sprintf("answers so far: %v", seen)
				}
			}
			return ""
		}); last != "" {
			t.Errorf("%s: DNS does not answer all four servers of %s: %s", pass, dom, last)
		}

		// The old records are gone, the new ones are in the store.
		legacy, err := o.app.overwatchStore.List(context.Background(), store.PrefixDomainBackends)
		if err != nil {
			t.Fatalf("%s: list store: %v", pass, err)
		}
		if len(legacy) != 0 {
			t.Errorf("%s: %d record(s) are left under %s", pass, len(legacy), store.PrefixDomainBackends)
		}
		stored, err := o.app.overwatchStore.List(context.Background(), store.PrefixBackends+dom+":")
		if err != nil {
			t.Fatalf("%s: list store: %v", pass, err)
		}
		if len(stored) != len(wantServers) {
			t.Errorf("%s: the store holds %d server records for %s, want %d", pass, len(stored), dom, len(wantServers))
		}

		// What else was in the store is untouched.
		body = o.mustCall("GET", "/api/v1/regions/ap-south", "", 200)
		if got := field(t, body, "region", "description"); got != "created by 1.1.10" {
			t.Errorf("%s: region ap-south: description = %v", pass, got)
		}
		if rcode, addrs := o.resolve("app.gslb.example.com"); len(addrs) != 1 || addrs[0] != "127.0.0.1" {
			t.Errorf("%s: config-file domain: rcode %s answers %v, want 127.0.0.1", pass, rcode, addrs)
		}
	}

	first := startOverwatchOn(t, dataDir)
	check(first, "first start")
	// A migrated server takes changes like any other.
	migratedID := dom + ":127.0.0.3:8080"
	first.mustCall("PUT", "/api/v1/servers/"+migratedID, `{"weight":10,"priority":1}`, 200)
	first.stop()

	second := startOverwatchOn(t, dataDir)
	check(second, "second start")
	// The migrated servers outlive the stale-backend sweep: a server created
	// through the API has no agent whose heartbeat could keep it alive.
	if status, raw := second.call("DELETE", "/api/v1/domains/"+dom+"/backends/"+migratedID, ""); status != 204 {
		t.Errorf("DELETE of a migrated backend: status %d body %s, want 204", status, raw)
	}
	if seen := second.answersOver(dom, 40); seen["127.0.0.3"] != 0 {
		t.Errorf("deleted migrated backend is still answered: %v", seen)
	}
}

// When the store file exists and cannot be opened, the overwatch must not
// start: without it the domains and servers created through the API would be
// missing from DNS with nothing but a log line to show for it.
func TestStartupFailsWhenAnExistingStoreCannotBeOpened(t *testing.T) {
	dataDir := t.TempDir()
	// Not a bbolt file.
	if err := os.WriteFile(filepath.Join(dataDir, "overwatch.db"), []byte(strings.Repeat("not a database ", 400)), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, fmt.Sprintf(migrationConfig, contractFreePort(t), contractFreePort(t), contractFreePort(t)))
	cfg.Overwatch.DataDir = dataDir

	app := NewApplication(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := app.Initialize()
	if err == nil {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = app.Shutdown(sctx)
		t.Fatal("Initialize succeeded with a store file that cannot be opened")
	}
	for _, want := range []string{filepath.Join(dataDir, "overwatch.db"), "refusing to start", "overwatch.data_dir"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	}
}

// When no store has ever existed and none can be created, the overwatch
// starts read-only and says so at WARN.
func TestStartupWithoutAStoreWarns(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, fmt.Sprintf(migrationConfig, contractFreePort(t), contractFreePort(t), contractFreePort(t)))
	cfg.Overwatch.DataDir = filepath.Join(file, "data")

	var logs strings.Builder
	app := NewApplication(cfg, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = app.Shutdown(sctx)
	})

	if app.overwatchStore != nil {
		t.Fatal("a store was opened below a regular file")
	}
	var warn string
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "RUNNING WITHOUT PERSISTENCE") {
			warn = line
		}
	}
	if warn == "" {
		t.Fatalf("no RUNNING WITHOUT PERSISTENCE line in the log:\n%s", logs.String())
	}
	for _, want := range []string{"level=WARN", "503", "data_dir=" + cfg.Overwatch.DataDir} {
		if !strings.Contains(warn, want) {
			t.Errorf("the warning does not contain %q: %s", want, warn)
		}
	}
}
