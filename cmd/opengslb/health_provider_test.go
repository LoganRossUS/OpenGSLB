// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// newTestBackendRegistry returns a backend registry without a store whose own
// log output is discarded.
func newTestBackendRegistry(t *testing.T) *overwatch.Registry {
	t.Helper()
	return overwatch.NewRegistry(overwatch.RegistryConfig{
		StaleThreshold: 15 * time.Second,
		RemoveAfter:    60 * time.Second,
		Logger:         slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	}, nil)
}

// TestCombinedHealthProvider_ExclusionsAreNotLoggedAtInfo covers finding
// 1B #8: IsHealthy runs for every candidate server of every query, so
// excluding an unhealthy or draining backend must not write to the log at
// INFO. The exclusion stays visible at DEBUG.
func TestCombinedHealthProvider_ExclusionsAreNotLoggedAtInfo(t *testing.T) {
	const service = "app.gslb.example.com"

	registry := newTestBackendRegistry(t)
	// An unhealthy backend (failed validation) and a draining one (agent
	// reported predictive-health draining).
	if err := registry.RegisterStatic(service, "10.0.1.10", 80, 100, "eu", "http"); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateValidation(service, "10.0.1.10", 80, false, "connection refused"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("agent-1", "eu", service, "10.0.1.11", 80, 100, true); err != nil {
		t.Fatal(err)
	}
	registry.UpdateDraining("agent-1", true, "cpu_threshold_exceeded", time.Now(), 97, 40, 0)

	for _, tc := range []struct {
		level     slog.Level
		wantLines bool
	}{
		{slog.LevelInfo, false},
		{slog.LevelDebug, true},
	} {
		t.Run(tc.level.String(), func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: tc.level}))
			provider := &combinedHealthProvider{backendRegistry: registry, logger: logger}

			if provider.IsHealthy("10.0.1.10", 80) {
				t.Error("backend that failed validation is reported healthy")
			}
			if provider.IsHealthy("10.0.1.11", 80) {
				t.Error("draining backend is reported healthy")
			}

			out := buf.String()
			if !tc.wantLines {
				if out != "" {
					t.Errorf("IsHealthy logged at INFO or above on the query path:\n%s", out)
				}
				return
			}
			if strings.Contains(out, "level=INFO") {
				t.Errorf("exclusion still logged at INFO:\n%s", out)
			}
			for _, want := range []string{"10.0.1.10", "10.0.1.11", "level=DEBUG"} {
				if !strings.Contains(out, want) {
					t.Errorf("debug log does not mention %q:\n%s", want, out)
				}
			}
		})
	}
}

// fillBackendRegistry registers n healthy static backends, each with its own
// address, and returns their addresses.
func fillBackendRegistry(tb testing.TB, registry *overwatch.Registry, n int) []string {
	tb.Helper()
	addrs := make([]string, n)
	for i := 0; i < n; i++ {
		addrs[i] = fmt.Sprintf("10.%d.%d.%d", 1+i/65536, (i/256)%256, i%256)
		service := fmt.Sprintf("svc-%d.gslb.example.com", i%50)
		if err := registry.RegisterStatic(service, addrs[i], 80, 100, "eu", "http"); err != nil {
			tb.Fatal(err)
		}
	}
	return addrs
}

// scanIsHealthy is the reference answer for combinedHealthProvider.IsHealthy
// without a health manager: a full scan of the backend registry. An address
// and port is healthy when at least one backend is registered there and every
// backend registered there is healthy and not draining.
func scanIsHealthy(registry *overwatch.Registry, address string, port int) bool {
	found := false
	for _, b := range registry.GetAllBackends() {
		if b.Address != address || b.Port != port {
			continue
		}
		found = true
		if b.Draining || b.EffectiveStatus != overwatch.StatusHealthy {
			return false
		}
	}
	return found
}

// TestCombinedHealthProvider_LookupDoesNotScanRegistry covers finding 1B #9:
// IsHealthy copied every backend of the registry (GetAllBackends) and scanned
// the copies, for each candidate server of each query. The work per call must
// not grow with the number of registered backends. Allocations are a
// deterministic measure of that: the scan allocated one copy per backend.
func TestCombinedHealthProvider_LookupDoesNotScanRegistry(t *testing.T) {
	const backends = 2000

	registry := newTestBackendRegistry(t)
	addrs := fillBackendRegistry(t, registry, backends)
	provider := &combinedHealthProvider{
		backendRegistry: registry,
		logger:          slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	}

	target := addrs[backends/2]
	if !provider.IsHealthy(target, 80) {
		t.Fatalf("%s:80 is registered and healthy but reported unhealthy", target)
	}
	allocs := testing.AllocsPerRun(50, func() {
		provider.IsHealthy(target, 80)
	})
	// An indexed lookup allocates nothing. The limit leaves room for the race
	// detector's bookkeeping and is far below one allocation per backend.
	if allocs > 8 {
		t.Errorf("IsHealthy made %.0f allocations per call with %d backends registered: it still copies or scans the registry", allocs, backends)
	}
}

// TestCombinedHealthProvider_IndexStaysCorrect walks one backend registry
// through additions, removals and every kind of health change and compares
// IsHealthy with a full scan after each step.
func TestCombinedHealthProvider_IndexStaysCorrect(t *testing.T) {
	const service = "app.gslb.example.com"

	registry := newTestBackendRegistry(t)
	provider := &combinedHealthProvider{backendRegistry: registry}

	endpoints := []struct {
		address string
		port    int
	}{
		{"10.0.1.10", 80}, {"10.0.1.10", 8080}, {"10.0.1.11", 80}, {"10.0.1.12", 80},
		{"2001:db8::10", 443}, {"192.0.2.99", 80},
	}
	check := func(step string, want map[string]bool) {
		t.Helper()
		for _, ep := range endpoints {
			got := provider.IsHealthy(ep.address, ep.port)
			if scan := scanIsHealthy(registry, ep.address, ep.port); got != scan {
				t.Errorf("%s: IsHealthy(%s, %d) = %v, a full scan says %v", step, ep.address, ep.port, got, scan)
			}
			key := fmt.Sprintf("%s|%d", ep.address, ep.port)
			if got != want[key] {
				t.Errorf("%s: IsHealthy(%s, %d) = %v, want %v", step, ep.address, ep.port, got, want[key])
			}
		}
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	check("empty registry", map[string]bool{})

	// Additions from the three registration sources.
	must(registry.RegisterStatic(service, "10.0.1.10", 80, 100, "eu", "http"))
	must(registry.RegisterStatic(service, "10.0.1.10", 8080, 100, "eu", "http"))
	must(registry.Register("agent-1", "eu", service, "10.0.1.11", 80, 100, true))
	must(registry.RegisterAPI(service, "10.0.1.12", 80, 100, "us"))
	must(registry.RegisterAPI(service, "2001:db8::10", 443, 100, "us"))
	check("after registration", map[string]bool{
		"10.0.1.10|80": true, "10.0.1.10|8080": true, "10.0.1.11|80": true, "10.0.1.12|80": true, "2001:db8::10|443": true,
	})

	// Health changes: failed validation, agent draining, manual override.
	must(registry.UpdateValidation(service, "10.0.1.10", 80, false, "connection refused"))
	registry.UpdateDraining("agent-1", true, "cpu_threshold_exceeded", time.Now(), 97, 40, 0)
	must(registry.SetOverride(service, "10.0.1.12", 80, false, "maintenance", "test"))
	check("after health changes", map[string]bool{
		"10.0.1.10|8080": true, "2001:db8::10|443": true,
	})

	// Recovery of each.
	must(registry.UpdateValidation(service, "10.0.1.10", 80, true, ""))
	registry.UpdateDraining("agent-1", false, "", time.Time{}, 20, 40, 0)
	must(registry.ClearOverride(service, "10.0.1.12", 80))
	check("after recovery", map[string]bool{
		"10.0.1.10|80": true, "10.0.1.10|8080": true, "10.0.1.11|80": true, "10.0.1.12|80": true, "2001:db8::10|443": true,
	})

	// Removal, then the same endpoint again under another service.
	must(registry.Deregister(service, "10.0.1.10", 80))
	must(registry.Deregister(service, "2001:db8::10", 443))
	check("after removal", map[string]bool{
		"10.0.1.10|8080": true, "10.0.1.11|80": true, "10.0.1.12|80": true,
	})
	must(registry.RegisterAPI("other.gslb.example.com", "10.0.1.10", 80, 100, "eu"))
	check("after re-adding under another service", map[string]bool{
		"10.0.1.10|80": true, "10.0.1.10|8080": true, "10.0.1.11|80": true, "10.0.1.12|80": true,
	})
}

// TestCombinedHealthProvider_SharedEndpoint pins the answer for an address
// and port that is registered under more than one service: it is healthy only
// when every backend registered there is. The full scan answered from
// whichever backend the map iteration happened to visit first, so the same
// query flipped between healthy and unhealthy.
func TestCombinedHealthProvider_SharedEndpoint(t *testing.T) {
	registry := newTestBackendRegistry(t)
	provider := &combinedHealthProvider{backendRegistry: registry}

	services := []string{"a.gslb.example.com", "b.gslb.example.com", "c.gslb.example.com"}
	for _, service := range services {
		if err := registry.RegisterStatic(service, "10.0.1.10", 443, 100, "eu", "http"); err != nil {
			t.Fatal(err)
		}
	}
	count := func() (healthy int) {
		for i := 0; i < 200; i++ {
			if provider.IsHealthy("10.0.1.10", 443) {
				healthy++
			}
		}
		return healthy
	}

	if got := count(); got != 200 {
		t.Errorf("all three backends healthy: %d of 200 lookups said healthy, want 200", got)
	}
	if err := registry.UpdateValidation(services[1], "10.0.1.10", 443, false, "timeout"); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 0 {
		t.Errorf("one of three backends unhealthy: %d of 200 lookups said healthy, want 0", got)
	}
	if err := registry.Deregister(services[1], "10.0.1.10", 443); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 200 {
		t.Errorf("unhealthy backend removed: %d of 200 lookups said healthy, want 200", got)
	}
}

// TestCombinedHealthProvider_ConcurrentUse runs lookups while backends are
// added, removed and change health. It is meaningful under -race, and checks
// at the end that the index agrees with a full scan.
func TestCombinedHealthProvider_ConcurrentUse(t *testing.T) {
	const (
		service = "app.gslb.example.com"
		writers = 4
		perW    = 40
		rounds  = 30
	)
	registry := newTestBackendRegistry(t)
	provider := &combinedHealthProvider{backendRegistry: registry}
	addr := func(w, i int) string { return fmt.Sprintf("10.9.%d.%d", w, i) }

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func(r int) {
			defer readers.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				provider.IsHealthy(addr(i%writers, (i+r)%perW), 80)
			}
		}(r)
	}

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for round := 0; round < rounds; round++ {
				for i := 0; i < perW; i++ {
					a := addr(w, i)
					switch (round + i) % 4 {
					case 0:
						_ = registry.RegisterStatic(service, a, 80, 100, "eu", "http")
					case 1:
						_ = registry.UpdateValidation(service, a, 80, i%2 == 0, "")
					case 2:
						_ = registry.Register(fmt.Sprintf("agent-%d", w), "eu", service, a, 80, 100, i%3 != 0)
					case 3:
						_ = registry.Deregister(service, a, 80)
					}
				}
			}
		}(w)
	}
	wg.Wait()
	close(stop)
	readers.Wait()

	for w := 0; w < writers; w++ {
		for i := 0; i < perW; i++ {
			if got, want := provider.IsHealthy(addr(w, i), 80), scanIsHealthy(registry, addr(w, i), 80); got != want {
				t.Errorf("IsHealthy(%s, 80) = %v, a full scan says %v", addr(w, i), got, want)
			}
		}
	}
}

// BenchmarkCombinedHealthProvider_IsHealthy measures one health lookup on the
// DNS query path against registries of different sizes. With an index the
// time and the allocations per lookup do not depend on the size.
func BenchmarkCombinedHealthProvider_IsHealthy(b *testing.B) {
	for _, n := range []int{10, 1000, 10000} {
		b.Run(fmt.Sprintf("backends=%d", n), func(b *testing.B) {
			registry := overwatch.NewRegistry(overwatch.RegistryConfig{
				StaleThreshold: 15 * time.Second,
				RemoveAfter:    60 * time.Second,
				Logger:         slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
			}, nil)
			addrs := fillBackendRegistry(b, registry, n)
			provider := &combinedHealthProvider{
				backendRegistry: registry,
				logger:          slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !provider.IsHealthy(addrs[i%n], 80) {
					b.Fatal("registered healthy backend reported unhealthy")
				}
			}
		})
	}
}
