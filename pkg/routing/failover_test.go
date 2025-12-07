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

package routing

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
)

func TestFailover_Algorithm(t *testing.T) {
	f := NewFailover()
	if got := f.Algorithm(); got != "failover" {
		t.Errorf("Algorithm() = %q, want %q", got, "failover")
	}
}

func TestFailover_Route_EmptySlice(t *testing.T) {
	f := NewFailover()

	_, err := f.Route(context.Background(), "example.com", []dns.ServerInfo{})
	if err != ErrNoHealthyServers {
		t.Errorf("Route() error = %v, want ErrNoHealthyServers", err)
	}
}

func TestFailover_Route_SingleServer(t *testing.T) {
	f := NewFailover()
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100, Region: "primary"},
	}

	got, err := f.Route(context.Background(), "example.com", servers)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if !got.Address.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("Route() address = %v, want 10.0.0.1", got.Address)
	}
}

func TestFailover_Route_AlwaysSelectsFirst(t *testing.T) {
	f := NewFailover()
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100, Region: "primary"},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100, Region: "secondary"},
		{Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 100, Region: "tertiary"},
	}

	// Run multiple times - should always return the first server
	for i := 0; i < 100; i++ {
		got, err := f.Route(context.Background(), "example.com", servers)
		if err != nil {
			t.Fatalf("Route() iteration %d error = %v", i, err)
		}
		if !got.Address.Equal(net.ParseIP("10.0.0.1")) {
			t.Errorf("Route() iteration %d: got %v, want 10.0.0.1 (primary)", i, got.Address)
		}
	}
}

func TestFailover_Route_FailoverToSecondary(t *testing.T) {
	f := NewFailover()

	// Simulate primary being unhealthy (filtered out by handler)
	// Router only receives healthy servers
	servers := []dns.ServerInfo{
		// 10.0.0.1 (primary) is unhealthy, not in list
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100, Region: "secondary"},
		{Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 100, Region: "tertiary"},
	}

	got, err := f.Route(context.Background(), "example.com", servers)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if !got.Address.Equal(net.ParseIP("10.0.0.2")) {
		t.Errorf("Route() = %v, want 10.0.0.2 (secondary)", got.Address)
	}
}

func TestFailover_Route_FailoverToTertiary(t *testing.T) {
	f := NewFailover()

	// Simulate primary and secondary both unhealthy
	servers := []dns.ServerInfo{
		// 10.0.0.1 (primary) is unhealthy, not in list
		// 10.0.0.2 (secondary) is unhealthy, not in list
		{Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 100, Region: "tertiary"},
	}

	got, err := f.Route(context.Background(), "example.com", servers)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if !got.Address.Equal(net.ParseIP("10.0.0.3")) {
		t.Errorf("Route() = %v, want 10.0.0.3 (tertiary)", got.Address)
	}
}

func TestFailover_Route_RecoveryToPrimary(t *testing.T) {
	f := NewFailover()

	// Step 1: Primary unhealthy, using secondary
	serversWithoutPrimary := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100, Region: "secondary"},
		{Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 100, Region: "tertiary"},
	}

	got, err := f.Route(context.Background(), "example.com", serversWithoutPrimary)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if !got.Address.Equal(net.ParseIP("10.0.0.2")) {
		t.Errorf("During failover: got %v, want 10.0.0.2", got.Address)
	}

	// Step 2: Primary recovers, should return to primary
	serversWithPrimary := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100, Region: "primary"},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100, Region: "secondary"},
		{Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 100, Region: "tertiary"},
	}

	got, err = f.Route(context.Background(), "example.com", serversWithPrimary)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if !got.Address.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("After recovery: got %v, want 10.0.0.1 (primary)", got.Address)
	}
}

func TestFailover_Route_DifferentDomains(t *testing.T) {
	f := NewFailover()

	// Different domains can have different server orders
	domain1Servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.1.1"), Port: 80, Region: "us-east"},
		{Address: net.ParseIP("10.0.1.2"), Port: 80, Region: "us-west"},
	}

	domain2Servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.2.1"), Port: 80, Region: "eu-west"},
		{Address: net.ParseIP("10.0.2.2"), Port: 80, Region: "eu-east"},
	}

	got1, err := f.Route(context.Background(), "app1.example.com", domain1Servers)
	if err != nil {
		t.Fatalf("Route(domain1) error = %v", err)
	}
	if !got1.Address.Equal(net.ParseIP("10.0.1.1")) {
		t.Errorf("Route(domain1) = %v, want 10.0.1.1", got1.Address)
	}

	got2, err := f.Route(context.Background(), "app2.example.com", domain2Servers)
	if err != nil {
		t.Fatalf("Route(domain2) error = %v", err)
	}
	if !got2.Address.Equal(net.ParseIP("10.0.2.1")) {
		t.Errorf("Route(domain2) = %v, want 10.0.2.1", got2.Address)
	}
}

func TestFailover_Route_Concurrent(t *testing.T) {
	f := NewFailover()
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100, Region: "primary"},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100, Region: "secondary"},
	}

	var wg sync.WaitGroup
	iterations := 100
	goroutines := 10

	errors := make(chan error, goroutines*iterations)
	results := make(chan string, goroutines*iterations)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				got, err := f.Route(context.Background(), "example.com", servers)
				if err != nil {
					errors <- err
				} else {
					results <- got.Address.String()
				}
			}
		}()
	}

	wg.Wait()
	close(errors)
	close(results)

	for err := range errors {
		t.Errorf("Concurrent Route() error: %v", err)
	}

	// All results should be the primary server
	for addr := range results {
		if addr != "10.0.0.1" {
			t.Errorf("Concurrent Route() returned %s, want 10.0.0.1", addr)
		}
	}
}

func TestFailover_Route_IgnoresWeight(t *testing.T) {
	f := NewFailover()

	// Even with higher weight on secondary, primary should be selected
	servers := []dns.ServerInfo{
		{Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 10, Region: "primary"},
		{Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 1000, Region: "secondary"},
	}

	got, err := f.Route(context.Background(), "example.com", servers)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if !got.Address.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("Route() = %v, want 10.0.0.1 (primary, regardless of weight)", got.Address)
	}
}

func TestFailover_ImplementsRouter(t *testing.T) {
	// Compile-time check that Failover implements dns.Router
	var _ dns.Router = (*Failover)(nil)
}
