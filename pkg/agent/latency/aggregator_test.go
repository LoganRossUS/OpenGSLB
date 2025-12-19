// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package latency

import (
	"net/netip"
	"testing"
	"time"
)

func TestAggregator_Record(t *testing.T) {
	agg := NewAggregator(AggregatorConfig{
		IPv4Prefix: 24,
		IPv6Prefix: 48,
		EWMAAlpha:  0.3,
		MaxSubnets: 100,
		SubnetTTL:  24 * time.Hour,
		MinSamples: 1,
	})

	// Record an observation
	obs := Observation{
		RemoteAddr: netip.MustParseAddr("192.168.1.100"),
		LocalPort:  8080,
		RTT:        10 * time.Millisecond,
		Timestamp:  time.Now(),
	}

	agg.Record(obs)

	// Check that it was bucketed into /24
	expectedSubnet := netip.MustParsePrefix("192.168.1.0/24")
	stats, found := agg.GetSubnet(expectedSubnet)

	if !found {
		t.Fatalf("expected subnet %s to be tracked", expectedSubnet)
	}

	if stats.SampleCount != 1 {
		t.Errorf("expected sample count 1, got %d", stats.SampleCount)
	}

	if stats.EWMA != 10*time.Millisecond {
		t.Errorf("expected EWMA 10ms, got %v", stats.EWMA)
	}
}

func TestAggregator_Record_IPv6(t *testing.T) {
	agg := NewAggregator(AggregatorConfig{
		IPv4Prefix: 24,
		IPv6Prefix: 48,
		EWMAAlpha:  0.3,
		MaxSubnets: 100,
		SubnetTTL:  24 * time.Hour,
		MinSamples: 1,
	})

	// Record an IPv6 observation
	obs := Observation{
		RemoteAddr: netip.MustParseAddr("2001:db8::1"),
		LocalPort:  8080,
		RTT:        20 * time.Millisecond,
		Timestamp:  time.Now(),
	}

	agg.Record(obs)

	// Check that it was bucketed into /48
	expectedSubnet := netip.MustParsePrefix("2001:db8::/48")
	stats, found := agg.GetSubnet(expectedSubnet)

	if !found {
		t.Fatalf("expected subnet %s to be tracked", expectedSubnet)
	}

	if stats.SampleCount != 1 {
		t.Errorf("expected sample count 1, got %d", stats.SampleCount)
	}
}

func TestAggregator_EWMA(t *testing.T) {
	alpha := 0.3
	agg := NewAggregator(AggregatorConfig{
		IPv4Prefix: 24,
		IPv6Prefix: 48,
		EWMAAlpha:  alpha,
		MaxSubnets: 100,
		SubnetTTL:  24 * time.Hour,
		MinSamples: 1,
	})

	addr := netip.MustParseAddr("10.0.0.1")
	now := time.Now()

	// First observation: 100ms
	agg.Record(Observation{
		RemoteAddr: addr,
		LocalPort:  80,
		RTT:        100 * time.Millisecond,
		Timestamp:  now,
	})

	subnet := netip.MustParsePrefix("10.0.0.0/24")
	stats, _ := agg.GetSubnet(subnet)
	if stats.EWMA != 100*time.Millisecond {
		t.Errorf("first EWMA should be 100ms, got %v", stats.EWMA)
	}

	// Second observation: 50ms
	// EWMA = 0.3 * 50 + 0.7 * 100 = 15 + 70 = 85ms
	agg.Record(Observation{
		RemoteAddr: addr,
		LocalPort:  80,
		RTT:        50 * time.Millisecond,
		Timestamp:  now,
	})

	stats, _ = agg.GetSubnet(subnet)
	expected := time.Duration(alpha*50+(1-alpha)*100) * time.Millisecond
	if stats.EWMA != expected {
		t.Errorf("second EWMA should be %v, got %v", expected, stats.EWMA)
	}

	// Third observation: 200ms
	// EWMA = 0.3 * 200ms + 0.7 * 85ms = 60ms + 59.5ms = 119.5ms
	agg.Record(Observation{
		RemoteAddr: addr,
		LocalPort:  80,
		RTT:        200 * time.Millisecond,
		Timestamp:  now,
	})

	stats, _ = agg.GetSubnet(subnet)
	// Calculate expected: alpha * new + (1-alpha) * previous
	// Previous was 85ms (85000000ns), new is 200ms
	prevEWMAns := float64(expected)
	newRTTns := float64(200 * time.Millisecond)
	expectedThird := time.Duration(alpha*newRTTns + (1-alpha)*prevEWMAns)
	if stats.EWMA != expectedThird {
		t.Errorf("third EWMA should be %v, got %v", expectedThird, stats.EWMA)
	}
}

func TestAggregator_MinMaxRTT(t *testing.T) {
	agg := NewAggregator(DefaultAggregatorConfig())

	addr := netip.MustParseAddr("172.16.0.1")
	now := time.Now()

	rtts := []time.Duration{
		50 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		30 * time.Millisecond,
	}

	for _, rtt := range rtts {
		agg.Record(Observation{
			RemoteAddr: addr,
			LocalPort:  443,
			RTT:        rtt,
			Timestamp:  now,
		})
	}

	subnet := netip.MustParsePrefix("172.16.0.0/24")
	stats, _ := agg.GetSubnet(subnet)

	if stats.MinRTT != 10*time.Millisecond {
		t.Errorf("expected MinRTT 10ms, got %v", stats.MinRTT)
	}

	if stats.MaxRTT != 100*time.Millisecond {
		t.Errorf("expected MaxRTT 100ms, got %v", stats.MaxRTT)
	}
}

func TestAggregator_GetReportable(t *testing.T) {
	agg := NewAggregator(AggregatorConfig{
		IPv4Prefix: 24,
		IPv6Prefix: 48,
		EWMAAlpha:  0.3,
		MaxSubnets: 100,
		SubnetTTL:  24 * time.Hour,
		MinSamples: 5, // Require 5 samples
	})

	now := time.Now()

	// Add 3 samples to subnet A (below threshold)
	for i := 0; i < 3; i++ {
		agg.Record(Observation{
			RemoteAddr: netip.MustParseAddr("192.168.1.1"),
			LocalPort:  80,
			RTT:        10 * time.Millisecond,
			Timestamp:  now,
		})
	}

	// Add 10 samples to subnet B (above threshold)
	for i := 0; i < 10; i++ {
		agg.Record(Observation{
			RemoteAddr: netip.MustParseAddr("192.168.2.1"),
			LocalPort:  80,
			RTT:        20 * time.Millisecond,
			Timestamp:  now,
		})
	}

	reportable := agg.GetReportable()

	if len(reportable) != 1 {
		t.Fatalf("expected 1 reportable subnet, got %d", len(reportable))
	}

	if reportable[0].Subnet.String() != "192.168.2.0/24" {
		t.Errorf("expected subnet 192.168.2.0/24, got %s", reportable[0].Subnet)
	}
}

func TestAggregator_Prune(t *testing.T) {
	agg := NewAggregator(AggregatorConfig{
		IPv4Prefix: 24,
		IPv6Prefix: 48,
		EWMAAlpha:  0.3,
		MaxSubnets: 100,
		SubnetTTL:  1 * time.Hour, // 1 hour TTL for testing
		MinSamples: 1,
	})

	// Add an old observation
	oldTime := time.Now().Add(-2 * time.Hour)
	agg.Record(Observation{
		RemoteAddr: netip.MustParseAddr("10.0.0.1"),
		LocalPort:  80,
		RTT:        10 * time.Millisecond,
		Timestamp:  oldTime,
	})

	// Add a recent observation
	agg.Record(Observation{
		RemoteAddr: netip.MustParseAddr("10.0.1.1"),
		LocalPort:  80,
		RTT:        10 * time.Millisecond,
		Timestamp:  time.Now(),
	})

	if agg.SubnetCount() != 2 {
		t.Fatalf("expected 2 subnets before prune, got %d", agg.SubnetCount())
	}

	// Prune should remove the old entry
	agg.Prune()

	if agg.SubnetCount() != 1 {
		t.Errorf("expected 1 subnet after prune, got %d", agg.SubnetCount())
	}

	// The recent subnet should still exist
	_, found := agg.GetSubnet(netip.MustParsePrefix("10.0.1.0/24"))
	if !found {
		t.Error("recent subnet should not have been pruned")
	}
}

func TestAggregator_MaxSubnets(t *testing.T) {
	maxSubnets := 5
	agg := NewAggregator(AggregatorConfig{
		IPv4Prefix: 32, // /32 so each IP is its own "subnet" for testing
		IPv6Prefix: 128,
		EWMAAlpha:  0.3,
		MaxSubnets: maxSubnets,
		SubnetTTL:  24 * time.Hour,
		MinSamples: 1,
	})

	now := time.Now()

	// Add more entries than max
	for i := 0; i < 10; i++ {
		addr := netip.AddrFrom4([4]byte{192, 168, 0, byte(i)})
		agg.Record(Observation{
			RemoteAddr: addr,
			LocalPort:  80,
			RTT:        10 * time.Millisecond,
			Timestamp:  now,
		})
	}

	// Should be capped at maxSubnets
	if agg.SubnetCount() > maxSubnets {
		t.Errorf("expected at most %d subnets, got %d", maxSubnets, agg.SubnetCount())
	}
}

func TestAggregator_ToReport(t *testing.T) {
	agg := NewAggregator(AggregatorConfig{
		IPv4Prefix: 24,
		IPv6Prefix: 48,
		EWMAAlpha:  0.3,
		MaxSubnets: 100,
		SubnetTTL:  24 * time.Hour,
		MinSamples: 1,
	})

	now := time.Now()

	// Add some observations
	agg.Record(Observation{
		RemoteAddr: netip.MustParseAddr("192.168.1.100"),
		LocalPort:  80,
		RTT:        50 * time.Millisecond,
		Timestamp:  now,
	})

	agg.Record(Observation{
		RemoteAddr: netip.MustParseAddr("10.0.0.5"),
		LocalPort:  80,
		RTT:        100 * time.Millisecond,
		Timestamp:  now,
	})

	report := agg.ToReport("agent-1", "my-service", "us-east-1")

	if report.AgentID != "agent-1" {
		t.Errorf("expected AgentID 'agent-1', got '%s'", report.AgentID)
	}

	if report.Backend != "my-service" {
		t.Errorf("expected Backend 'my-service', got '%s'", report.Backend)
	}

	if report.Region != "us-east-1" {
		t.Errorf("expected Region 'us-east-1', got '%s'", report.Region)
	}

	if len(report.Subnets) != 2 {
		t.Errorf("expected 2 subnets in report, got %d", len(report.Subnets))
	}

	// Check that subnets are sorted
	if len(report.Subnets) >= 2 {
		if report.Subnets[0].Subnet > report.Subnets[1].Subnet {
			t.Error("subnets should be sorted by subnet string")
		}
	}
}

func TestAggregator_Clear(t *testing.T) {
	agg := NewAggregator(DefaultAggregatorConfig())

	now := time.Now()

	// Add some observations
	for i := 0; i < 5; i++ {
		addr := netip.AddrFrom4([4]byte{192, 168, byte(i), 1})
		agg.Record(Observation{
			RemoteAddr: addr,
			LocalPort:  80,
			RTT:        10 * time.Millisecond,
			Timestamp:  now,
		})
	}

	if agg.SubnetCount() == 0 {
		t.Fatal("expected some subnets before clear")
	}

	agg.Clear()

	if agg.SubnetCount() != 0 {
		t.Errorf("expected 0 subnets after clear, got %d", agg.SubnetCount())
	}
}

func TestAggregator_DefaultConfig(t *testing.T) {
	cfg := DefaultAggregatorConfig()

	if cfg.IPv4Prefix != 24 {
		t.Errorf("expected default IPv4Prefix 24, got %d", cfg.IPv4Prefix)
	}

	if cfg.IPv6Prefix != 48 {
		t.Errorf("expected default IPv6Prefix 48, got %d", cfg.IPv6Prefix)
	}

	if cfg.EWMAAlpha != 0.3 {
		t.Errorf("expected default EWMAAlpha 0.3, got %f", cfg.EWMAAlpha)
	}

	if cfg.MaxSubnets != 100000 {
		t.Errorf("expected default MaxSubnets 100000, got %d", cfg.MaxSubnets)
	}

	if cfg.SubnetTTL != 168*time.Hour {
		t.Errorf("expected default SubnetTTL 168h, got %v", cfg.SubnetTTL)
	}

	if cfg.MinSamples != 5 {
		t.Errorf("expected default MinSamples 5, got %d", cfg.MinSamples)
	}
}
