// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package config

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestReferenceOverwatch_DNSSection keeps the dns section of
// config/reference-overwatch.yaml honest: the section parses as shipped, and
// the commented-out zones example parses and validates once uncommented, with
// the values the comments promise.
func TestReferenceOverwatch_DNSSection(t *testing.T) {
	data, err := os.ReadFile("../../config/reference-overwatch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	start := strings.Index(text, "\ndns:\n")
	end := strings.Index(text, "# API SERVER CONFIGURATION")
	if start < 0 || end < start {
		t.Fatal("dns section not found in the reference configuration")
	}
	section := text[start:end]

	cfg, err := Parse([]byte(section))
	if err != nil {
		t.Fatalf("dns section does not parse: %v", err)
	}
	if cfg.DNS.MaxUDPSize != DefaultMaxUDPSize || cfg.DNS.ReturnLastHealthy || len(cfg.DNS.Zones) != 0 {
		t.Errorf("dns section as shipped = %+v, want the defaults and no zones", cfg.DNS)
	}
	if err := cfg.validateDNS(); err != nil {
		t.Errorf("dns section as shipped is not valid: %v", err)
	}

	// Uncomment the zones example: every line from "  # zones:" on that is
	// a comment at that indentation.
	at := strings.Index(section, "  # zones:\n")
	if at < 0 {
		t.Fatal("commented zones example not found")
	}
	uncomment := regexp.MustCompile(`(?m)^  # ?`)
	withZones := section[:at] + uncomment.ReplaceAllString(section[at:], "  ")

	cfg, err = Parse([]byte(withZones))
	if err != nil {
		t.Fatalf("zones example does not parse:\n%s\n%v", withZones[at:], err)
	}
	if err := cfg.validateDNS(); err != nil {
		t.Fatalf("zones example is not valid: %v", err)
	}
	if len(cfg.DNS.Zones) != 2 {
		t.Fatalf("zones example has %d zones, want 2: %+v", len(cfg.DNS.Zones), cfg.DNS.Zones)
	}
	plain, object := cfg.DNS.Zones[0], cfg.DNS.Zones[1]
	if plain.Name != "example.com" || plain.TTL != 0 || plain.SOA != (ZoneSOA{}) || plain.Nameservers != nil {
		t.Errorf("plain-form zone = %+v", plain)
	}
	wantSOA := ZoneSOA{
		PrimaryNS:   "ns1.internal.corp",
		Contact:     "dns-admin@internal.corp",
		Serial:      2026112001,
		Refresh:     3600,
		Retry:       600,
		Expire:      604800,
		NegativeTTL: 60,
	}
	if object.Name != "internal.corp" || object.TTL != 3600 || object.SOA != wantSOA ||
		strings.Join(object.Nameservers, ",") != "ns1.internal.corp,ns2.internal.corp" {
		t.Errorf("object-form zone = %+v", object)
	}
}
