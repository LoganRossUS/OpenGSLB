// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package geo

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

// =============================================================================
// CustomMappings Tests
// =============================================================================

func TestCustomMappings_AddAndLookup(t *testing.T) {
	cm := NewCustomMappings(nil)

	// Add a mapping
	err := cm.Add(CustomMapping{
		CIDR:    "10.1.0.0/16",
		Region:  "us-east-1",
		Comment: "Test mapping",
		Source:  "test",
	})
	if err != nil {
		t.Fatalf("failed to add mapping: %v", err)
	}

	// Lookup an IP in the range
	result := cm.Lookup(net.ParseIP("10.1.50.100"))
	if !result.Found {
		t.Fatal("expected to find mapping for 10.1.50.100")
	}
	if result.Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", result.Region)
	}
	if result.CIDR != "10.1.0.0/16" {
		t.Errorf("expected CIDR 10.1.0.0/16, got %s", result.CIDR)
	}

	// Lookup an IP outside the range
	result = cm.Lookup(net.ParseIP("192.168.1.1"))
	if result.Found {
		t.Error("expected no match for 192.168.1.1")
	}
}

func TestCustomMappings_LongestPrefixMatch(t *testing.T) {
	cm := NewCustomMappings(nil)

	// Add overlapping ranges
	err := cm.Add(CustomMapping{CIDR: "10.0.0.0/8", Region: "broad", Source: "test"})
	if err != nil {
		t.Fatalf("failed to add broad mapping: %v", err)
	}
	err = cm.Add(CustomMapping{CIDR: "10.1.0.0/16", Region: "medium", Source: "test"})
	if err != nil {
		t.Fatalf("failed to add medium mapping: %v", err)
	}
	err = cm.Add(CustomMapping{CIDR: "10.1.50.0/24", Region: "specific", Source: "test"})
	if err != nil {
		t.Fatalf("failed to add specific mapping: %v", err)
	}

	tests := []struct {
		ip       string
		expected string
	}{
		{"10.2.0.1", "broad"},      // Only /8 matches
		{"10.1.100.1", "medium"},   // /16 is more specific than /8
		{"10.1.50.100", "specific"}, // /24 is most specific
	}

	for _, tt := range tests {
		result := cm.Lookup(net.ParseIP(tt.ip))
		if !result.Found {
			t.Errorf("expected to find mapping for %s", tt.ip)
			continue
		}
		if result.Region != tt.expected {
			t.Errorf("for IP %s: expected region %s, got %s", tt.ip, tt.expected, result.Region)
		}
	}
}

func TestCustomMappings_Remove(t *testing.T) {
	cm := NewCustomMappings(nil)

	// Add and then remove
	err := cm.Add(CustomMapping{CIDR: "10.1.0.0/16", Region: "us-east-1", Source: "test"})
	if err != nil {
		t.Fatalf("failed to add mapping: %v", err)
	}

	err = cm.Remove("10.1.0.0/16")
	if err != nil {
		t.Fatalf("failed to remove mapping: %v", err)
	}

	// Verify it's gone
	result := cm.Lookup(net.ParseIP("10.1.50.100"))
	if result.Found {
		t.Error("expected no match after removal")
	}

	// Removing non-existent should error
	err = cm.Remove("10.1.0.0/16")
	if err == nil {
		t.Error("expected error when removing non-existent mapping")
	}
}

func TestCustomMappings_LoadFromConfig(t *testing.T) {
	cm := NewCustomMappings(nil)

	mappings := []CustomMapping{
		{CIDR: "10.1.0.0/16", Region: "us-east-1", Comment: "East"},
		{CIDR: "10.2.0.0/16", Region: "us-west-2", Comment: "West"},
	}

	err := cm.LoadFromConfig(mappings)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cm.Count() != 2 {
		t.Errorf("expected 2 mappings, got %d", cm.Count())
	}

	// All should have source "config"
	list := cm.List()
	for _, m := range list {
		if m.Source != "config" {
			t.Errorf("expected source 'config', got %s", m.Source)
		}
	}
}

func TestCustomMappings_InvalidCIDR(t *testing.T) {
	cm := NewCustomMappings(nil)

	err := cm.Add(CustomMapping{CIDR: "invalid", Region: "us-east-1", Source: "test"})
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}

	err = cm.LoadFromConfig([]CustomMapping{{CIDR: "not-a-cidr", Region: "us-east-1"}})
	if err == nil {
		t.Error("expected error for invalid CIDR in config")
	}
}

func TestCustomMappings_IPv6(t *testing.T) {
	cm := NewCustomMappings(nil)

	err := cm.Add(CustomMapping{
		CIDR:   "2001:db8::/32",
		Region: "ipv6-region",
		Source: "test",
	})
	if err != nil {
		t.Fatalf("failed to add IPv6 mapping: %v", err)
	}

	result := cm.Lookup(net.ParseIP("2001:db8::1"))
	if !result.Found {
		t.Error("expected to find IPv6 mapping")
	}
	if result.Region != "ipv6-region" {
		t.Errorf("expected region ipv6-region, got %s", result.Region)
	}
}

// =============================================================================
// ECS Tests
// =============================================================================

func TestParseECS_NoECS(t *testing.T) {
	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	result := ParseECS(msg)
	if result.Found {
		t.Error("expected no ECS in message without OPT record")
	}
}

func TestParseECS_WithECS(t *testing.T) {
	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	// Add OPT record with ECS
	opt := &dns.OPT{
		Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT},
		Option: []dns.EDNS0{
			&dns.EDNS0_SUBNET{
				Code:          dns.EDNS0SUBNET,
				Family:        1, // IPv4
				SourceNetmask: 24,
				SourceScope:   0,
				Address:       net.ParseIP("203.0.113.0"),
			},
		},
	}
	msg.Extra = append(msg.Extra, opt)

	result := ParseECS(msg)
	if !result.Found {
		t.Fatal("expected to find ECS")
	}
	if !result.IP.Equal(net.ParseIP("203.0.113.0")) {
		t.Errorf("expected IP 203.0.113.0, got %v", result.IP)
	}
	if result.SourceNetmask != 24 {
		t.Errorf("expected netmask 24, got %d", result.SourceNetmask)
	}
}

func TestParseECS_NilMessage(t *testing.T) {
	result := ParseECS(nil)
	if result.Found {
		t.Error("expected no ECS for nil message")
	}
}

func TestGetClientIP_ECSEnabled(t *testing.T) {
	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	// Add ECS with a different IP
	opt := &dns.OPT{
		Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT},
		Option: []dns.EDNS0{
			&dns.EDNS0_SUBNET{
				Code:          dns.EDNS0SUBNET,
				Family:        1,
				SourceNetmask: 24,
				Address:       net.ParseIP("203.0.113.50"),
			},
		},
	}
	msg.Extra = append(msg.Extra, opt)

	sourceAddr := &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 53}

	// With ECS enabled, should return ECS IP
	ip := GetClientIP(msg, sourceAddr, true)
	if !ip.Equal(net.ParseIP("203.0.113.50")) {
		t.Errorf("expected ECS IP 203.0.113.50, got %v", ip)
	}

	// With ECS disabled, should return source IP
	ip = GetClientIP(msg, sourceAddr, false)
	if !ip.Equal(net.ParseIP("8.8.8.8")) {
		t.Errorf("expected source IP 8.8.8.8, got %v", ip)
	}
}

func TestGetClientIP_NoECS(t *testing.T) {
	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	sourceAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 53}

	ip := GetClientIP(msg, sourceAddr, true)
	if !ip.Equal(net.ParseIP("192.168.1.100")) {
		t.Errorf("expected source IP 192.168.1.100, got %v", ip)
	}
}

func TestGetClientIP_TCPAddr(t *testing.T) {
	msg := new(dns.Msg)
	sourceAddr := &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 12345}

	ip := GetClientIP(msg, sourceAddr, false)
	if !ip.Equal(net.ParseIP("10.0.0.5")) {
		t.Errorf("expected IP 10.0.0.5, got %v", ip)
	}
}

func TestAddECSResponse(t *testing.T) {
	msg := new(dns.Msg)
	msg.SetReply(&dns.Msg{})

	clientIP := net.ParseIP("192.0.2.100")
	AddECSResponse(msg, clientIP, 24, 24)

	// Find the OPT record
	var opt *dns.OPT
	for _, rr := range msg.Extra {
		if o, ok := rr.(*dns.OPT); ok {
			opt = o
			break
		}
	}

	if opt == nil {
		t.Fatal("expected OPT record in response")
	}

	// Find the ECS option
	var ecs *dns.EDNS0_SUBNET
	for _, o := range opt.Option {
		if e, ok := o.(*dns.EDNS0_SUBNET); ok {
			ecs = e
			break
		}
	}

	if ecs == nil {
		t.Fatal("expected ECS option in OPT record")
	}

	if ecs.Family != 1 { // IPv4
		t.Errorf("expected family 1, got %d", ecs.Family)
	}
	if ecs.SourceNetmask != 24 {
		t.Errorf("expected source netmask 24, got %d", ecs.SourceNetmask)
	}
	if ecs.SourceScope != 24 {
		t.Errorf("expected scope 24, got %d", ecs.SourceScope)
	}
}
