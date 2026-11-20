// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"sync"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/routing"
)

// A query handler reads the entry Lookup returned after the registry lock is
// released. Agent heartbeats, the REST API and the backend registry change
// the servers and settings of a domain at any time. This test fails under
// -race if such a change writes to an entry a reader may hold.
func TestRegistry_UpdatesDoNotMutatePublishedEntries(t *testing.T) {
	const domain = "app.example.com"
	typed := func(string) (routing.Router, error) { return routing.NewRoundRobinRouter(), nil }
	untyped := func(string) (interface{}, error) { return routing.NewRoundRobinRouter(), nil }

	r := NewRegistry()
	if err := r.RegisterDomain(domain, 30, "round-robin", typed); err != nil {
		t.Fatalf("RegisterDomain: %v", err)
	}
	if err := r.RegisterServer(domain, "10.0.0.1", 80, 100, "r1"); err != nil {
		t.Fatalf("RegisterServer: %v", err)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				entry := r.Lookup(domain)
				if entry == nil {
					t.Error("domain disappeared")
					return
				}
				// What the DNS handler does with an entry.
				total := int(entry.TTL)
				for _, server := range entry.Servers {
					total += len(server.Address) + server.Port + server.Weight + len(server.Region)
				}
				if entry.Router == nil || total == 0 {
					t.Error("entry is incomplete")
					return
				}
			}
		}()
	}

	for i := 0; i < 500; i++ {
		// An agent heartbeat: upsert of a known server.
		if err := r.RegisterServer(domain, "10.0.0.1", 80, 100+i%3, "r1"); err != nil {
			t.Fatalf("RegisterServer (update): %v", err)
		}
		// A server that comes and goes.
		if err := r.RegisterServer(domain, "10.0.0.2", 80, 100, "r2"); err != nil {
			t.Fatalf("RegisterServer (add): %v", err)
		}
		if err := r.UpdateServerWeight(domain, "10.0.0.2", 80, 50); err != nil {
			t.Fatalf("UpdateServerWeight: %v", err)
		}
		if err := r.DeregisterServer(domain, "10.0.0.2", 80); err != nil {
			t.Fatalf("DeregisterServer: %v", err)
		}
		if err := r.UpdateDomainSettings(domain, uint32(30+i%2), "round-robin", untyped); err != nil {
			t.Fatalf("UpdateDomainSettings: %v", err)
		}
	}
	close(stop)
	readers.Wait()

	entry := r.Lookup(domain)
	if len(entry.Servers) != 1 || entry.Servers[0].Address.String() != "10.0.0.1" {
		t.Errorf("servers after the updates = %v, want only 10.0.0.1", entry.Servers)
	}
}

// An entry obtained before an update keeps describing the state before it.
func TestRegistry_LookupReturnsStableSnapshot(t *testing.T) {
	const domain = "app.example.com"
	typed := func(string) (routing.Router, error) { return routing.NewRoundRobinRouter(), nil }
	untyped := func(string) (interface{}, error) { return routing.NewRoundRobinRouter(), nil }

	r := NewRegistry()
	if err := r.RegisterDomain(domain, 30, "round-robin", typed); err != nil {
		t.Fatalf("RegisterDomain: %v", err)
	}
	if err := r.RegisterServer(domain, "10.0.0.1", 80, 100, "r1"); err != nil {
		t.Fatalf("RegisterServer: %v", err)
	}
	if err := r.RegisterServer(domain, "10.0.0.2", 80, 100, "r1"); err != nil {
		t.Fatalf("RegisterServer: %v", err)
	}

	before := r.Lookup(domain)

	if err := r.RegisterServer(domain, "10.0.0.1", 80, 7, "r9"); err != nil {
		t.Fatalf("RegisterServer (update): %v", err)
	}
	if err := r.RegisterServer(domain, "10.0.0.3", 80, 100, "r1"); err != nil {
		t.Fatalf("RegisterServer (add): %v", err)
	}
	if err := r.UpdateServerWeight(domain, "10.0.0.2", 80, 1); err != nil {
		t.Fatalf("UpdateServerWeight: %v", err)
	}
	if err := r.DeregisterServer(domain, "10.0.0.2", 80); err != nil {
		t.Fatalf("DeregisterServer: %v", err)
	}
	if err := r.UpdateDomainSettings(domain, 5, "round-robin", untyped); err != nil {
		t.Fatalf("UpdateDomainSettings: %v", err)
	}

	if before.TTL != 30 || len(before.Servers) != 2 ||
		before.Servers[0].Address.String() != "10.0.0.1" || before.Servers[0].Weight != 100 || before.Servers[0].Region != "r1" ||
		before.Servers[1].Address.String() != "10.0.0.2" || before.Servers[1].Weight != 100 {
		t.Errorf("entry obtained before the updates changed: ttl %d servers %+v", before.TTL, before.Servers)
	}

	after := r.Lookup(domain)
	if after.TTL != 5 || len(after.Servers) != 2 {
		t.Fatalf("entry after the updates: ttl %d servers %+v", after.TTL, after.Servers)
	}
	got := map[string]int{}
	for _, s := range after.Servers {
		got[s.Address.String()] = s.Weight
	}
	if got["10.0.0.1"] != 7 || got["10.0.0.3"] != 100 {
		t.Errorf("servers after the updates = %v, want 10.0.0.1 (weight 7) and 10.0.0.3 (weight 100)", got)
	}
}
