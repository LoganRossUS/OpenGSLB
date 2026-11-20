// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package config

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestParse_ZonesStringForm checks that the plain string form of dns.zones,
// which every existing configuration uses, still parses.
func TestParse_ZonesStringForm(t *testing.T) {
	cfg, err := Parse([]byte(`
dns:
  zones:
    - gslb.example.com
    - "internal.corp."
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"gslb.example.com", "internal.corp."}
	if got := cfg.DNS.ZoneNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ZoneNames = %v, want %v", got, want)
	}
	for _, z := range cfg.DNS.Zones {
		if z.TTL != 0 || z.SOA != (ZoneSOA{}) || len(z.Nameservers) != 0 {
			t.Errorf("zone %q: string form must leave every setting unset, got %+v", z.Name, z)
		}
	}
}

// TestParse_ZonesObjectForm checks the object form with SOA and NS settings,
// mixed with the string form in one list.
func TestParse_ZonesObjectForm(t *testing.T) {
	cfg, err := Parse([]byte(`
dns:
  zones:
    - gslb.example.com
    - name: internal.corp
      ttl: 7200
      soa:
        primary_ns: ns1.internal.corp
        contact: dns-admin@internal.corp
        serial: 2026112001
        refresh: 7200
        retry: 900
        expire: 1209600
        negative_ttl: 30
      nameservers:
        - ns1.internal.corp
        - ns2.internal.corp
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.DNS.Zones) != 2 {
		t.Fatalf("zones = %d, want 2", len(cfg.DNS.Zones))
	}
	if cfg.DNS.Zones[0].Name != "gslb.example.com" {
		t.Errorf("zones[0].name = %q", cfg.DNS.Zones[0].Name)
	}
	want := Zone{
		Name: "internal.corp",
		TTL:  7200,
		SOA: ZoneSOA{
			PrimaryNS:   "ns1.internal.corp",
			Contact:     "dns-admin@internal.corp",
			Serial:      2026112001,
			Refresh:     7200,
			Retry:       900,
			Expire:      1209600,
			NegativeTTL: 30,
		},
		Nameservers: []string{"ns1.internal.corp", "ns2.internal.corp"},
	}
	if !reflect.DeepEqual(cfg.DNS.Zones[1], want) {
		t.Errorf("zones[1] = %+v, want %+v", cfg.DNS.Zones[1], want)
	}
	if err := validateOverwatchWithDNS(cfg.DNS); err != nil {
		t.Errorf("valid zone settings rejected: %v", err)
	}
}

func TestParse_ZonesRejectsOtherYAMLKinds(t *testing.T) {
	_, err := Parse([]byte(`
dns:
  zones:
    - [a, b]
`))
	if err == nil {
		t.Fatal("expected an error for a zone that is neither a string nor a mapping")
	}
}

// TestZone_MarshalRoundTrip checks that a zone with no settings is written
// back as a plain string and a zone with settings as a mapping.
func TestZone_MarshalRoundTrip(t *testing.T) {
	in := DNSConfig{Zones: []Zone{
		{Name: "gslb.example.com"},
		{Name: "internal.corp", Nameservers: []string{"ns1.internal.corp"}},
	}}
	out, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(out), "- gslb.example.com\n") {
		t.Errorf("zone without settings was not written as a plain string:\n%s", out)
	}
	var back DNSConfig
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(back.Zones, in.Zones) {
		t.Errorf("round trip = %+v, want %+v", back.Zones, in.Zones)
	}
}

// validateOverwatchWithDNS validates a known-good overwatch configuration with
// the given dns section.
func validateOverwatchWithDNS(d DNSConfig) error {
	cfg := validOverwatchConfig()
	cfg.DNS = d
	return cfg.Validate()
}

func TestValidate_Zones(t *testing.T) {
	long := strings.Repeat("a", 64)
	tests := []struct {
		name    string
		zones   []Zone
		wantErr string // empty means valid
	}{
		{name: "string form", zones: []Zone{{Name: "gslb.example.com"}}},
		{name: "trailing dot", zones: []Zone{{Name: "gslb.example.com."}}},
		{name: "empty name", zones: []Zone{{Name: ""}}, wantErr: "zones[0].name is required"},
		{name: "root zone", zones: []Zone{{Name: "."}}, wantErr: "root zone"},
		{name: "label too long", zones: []Zone{{Name: long + ".example.com"}}, wantErr: "not a valid DNS name"},
		{name: "empty label", zones: []Zone{{Name: "gslb..example.com"}}, wantErr: "not a valid DNS name"},
		{
			name:    "duplicate ignoring case and dot",
			zones:   []Zone{{Name: "gslb.example.com"}, {Name: "GSLB.Example.com."}},
			wantErr: "duplicate zone",
		},
		{name: "negative ttl", zones: []Zone{{Name: "z.example", TTL: -1}}, wantErr: "zones[0].ttl"},
		{name: "ttl too large", zones: []Zone{{Name: "z.example", TTL: 1 << 31}}, wantErr: "zones[0].ttl"},
		{name: "negative refresh", zones: []Zone{{Name: "z.example", SOA: ZoneSOA{Refresh: -1}}}, wantErr: "soa.refresh"},
		{name: "negative retry", zones: []Zone{{Name: "z.example", SOA: ZoneSOA{Retry: -1}}}, wantErr: "soa.retry"},
		{name: "negative expire", zones: []Zone{{Name: "z.example", SOA: ZoneSOA{Expire: -1}}}, wantErr: "soa.expire"},
		{name: "negative negative_ttl", zones: []Zone{{Name: "z.example", SOA: ZoneSOA{NegativeTTL: -1}}}, wantErr: "soa.negative_ttl"},
		{name: "bad primary_ns", zones: []Zone{{Name: "z.example", SOA: ZoneSOA{PrimaryNS: "ns1..z.example"}}}, wantErr: "soa.primary_ns"},
		{name: "contact as mailbox", zones: []Zone{{Name: "z.example", SOA: ZoneSOA{Contact: "dns.admin@z.example"}}}},
		{name: "contact as dns name", zones: []Zone{{Name: "z.example", SOA: ZoneSOA{Contact: "hostmaster.z.example."}}}},
		{name: "bad contact", zones: []Zone{{Name: "z.example", SOA: ZoneSOA{Contact: "admin@"}}}, wantErr: "soa.contact"},
		{name: "empty nameserver", zones: []Zone{{Name: "z.example", Nameservers: []string{""}}}, wantErr: "nameservers[0]"},
		{name: "bad nameserver", zones: []Zone{{Name: "z.example", Nameservers: []string{"ns1.z.example", "ns..bad"}}}, wantErr: "nameservers[1]"},
		{
			name:    "duplicate nameserver",
			zones:   []Zone{{Name: "z.example", Nameservers: []string{"ns1.z.example", "NS1.z.example."}}},
			wantErr: "duplicate nameserver",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOverwatchWithDNS(DNSConfig{Zones: tt.zones})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestMailboxToDNSName(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "hostmaster@example.com", want: "hostmaster.example.com."},
		{in: "dns.admin@example.com", want: `dns\.admin.example.com.`},
		{in: "hostmaster.example.com", want: "hostmaster.example.com."},
		{in: "hostmaster.example.com.", want: "hostmaster.example.com."},
		{in: "admin@", wantErr: true},
		{in: "@example.com", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := MailboxToDNSName(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("MailboxToDNSName(%q) = %q, want an error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("MailboxToDNSName(%q) = %q, %v, want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestParse_MaxUDPSizeDefault(t *testing.T) {
	cfg, err := Parse([]byte("dns:\n  listen_address: \":53\"\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.DNS.MaxUDPSize != 1232 {
		t.Errorf("default max_udp_size = %d, want 1232", cfg.DNS.MaxUDPSize)
	}

	cfg, err = Parse([]byte("dns:\n  max_udp_size: 4096\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.DNS.MaxUDPSize != 4096 {
		t.Errorf("max_udp_size = %d, want 4096", cfg.DNS.MaxUDPSize)
	}
}

func TestValidate_MaxUDPSize(t *testing.T) {
	for _, size := range []int{0, 512, 1232, 1400, 4096} {
		if err := validateOverwatchWithDNS(DNSConfig{MaxUDPSize: size}); err != nil {
			t.Errorf("max_udp_size %d rejected: %v", size, err)
		}
	}
	for _, size := range []int{-1, 1, 511, 4097, 65535} {
		err := validateOverwatchWithDNS(DNSConfig{MaxUDPSize: size})
		if err == nil || !strings.Contains(err.Error(), "max_udp_size") {
			t.Errorf("max_udp_size %d: error = %v, want a max_udp_size error", size, err)
		}
	}
}
