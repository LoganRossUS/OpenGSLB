// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
)

func testRouter(_ string) (routing.Router, error) {
	return routing.NewRoundRobinRouter(), nil
}

func testDynamicRouter(_ string) (interface{}, error) {
	return routing.NewRoundRobinRouter(), nil
}

// registryWith builds a registry from "domain" -> "address:port" lists.
func registryWith(t *testing.T, domains map[string][]string) *dns.Registry {
	t.Helper()
	r := dns.NewRegistry()
	for name, servers := range domains {
		if err := r.RegisterDomain(name, 30, "round-robin", testRouter); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
		for _, s := range servers {
			var address string
			var port int
			if _, err := fmt.Sscanf(strings.Replace(s, ":", " ", 1), "%s %d", &address, &port); err != nil {
				t.Fatalf("bad server %q: %v", s, err)
			}
			if err := r.RegisterServer(name, address, port, 100, "r1"); err != nil {
				t.Fatalf("register %s in %s: %v", s, name, err)
			}
		}
	}
	return r
}

// registryState renders a registry as "domain=[servers] ..." for comparison.
func registryState(r *dns.Registry) string {
	names := r.Domains()
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		entry := r.Lookup(name)
		servers := make([]string, 0, len(entry.Servers))
		for _, s := range entry.Servers {
			servers = append(servers, fmt.Sprintf("%s:%d", s.Address, s.Port))
		}
		sort.Strings(servers)
		parts = append(parts, fmt.Sprintf("%s=%v", name, servers))
	}
	return strings.Join(parts, " ")
}

// A change that reaches the DNS registry while a reload is building its
// replacement must be in the result, whichever side of the reload's snapshot
// it fell on.
func TestDNSStateSync_ChangesDuringRebuildAreNotLost(t *testing.T) {
	live := registryWith(t, map[string][]string{
		"a.example.com":    {"10.0.0.1:80"},
		"gone.example.com": {"10.0.0.9:80"},
	})
	s := newDNSStateSync(live)

	err := s.rebuild(func() (*dns.Registry, error) {
		// The reload has taken its snapshot: it saw what live holds now.
		fresh := registryWith(t, map[string][]string{
			"a.example.com":    {"10.0.0.1:80"},
			"gone.example.com": {"10.0.0.9:80"},
		})

		// Changes that arrive before the swap.
		if err := s.RegisterDomainDynamic("new.example.com", 30, "round-robin", testDynamicRouter); err != nil {
			t.Errorf("RegisterDomainDynamic: %v", err)
		}
		if err := s.RegisterServer("new.example.com", "10.0.0.5", 80, 100, "r1"); err != nil {
			t.Errorf("RegisterServer(new): %v", err)
		}
		if err := s.RegisterServer("a.example.com", "10.0.0.2", 80, 100, "r1"); err != nil {
			t.Errorf("RegisterServer(a): %v", err)
		}
		if err := s.DeregisterServer("a.example.com", "10.0.0.1", 80); err != nil {
			t.Errorf("DeregisterServer(a): %v", err)
		}
		s.Remove("gone.example.com")
		return fresh, nil
	})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	want := "a.example.com.=[10.0.0.2:80] new.example.com.=[10.0.0.5:80]"
	if got := registryState(live); got != want {
		t.Errorf("live registry after rebuild:\n got %s\nwant %s", got, want)
	}
}

// Changes are applied to the live registry immediately, also during a rebuild.
func TestDNSStateSync_AppliesToLiveRegistry(t *testing.T) {
	live := registryWith(t, map[string][]string{"a.example.com": nil})
	s := newDNSStateSync(live)

	if err := s.RegisterServer("a.example.com", "10.0.0.1", 80, 100, "r1"); err != nil {
		t.Fatalf("RegisterServer: %v", err)
	}
	if err := s.RegisterServer("missing.example.com", "10.0.0.1", 80, 100, "r1"); err == nil {
		t.Error("RegisterServer for an unknown domain returned no error")
	}
	if err := s.UpdateDomainSettings("a.example.com", 5, "round-robin", testDynamicRouter); err != nil {
		t.Fatalf("UpdateDomainSettings: %v", err)
	}
	if ttl := live.Lookup("a.example.com").TTL; ttl != 5 {
		t.Errorf("TTL = %d, want 5", ttl)
	}
	if got, want := registryState(live), "a.example.com.=[10.0.0.1:80]"; got != want {
		t.Errorf("live registry: got %s, want %s", got, want)
	}
}

// A failed build leaves the live registry as it was, including the changes
// made while the build ran, and later rebuilds start from a clean journal.
func TestDNSStateSync_FailedRebuildKeepsLiveRegistry(t *testing.T) {
	live := registryWith(t, map[string][]string{"a.example.com": {"10.0.0.1:80"}})
	s := newDNSStateSync(live)

	buildErr := errors.New("store unavailable")
	err := s.rebuild(func() (*dns.Registry, error) {
		if err := s.RegisterServer("a.example.com", "10.0.0.2", 80, 100, "r1"); err != nil {
			t.Errorf("RegisterServer: %v", err)
		}
		return nil, buildErr
	})
	if !errors.Is(err, buildErr) {
		t.Fatalf("rebuild error = %v, want %v", err, buildErr)
	}
	want := "a.example.com.=[10.0.0.1:80 10.0.0.2:80]"
	if got := registryState(live); got != want {
		t.Errorf("live registry after failed rebuild:\n got %s\nwant %s", got, want)
	}

	// The change recorded during the failed rebuild must not be replayed by
	// the next one.
	if err := s.DeregisterServer("a.example.com", "10.0.0.2", 80); err != nil {
		t.Fatalf("DeregisterServer: %v", err)
	}
	err = s.rebuild(func() (*dns.Registry, error) {
		return registryWith(t, map[string][]string{"a.example.com": {"10.0.0.1:80"}}), nil
	})
	if err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	if got, want := registryState(live), "a.example.com.=[10.0.0.1:80]"; got != want {
		t.Errorf("live registry after second rebuild: got %s, want %s", got, want)
	}
}
