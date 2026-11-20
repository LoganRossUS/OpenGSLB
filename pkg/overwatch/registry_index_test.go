// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/store"
)

func newIndexTestRegistry(st store.Store) *Registry {
	return NewRegistry(RegistryConfig{
		StaleThreshold: 15 * time.Second,
		RemoveAfter:    60 * time.Second,
		Logger:         slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	}, st)
}

// assertIndexConsistent checks that the endpoint index holds exactly the
// backends of the registry, each once, under its own address and port, and
// ordered by service within an endpoint.
func assertIndexConsistent(t *testing.T, r *Registry, step string) {
	t.Helper()
	r.mu.RLock()
	defer r.mu.RUnlock()

	indexed := 0
	for ek, list := range r.byEndpoint {
		if len(list) == 0 {
			t.Errorf("%s: empty index entry left behind for %v", step, ek)
		}
		if !sort.SliceIsSorted(list, func(i, j int) bool { return list[i].Service < list[j].Service }) {
			t.Errorf("%s: backends at %v are not ordered by service", step, ek)
		}
		for _, b := range list {
			indexed++
			if b.Address != ek.address || b.Port != ek.port {
				t.Errorf("%s: backend %s:%d is indexed under %v", step, b.Address, b.Port, ek)
			}
			if r.backends[backendKey(b.Service, b.Address, b.Port)] != b {
				t.Errorf("%s: index holds a backend that is not in the registry: %s %s:%d", step, b.Service, b.Address, b.Port)
			}
		}
	}
	if indexed != len(r.backends) {
		t.Errorf("%s: index holds %d backends, registry holds %d", step, indexed, len(r.backends))
	}
}

// TestRegistry_EndpointIndexFollowsEveryMutation runs every operation that
// adds or removes backends and checks the index after each one.
func TestRegistry_EndpointIndexFollowsEveryMutation(t *testing.T) {
	r := newIndexTestRegistry(nil)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	assertIndexConsistent(t, r, "new registry")

	must(r.RegisterStatic("b.example.com", "10.0.0.1", 80, 100, "eu", "http"))
	must(r.RegisterStatic("a.example.com", "10.0.0.1", 80, 100, "eu", "http"))
	must(r.RegisterStatic("a.example.com", "10.0.0.1", 81, 100, "eu", "http"))
	must(r.RegisterAPI("c.example.com", "10.0.0.1", 80, 100, "eu"))
	must(r.Register("agent-1", "eu", "a.example.com", "10.0.0.2", 80, 100, true))
	assertIndexConsistent(t, r, "after registration")

	// Registering again updates in place and must not index twice.
	must(r.RegisterStatic("a.example.com", "10.0.0.1", 80, 50, "eu", "http"))
	must(r.Register("agent-1", "eu", "a.example.com", "10.0.0.2", 80, 50, false))
	if err := r.RegisterAPI("c.example.com", "10.0.0.1", 80, 100, "eu"); err == nil {
		t.Error("registering an existing API backend again succeeded")
	}
	assertIndexConsistent(t, r, "after re-registration")

	must(r.Update("a.example.com", "10.0.0.1", 80, 10, "us"))
	must(r.UpdateValidation("a.example.com", "10.0.0.1", 80, false, "down"))
	must(r.SetOverride("b.example.com", "10.0.0.1", 80, false, "test", "test"))
	must(r.ClearOverride("b.example.com", "10.0.0.1", 80))
	assertIndexConsistent(t, r, "after updates")

	must(r.Deregister("a.example.com", "10.0.0.1", 80))
	assertIndexConsistent(t, r, "after removing one of three at an endpoint")
	if err := r.Deregister("a.example.com", "10.0.0.1", 80); err == nil {
		t.Error("removing a backend twice succeeded")
	}
	must(r.Deregister("a.example.com", "10.0.0.1", 81))
	assertIndexConsistent(t, r, "after removing the only backend at an endpoint")
	if _, found := r.GetBackendByEndpoint("10.0.0.1", 81); found {
		t.Error("removed backend is still found by endpoint")
	}

	// Stale removal: an agent backend whose heartbeat is older than
	// RemoveAfter is deleted by the stale check.
	r.mu.Lock()
	r.backends[backendKey("a.example.com", "10.0.0.2", 80)].AgentLastSeen = time.Now().Add(-time.Hour)
	r.mu.Unlock()
	r.checkStaleBackends()
	if _, found := r.GetBackend("a.example.com", "10.0.0.2", 80); found {
		t.Fatal("expired agent backend was not removed")
	}
	if _, found := r.GetBackendByEndpoint("10.0.0.2", 80); found {
		t.Error("expired agent backend is still found by endpoint")
	}
	assertIndexConsistent(t, r, "after stale removal")
}

// TestRegistry_EndpointIndexLoadedFromStore checks that backends restored
// from the store at startup are indexed, including one that replaces a
// backend registered before Start.
func TestRegistry_EndpointIndexLoadedFromStore(t *testing.T) {
	st, err := store.NewBboltStore(filepath.Join(t.TempDir(), "kv.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	first := newIndexTestRegistry(st)
	if err := first.RegisterAPI("a.example.com", "10.0.0.1", 80, 100, "eu"); err != nil {
		t.Fatal(err)
	}
	if err := first.RegisterAPI("a.example.com", "10.0.0.2", 80, 100, "eu"); err != nil {
		t.Fatal(err)
	}
	if pairs, err := st.List(context.Background(), "backends/"); err != nil || len(pairs) != 2 {
		t.Fatalf("store holds %d backends (err %v), want 2", len(pairs), err)
	}

	second := newIndexTestRegistry(st)
	// A backend that is also in the store, registered before the load.
	if err := second.RegisterStatic("a.example.com", "10.0.0.1", 80, 100, "eu", "http"); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Stop() }()

	assertIndexConsistent(t, second, "after loading from the store")
	for _, addr := range []string{"10.0.0.1", "10.0.0.2"} {
		if b, found := second.GetBackendByEndpoint(addr, 80); !found || b.Address != addr {
			t.Errorf("backend %s:80 restored from the store is not found by endpoint", addr)
		}
	}
}

// TestRegistry_GetBackendByEndpoint covers the lookup itself, including an
// address and port shared by several services.
func TestRegistry_GetBackendByEndpoint(t *testing.T) {
	r := newIndexTestRegistry(nil)
	for _, service := range []string{"c.example.com", "a.example.com", "b.example.com"} {
		if err := r.RegisterStatic(service, "10.0.0.1", 443, 100, "eu", "http"); err != nil {
			t.Fatal(err)
		}
	}

	if _, found := r.GetBackendByEndpoint("10.0.0.9", 443); found {
		t.Error("unknown address found")
	}
	if _, found := r.GetBackendByEndpoint("10.0.0.1", 80); found {
		t.Error("unknown port found")
	}

	// All healthy: the first by service name, every time.
	for i := 0; i < 50; i++ {
		b, found := r.GetBackendByEndpoint("10.0.0.1", 443)
		if !found || b.Service != "a.example.com" {
			t.Fatalf("all healthy: got %q (found %v), want a.example.com", b.Service, found)
		}
	}
	if !r.IsHealthy("10.0.0.1", 443) {
		t.Error("IsHealthy = false with every backend healthy")
	}

	// One unhealthy: that one decides.
	if err := r.UpdateValidation("c.example.com", "10.0.0.1", 443, false, "down"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		b, _ := r.GetBackendByEndpoint("10.0.0.1", 443)
		if b.Service != "c.example.com" || b.EffectiveStatus != StatusUnhealthy {
			t.Fatalf("one unhealthy: got %q with status %q, want c.example.com unhealthy", b.Service, b.EffectiveStatus)
		}
	}
	if r.IsHealthy("10.0.0.1", 443) {
		t.Error("IsHealthy = true with an unhealthy backend at the endpoint")
	}

	// The result is a copy: changing it does not change the registry.
	b, _ := r.GetBackendByEndpoint("10.0.0.1", 443)
	b.EffectiveStatus = StatusHealthy
	if again, _ := r.GetBackendByEndpoint("10.0.0.1", 443); again.EffectiveStatus != StatusUnhealthy {
		t.Error("changing the returned backend changed the registry")
	}
}

// TestRegistry_GetLatencyByEndpoint checks that the latency lookup of the
// DNS query path uses the endpoint index and prefers a backend with samples.
func TestRegistry_GetLatencyByEndpoint(t *testing.T) {
	r := newIndexTestRegistry(nil)
	for _, service := range []string{"a.example.com", "b.example.com"} {
		if err := r.RegisterStatic(service, "10.0.0.1", 443, 100, "eu", "http"); err != nil {
			t.Fatal(err)
		}
	}
	if info := r.GetLatency("10.0.0.1", 443); info.HasData {
		t.Errorf("latency data reported before any sample: %+v", info)
	}
	if info := r.GetLatency("10.0.0.9", 443); info.HasData {
		t.Errorf("latency data reported for an unknown endpoint: %+v", info)
	}
	// Only the second backend, by service name, has a sample.
	if err := r.UpdateValidationWithLatency("b.example.com", "10.0.0.1", 443, true, "", 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	info := r.GetLatency("10.0.0.1", 443)
	if !info.HasData || info.Samples != 1 || info.LastLatency != 20*time.Millisecond {
		t.Errorf("GetLatency = %+v, want the one sample of 20ms", info)
	}
}

// TestRegistry_BackendMapWritesGoThroughIndex reads the package source and
// fails when code outside registry_index.go writes to the backends map
// directly. Such a write would leave the endpoint index out of date, and the
// DNS query path would no longer find the backend.
func TestRegistry_BackendMapWritesGoThroughIndex(t *testing.T) {
	direct := regexp.MustCompile(`\.backends\[[^\]]*\]\s*=[^=]|delete\(\s*\w+\.backends\b|\.backends\s*=[^=]`)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file == "registry_index.go" || strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(src), "\n") {
			if direct.MatchString(line) {
				t.Errorf("%s:%d writes to the backends map directly; use putBackend or removeBackend so the endpoint index stays correct:\n\t%s",
					file, n+1, strings.TrimSpace(line))
			}
		}
	}
}

// BenchmarkRegistry_GetBackendByEndpoint measures the indexed lookup against
// registries of different sizes.
func BenchmarkRegistry_GetBackendByEndpoint(b *testing.B) {
	for _, n := range []int{10, 1000, 10000} {
		b.Run(fmt.Sprintf("backends=%d", n), func(b *testing.B) {
			r := newIndexTestRegistry(nil)
			addrs := make([]string, n)
			for i := range addrs {
				addrs[i] = fmt.Sprintf("10.%d.%d.%d", 1+i/65536, (i/256)%256, i%256)
				if err := r.RegisterStatic("app.example.com", addrs[i], 80, 100, "eu", "http"); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, found := r.GetBackendByEndpoint(addrs[i%n], 80); !found {
					b.Fatal("registered backend not found")
				}
			}
		})
	}
}
